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
	db    *sql.DB
	LogDB *LogDatabase // separate logs.db; may be nil
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*Database, error) {
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// NOTE: the modernc.org/sqlite driver only honours `_pragma=...` DSN
	// directives — the mattn-style `_journal`/`_timeout`/`_fk` params are
	// silently ignored. Setting busy_timeout here makes it active from the
	// very first statement (including migrations), so concurrent opens wait
	// for the lock instead of failing immediately with SQLITE_BUSY.
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// SQLite works best with a single writer connection.
	sqlDB.SetMaxOpenConns(1)

	// Explicit WAL tuning pragmas. journal_mode and busy_timeout are also set
	// via the DSN above; re-asserting them here keeps the settings verifiable
	// and matches OpenLogDB.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-32000",
	} {
		if _, err := sqlDB.Exec(pragma); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("pragma: %w", err)
		}
	}

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

// LogDatabase wraps the separate logs.db for high-volume agent/task log tables.
type LogDatabase struct {
	db *sql.DB
}

// OpenLogDB opens (or creates) the log SQLite database at path with WAL mode.
func OpenLogDB(path string) (*LogDatabase, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create log db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path+"?_fk=false")
	if err != nil {
		return nil, fmt.Errorf("open log sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-32000",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := sqlDB.Exec(pragma); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("log db pragma: %w", err)
		}
	}
	return &LogDatabase{db: sqlDB}, nil
}

// Close closes the log database.
func (ld *LogDatabase) Close() error { return ld.db.Close() }

// RawDB exposes the underlying *sql.DB of the log database.
func (ld *LogDatabase) RawDB() *sql.DB {
	return ld.db
}
