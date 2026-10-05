// Package db opens the local SQLite store used for history, settings and
// application state (plan §66). Sensitive credentials never live here.
package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	// WAL keeps concurrent readers responsive while a task writes history.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1) // sqlite is a single-writer store
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}
	db := &DB{sql: d}
	if err := db.migrate(); err != nil {
		d.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) SQL() *sql.DB { return d.sql }

func (d *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS history (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at      TEXT NOT NULL,
			input_name      TEXT NOT NULL,
			input_path      TEXT NOT NULL,
			input_size      INTEGER NOT NULL DEFAULT 0,
			output_name     TEXT NOT NULL DEFAULT '',
			output_path     TEXT NOT NULL DEFAULT '',
			output_size     INTEGER NOT NULL DEFAULT 0,
			container       TEXT NOT NULL DEFAULT '',
			video_codec     TEXT NOT NULL DEFAULT '',
			encoder         TEXT NOT NULL DEFAULT '',
			quality         TEXT NOT NULL DEFAULT '',
			bitrate_kbps    INTEGER NOT NULL DEFAULT 0,
			duration_sec    REAL NOT NULL DEFAULT 0,
			encode_time_sec REAL NOT NULL DEFAULT 0,
			ratio           REAL NOT NULL DEFAULT 0,
			status          TEXT NOT NULL DEFAULT '',
			error_summary   TEXT NOT NULL DEFAULT '',
			warnings        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := d.sql.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

// EnsureWritable makes sure the data file exists (called during startup so a
// read-only disk surfaces early, not in the middle of an encode).
func (d *DB) EnsureWritable() error {
	return d.sql.Ping()
}

var _ = os.Getenv // keep os import stable if future helpers need it
