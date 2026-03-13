package queries

import (
	"database/sql"
	"fmt"
)

// UpsertTag inserts or updates a tag by (name, type). Returns the tag's actual ID.
func UpsertTag(db *sql.DB, id, name, tagType string) (string, error) {
	_, err := db.Exec(`
		INSERT INTO tags (id, name, type)
		VALUES (?, ?, ?)
		ON CONFLICT(name, type) DO NOTHING
	`, id, name, tagType)
	if err != nil {
		return "", fmt.Errorf("upserting tag %q (%s): %w", name, tagType, err)
	}

	var actualID string
	err = db.QueryRow(`SELECT id FROM tags WHERE name = ? AND type = ?`, name, tagType).Scan(&actualID)
	if err != nil {
		return "", fmt.Errorf("fetching tag id for %q: %w", name, err)
	}
	return actualID, nil
}

// UpsertWorkTag links a tag to a work.
func UpsertWorkTag(db *sql.DB, workID, tagID string) error {
	_, err := db.Exec(`
		INSERT INTO work_tags (work_id, tag_id)
		VALUES (?, ?)
		ON CONFLICT DO NOTHING
	`, workID, tagID)
	if err != nil {
		return fmt.Errorf("upserting work_tag (%s, %s): %w", workID, tagID, err)
	}
	return nil
}

// DeleteWorkTags removes all tag links for a work.
func DeleteWorkTags(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM work_tags WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting work_tags for %q: %w", workID, err)
	}
	return nil
}
