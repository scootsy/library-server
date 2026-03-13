// Package migrations defines the versioned database schema migrations.
// Each migration runs inside a transaction; if any statement fails the
// entire migration is rolled back and the application exits.
package migrations

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// migration is a single versioned schema change.
type migration struct {
	version int
	apply   func(tx *sql.Tx) error
}

// all migrations in ascending version order.
var all = []migration{
	{version: 1, apply: v1},
}

// Run executes any pending migrations against db.
func Run(db *sql.DB) error {
	if err := ensureVersionTable(db); err != nil {
		return fmt.Errorf("ensuring schema_version table: %w", err)
	}

	current, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	for _, m := range all {
		if m.version <= current {
			continue
		}
		slog.Info("applying database migration", "version", m.version)
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("applying migration v%d: %w", m.version, err)
		}
		slog.Info("migration applied", "version", m.version)
	}
	return nil
}

func ensureVersionTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER NOT NULL,
		applied_at TEXT    NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

func currentVersion(db *sql.DB) (int, error) {
	var v int
	err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&v)
	return v, err
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = m.apply(tx); err != nil {
		return err
	}

	_, err = tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version)
	if err != nil {
		return fmt.Errorf("recording version: %w", err)
	}

	return tx.Commit()
}

// v1 creates the initial schema.
func v1(tx *sql.Tx) error {
	stmts := []string{
		// ── APP SETTINGS ──────────────────────────────────────────────────────
		`CREATE TABLE app_settings (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// ── MEDIA ROOTS ───────────────────────────────────────────────────────
		`CREATE TABLE media_roots (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			root_path   TEXT NOT NULL UNIQUE,
			scan_config TEXT NOT NULL DEFAULT '{}',
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// ── WORKS ─────────────────────────────────────────────────────────────
		`CREATE TABLE works (
			id                 TEXT PRIMARY KEY,
			media_root_id      TEXT NOT NULL REFERENCES media_roots(id) ON DELETE CASCADE,
			directory_path     TEXT NOT NULL,
			title              TEXT NOT NULL,
			sort_title         TEXT NOT NULL,
			subtitle           TEXT,
			language           TEXT,
			publisher          TEXT,
			publish_date       TEXT,
			description        TEXT,
			description_format TEXT DEFAULT 'plain',
			page_count         INTEGER,
			duration_seconds   INTEGER,
			is_abridged        INTEGER DEFAULT 0,
			has_media_overlay  INTEGER DEFAULT 0,
			match_confidence   REAL,
			match_method       TEXT,
			primary_source     TEXT,
			needs_review       INTEGER DEFAULT 1,
			sidecar_hash       TEXT,
			added_at           TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at         TEXT NOT NULL DEFAULT (datetime('now')),
			last_scanned_at    TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (media_root_id, directory_path)
		)`,
		`CREATE INDEX idx_works_media_root   ON works(media_root_id)`,
		`CREATE INDEX idx_works_title        ON works(sort_title COLLATE NOCASE)`,
		`CREATE INDEX idx_works_publish_date ON works(publish_date)`,
		`CREATE INDEX idx_works_needs_review ON works(needs_review) WHERE needs_review = 1`,
		`CREATE INDEX idx_works_added_at     ON works(added_at)`,

		// ── FULL-TEXT SEARCH ──────────────────────────────────────────────────
		`CREATE VIRTUAL TABLE works_fts USING fts5(
			title,
			subtitle,
			description,
			contributors,
			series,
			publisher,
			tags,
			content=works,
			content_rowid=rowid
		)`,
		// Insert trigger: populate FTS with core fields; denormalized fields
		// (contributors, series, tags) are updated by the application after
		// the junction tables are written.
		`CREATE TRIGGER works_fts_insert AFTER INSERT ON works BEGIN
			INSERT INTO works_fts(rowid, title, subtitle, description, contributors, series, publisher, tags)
			VALUES (new.rowid, new.title, new.subtitle, new.description, '', '', new.publisher, '');
		END`,
		`CREATE TRIGGER works_fts_delete AFTER DELETE ON works BEGIN
			INSERT INTO works_fts(works_fts, rowid, title, subtitle, description, contributors, series, publisher, tags)
			VALUES ('delete', old.rowid, old.title, old.subtitle, old.description, '', '', old.publisher, '');
		END`,
		`CREATE TRIGGER works_fts_update AFTER UPDATE ON works BEGIN
			INSERT INTO works_fts(works_fts, rowid, title, subtitle, description, contributors, series, publisher, tags)
			VALUES ('delete', old.rowid, old.title, old.subtitle, old.description, '', '', old.publisher, '');
			INSERT INTO works_fts(rowid, title, subtitle, description, contributors, series, publisher, tags)
			VALUES (new.rowid, new.title, new.subtitle, new.description, '', '', new.publisher, '');
		END`,

		// ── IDENTIFIERS ───────────────────────────────────────────────────────
		`CREATE TABLE identifiers (
			work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			type    TEXT NOT NULL,
			value   TEXT NOT NULL,
			PRIMARY KEY (work_id, type)
		)`,
		`CREATE INDEX idx_identifiers_lookup ON identifiers(type, value)`,

		// ── CONTRIBUTORS ──────────────────────────────────────────────────────
		`CREATE TABLE contributors (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			sort_name   TEXT NOT NULL,
			identifiers TEXT DEFAULT '{}',
			UNIQUE (name, sort_name)
		)`,
		`CREATE INDEX idx_contributors_name ON contributors(sort_name COLLATE NOCASE)`,
		`CREATE TABLE work_contributors (
			work_id        TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			contributor_id TEXT    NOT NULL REFERENCES contributors(id) ON DELETE CASCADE,
			role           TEXT    NOT NULL DEFAULT 'author',
			position       INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (work_id, contributor_id, role)
		)`,
		`CREATE INDEX idx_work_contributors_work        ON work_contributors(work_id)`,
		`CREATE INDEX idx_work_contributors_contributor ON work_contributors(work_id, contributor_id)`,
		`CREATE INDEX idx_work_contributors_role        ON work_contributors(role)`,

		// ── SERIES ────────────────────────────────────────────────────────────
		`CREATE TABLE series (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			identifiers TEXT DEFAULT '{}'
		)`,
		`CREATE INDEX idx_series_name ON series(name COLLATE NOCASE)`,
		`CREATE TABLE work_series (
			work_id   TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			series_id TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
			position  REAL,
			PRIMARY KEY (work_id, series_id)
		)`,
		`CREATE INDEX idx_work_series_series ON work_series(series_id, position)`,

		// ── TAGS & GENRES ─────────────────────────────────────────────────────
		`CREATE TABLE tags (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'tag',
			UNIQUE (name, type)
		)`,
		`CREATE INDEX idx_tags_name ON tags(name COLLATE NOCASE)`,
		`CREATE TABLE work_tags (
			work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			tag_id  TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY (work_id, tag_id)
		)`,
		`CREATE INDEX idx_work_tags_tag ON work_tags(tag_id)`,

		// ── FILES ─────────────────────────────────────────────────────────────
		`CREATE TABLE work_files (
			id                TEXT    PRIMARY KEY,
			work_id           TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			filename          TEXT    NOT NULL,
			format            TEXT    NOT NULL,
			size_bytes        INTEGER NOT NULL,
			checksum_sha256   TEXT,
			duration_seconds  INTEGER,
			bitrate_kbps      INTEGER,
			codec             TEXT,
			has_media_overlay INTEGER DEFAULT 0,
			added_at          TEXT    NOT NULL DEFAULT (datetime('now')),
			UNIQUE (work_id, filename)
		)`,
		`CREATE INDEX idx_work_files_work   ON work_files(work_id)`,
		`CREATE INDEX idx_work_files_format ON work_files(format)`,

		// ── AUDIOBOOK CHAPTERS ────────────────────────────────────────────────
		`CREATE TABLE audiobook_chapters (
			id             TEXT    PRIMARY KEY,
			work_id        TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			title          TEXT    NOT NULL,
			start_seconds  REAL    NOT NULL,
			end_seconds    REAL    NOT NULL,
			index_position INTEGER NOT NULL,
			UNIQUE (work_id, index_position)
		)`,
		`CREATE INDEX idx_chapters_work ON audiobook_chapters(work_id, index_position)`,

		// ── RATINGS ───────────────────────────────────────────────────────────
		`CREATE TABLE ratings (
			work_id    TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			source     TEXT NOT NULL,
			score      REAL NOT NULL,
			max_score  REAL NOT NULL DEFAULT 5.0,
			count      INTEGER,
			fetched_at TEXT NOT NULL,
			PRIMARY KEY (work_id, source)
		)`,

		// ── COVERS ────────────────────────────────────────────────────────────
		`CREATE TABLE covers (
			work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			source      TEXT    NOT NULL,
			filename    TEXT    NOT NULL,
			width       INTEGER,
			height      INTEGER,
			is_selected INTEGER NOT NULL DEFAULT 0,
			fetched_at  TEXT,
			PRIMARY KEY (work_id, source)
		)`,

		// ── USERS & AUTH ──────────────────────────────────────────────────────
		`CREATE TABLE users (
			id            TEXT PRIMARY KEY,
			username      TEXT NOT NULL UNIQUE,
			display_name  TEXT,
			email         TEXT UNIQUE,
			password_hash TEXT,
			oidc_subject  TEXT UNIQUE,
			oidc_issuer   TEXT,
			role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user', 'guest')),
			is_active     INTEGER NOT NULL DEFAULT 1,
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			last_login_at TEXT
		)`,
		`CREATE TABLE sessions (
			id           TEXT PRIMARY KEY,
			user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_name  TEXT,
			device_type  TEXT,
			created_at   TEXT NOT NULL DEFAULT (datetime('now')),
			expires_at   TEXT NOT NULL,
			last_used_at TEXT
		)`,
		`CREATE INDEX idx_sessions_user    ON sessions(user_id)`,
		`CREATE INDEX idx_sessions_expires ON sessions(expires_at)`,

		// ── USER STATE ────────────────────────────────────────────────────────
		`CREATE TABLE user_progress (
			user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			work_id                TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			ebook_cfi              TEXT,
			ebook_percent          REAL,
			ebook_chapter          TEXT,
			audio_position_seconds REAL,
			audio_chapter_index    INTEGER,
			is_finished            INTEGER NOT NULL DEFAULT 0,
			started_at             TEXT,
			finished_at            TEXT,
			updated_at             TEXT NOT NULL DEFAULT (datetime('now')),
			last_device_id         TEXT,
			PRIMARY KEY (user_id, work_id)
		)`,
		`CREATE INDEX idx_progress_user ON user_progress(user_id)`,
		`CREATE TABLE user_annotations (
			id                     TEXT PRIMARY KEY,
			user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			work_id                TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			type                   TEXT NOT NULL CHECK (type IN ('bookmark', 'highlight', 'note')),
			ebook_cfi              TEXT,
			audio_position_seconds REAL,
			text                   TEXT,
			color                  TEXT,
			created_at             TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at             TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX idx_annotations_user_work ON user_annotations(user_id, work_id)`,

		// ── SYNC DEVICES ──────────────────────────────────────────────────────
		`CREATE TABLE sync_devices (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			device_name TEXT NOT NULL,
			device_type TEXT NOT NULL,
			device_id   TEXT,
			last_sync_at TEXT,
			settings    TEXT DEFAULT '{}',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX idx_sync_devices_user ON sync_devices(user_id)`,
		`CREATE TABLE sync_state (
			device_id        TEXT NOT NULL REFERENCES sync_devices(id) ON DELETE CASCADE,
			work_id          TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			last_synced_hash TEXT,
			last_synced_at   TEXT NOT NULL,
			PRIMARY KEY (device_id, work_id)
		)`,

		// ── COLLECTIONS ───────────────────────────────────────────────────────
		`CREATE TABLE collections (
			id              TEXT PRIMARY KEY,
			user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name            TEXT NOT NULL,
			description     TEXT,
			collection_type TEXT NOT NULL DEFAULT 'manual'
			                CHECK (collection_type IN ('manual', 'smart', 'device')),
			smart_filter    TEXT,
			device_id       TEXT,
			is_public       INTEGER NOT NULL DEFAULT 0,
			sort_order      INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (user_id, name)
		)`,
		`CREATE TABLE collection_works (
			collection_id TEXT    NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
			work_id       TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			position      INTEGER NOT NULL DEFAULT 0,
			added_at      TEXT    NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (collection_id, work_id)
		)`,

		// ── METADATA TASKS ────────────────────────────────────────────────────
		`CREATE TABLE metadata_tasks (
			id           TEXT PRIMARY KEY,
			work_id      TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			status       TEXT NOT NULL DEFAULT 'pending'
			             CHECK (status IN ('pending', 'running', 'completed', 'failed', 'review')),
			task_type    TEXT NOT NULL DEFAULT 'auto_match'
			             CHECK (task_type IN ('auto_match', 'refresh', 'manual_search')),
			priority     INTEGER NOT NULL DEFAULT 0,
			candidates   TEXT,
			selected     INTEGER,
			error        TEXT,
			created_at   TEXT NOT NULL DEFAULT (datetime('now')),
			started_at   TEXT,
			completed_at TEXT
		)`,
		`CREATE INDEX idx_tasks_status ON metadata_tasks(status, priority DESC)`,
		`CREATE INDEX idx_tasks_work   ON metadata_tasks(work_id)`,

		// ── SOURCE CACHE ──────────────────────────────────────────────────────
		`CREATE TABLE source_cache (
			work_id    TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			source     TEXT NOT NULL,
			query_used TEXT NOT NULL,
			response   TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			PRIMARY KEY (work_id, source)
		)`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("executing statement %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
