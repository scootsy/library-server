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

// ListSeries returns all series with their work count.
func ListSeries(db *sql.DB, limit, offset int) ([]struct {
	Series
	WorkCount int
}, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM series`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting series: %w", err)
	}

	rows, err := db.Query(`
		SELECT s.id, s.name, s.identifiers, COUNT(ws.work_id) as work_count
		FROM series s
		LEFT JOIN work_series ws ON ws.series_id = s.id
		GROUP BY s.id
		ORDER BY s.name COLLATE NOCASE
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing series: %w", err)
	}
	defer rows.Close()

	var results []struct {
		Series
		WorkCount int
	}
	for rows.Next() {
		var s Series
		var identJSON string
		var count int
		if err := rows.Scan(&s.ID, &s.Name, &identJSON, &count); err != nil {
			return nil, 0, fmt.Errorf("scanning series: %w", err)
		}
		s.Identifiers = make(map[string]string)
		if identJSON != "" && identJSON != "{}" {
			_ = json.Unmarshal([]byte(identJSON), &s.Identifiers)
		}
		results = append(results, struct {
			Series
			WorkCount int
		}{s, count})
	}
	return results, total, rows.Err()
}

// GetSeriesByID returns a series by ID, or nil if not found.
func GetSeriesByID(db *sql.DB, id string) (*Series, error) {
	var s Series
	var identJSON string
	err := db.QueryRow(`SELECT id, name, identifiers FROM series WHERE id = ?`, id).Scan(
		&s.ID, &s.Name, &identJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying series %q: %w", id, err)
	}
	s.Identifiers = make(map[string]string)
	if identJSON != "" && identJSON != "{}" {
		_ = json.Unmarshal([]byte(identJSON), &s.Identifiers)
	}
	return &s, nil
}

// GetWorksBySeries returns all works in a given series, ordered by position.
func GetWorksBySeries(db *sql.DB, seriesID string, limit, offset int) ([]struct {
	*WorkSummary
	Position *float64
}, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM work_series WHERE series_id = ?`, seriesID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting works in series: %w", err)
	}

	rows, err := db.Query(`
		SELECT w.id, w.title, w.sort_title, COALESCE(w.subtitle,''), COALESCE(w.language,''),
		       COALESCE(w.publisher,''), COALESCE(w.publish_date,''),
		       COALESCE(w.page_count,0), COALESCE(w.duration_seconds,0),
		       COALESCE(w.match_confidence,0), w.needs_review, w.has_media_overlay,
		       w.added_at, w.updated_at, ws.position
		FROM work_series ws
		JOIN works w ON w.id = ws.work_id
		WHERE ws.series_id = ?
		ORDER BY ws.position, w.sort_title
		LIMIT ? OFFSET ?
	`, seriesID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying works by series: %w", err)
	}
	defer rows.Close()

	var results []struct {
		*WorkSummary
		Position *float64
	}
	for rows.Next() {
		var w WorkSummary
		var needsReview, hasOverlay int
		var addedAt, updatedAt string
		var pos *float64
		if err := rows.Scan(
			&w.ID, &w.Title, &w.SortTitle, &w.Subtitle, &w.Language,
			&w.Publisher, &w.PublishDate,
			&w.PageCount, &w.DurationSeconds,
			&w.MatchConfidence, &needsReview, &hasOverlay,
			&addedAt, &updatedAt, &pos,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning series work: %w", err)
		}
		w.NeedsReview = needsReview != 0
		w.HasMediaOverlay = hasOverlay != 0
		w.AddedAt = parseDBTime(addedAt)
		w.UpdatedAt = parseDBTime(updatedAt)
		results = append(results, struct {
			*WorkSummary
			Position *float64
		}{&w, pos})
	}
	return results, total, rows.Err()
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
