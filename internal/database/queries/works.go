package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
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

// WorkListParams controls pagination, sorting, and filtering for ListWorks.
type WorkListParams struct {
	Limit       int
	Offset      int
	SortBy      string // sort_title, added_at, updated_at, publish_date
	SortOrder   string // asc, desc
	NeedsReview *bool
	Language    string
	Format      string // filter to works that have at least one file with this format
}

// WorkSummary is a lightweight view of a work for list endpoints.
type WorkSummary struct {
	ID              string
	Title           string
	SortTitle       string
	Subtitle        string
	Language        string
	Publisher       string
	PublishDate     string
	PageCount       int
	DurationSeconds int
	MatchConfidence float64
	NeedsReview     bool
	HasMediaOverlay bool
	AddedAt         time.Time
	UpdatedAt       time.Time
}

// ListWorks returns a paginated, sorted list of works.
func ListWorks(db *sql.DB, p WorkListParams) ([]*WorkSummary, int, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 50
	}

	// Validate sort column to prevent injection via column name.
	sortCol := "sort_title"
	switch p.SortBy {
	case "added_at":
		sortCol = "added_at"
	case "updated_at":
		sortCol = "updated_at"
	case "publish_date":
		sortCol = "publish_date"
	case "title", "sort_title":
		sortCol = "sort_title"
	}
	sortDir := "ASC"
	if p.SortOrder == "desc" {
		sortDir = "DESC"
	}

	var args []any
	where := "1=1"

	if p.NeedsReview != nil {
		where += " AND needs_review = ?"
		args = append(args, boolToInt(*p.NeedsReview))
	}
	if p.Language != "" {
		where += " AND language = ?"
		args = append(args, p.Language)
	}
	if p.Format != "" {
		where += " AND id IN (SELECT work_id FROM work_files WHERE format = ?)"
		args = append(args, p.Format)
	}

	// Count total matching rows.
	var total int
	countQuery := "SELECT COUNT(*) FROM works WHERE " + where
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting works: %w", err)
	}

	// Fetch page. Sort column and direction are validated above, not user-supplied strings.
	query := fmt.Sprintf(`
		SELECT id, title, sort_title, COALESCE(subtitle,''), COALESCE(language,''),
		       COALESCE(publisher,''), COALESCE(publish_date,''),
		       COALESCE(page_count,0), COALESCE(duration_seconds,0),
		       COALESCE(match_confidence,0), needs_review, has_media_overlay,
		       added_at, updated_at
		FROM works
		WHERE %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, where, sortCol, sortDir)

	pageArgs := append(args, p.Limit, p.Offset)
	rows, err := db.Query(query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing works: %w", err)
	}
	defer rows.Close()

	var works []*WorkSummary
	for rows.Next() {
		var w WorkSummary
		var needsReview, hasOverlay int
		var addedAt, updatedAt string
		if err := rows.Scan(
			&w.ID, &w.Title, &w.SortTitle, &w.Subtitle, &w.Language,
			&w.Publisher, &w.PublishDate,
			&w.PageCount, &w.DurationSeconds,
			&w.MatchConfidence, &needsReview, &hasOverlay,
			&addedAt, &updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning work row: %w", err)
		}
		w.NeedsReview = needsReview != 0
		w.HasMediaOverlay = hasOverlay != 0
		w.AddedAt = parseDBTime(addedAt)
		w.UpdatedAt = parseDBTime(updatedAt)
		works = append(works, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating work rows: %w", err)
	}
	return works, total, nil
}

// SearchWorks performs a full-text search using FTS5 and returns matching work IDs.
func SearchWorks(db *sql.DB, query string, limit, offset int) ([]*WorkSummary, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var total int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM works_fts WHERE works_fts MATCH ?
	`, query).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting FTS results: %w", err)
	}

	rows, err := db.Query(`
		SELECT w.id, w.title, w.sort_title, COALESCE(w.subtitle,''), COALESCE(w.language,''),
		       COALESCE(w.publisher,''), COALESCE(w.publish_date,''),
		       COALESCE(w.page_count,0), COALESCE(w.duration_seconds,0),
		       COALESCE(w.match_confidence,0), w.needs_review, w.has_media_overlay,
		       w.added_at, w.updated_at
		FROM works_fts fts
		JOIN works w ON w.rowid = fts.rowid
		WHERE works_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("searching works: %w", err)
	}
	defer rows.Close()

	var works []*WorkSummary
	for rows.Next() {
		var w WorkSummary
		var needsReview, hasOverlay int
		var addedAt, updatedAt string
		if err := rows.Scan(
			&w.ID, &w.Title, &w.SortTitle, &w.Subtitle, &w.Language,
			&w.Publisher, &w.PublishDate,
			&w.PageCount, &w.DurationSeconds,
			&w.MatchConfidence, &needsReview, &hasOverlay,
			&addedAt, &updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning FTS result: %w", err)
		}
		w.NeedsReview = needsReview != 0
		w.HasMediaOverlay = hasOverlay != 0
		w.AddedAt = parseDBTime(addedAt)
		w.UpdatedAt = parseDBTime(updatedAt)
		works = append(works, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating FTS rows: %w", err)
	}
	return works, total, nil
}

// UpdateWorkMetadata updates user-editable metadata fields on a work.
func UpdateWorkMetadata(db *sql.DB, id string, fields map[string]any) error {
	// Allowed fields whitelist to prevent arbitrary column updates.
	allowed := map[string]bool{
		"title": true, "sort_title": true, "subtitle": true,
		"language": true, "publisher": true, "publish_date": true,
		"description": true, "description_format": true,
		"page_count": true, "duration_seconds": true,
	}

	var setClauses []string
	var args []any
	for k, v := range fields {
		if !allowed[k] {
			continue
		}
		setClauses = append(setClauses, k+" = ?")
		args = append(args, v)
	}
	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, "updated_at = datetime('now')")
	args = append(args, id)

	query := "UPDATE works SET " + joinStrings(setClauses, ", ") + " WHERE id = ?"
	_, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("updating work %q: %w", id, err)
	}
	return nil
}

// DeleteWork deletes a work and all related records (cascade).
func DeleteWork(db *sql.DB, id string) error {
	result, err := db.Exec(`DELETE FROM works WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting work %q: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("work %q not found", id)
	}
	return nil
}

