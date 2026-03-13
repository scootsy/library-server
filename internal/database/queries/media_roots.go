package queries

import (
	"database/sql"
	"fmt"
	"time"
)

// MediaRoot represents a configured library root directory.
type MediaRoot struct {
	ID         string
	Name       string
	RootPath   string
	ScanConfig string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// UpsertMediaRoot inserts or updates a media root record. Returns the root's ID.
func UpsertMediaRoot(db *sql.DB, id, name, rootPath string) error {
	_, err := db.Exec(`
		INSERT INTO media_roots (id, name, root_path)
		VALUES (?, ?, ?)
		ON CONFLICT(root_path) DO UPDATE SET
			name       = excluded.name,
			updated_at = datetime('now')
	`, id, name, rootPath)
	if err != nil {
		return fmt.Errorf("upserting media root: %w", err)
	}
	return nil
}

// GetMediaRootByPath returns the media root with the given path, or nil if not found.
func GetMediaRootByPath(db *sql.DB, path string) (*MediaRoot, error) {
	row := db.QueryRow(`
		SELECT id, name, root_path, scan_config, created_at, updated_at
		FROM media_roots
		WHERE root_path = ?
	`, path)
	return scanMediaRoot(row)
}

// GetMediaRootByID returns the media root with the given ID, or nil if not found.
func GetMediaRootByID(db *sql.DB, id string) (*MediaRoot, error) {
	row := db.QueryRow(`
		SELECT id, name, root_path, scan_config, created_at, updated_at
		FROM media_roots
		WHERE id = ?
	`, id)
	return scanMediaRoot(row)
}

// ListMediaRoots returns all configured media roots.
func ListMediaRoots(db *sql.DB) ([]*MediaRoot, error) {
	rows, err := db.Query(`
		SELECT id, name, root_path, scan_config, created_at, updated_at
		FROM media_roots
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing media roots: %w", err)
	}
	defer rows.Close()

	var roots []*MediaRoot
	for rows.Next() {
		var r MediaRoot
		var createdAt, updatedAt string
		if err := rows.Scan(&r.ID, &r.Name, &r.RootPath, &r.ScanConfig, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning media root row: %w", err)
		}
		r.CreatedAt = parseDBTime(createdAt)
		r.UpdatedAt = parseDBTime(updatedAt)
		roots = append(roots, &r)
	}
	return roots, rows.Err()
}

func scanMediaRoot(row *sql.Row) (*MediaRoot, error) {
	var r MediaRoot
	var createdAt, updatedAt string
	err := row.Scan(&r.ID, &r.Name, &r.RootPath, &r.ScanConfig, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning media root: %w", err)
	}
	r.CreatedAt = parseDBTime(createdAt)
	r.UpdatedAt = parseDBTime(updatedAt)
	return &r, nil
}
