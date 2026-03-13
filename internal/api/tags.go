package api

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/database/queries"
)

// TagsHandler handles REST endpoints for tags.
type TagsHandler struct {
	db *sql.DB
}

// List returns all tags with work counts.
func (h *TagsHandler) List(w http.ResponseWriter, r *http.Request) {
	tags, err := queries.ListTags(h.db)
	if err != nil {
		slog.Error("failed to list tags", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}

	type tagItem struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		WorkCount int    `json:"work_count"`
	}

	items := make([]tagItem, 0, len(tags))
	for _, t := range tags {
		items = append(items, tagItem{
			ID:        t.ID,
			Name:      t.Name,
			Type:      t.Type,
			WorkCount: t.WorkCount,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// Get returns a tag with its works.
func (h *TagsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	tag, err := queries.GetTagByID(h.db, id)
	if err != nil {
		slog.Error("failed to get tag", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tag")
		return
	}
	if tag == nil {
		writeError(w, http.StatusNotFound, "tag not found")
		return
	}

	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	works, total, err := queries.GetWorksByTag(h.db, id, limit, offset)
	if err != nil {
		slog.Error("failed to get works by tag", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get works")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tag":   tag,
		"works": paginatedResponse{Data: works, Total: total, Limit: limit, Offset: offset},
	})
}
