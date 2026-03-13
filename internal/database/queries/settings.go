package queries

import (
	"database/sql"
	"fmt"
)

// GetSetting retrieves a single app setting by key. Returns empty string if not found.
func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting inserts or updates an app setting.
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO app_settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = datetime('now')
	`, key, value)
	if err != nil {
		return fmt.Errorf("setting %q: %w", key, err)
	}
	return nil
}

// GetAllSettings returns all app settings as a key-value map.
func GetAllSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT key, value FROM app_settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("listing settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

// DeleteSetting removes an app setting.
func DeleteSetting(db *sql.DB, key string) error {
	_, err := db.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("deleting setting %q: %w", key, err)
	}
	return nil
}

// GetReviewQueue returns works that need review, with their most recent task.
func GetReviewQueue(db *sql.DB, limit, offset int) ([]*WorkSummary, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM works WHERE needs_review = 1`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting review queue: %w", err)
	}

	rows, err := db.Query(`
		SELECT w.id, w.title, w.sort_title, COALESCE(w.subtitle,''), COALESCE(w.language,''),
		       COALESCE(w.publisher,''), COALESCE(w.publish_date,''),
		       COALESCE(w.page_count,0), COALESCE(w.duration_seconds,0),
		       COALESCE(w.match_confidence,0), w.needs_review, w.has_media_overlay,
		       w.added_at, w.updated_at
		FROM works w
		WHERE w.needs_review = 1
		ORDER BY w.added_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying review queue: %w", err)
	}
	defer rows.Close()

	return scanWorkSummaryRows(rows, total)
}

// GetMetadataTaskByID returns a single metadata task by ID.
func GetMetadataTaskByID(db *sql.DB, id string) (*MetadataTask, error) {
	row := db.QueryRow(`
		SELECT id, work_id, status, task_type, priority,
		       COALESCE(candidates,''), selected, COALESCE(error,''),
		       created_at,
		       COALESCE(started_at,''), COALESCE(completed_at,'')
		FROM metadata_tasks
		WHERE id = ?
	`, id)

	var t MetadataTask
	var createdAt, startedAt, completedAt string
	err := row.Scan(
		&t.ID, &t.WorkID, &t.Status, &t.TaskType, &t.Priority,
		&t.Candidates, &t.Selected, &t.Error,
		&createdAt, &startedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying metadata task %q: %w", id, err)
	}
	t.CreatedAt = parseDBTime(createdAt)
	if startedAt != "" {
		ts := parseDBTime(startedAt)
		t.StartedAt = &ts
	}
	if completedAt != "" {
		tc := parseDBTime(completedAt)
		t.CompletedAt = &tc
	}
	return &t, nil
}
