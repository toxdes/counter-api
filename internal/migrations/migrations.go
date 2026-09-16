package migrations

import (
	"context"
	"counter/internal/database"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
)

// migrationFiles contains the only migration history embedded in the binary.
// The repository-level migrations directory is not used at runtime.
//
//go:embed *.sql
var migrationFiles embed.FS

var migrationFilenamePattern = regexp.MustCompile(`^([0-9]+)_[A-Za-z0-9][A-Za-z0-9_-]*\.(up|down)\.sql$`)

type migrationFile struct {
	version  int64
	up       string
	down     string
	upPath   string
	downPath string
}

// RunUp executes all pending migrations while holding a database-wide
// advisory lock. Each migration and its tracking record are committed as one
// transaction.
func RunUp(db *database.DB) error {
	return withMigrationLock(context.Background(), db, func(conn *sqlx.Conn) error {
		if err := ensureSchemaMigrationsTable(conn); err != nil {
			return fmt.Errorf("failed to create schema_migrations table: %w", err)
		}

		status, err := migrationStatus(context.Background(), conn)
		if err != nil {
			return fmt.Errorf("failed to get migration status: %w", err)
		}
		files, err := discoverMigrations()
		if err != nil {
			return fmt.Errorf("failed to discover migrations: %w", err)
		}

		for _, file := range files {
			if status[file.version] {
				fmt.Printf("Migration %d already applied, skipping\n", file.version)
				continue
			}

			fmt.Printf("Applying migration %d from %s\n", file.version, file.upPath)
			if err := executeMigration(context.Background(), conn, file.version, file.up, true); err != nil {
				return fmt.Errorf("failed to apply migration %d: %w", file.version, err)
			}
			fmt.Printf("Migration %d applied successfully\n", file.version)
		}
		return nil
	})
}

// RunDown rolls back exactly the highest applied canonical migration. A
// caller can invoke it repeatedly to roll back additional versions.
func RunDown(db *database.DB) error {
	return withMigrationLock(context.Background(), db, func(conn *sqlx.Conn) error {
		if err := ensureSchemaMigrationsTable(conn); err != nil {
			return fmt.Errorf("failed to create schema_migrations table: %w", err)
		}

		status, err := migrationStatus(context.Background(), conn)
		if err != nil {
			return fmt.Errorf("failed to get migration status: %w", err)
		}
		files, err := discoverMigrations()
		if err != nil {
			return fmt.Errorf("failed to discover migrations: %w", err)
		}

		for index := len(files) - 1; index >= 0; index-- {
			file := files[index]
			if !status[file.version] {
				continue
			}

			fmt.Printf("Rolling back migration %d from %s\n", file.version, file.downPath)
			if err := executeMigration(context.Background(), conn, file.version, file.down, false); err != nil {
				return fmt.Errorf("failed to rollback migration %d: %w", file.version, err)
			}
			fmt.Printf("Migration %d rolled back successfully\n", file.version)
			break
		}
		return nil
	})
}

func withMigrationLock(ctx context.Context, db *database.DB, fn func(*sqlx.Conn) error) (err error) {
	conn, err := db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	locked := false
	defer func() {
		if locked {
			if _, unlockErr := conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext('counter-api:migrations'))`); err == nil && unlockErr != nil {
				err = fmt.Errorf("release migration lock: %w", unlockErr)
			}
		}
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close migration connection: %w", closeErr)
		}
	}()

	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext('counter-api:migrations'))`); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	locked = true
	return fn(conn)
}

func ensureSchemaMigrationsTable(conn *sqlx.Conn) error {
	_, err := conn.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	return err
}

func migrationStatus(ctx context.Context, conn *sqlx.Conn) (map[int64]bool, error) {
	rows, err := conn.QueryxContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	status := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		status[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return status, nil
}

func executeMigration(ctx context.Context, conn *sqlx.Conn, version int64, content string, up bool) error {
	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, content); err != nil {
		return err
	}
	if up {
		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING", version)
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1", version)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func discoverMigrations() ([]migrationFile, error) {
	upPaths, err := migrationPaths(".up.sql")
	if err != nil {
		return nil, err
	}
	downPaths, err := migrationPaths(".down.sql")
	if err != nil {
		return nil, err
	}
	if err := validateMigrationPaths(upPaths, downPaths); err != nil {
		return nil, err
	}

	upByVersion := pathsByVersion(upPaths)
	downByVersion := pathsByVersion(downPaths)
	versions := make([]int64, 0, len(upByVersion))
	for version := range upByVersion {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })

	files := make([]migrationFile, 0, len(versions))
	for _, version := range versions {
		upPath := upByVersion[version]
		downPath := downByVersion[version]
		up, err := migrationFiles.ReadFile(upPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", upPath, err)
		}
		down, err := migrationFiles.ReadFile(downPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", downPath, err)
		}
		files = append(files, migrationFile{
			version:  version,
			up:       string(up),
			down:     string(down),
			upPath:   upPath,
			downPath: downPath,
		})
	}
	return files, nil
}

func migrationPaths(suffix string) ([]string, error) {
	var paths []string
	err := fs.WalkDir(migrationFiles, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, suffix) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func validateMigrationPaths(upPaths, downPaths []string) error {
	upByVersion, err := indexMigrationPaths(upPaths, "up")
	if err != nil {
		return err
	}
	downByVersion, err := indexMigrationPaths(downPaths, "down")
	if err != nil {
		return err
	}
	for version := range upByVersion {
		if _, ok := downByVersion[version]; !ok {
			return fmt.Errorf("missing down migration for version %d", version)
		}
	}
	for version := range downByVersion {
		if _, ok := upByVersion[version]; !ok {
			return fmt.Errorf("missing up migration for version %d", version)
		}
	}
	return nil
}

func indexMigrationPaths(paths []string, direction string) (map[int64]string, error) {
	indexed := make(map[int64]string, len(paths))
	for _, path := range paths {
		version, err := extractVersion(path)
		if err != nil {
			return nil, err
		}
		if _, exists := indexed[version]; exists {
			return nil, fmt.Errorf("duplicate %s migration version %d", direction, version)
		}
		indexed[version] = path
	}
	return indexed, nil
}

func pathsByVersion(paths []string) map[int64]string {
	indexed := make(map[int64]string, len(paths))
	for _, path := range paths {
		version, _ := extractVersion(path)
		indexed[version] = path
	}
	return indexed
}

func extractVersion(filename string) (int64, error) {
	name := filename
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		name = name[slash+1:]
	}
	matches := migrationFilenamePattern.FindStringSubmatch(name)
	if matches == nil {
		return 0, fmt.Errorf("invalid migration filename format: %s", filename)
	}
	version, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("invalid migration version in filename: %s", filename)
	}
	return version, nil
}
