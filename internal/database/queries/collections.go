package queries

import (
	"database/sql"
	"fmt"
	"time"
)

// Collection represents a virtual shelf or library.
type Collection struct {
	ID             string
	UserID         string
	Name           string
	Description    string
	CollectionType string // manual, smart, device
	SmartFilter    string // JSON filter rules for smart collections
	DeviceID       string
	IsPublic       bool
	SortOrder      int
	CreatedAt      time.Time
}

// ListCollections returns all collections, optionally filtered by user.
func ListCollections(db *sql.DB, userID string) ([]*Collection, error) {
	var rows *sql.Rows
	var err error

	if userID != "" {
		rows, err = db.Query(`
			SELECT id, user_id, name, COALESCE(description,''), collection_type,
			       COALESCE(smart_filter,''), COALESCE(device_id,''),
			       is_public, sort_order, created_at
			FROM collections
			WHERE user_id = ?
			ORDER BY sort_order, name
		`, userID)
	} else {
		rows, err = db.Query(`
			SELECT id, user_id, name, COALESCE(description,''), collection_type,
			       COALESCE(smart_filter,''), COALESCE(device_id,''),
			       is_public, sort_order, created_at
			FROM collections
			ORDER BY sort_order, name
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("listing collections: %w", err)
	}
	defer rows.Close()

	var collections []*Collection
	for rows.Next() {
		c, err := scanCollectionRow(rows)
		if err != nil {
			return nil, err
		}
		collections = append(collections, c)
	}
	return collections, rows.Err()
}

// GetCollectionByID returns a collection by ID, or nil if not found.
func GetCollectionByID(db *sql.DB, id string) (*Collection, error) {
	row := db.QueryRow(`
		SELECT id, user_id, name, COALESCE(description,''), collection_type,
		       COALESCE(smart_filter,''), COALESCE(device_id,''),
		       is_public, sort_order, created_at
		FROM collections
		WHERE id = ?
	`, id)

	var c Collection
	var isPublic int
	var createdAt string
	err := row.Scan(
		&c.ID, &c.UserID, &c.Name, &c.Description, &c.CollectionType,
		&c.SmartFilter, &c.DeviceID,
		&isPublic, &c.SortOrder, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying collection %q: %w", id, err)
	}
	c.IsPublic = isPublic != 0
	c.CreatedAt = parseDBTime(createdAt)
	return &c, nil
}

// CreateCollection inserts a new collection.
func CreateCollection(db *sql.DB, c *Collection) error {
	_, err := db.Exec(`
		INSERT INTO collections (id, user_id, name, description, collection_type, smart_filter, device_id, is_public, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		c.ID, c.UserID, c.Name,
		nullableString(c.Description), c.CollectionType,
		nullableString(c.SmartFilter), nullableString(c.DeviceID),
		boolToInt(c.IsPublic), c.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("creating collection %q: %w", c.Name, err)
	}
	return nil
}

// UpdateCollection updates a collection's mutable fields.
func UpdateCollection(db *sql.DB, c *Collection) error {
	_, err := db.Exec(`
		UPDATE collections
		SET name = ?, description = ?, collection_type = ?,
		    smart_filter = ?, device_id = ?,
		    is_public = ?, sort_order = ?
		WHERE id = ?
	`,
		c.Name, nullableString(c.Description), c.CollectionType,
		nullableString(c.SmartFilter), nullableString(c.DeviceID),
		boolToInt(c.IsPublic), c.SortOrder,
		c.ID,
	)
	if err != nil {
		return fmt.Errorf("updating collection %q: %w", c.ID, err)
	}
	return nil
}

// DeleteCollection removes a collection and its work associations.
func DeleteCollection(db *sql.DB, id string) error {
	result, err := db.Exec(`DELETE FROM collections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting collection %q: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("collection %q not found", id)
	}
	return nil
}

// AddWorkToCollection adds a work to a manual collection.
func AddWorkToCollection(db *sql.DB, collectionID, workID string, position int) error {
	_, err := db.Exec(`
		INSERT INTO collection_works (collection_id, work_id, position)
		VALUES (?, ?, ?)
		ON CONFLICT(collection_id, work_id) DO UPDATE SET position = excluded.position
	`, collectionID, workID, position)
	if err != nil {
		return fmt.Errorf("adding work to collection: %w", err)
	}
	return nil
}

// RemoveWorkFromCollection removes a work from a collection.
func RemoveWorkFromCollection(db *sql.DB, collectionID, workID string) error {
	_, err := db.Exec(`DELETE FROM collection_works WHERE collection_id = ? AND work_id = ?`, collectionID, workID)
	if err != nil {
		return fmt.Errorf("removing work from collection: %w", err)
	}
	return nil
}

// GetCollectionWorks returns all works in a collection.
func GetCollectionWorks(db *sql.DB, collectionID string, limit, offset int) ([]*WorkSummary, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM collection_works WHERE collection_id = ?`, collectionID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting collection works: %w", err)
	}

	rows, err := db.Query(`
		SELECT w.id, w.title, w.sort_title, COALESCE(w.subtitle,''), COALESCE(w.language,''),
		       COALESCE(w.publisher,''), COALESCE(w.publish_date,''),
		       COALESCE(w.page_count,0), COALESCE(w.duration_seconds,0),
		       COALESCE(w.match_confidence,0), w.needs_review, w.has_media_overlay,
		       w.added_at, w.updated_at
		FROM collection_works cw
		JOIN works w ON w.id = cw.work_id
		WHERE cw.collection_id = ?
		ORDER BY cw.position, w.sort_title
		LIMIT ? OFFSET ?
	`, collectionID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying collection works: %w", err)
	}
	defer rows.Close()

	return scanWorkSummaryRows(rows, total)
}

// CountCollections returns the total number of collections.
func CountCollections(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM collections`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting collections: %w", err)
	}
	return count, nil
}

func scanCollectionRow(rows *sql.Rows) (*Collection, error) {
	var c Collection
	var isPublic int
	var createdAt string
	if err := rows.Scan(
		&c.ID, &c.UserID, &c.Name, &c.Description, &c.CollectionType,
		&c.SmartFilter, &c.DeviceID,
		&isPublic, &c.SortOrder, &createdAt,
	); err != nil {
		return nil, fmt.Errorf("scanning collection row: %w", err)
	}
	c.IsPublic = isPublic != 0
	c.CreatedAt = parseDBTime(createdAt)
	return &c, nil
}
