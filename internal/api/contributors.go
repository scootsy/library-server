package api

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/database/queries"
)

// ContributorsHandler handles REST endpoints for contributors.
type ContributorsHandler struct {
	db *sql.DB
}

// List returns a paginated list of contributors with work counts.
func (h *ContributorsHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	contribs, total, err := queries.ListContributors(h.db, limit, offset)
	if err != nil {
		slog.Error("failed to list contributors", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list contributors")
		return
	}

	type contribItem struct {
		ID        string            `json:"id"`
		Name      string            `json:"name"`
		SortName  string            `json:"sort_name"`
		WorkCount int               `json:"work_count"`
		Identifiers map[string]string `json:"identifiers,omitempty"`
	}

	items := make([]contribItem, 0, len(contribs))
	for _, c := range contribs {
		items = append(items, contribItem{
			ID:          c.ID,
			Name:        c.Name,
			SortName:    c.SortName,
			WorkCount:   c.WorkCount,
			Identifiers: c.Identifiers,
		})
	}

	writeJSON(w, http.StatusOK, paginatedResponse{
		Data:   items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Get returns a contributor with their works.
func (h *ContributorsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	contrib, err := queries.GetContributorByID(h.db, id)
	if err != nil {
		slog.Error("failed to get contributor", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get contributor")
		return
	}
	if contrib == nil {
		writeError(w, http.StatusNotFound, "contributor not found")
		return
	}

	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	works, total, err := queries.GetWorksByContributor(h.db, id, limit, offset)
	if err != nil {
		slog.Error("failed to get works by contributor", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get works")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"contributor": contrib,
		"works":       paginatedResponse{Data: works, Total: total, Limit: limit, Offset: offset},
	})
}
