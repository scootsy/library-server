package queries

import (
	"database/sql"
	"fmt"
	"time"
)

// UpsertWorkRating inserts or updates a per-source rating for a work.
func UpsertWorkRating(db *sql.DB, rating *Rating) error {
	fetchedAt := rating.FetchedAt
	if fetchedAt.IsZero() {
		fetchedAt = time.Now().UTC()
	}

	_, err := db.Exec(`
		INSERT INTO ratings (work_id, source, score, max_score, count, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(work_id, source) DO UPDATE SET
			score = excluded.score,
			max_score = excluded.max_score,
			count = excluded.count,
			fetched_at = excluded.fetched_at
	`,
		rating.WorkID,
		rating.Source,
		rating.Score,
		rating.MaxScore,
		nullableInt(rating.Count),
		fetchedAt.UTC().Format(time.DateTime),
	)
	if err != nil {
		return fmt.Errorf("upserting rating for work %q source %q: %w", rating.WorkID, rating.Source, err)
	}
	return nil
}

// DeleteWorkRatings removes all rating records for a work.
func DeleteWorkRatings(db *sql.DB, workID string) error {
	_, err := db.Exec(`DELETE FROM ratings WHERE work_id = ?`, workID)
	if err != nil {
		return fmt.Errorf("deleting ratings for work %q: %w", workID, err)
	}
	return nil
}