// CountWorks returns the total number of works.
func CountWorks(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM works`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting works: %w", err)
	}
	return count, nil
}

// CountNeedsReview returns works flagged for review.
func CountNeedsReview(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM works WHERE needs_review = 1`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting review works: %w", err)
	}
	return count, nil
}

// RecentWorks returns the most recently added works.
func RecentWorks(db *sql.DB, limit int) ([]*WorkSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := db.Query(`
		SELECT id, title, sort_title, COALESCE(subtitle,''), COALESCE(language,''),
		       COALESCE(publisher,''), COALESCE(publish_date,''),
		       COALESCE(page_count,0), COALESCE(duration_seconds,0),
		       COALESCE(match_confidence,0), needs_review, has_media_overlay,
		       added_at, updated_at
		FROM works
		ORDER BY added_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent works: %w", err)
	}
	defer rows.Close()

	var works []*WorkSummary
	for rows.Next() {
		var w WorkSummary
		var needsReview, hasOverlay int
		var addedAt, updatedAt string
		if err := rows.Scan(
			&w.ID, &w.Title, &w.SortTitle, &w.Subtitle, &w.Language,
			&w.Publisher, &w.PublishDate,
			&w.PageCount, &w.DurationSeconds,
			&w.MatchConfidence, &needsReview, &hasOverlay,
			&addedAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning recent work: %w", err)
		}
		w.NeedsReview = needsReview != 0
		w.HasMediaOverlay = hasOverlay != 0
		w.AddedAt = parseDBTime(addedAt)
		w.UpdatedAt = parseDBTime(updatedAt)
		works = append(works, &w)
	}
	return works, rows.Err()
}

// scanWorkSummaryRows reads work summary rows from an already-queried result set.
func scanWorkSummaryRows(rows *sql.Rows, total int) ([]*WorkSummary, int, error) {
	var works []*WorkSummary
	for rows.Next() {
		var w WorkSummary
		var needsReview, hasOverlay int
		var addedAt, updatedAt string
		if err := rows.Scan(
			&w.ID, &w.Title, &w.SortTitle, &w.Subtitle, &w.Language,
			&w.Publisher, &w.PublishDate,
			&w.PageCount, &w.DurationSeconds,
			&w.MatchConfidence, &needsReview, &hasOverlay,
			&addedAt, &updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning work summary: %w", err)
		}
		w.NeedsReview = needsReview != 0
		w.HasMediaOverlay = hasOverlay != 0
		w.AddedAt = parseDBTime(addedAt)
		w.UpdatedAt = parseDBTime(updatedAt)
		works = append(works, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating rows: %w", err)
	}
	return works, total, nil
}

// joinStrings joins string slices with a separator.
func joinStrings(s []string, sep string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += sep
		}
		result += v
	}
	return result
}

// GetWorkContributors returns all contributors for a work with their roles.
func GetWorkContributors(db *sql.DB, workID string) ([]struct {
	Contributor
	Role     string
	Position int
}, error) {
	rows, err := db.Query(`
		SELECT c.id, c.name, c.sort_name, c.identifiers, wc.role, wc.position
		FROM work_contributors wc
		JOIN contributors c ON c.id = wc.contributor_id
		WHERE wc.work_id = ?
		ORDER BY wc.position
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying work contributors: %w", err)
	}
	defer rows.Close()

	type result struct {
		Contributor
		Role     string
		Position int
	}
	var results []struct {
		Contributor
		Role     string
		Position int
	}
	for rows.Next() {
		var r result
		var identJSON string
		if err := rows.Scan(&r.ID, &r.Name, &r.SortName, &identJSON, &r.Role, &r.Position); err != nil {
			return nil, fmt.Errorf("scanning work contributor: %w", err)
		}
		r.Identifiers = make(map[string]string)
		if identJSON != "" && identJSON != "{}" {
			_ = json.Unmarshal([]byte(identJSON), &r.Identifiers)
		}
		results = append(results, struct {
			Contributor
			Role     string
			Position int
		}(r))
	}
	return results, rows.Err()
}

