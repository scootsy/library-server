package queries

import (
	"database/sql"
	"fmt"
	"time"
)

// Work represents a single intellectual work (book or audiobook).
type Work struct {
	ID                string
	MediaRootID       string
	DirectoryPath     string
	Title             string
	SortTitle         string
	Subtitle          string
	Language          string
	Publisher         string
	PublishDate       string
	Description       string
	DescriptionFormat string
	PageCount         int
	DurationSeconds   int
	IsAbridged        bool
	HasMediaOverlay   bool
	MatchConfidence   float64
	MatchMethod       string
	PrimarySource     string
	NeedsReview       bool
	SidecarHash       string
	AddedAt           time.Time
	UpdatedAt         time.Time
	LastScannedAt     time.Time
}

// UpsertWork inserts or updates a work record. The work is identified by
// (media_root_id, directory_path). Returns the work's UUID.
func UpsertWork(db *sql.DB, w *Work) error {
	_, err := db.Exec(`
		INSERT INTO works (
			id, media_root_id, directory_path,
			title, sort_title, subtitle, language,
			publisher, publish_date, description, description_format,
			page_count, duration_seconds, is_abridged, has_media_overlay,
			match_confidence, match_method, primary_source,
			needs_review, sidecar_hash, last_scanned_at
		) VALUES (
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, datetime('now')
		)
		ON CONFLICT(media_root_id, directory_path) DO UPDATE SET
			title              = excluded.title,
			sort_title         = excluded.sort_title,
			subtitle           = excluded.subtitle,
			language           = excluded.language,
			publisher          = excluded.publisher,
			publish_date       = excluded.publish_date,
			description        = excluded.description,
			description_format = excluded.description_format,
			page_count         = excluded.page_count,
			duration_seconds   = excluded.duration_seconds,
			is_abridged        = excluded.is_abridged,
			has_media_overlay  = excluded.has_media_overlay,
			match_confidence   = excluded.match_confidence,
			match_method       = excluded.match_method,
			primary_source     = excluded.primary_source,
			needs_review       = excluded.needs_review,
			sidecar_hash       = excluded.sidecar_hash,
			updated_at         = datetime('now'),
			last_scanned_at    = datetime('now')
	`,
		w.ID, w.MediaRootID, w.DirectoryPath,
		w.Title, w.SortTitle, nullableString(w.Subtitle), nullableString(w.Language),
		nullableString(w.Publisher), nullableString(w.PublishDate),
		nullableString(w.Description), w.DescriptionFormat,
		nullableInt(w.PageCount), nullableInt(w.DurationSeconds),
		boolToInt(w.IsAbridged), boolToInt(w.HasMediaOverlay),
		nullableFloat(w.MatchConfidence), nullableString(w.MatchMethod),
		nullableString(w.PrimarySource),
		boolToInt(w.NeedsReview), nullableString(w.SidecarHash),
	)
	if err != nil {
		return fmt.Errorf("upserting work %q: %w", w.DirectoryPath, err)
	}
	return nil
}

// GetWorkByPath returns the work at the given (media_root_id, directory_path), or nil.
func GetWorkByPath(db *sql.DB, mediaRootID, dirPath string) (*Work, error) {
	row := db.QueryRow(`
		SELECT id, media_root_id, directory_path,
		       title, sort_title, COALESCE(subtitle,''), COALESCE(language,''),
		       COALESCE(publisher,''), COALESCE(publish_date,''),
		       COALESCE(description,''), COALESCE(description_format,'plain'),
		       COALESCE(page_count,0), COALESCE(duration_seconds,0),
		       is_abridged, has_media_overlay,
		       COALESCE(match_confidence,0), COALESCE(match_method,''),
		       COALESCE(primary_source,''),
		       needs_review, COALESCE(sidecar_hash,''),
		       added_at, updated_at, last_scanned_at
		FROM works
		WHERE media_root_id = ? AND directory_path = ?
	`, mediaRootID, dirPath)
	return scanWork(row)
}

// GetWorkByID returns the work with the given ID, or nil if not found.
func GetWorkByID(db *sql.DB, id string) (*Work, error) {
	row := db.QueryRow(`
		SELECT id, media_root_id, directory_path,
		       title, sort_title, COALESCE(subtitle,''), COALESCE(language,''),
		       COALESCE(publisher,''), COALESCE(publish_date,''),
		       COALESCE(description,''), COALESCE(description_format,'plain'),
		       COALESCE(page_count,0), COALESCE(duration_seconds,0),
		       is_abridged, has_media_overlay,
		       COALESCE(match_confidence,0), COALESCE(match_method,''),
		       COALESCE(primary_source,''),
		       needs_review, COALESCE(sidecar_hash,''),
		       added_at, updated_at, last_scanned_at
		FROM works
		WHERE id = ?
	`, id)
	return scanWork(row)
}

