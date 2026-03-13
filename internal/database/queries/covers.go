package queries

import (
	"database/sql"
	"fmt"
)

// Cover represents a cover image source for a work.
type Cover struct {
	WorkID     string
	Source     string
	Filename   string
	Width      int
	Height     int
	IsSelected bool
}

// UpsertCover inserts or updates a cover record.
func UpsertCover(db *sql.DB, c *Cover) error {
	_, err := db.Exec(`
		INSERT INTO covers (work_id, source, filename, width, height, is_selected)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(work_id, source) DO UPDATE SET
			filename    = excluded.filename,
			width       = excluded.width,
			height      = excluded.height,
			is_selected = excluded.is_selected
	`,
		c.WorkID, c.Source, c.Filename,
		nullableInt(c.Width), nullableInt(c.Height),
		boolToInt(c.IsSelected),
	)
	if err != nil {
		return fmt.Errorf("upserting cover (%s, %s): %w", c.WorkID, c.Source, err)
	}
	return nil
}

// DeleteWorkCovers removes all cover records for a work.
func DeleteWorkCovers(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM covers WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting covers for %q: %w", workID, err)
	}
	return nil
}

// GetSelectedCover returns the currently selected cover for a work.
func GetSelectedCover(db *sql.DB, workID string) (*Cover, error) {
	row := db.QueryRow(`
		SELECT work_id, source, filename, COALESCE(width,0), COALESCE(height,0), is_selected
		FROM covers
		WHERE work_id = ? AND is_selected = 1
	`, workID)

	var c Cover
	var isSelected int
	err := row.Scan(&c.WorkID, &c.Source, &c.Filename, &c.Width, &c.Height, &isSelected)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning cover: %w", err)
	}
	c.IsSelected = isSelected != 0
	return &c, nil
}
