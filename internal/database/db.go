package database

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// DBConfig holds database connection configuration
type DBConfig struct {
	DatabaseURL            string
	MaxOpenConns           int
	MaxIdleConns           int
	ConnMaxIdleTime        time.Duration
	StatementTimeout       time.Duration
	LockTimeout            time.Duration
	IdleTransactionTimeout time.Duration
}

// DB wraps sqlx.DB with application-specific methods
type DB struct {
	*sqlx.DB
}

// NewDB creates a new database connection pool
func NewDB(cfg *DBConfig) (*DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("database configuration is required")
	}
	if cfg.MaxOpenConns < 1 {
		return nil, fmt.Errorf("max open connections must be at least 1")
	}
	if cfg.MaxIdleConns < 0 || cfg.MaxIdleConns > cfg.MaxOpenConns {
		return nil, fmt.Errorf("max idle connections must be between 0 and max open connections")
	}

	databaseURL, err := withPostgresTimeouts(cfg.DatabaseURL, cfg.StatementTimeout, cfg.LockTimeout, cfg.IdleTransactionTimeout)
	if err != nil {
		return nil, err
	}
	db, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Hour)
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	return &DB{DB: db}, nil
}

// PoolMetrics is a snapshot of the database/sql pool counters suitable for
// exporting through the application's metrics endpoint.
type PoolMetrics struct {
	OpenConnections int
	InUse           int
	Idle            int
	WaitCount       int64
	WaitDuration    time.Duration
}

func (db *DB) PoolMetrics() PoolMetrics {
	stats := db.DB.Stats()
	return PoolMetrics{
		OpenConnections: stats.OpenConnections,
		InUse:           stats.InUse,
		Idle:            stats.Idle,
		WaitCount:       stats.WaitCount,
		WaitDuration:    stats.WaitDuration,
	}
}

func withPostgresTimeouts(databaseURL string, statementTimeout, lockTimeout, idleTransactionTimeout time.Duration) (string, error) {
	options := make([]string, 0, 3)
	if statementTimeout > 0 {
		options = append(options, fmt.Sprintf("-c statement_timeout=%dms", statementTimeout.Milliseconds()))
	}
	if lockTimeout > 0 {
		options = append(options, fmt.Sprintf("-c lock_timeout=%dms", lockTimeout.Milliseconds()))
	}
	if idleTransactionTimeout > 0 {
		options = append(options, fmt.Sprintf("-c idle_in_transaction_session_timeout=%dms", idleTransactionTimeout.Milliseconds()))
	}
	if len(options) == 0 {
		return databaseURL, nil
	}

	// PostgreSQL URLs are the supported deployment format. Preserve an
	// existing options query parameter so operators can keep search_path and
	// other session settings.
	if !strings.Contains(databaseURL, "://") {
		return strings.TrimSpace(databaseURL) + " options='" + strings.Join(options, " ") + "'", nil
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}
	query := parsed.Query()
	if existing := query.Get("options"); existing != "" {
		options = append([]string{existing}, options...)
	}
	query.Set("options", strings.Join(options, " "))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// Ping checks if the database connection is alive
func (db *DB) Ping() error {
	return db.DB.Ping()
}

// Close closes the database connection pool
func (db *DB) Close() error {
	return db.DB.Close()
}
