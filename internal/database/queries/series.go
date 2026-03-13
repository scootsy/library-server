package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Series represents a book series or universe.
type Series struct {
	ID          string
	Name        string
	Identifiers map[string]string
}

// UpsertSeries inserts or updates a series by name. Returns the series' actual ID.
func UpsertSeries(db *sql.DB, id, name string, identifiers map[string]string) (string, error) {
	identJSON, err := json.Marshal(identifiers)
	if err != nil {
		return "", fmt.Errorf("marshalling series identifiers: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO series (id, name, identifiers)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			identifiers = excluded.identifiers
	`, id, name, string(identJSON))
	if err != nil {
		return "", fmt.Errorf("upserting series %q: %w", name, err)
	}

	var actualID string
	err = db.QueryRow(`SELECT id FROM series WHERE name = ?`, name).Scan(&actualID)
	if err != nil {
		return "", fmt.Errorf("fetching series id for %q: %w", name, err)
	}
	return actualID, nil
}

// UpsertWorkSeries links a work to a series with an optional position.
func UpsertWorkSeries(db *sql.DB, workID, seriesID string, position *float64) error {
	_, err := db.Exec(`
		INSERT INTO work_series (work_id, series_id, position)
		VALUES (?, ?, ?)
		ON CONFLICT(work_id, series_id) DO UPDATE SET
			position = excluded.position
	`, workID, seriesID, position)
	if err != nil {
		return fmt.Errorf("upserting work_series (%s, %s): %w", workID, seriesID, err)
	}
	return nil
}

// DeleteWorkSeries removes all series links for a work.
func DeleteWorkSeries(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM work_series WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting work_series for %q: %w", workID, err)
	}
	return nil
}
