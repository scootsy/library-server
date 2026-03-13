package queries

import (
	"database/sql"
	"fmt"
)

// WorkFile represents a physical media file associated with a work.
type WorkFile struct {
	ID              string
	WorkID          string
	Filename        string
	Format          string
	SizeBytes       int64
	ChecksumSHA256  string
	DurationSeconds int
	BitrateKbps     int
	Codec           string
	HasMediaOverlay bool
}

// UpsertWorkFile inserts or updates a work file record.
func UpsertWorkFile(db *sql.DB, f *WorkFile) error {
	_, err := db.Exec(`
		INSERT INTO work_files (
			id, work_id, filename, format, size_bytes,
			checksum_sha256, duration_seconds, bitrate_kbps, codec, has_media_overlay
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(work_id, filename) DO UPDATE SET
			format            = excluded.format,
			size_bytes        = excluded.size_bytes,
			checksum_sha256   = excluded.checksum_sha256,
			duration_seconds  = excluded.duration_seconds,
			bitrate_kbps      = excluded.bitrate_kbps,
			codec             = excluded.codec,
			has_media_overlay = excluded.has_media_overlay
	`,
		f.ID, f.WorkID, f.Filename, f.Format, f.SizeBytes,
		nullableString(f.ChecksumSHA256),
		nullableInt(f.DurationSeconds),
		nullableInt(f.BitrateKbps),
		nullableString(f.Codec),
		boolToInt(f.HasMediaOverlay),
	)
	if err != nil {
		return fmt.Errorf("upserting work file %q: %w", f.Filename, err)
	}
	return nil
}

// DeleteWorkFiles removes all file records for a work.
func DeleteWorkFiles(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM work_files WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting work_files for %q: %w", workID, err)
	}
	return nil
}

// AudiobookChapter is a single chapter in an audiobook.
type AudiobookChapter struct {
	ID            string
	WorkID        string
	Title         string
	StartSeconds  float64
	EndSeconds    float64
	IndexPosition int
}

// UpsertAudiobookChapter inserts or updates a chapter record.
func UpsertAudiobookChapter(db *sql.DB, c *AudiobookChapter) error {
	_, err := db.Exec(`
		INSERT INTO audiobook_chapters (id, work_id, title, start_seconds, end_seconds, index_position)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(work_id, index_position) DO UPDATE SET
			title         = excluded.title,
			start_seconds = excluded.start_seconds,
			end_seconds   = excluded.end_seconds
	`, c.ID, c.WorkID, c.Title, c.StartSeconds, c.EndSeconds, c.IndexPosition)
	if err != nil {
		return fmt.Errorf("upserting chapter %d for work %q: %w", c.IndexPosition, c.WorkID, err)
	}
	return nil
}

// DeleteWorkChapters removes all chapter records for a work.
func DeleteWorkChapters(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM audiobook_chapters WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting chapters for %q: %w", workID, err)
	}
	return nil
}
