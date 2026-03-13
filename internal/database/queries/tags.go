package queries

import (
	"database/sql"
	"fmt"
)

// Tag represents a tag or genre.
type Tag struct {
	ID   string
	Name string
	Type string
}

// ListTags returns all tags with optional work count.
func ListTags(db *sql.DB) ([]struct {
	Tag
	WorkCount int
}, error) {
	rows, err := db.Query(`
		SELECT t.id, t.name, t.type, COUNT(wt.work_id) as work_count
		FROM tags t
		LEFT JOIN work_tags wt ON wt.tag_id = t.id
		GROUP BY t.id
		ORDER BY t.name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer rows.Close()

	var results []struct {
		Tag
		WorkCount int
	}
	for rows.Next() {
		var t Tag
		var count int
		if err := rows.Scan(&t.ID, &t.Name, &t.Type, &count); err != nil {
			return nil, fmt.Errorf("scanning tag: %w", err)
		}
		results = append(results, struct {
			Tag
			WorkCount int
		}{t, count})
	}
	return results, rows.Err()
}

// GetTagByID returns a tag by ID, or nil if not found.
func GetTagByID(db *sql.DB, id string) (*Tag, error) {
	var t Tag
	err := db.QueryRow(`SELECT id, name, type FROM tags WHERE id = ?`, id).Scan(&t.ID, &t.Name, &t.Type)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying tag %q: %w", id, err)
	}
	return &t, nil
}

// GetWorksByTag returns all works tagged with the given tag ID.
func GetWorksByTag(db *sql.DB, tagID string, limit, offset int) ([]*WorkSummary, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM work_tags WHERE tag_id = ?`, tagID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting works for tag: %w", err)
	}

	rows, err := db.Query(`
		SELECT w.id, w.title, w.sort_title, COALESCE(w.subtitle,''), COALESCE(w.language,''),
		       COALESCE(w.publisher,''), COALESCE(w.publish_date,''),
		       COALESCE(w.page_count,0), COALESCE(w.duration_seconds,0),
		       COALESCE(w.match_confidence,0), w.needs_review, w.has_media_overlay,
		       w.added_at, w.updated_at
		FROM work_tags wt
		JOIN works w ON w.id = wt.work_id
		WHERE wt.tag_id = ?
		ORDER BY w.sort_title
		LIMIT ? OFFSET ?
	`, tagID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying works by tag: %w", err)
	}
	defer rows.Close()

	return scanWorkSummaryRows(rows, total)
}

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
		ON CONFLICT(work_id, tag_id) DO NOTHING
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
