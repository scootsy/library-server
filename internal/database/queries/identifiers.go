package queries

import (
	"database/sql"
	"fmt"
)

// UpsertIdentifier inserts or replaces an identifier for a work.
func UpsertIdentifier(db *sql.DB, workID, idType, value string) error {
	_, err := db.Exec(`
		INSERT INTO identifiers (work_id, type, value)
		VALUES (?, ?, ?)
		ON CONFLICT(work_id, type) DO UPDATE SET value = excluded.value
	`, workID, idType, value)
	if err != nil {
		return fmt.Errorf("upserting identifier (%s, %s): %w", idType, value, err)
	}
	return nil
}

// DeleteWorkIdentifiers removes all identifiers for a work.
func DeleteWorkIdentifiers(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM identifiers WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting identifiers for %q: %w", workID, err)
	}
	return nil
}

// GetWorkIdentifiers returns all identifiers for a work as a type→value map.
func GetWorkIdentifiers(db *sql.DB, workID string) (map[string]string, error) {
	rows, err := db.Query(`SELECT type, value FROM identifiers WHERE work_id = ?`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying identifiers for %q: %w", workID, err)
	}
	defer rows.Close()

	ids := make(map[string]string)
	for rows.Next() {
		var idType, value string
		if err := rows.Scan(&idType, &value); err != nil {
			return nil, fmt.Errorf("scanning identifier row: %w", err)
		}
		ids[idType] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating identifier rows: %w", err)
	}
	return ids, nil
}

// GetWorkDirectoryPath returns the absolute directory path for a work by
// joining the media root path with the work's relative directory path.
func GetWorkDirectoryPath(db *sql.DB, workID string) (rootPath, dirPath string, err error) {
	row := db.QueryRow(`
		SELECT mr.root_path, w.directory_path
		FROM works w
		JOIN media_roots mr ON mr.id = w.media_root_id
		WHERE w.id = ?
	`, workID)
	err = row.Scan(&rootPath, &dirPath)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("work %q not found", workID)
	}
	if err != nil {
		return "", "", fmt.Errorf("querying work directory path: %w", err)
	}
	return rootPath, dirPath, nil
}
