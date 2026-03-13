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

// DeleteWorkContributors removes all contributor links for a work.
// Called before re-inserting updated contributor data.
func DeleteWorkContributors(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM work_contributors WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting work_contributors for %q: %w", workID, err)
	}
	return nil
}
