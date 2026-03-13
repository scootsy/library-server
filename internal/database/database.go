// Package database provides SQLite connection management and migration running.
package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/scootsy/library-server/internal/database/migrations"

	_ "github.com/mattn/go-sqlite3"
)

// Open opens (or creates) the SQLite database at path, configures WAL mode
// and foreign keys, and runs any pending schema migrations.
func Open(path string) (*sql.DB, error) {
	// SQLite DSN parameters:
	//   _journal_mode=WAL   — better concurrent read performance
	//   _foreign_keys=on    — enforce FK constraints
	//   _busy_timeout=5000  — wait up to 5 s when the DB is locked
	dsn := path + "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database at %q: %w", path, err)
	}

	// SQLite performs best with a single writer connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if err := migrations.Run(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	slog.Info("database ready", "path", path)
	return db, nil
}
