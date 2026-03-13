package queries

import (
	"log/slog"
	"time"
)

// parseDBTime parses a datetime string from SQLite (time.DateTime format).
// Returns the zero time and logs a warning if parsing fails.
func parseDBTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.DateTime, s)
	if err != nil {
		slog.Warn("failed to parse database timestamp", "value", s, "error", err)
		return time.Time{}
	}
	return t
}

// nullableString returns nil for empty strings, or the string value for SQL insertion.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableInt returns nil for zero values, or the int value for SQL insertion.
func nullableInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

// nullableFloat returns nil for zero values, or the float value for SQL insertion.
func nullableFloat(f float64) any {
	if f == 0 {
		return nil
	}
	return f
}

// boolToInt converts a boolean to 0 or 1 for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
