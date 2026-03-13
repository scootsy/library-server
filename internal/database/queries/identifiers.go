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
