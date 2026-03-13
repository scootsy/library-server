package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MetadataTask mirrors the metadata_tasks table.
type MetadataTask struct {
	ID          string
	WorkID      string
	Status      string  // pending, running, completed, failed, review
	TaskType    string  // auto_match, refresh, manual_search
	Priority    int
	Candidates  string  // JSON-encoded []Candidate
	Selected    *int    // index into candidates slice; nil = none selected
	Error       string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// EnqueueMetadataTask inserts a new pending task for the given work.
// If a pending or running task already exists for the work it is returned
// unchanged (no duplicate tasks).
func EnqueueMetadataTask(db *sql.DB, t *MetadataTask) error {
	_, err := db.Exec(`
		INSERT INTO metadata_tasks (id, work_id, status, task_type, priority)
		VALUES (?, ?, 'pending', ?, ?)
		ON CONFLICT DO NOTHING
	`, t.ID, t.WorkID, t.TaskType, t.Priority)
	if err != nil {
		return fmt.Errorf("enqueueing metadata task for work %q: %w", t.WorkID, err)
	}
	return nil
}

// DequeueMetadataTask claims the highest-priority pending task and marks it
// as running. Returns nil, nil when the queue is empty.
func DequeueMetadataTask(db *sql.DB) (*MetadataTask, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning dequeue transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	row := tx.QueryRow(`
		SELECT id, work_id, status, task_type, priority,
		       COALESCE(candidates,''), COALESCE(error,''),
		       created_at
		FROM metadata_tasks
		WHERE status = 'pending'
		ORDER BY priority DESC, created_at ASC
		LIMIT 1
	`)

	var t MetadataTask
	var createdAt string
	err = row.Scan(
		&t.ID, &t.WorkID, &t.Status, &t.TaskType, &t.Priority,
		&t.Candidates, &t.Error, &createdAt,
	)
	if err == sql.ErrNoRows {
		_ = tx.Rollback()
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning dequeue row: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)

	_, err = tx.Exec(`
		UPDATE metadata_tasks
		SET status = 'running', started_at = datetime('now')
		WHERE id = ?
	`, t.ID)
	if err != nil {
		return nil, fmt.Errorf("marking task running: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing dequeue: %w", err)
	}

	t.Status = "running"
	return &t, nil
}

// CompleteMetadataTask marks a running task as completed or failed and stores
// the JSON-encoded candidates. status must be "completed", "failed", or "review".
func CompleteMetadataTask(db *sql.DB, taskID, status, candidates, errMsg string) error {
	_, err := db.Exec(`
		UPDATE metadata_tasks
		SET status = ?, candidates = ?, error = ?, completed_at = datetime('now')
		WHERE id = ?
	`, status, nullableString(candidates), nullableString(errMsg), taskID)
	if err != nil {
		return fmt.Errorf("completing metadata task %q: %w", taskID, err)
	}
	return nil
}

// SetTaskSelected records which candidate index the user (or engine) chose.
func SetTaskSelected(db *sql.DB, taskID string, selected int) error {
	_, err := db.Exec(`
		UPDATE metadata_tasks SET selected = ? WHERE id = ?
	`, selected, taskID)
	if err != nil {
		return fmt.Errorf("setting task selected for %q: %w", taskID, err)
	}
	return nil
}

// GetPendingTaskCount returns the number of pending metadata tasks.
func GetPendingTaskCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM metadata_tasks WHERE status = 'pending'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting pending tasks: %w", err)
	}
	return count, nil
}

// GetTasksForWork returns all metadata tasks for a work, newest first.
func GetTasksForWork(db *sql.DB, workID string) ([]*MetadataTask, error) {
	rows, err := db.Query(`
		SELECT id, work_id, status, task_type, priority,
		       COALESCE(candidates,''), COALESCE(error,''),
		       created_at,
		       COALESCE(started_at,''), COALESCE(completed_at,'')
		FROM metadata_tasks
		WHERE work_id = ?
		ORDER BY created_at DESC
	`, workID)
	if err != nil {
		return nil, fmt.Errorf("querying tasks for work %q: %w", workID, err)
	}
	defer rows.Close()

	var tasks []*MetadataTask
	for rows.Next() {
		var t MetadataTask
		var createdAt, startedAt, completedAt string
		if err := rows.Scan(
			&t.ID, &t.WorkID, &t.Status, &t.TaskType, &t.Priority,
			&t.Candidates, &t.Error,
			&createdAt, &startedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning task row: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
		if startedAt != "" {
			ts, _ := time.Parse(time.DateTime, startedAt)
			t.StartedAt = &ts
		}
		if completedAt != "" {
			tc, _ := time.Parse(time.DateTime, completedAt)
			t.CompletedAt = &tc
		}
		tasks = append(tasks, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating task rows: %w", err)
	}
	return tasks, nil
}

// ── Source cache ──────────────────────────────────────────────────────────────

// SourceCacheEntry mirrors the source_cache table.
type SourceCacheEntry struct {
	WorkID    string
	Source    string
	QueryUsed string
	Response  string // JSON-encoded raw API response
	FetchedAt time.Time
}

// UpsertSourceCache inserts or replaces a source cache entry for (work_id, source).
func UpsertSourceCache(db *sql.DB, e *SourceCacheEntry) error {
	response, err := json.Marshal(e.Response)
	if err != nil {
		return fmt.Errorf("marshalling source cache response: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO source_cache (work_id, source, query_used, response, fetched_at)
		VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(work_id, source) DO UPDATE SET
			query_used = excluded.query_used,
			response   = excluded.response,
			fetched_at = excluded.fetched_at
	`, e.WorkID, e.Source, e.QueryUsed, string(response))
	if err != nil {
		return fmt.Errorf("upserting source cache for work %q source %q: %w", e.WorkID, e.Source, err)
	}
	return nil
}

// GetSourceCache retrieves a cached source response for (work_id, source).
// Returns nil, nil when no cache entry exists.
func GetSourceCache(db *sql.DB, workID, source string) (*SourceCacheEntry, error) {
	row := db.QueryRow(`
		SELECT work_id, source, query_used, response, fetched_at
		FROM source_cache
		WHERE work_id = ? AND source = ?
	`, workID, source)

	var e SourceCacheEntry
	var fetchedAt string
	err := row.Scan(&e.WorkID, &e.Source, &e.QueryUsed, &e.Response, &fetchedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning source cache row: %w", err)
	}
	e.FetchedAt, _ = time.Parse(time.DateTime, fetchedAt)
	return &e, nil
}

// PurgeExpiredSourceCache deletes source cache entries older than retentionDays.
func PurgeExpiredSourceCache(db *sql.DB, retentionDays int) (int64, error) {
	result, err := db.Exec(`
		DELETE FROM source_cache
		WHERE fetched_at < datetime('now', ? || ' days')
	`, fmt.Sprintf("-%d", retentionDays))
	if err != nil {
		return 0, fmt.Errorf("purging expired source cache: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}
	return n, nil
}
