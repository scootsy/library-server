package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Contributor is a person associated with a work (author, narrator, etc.).
type Contributor struct {
	ID          string
	Name        string
	SortName    string
	Identifiers map[string]string
}

// WorkContributor links a work to a contributor with a role and display order.
type WorkContributor struct {
	WorkID        string
	ContributorID string
	Role          string
	Position      int
}

// ListContributors returns all contributors with their work count.
func ListContributors(db *sql.DB, limit, offset int) ([]struct {
	Contributor
	WorkCount int
}, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM contributors`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting contributors: %w", err)
	}

	rows, err := db.Query(`
		SELECT c.id, c.name, c.sort_name, c.identifiers, COUNT(DISTINCT wc.work_id) as work_count
		FROM contributors c
		LEFT JOIN work_contributors wc ON wc.contributor_id = c.id
		GROUP BY c.id
		ORDER BY c.sort_name COLLATE NOCASE
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing contributors: %w", err)
	}
	defer rows.Close()

	var results []struct {
		Contributor
		WorkCount int
	}
	for rows.Next() {
		var c Contributor
		var identJSON string
		var count int
		if err := rows.Scan(&c.ID, &c.Name, &c.SortName, &identJSON, &count); err != nil {
			return nil, 0, fmt.Errorf("scanning contributor: %w", err)
		}
		c.Identifiers = make(map[string]string)
		if identJSON != "" && identJSON != "{}" {
			_ = json.Unmarshal([]byte(identJSON), &c.Identifiers)
		}
		results = append(results, struct {
			Contributor
			WorkCount int
		}{c, count})
	}
	return results, total, rows.Err()
}

// GetContributorByID returns a contributor by ID, or nil if not found.
func GetContributorByID(db *sql.DB, id string) (*Contributor, error) {
	var c Contributor
	var identJSON string
	err := db.QueryRow(`SELECT id, name, sort_name, identifiers FROM contributors WHERE id = ?`, id).Scan(
		&c.ID, &c.Name, &c.SortName, &identJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying contributor %q: %w", id, err)
	}
	c.Identifiers = make(map[string]string)
	if identJSON != "" && identJSON != "{}" {
		_ = json.Unmarshal([]byte(identJSON), &c.Identifiers)
	}
	return &c, nil
}

// GetWorksByContributor returns all works by a given contributor.
func GetWorksByContributor(db *sql.DB, contributorID string, limit, offset int) ([]*WorkSummary, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var total int
	err := db.QueryRow(`SELECT COUNT(DISTINCT work_id) FROM work_contributors WHERE contributor_id = ?`, contributorID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting works for contributor: %w", err)
	}

	rows, err := db.Query(`
		SELECT w.id, w.title, w.sort_title, COALESCE(w.subtitle,''), COALESCE(w.language,''),
		       COALESCE(w.publisher,''), COALESCE(w.publish_date,''),
		       COALESCE(w.page_count,0), COALESCE(w.duration_seconds,0),
		       COALESCE(w.match_confidence,0), w.needs_review, w.has_media_overlay,
		       w.added_at, w.updated_at
		FROM work_contributors wc
		JOIN works w ON w.id = wc.work_id
		WHERE wc.contributor_id = ?
		GROUP BY w.id
		ORDER BY w.sort_title
		LIMIT ? OFFSET ?
	`, contributorID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying works by contributor: %w", err)
	}
	defer rows.Close()

	return scanWorkSummaryRows(rows, total)
}

// UpsertContributor inserts or updates a contributor by (name, sort_name).
// The provided id is used only on insert; on conflict the existing row is returned.
func UpsertContributor(db *sql.DB, id, name, sortName string, identifiers map[string]string) (string, error) {
	identJSON, err := json.Marshal(identifiers)
	if err != nil {
		return "", fmt.Errorf("marshalling contributor identifiers: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO contributors (id, name, sort_name, identifiers)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name, sort_name) DO UPDATE SET
			identifiers = excluded.identifiers
	`, id, name, sortName, string(identJSON))
	if err != nil {
		return "", fmt.Errorf("upserting contributor %q: %w", name, err)
	}

	// Return the actual ID (may differ from the provided one if row already existed).
	var actualID string
	err = db.QueryRow(`SELECT id FROM contributors WHERE name = ? AND sort_name = ?`, name, sortName).Scan(&actualID)
	if err != nil {
		return "", fmt.Errorf("fetching contributor id for %q: %w", name, err)
	}
	return actualID, nil
}

// UpsertWorkContributor links a contributor to a work with the given role/position.
func UpsertWorkContributor(db *sql.DB, workID, contributorID, role string, position int) error {
	_, err := db.Exec(`
		INSERT INTO work_contributors (work_id, contributor_id, role, position)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(work_id, contributor_id, role) DO UPDATE SET
			position = excluded.position
	`, workID, contributorID, role, position)
	if err != nil {
		return fmt.Errorf("upserting work_contributor (%s, %s, %s): %w", workID, contributorID, role, err)
	}
	return nil
}

// GetWorkAuthorNames returns the names of all authors for a work, ordered by position.
func GetWorkAuthorNames(db *sql.DB, workID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT c.name
		FROM work_contributors wc
		JOIN contributors c ON c.id = wc.contributor_id
		WHERE wc.work_id = ? AND wc.role = 'author'
		ORDER BY wc.position
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying authors for work %q: %w", workID, err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning author name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// DeleteWorkContributors removes all contributor links for a work.
// Called before re-inserting updated contributor data.
func DeleteWorkContributors(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM work_contributors WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting work_contributors for %q: %w", workID, err)
	}
	return nil
}
