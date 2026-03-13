package api

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/database/queries"
)

// DashboardHandler handles the dashboard summary endpoint.
type DashboardHandler struct {
	db *sql.DB
}

// Get returns a summary of the library for the dashboard view.
func (h *DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	totalWorks, err := queries.CountWorks(h.db)
	if err != nil {
		slog.Error("failed to count works", "error", err)
	}

	needsReview, err := queries.CountNeedsReview(h.db)
	if err != nil {
		slog.Error("failed to count review items", "error", err)
	}

	pendingTasks, err := queries.GetPendingTaskCount(h.db)
	if err != nil {
		slog.Error("failed to count pending tasks", "error", err)
	}

	totalCollections, err := queries.CountCollections(h.db)
	if err != nil {
		slog.Error("failed to count collections", "error", err)
	}

	recentWorks, err := queries.RecentWorks(h.db, 10)
	if err != nil {
		slog.Error("failed to get recent works", "error", err)
	}

	mediaRoots, err := queries.ListMediaRoots(h.db)
	if err != nil {
		slog.Error("failed to list media roots", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total_works":       totalWorks,
		"needs_review":      needsReview,
		"pending_tasks":     pendingTasks,
		"total_collections": totalCollections,
		"recent_works":      recentWorks,
		"media_roots":       mediaRoots,
	})
}