// GetWorkSeries returns all series memberships for a work.
func GetWorkSeries(db *sql.DB, workID string) ([]struct {
	Series
	Position *float64
}, error) {
	rows, err := db.Query(`
		SELECT s.id, s.name, s.identifiers, ws.position
		FROM work_series ws
		JOIN series s ON s.id = ws.series_id
		WHERE ws.work_id = ?
		ORDER BY ws.position
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying work series: %w", err)
	}
	defer rows.Close()

	var results []struct {
		Series
		Position *float64
	}
	for rows.Next() {
		var s Series
		var identJSON string
		var pos *float64
		if err := rows.Scan(&s.ID, &s.Name, &identJSON, &pos); err != nil {
			return nil, fmt.Errorf("scanning work series: %w", err)
		}
		s.Identifiers = make(map[string]string)
		if identJSON != "" && identJSON != "{}" {
			_ = json.Unmarshal([]byte(identJSON), &s.Identifiers)
		}
		results = append(results, struct {
			Series
			Position *float64
		}{s, pos})
	}
	return results, rows.Err()
}

// GetWorkTags returns all tags for a work.
func GetWorkTags(db *sql.DB, workID string) ([]Tag, error) {
	rows, err := db.Query(`
		SELECT t.id, t.name, t.type
		FROM work_tags wt
		JOIN tags t ON t.id = wt.tag_id
		WHERE wt.work_id = ?
		ORDER BY t.name
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying work tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Type); err != nil {
			return nil, fmt.Errorf("scanning work tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// GetWorkFiles returns all files for a work.
func GetWorkFiles(db *sql.DB, workID string) ([]*WorkFile, error) {
	rows, err := db.Query(`
		SELECT id, work_id, filename, format, size_bytes,
		       COALESCE(checksum_sha256,''), COALESCE(duration_seconds,0),
		       COALESCE(bitrate_kbps,0), COALESCE(codec,''), has_media_overlay
		FROM work_files
		WHERE work_id = ?
		ORDER BY format, filename
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying work files: %w", err)
	}
	defer rows.Close()

	var files []*WorkFile
	for rows.Next() {
		var f WorkFile
		var hasOverlay int
		if err := rows.Scan(
			&f.ID, &f.WorkID, &f.Filename, &f.Format, &f.SizeBytes,
			&f.ChecksumSHA256, &f.DurationSeconds, &f.BitrateKbps, &f.Codec, &hasOverlay,
		); err != nil {
			return nil, fmt.Errorf("scanning work file: %w", err)
		}
		f.HasMediaOverlay = hasOverlay != 0
		files = append(files, &f)
	}
	return files, rows.Err()
}

// GetWorkChapters returns all audiobook chapters for a work.
func GetWorkChapters(db *sql.DB, workID string) ([]*AudiobookChapter, error) {
	rows, err := db.Query(`
		SELECT id, work_id, title, start_seconds, end_seconds, index_position
		FROM audiobook_chapters
		WHERE work_id = ?
		ORDER BY index_position
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying chapters: %w", err)
	}
	defer rows.Close()

	var chapters []*AudiobookChapter
	for rows.Next() {
		var c AudiobookChapter
		if err := rows.Scan(&c.ID, &c.WorkID, &c.Title, &c.StartSeconds, &c.EndSeconds, &c.IndexPosition); err != nil {
			return nil, fmt.Errorf("scanning chapter: %w", err)
		}
		chapters = append(chapters, &c)
	}
	return chapters, rows.Err()
}

// GetWorkCovers returns all covers for a work.
func GetWorkCovers(db *sql.DB, workID string) ([]*Cover, error) {
	rows, err := db.Query(`
		SELECT work_id, source, filename, COALESCE(width,0), COALESCE(height,0), is_selected
		FROM covers
		WHERE work_id = ?
		ORDER BY is_selected DESC, source
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying covers: %w", err)
	}
	defer rows.Close()

	var covers []*Cover
	for rows.Next() {
		var c Cover
		var isSelected int
		if err := rows.Scan(&c.WorkID, &c.Source, &c.Filename, &c.Width, &c.Height, &isSelected); err != nil {
			return nil, fmt.Errorf("scanning cover: %w", err)
		}
		c.IsSelected = isSelected != 0
		covers = append(covers, &c)
	}
	return covers, rows.Err()
}

// SelectCover sets the specified cover as selected and deselects all others.
func SelectCover(db *sql.DB, workID, source string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Error("failed to rollback cover selection", "work_id", workID, "error", rbErr)
			}
		}
	}()

	_, err = tx.Exec(`UPDATE covers SET is_selected = 0 WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deselecting covers: %w", err)
	}

	result, err := tx.Exec(`UPDATE covers SET is_selected = 1 WHERE work_id = ? AND source = ?`, workID, source)
	if err != nil {
		return fmt.Errorf("selecting cover: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cover from source %q not found for work %q", source, workID)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing cover selection: %w", err)
	}
	return nil
}

// GetWorkRatings returns all per-source ratings for a work.
func GetWorkRatings(db *sql.DB, workID string) ([]Rating, error) {
	rows, err := db.Query(`
		SELECT work_id, source, score, max_score, COALESCE(count,0), fetched_at
		FROM ratings
		WHERE work_id = ?
		ORDER BY source
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying ratings: %w", err)
	}
	defer rows.Close()

	var ratings []Rating
	for rows.Next() {
		var r Rating
		var fetchedAt string
		if err := rows.Scan(&r.WorkID, &r.Source, &r.Score, &r.MaxScore, &r.Count, &fetchedAt); err != nil {
			return nil, fmt.Errorf("scanning rating: %w", err)
		}
		r.FetchedAt = parseDBTime(fetchedAt)
		ratings = append(ratings, r)
	}
	return ratings, rows.Err()
}

// Rating represents a per-source rating for a work.
type Rating struct {
	WorkID    string
	Source    string
	Score     float64
	MaxScore  float64
	Count     int
	FetchedAt time.Time
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
