package database

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// DBConfig holds database connection configuration
type DBConfig struct {
	DatabaseURL  string
	MaxOpenConns int
	MaxIdleConns int
}

// DB wraps sqlx.DB with application-specific methods
type DB struct {
	*sqlx.DB
}

// NewDB creates a new database connection pool
func NewDB(cfg *DBConfig) (*DB, error) {
	db, err := sqlx.Connect("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Hour)

	return &DB{DB: db}, nil
}

// Ping checks if the database connection is alive
func (db *DB) Ping() error {
	return db.DB.Ping()
}

// Close closes the database connection pool
func (db *DB) Close() error {
	return db.DB.Close()
}
