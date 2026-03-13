package api

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/database/queries"
)

// SeriesHandler handles REST endpoints for series.
type SeriesHandler struct {
	db *sql.DB
}

// List returns a paginated list of series with work counts.
func (h *SeriesHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	seriesList, total, err := queries.ListSeries(h.db, limit, offset)
	if err != nil {
		slog.Error("failed to list series", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list series")
		return
	}

	type seriesItem struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		WorkCount   int               `json:"work_count"`
		Identifiers map[string]string `json:"identifiers,omitempty"`
	}

	items := make([]seriesItem, 0, len(seriesList))
	for _, s := range seriesList {
		items = append(items, seriesItem{
			ID:          s.ID,
			Name:        s.Name,
			WorkCount:   s.WorkCount,
			Identifiers: s.Identifiers,
		})
	}

	writeJSON(w, http.StatusOK, paginatedResponse{
		Data:   items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Get returns a series with its works ordered by position.
func (h *SeriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	series, err := queries.GetSeriesByID(h.db, id)
	if err != nil {
		slog.Error("failed to get series", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get series")
		return
	}
	if series == nil {
		writeError(w, http.StatusNotFound, "series not found")
		return
	}

	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	works, total, err := queries.GetWorksBySeries(h.db, id, limit, offset)
	if err != nil {
		slog.Error("failed to get works by series", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get works")
		return
	}

	type workWithPosition struct {
		*queries.WorkSummary
		Position *float64 `json:"position"`
	}
	worksResp := make([]workWithPosition, 0, len(works))
	for _, w := range works {
		worksResp = append(worksResp, workWithPosition{
			WorkSummary: w.WorkSummary,
			Position:    w.Position,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"series": series,
		"works":  paginatedResponse{Data: worksResp, Total: total, Limit: limit, Offset: offset},
	})
}