// TouchLastScanned updates last_scanned_at for the given work.
func TouchLastScanned(db *sql.DB, workID string) error {
	_, err := db.Exec(`UPDATE works SET last_scanned_at = datetime('now') WHERE id = ?`, workID)
	if err != nil {
		return fmt.Errorf("touching last_scanned_at for %q: %w", workID, err)
	}
	return nil
}

// UpdateFTSDenormalized rebuilds the FTS row for a work with current
// denormalized contributor, series, and tag data.
func UpdateFTSDenormalized(db *sql.DB, workID string) error {
	// Fetch denormalized contributor names for this work
	contribRow := db.QueryRow(`
		SELECT GROUP_CONCAT(c.name, ', ')
		FROM work_contributors wc
		JOIN contributors c ON c.id = wc.contributor_id
		WHERE wc.work_id = ?
		ORDER BY wc.position
	`, workID)
	var contrib sql.NullString
	if err := contribRow.Scan(&contrib); err != nil {
		return fmt.Errorf("fetching contributors for FTS: %w", err)
	}

	seriesRow := db.QueryRow(`
		SELECT GROUP_CONCAT(s.name, ', ')
		FROM work_series ws
		JOIN series s ON s.id = ws.series_id
		WHERE ws.work_id = ?
	`, workID)
	var seriesStr sql.NullString
	if err := seriesRow.Scan(&seriesStr); err != nil {
		return fmt.Errorf("fetching series for FTS: %w", err)
	}

	tagsRow := db.QueryRow(`
		SELECT GROUP_CONCAT(t.name, ', ')
		FROM work_tags wt
		JOIN tags t ON t.id = wt.tag_id
		WHERE wt.work_id = ?
	`, workID)
	var tagsStr sql.NullString
	if err := tagsRow.Scan(&tagsStr); err != nil {
		return fmt.Errorf("fetching tags for FTS: %w", err)
	}

	// Get work rowid
	var rowid int64
	if err := db.QueryRow(`SELECT rowid FROM works WHERE id = ?`, workID).Scan(&rowid); err != nil {
		return fmt.Errorf("fetching rowid for FTS update: %w", err)
	}

	// Delete old FTS row and reinsert with updated denormalized fields
	row := db.QueryRow(`
		SELECT title, COALESCE(subtitle,''), COALESCE(description,''), COALESCE(publisher,'')
		FROM works WHERE id = ?
	`, workID)
	var title, subtitle, description, publisher string
	if err := row.Scan(&title, &subtitle, &description, &publisher); err != nil {
		return fmt.Errorf("fetching work fields for FTS: %w", err)
	}

	_, err := db.Exec(`
		INSERT INTO works_fts(works_fts, rowid, title, subtitle, description, contributors, series, publisher, tags)
		VALUES ('delete', ?, ?, ?, ?, ?, ?, ?, ?)
	`, rowid, title, subtitle, description, contrib.String, seriesStr.String, publisher, tagsStr.String)
	if err != nil {
		return fmt.Errorf("deleting old FTS row: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO works_fts(rowid, title, subtitle, description, contributors, series, publisher, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, rowid, title, subtitle, description, contrib.String, seriesStr.String, publisher, tagsStr.String)
	if err != nil {
		return fmt.Errorf("inserting new FTS row: %w", err)
	}

	return nil
}

func scanWork(row *sql.Row) (*Work, error) {
	var w Work
	var addedAt, updatedAt, lastScannedAt string
	var isAbridged, hasMediaOverlay, needsReview int
	err := row.Scan(
		&w.ID, &w.MediaRootID, &w.DirectoryPath,
		&w.Title, &w.SortTitle, &w.Subtitle, &w.Language,
		&w.Publisher, &w.PublishDate,
		&w.Description, &w.DescriptionFormat,
		&w.PageCount, &w.DurationSeconds,
		&isAbridged, &hasMediaOverlay,
		&w.MatchConfidence, &w.MatchMethod,
		&w.PrimarySource,
		&needsReview, &w.SidecarHash,
		&addedAt, &updatedAt, &lastScannedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning work: %w", err)
	}
	w.IsAbridged = isAbridged != 0
	w.HasMediaOverlay = hasMediaOverlay != 0
	w.NeedsReview = needsReview != 0
	w.AddedAt = parseDBTime(addedAt)
	w.UpdatedAt = parseDBTime(updatedAt)
	w.LastScannedAt = parseDBTime(lastScannedAt)
	return &w, nil
}
