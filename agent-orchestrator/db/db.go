// Package db provides SQLite persistence for the agent orchestrator.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Database wraps a *sql.DB with convenience helpers.
type Database struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*Database, error) {
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_fk=true")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// SQLite works best with a single writer connection.
	sqlDB.SetMaxOpenConns(1)

	d := &Database{db: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return d, nil
}

// Close closes the database connection.
func (d *Database) Close() error {
	return d.db.Close()
}

// Ping verifies the connection is alive.
func (d *Database) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// RawDB exposes the underlying *sql.DB for packages that need direct query
// access (e.g. the metrics Collector).
func (d *Database) RawDB() *sql.DB {
	return d.db
}

// withTx runs fn inside a database transaction.
// If fn returns an error the transaction is rolled back, otherwise it is committed.
func (d *Database) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// withImmediateTx runs fn inside a BEGIN IMMEDIATE transaction.
// SQLite's IMMEDIATE mode acquires a reserved lock at begin time, preventing
// any other writer from starting until this transaction commits or rolls back.
// This is the correct isolation level for "check-then-act" operations like
// ClaimTask where two agents must not claim the same task.
func (d *Database) withImmediateTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	// Escalate to IMMEDIATE so the write lock is taken at BEGIN time.
	if _, err := tx.ExecContext(ctx, "SAVEPOINT _immediate"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
