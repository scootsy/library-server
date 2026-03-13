package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/database/queries"
)

// WorksHandler handles REST endpoints for works.
type WorksHandler struct {
	db     *sql.DB
	config *config.Config
}

// List returns a paginated list of works.
func (h *WorksHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	params := queries.WorkListParams{
		Limit:     limit,
		Offset:    offset,
		SortBy:    r.URL.Query().Get("sort"),
		SortOrder: r.URL.Query().Get("order"),
		Language:  r.URL.Query().Get("language"),
		Format:    r.URL.Query().Get("format"),
	}

	if nr := r.URL.Query().Get("needs_review"); nr != "" {
		val := nr == "true" || nr == "1"
		params.NeedsReview = &val
	}

	works, total, err := queries.ListWorks(h.db, params)
	if err != nil {
		slog.Error("failed to list works", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list works")
		return
	}

	writeJSON(w, http.StatusOK, paginatedResponse{
		Data:   works,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// Search performs a full-text search on works.
func (h *WorksHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	works, total, err := queries.SearchWorks(h.db, q, limit, offset)
	if err != nil {
		slog.Error("search failed", "query", q, "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, paginatedResponse{
		Data:   works,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// workDetailResponse provides the full detail view of a work.
type workDetailResponse struct {
	*queries.Work
	Contributors []workContributorResponse `json:"contributors"`
	Series       []workSeriesResponse      `json:"series"`
	Tags         []queries.Tag             `json:"tags"`
	Files        []*queries.WorkFile       `json:"files"`
	Identifiers  map[string]string         `json:"identifiers"`
	Covers       []*queries.Cover          `json:"covers"`
	Ratings      []queries.Rating          `json:"ratings"`
	Chapters     []*queries.AudiobookChapter `json:"chapters,omitempty"`
}

type workContributorResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SortName string `json:"sort_name"`
	Role     string `json:"role"`
	Position int    `json:"position"`
}

type workSeriesResponse struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Position *float64 `json:"position"`
}

// Get returns the full detail view for a single work.
func (h *WorksHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	work, err := queries.GetWorkByID(h.db, id)
	if err != nil {
		slog.Error("failed to get work", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get work")
		return
	}
	if work == nil {
		writeError(w, http.StatusNotFound, "work not found")
		return
	}

	// Fetch related data in sequence (all are fast queries on indexed columns).
	contribs, err := queries.GetWorkContributors(h.db, id)
	if err != nil {
		slog.Error("failed to get contributors", "work_id", id, "error", err)
	}
	var contribResp []workContributorResponse
	for _, c := range contribs {
		contribResp = append(contribResp, workContributorResponse{
			ID: c.ID, Name: c.Name, SortName: c.SortName,
			Role: c.Role, Position: c.Position,
		})
	}

	seriesList, err := queries.GetWorkSeries(h.db, id)
	if err != nil {
		slog.Error("failed to get series", "work_id", id, "error", err)
	}
	var seriesResp []workSeriesResponse
	for _, s := range seriesList {
		seriesResp = append(seriesResp, workSeriesResponse{
			ID: s.ID, Name: s.Name, Position: s.Position,
		})
	}

	tags, err := queries.GetWorkTags(h.db, id)
	if err != nil {
		slog.Error("failed to get tags", "work_id", id, "error", err)
	}

	files, err := queries.GetWorkFiles(h.db, id)
	if err != nil {
		slog.Error("failed to get files", "work_id", id, "error", err)
	}

	identifiers, err := queries.GetWorkIdentifiers(h.db, id)
	if err != nil {
		slog.Error("failed to get identifiers", "work_id", id, "error", err)
	}

	covers, err := queries.GetWorkCovers(h.db, id)
	if err != nil {
		slog.Error("failed to get covers", "work_id", id, "error", err)
	}

	ratings, err := queries.GetWorkRatings(h.db, id)
	if err != nil {
		slog.Error("failed to get ratings", "work_id", id, "error", err)
	}

	chapters, err := queries.GetWorkChapters(h.db, id)
	if err != nil {
		slog.Error("failed to get chapters", "work_id", id, "error", err)
	}

	resp := workDetailResponse{
		Work:         work,
		Contributors: contribResp,
		Series:       seriesResp,
		Tags:         tags,
		Files:        files,
		Identifiers:  identifiers,
		Covers:       covers,
		Ratings:      ratings,
		Chapters:     chapters,
	}

	writeJSON(w, http.StatusOK, resp)
}

// Update modifies user-editable metadata fields on a work.
func (h *WorksHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	work, err := queries.GetWorkByID(h.db, id)
	if err != nil {
		slog.Error("failed to get work", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get work")
		return
	}
	if work == nil {
		writeError(w, http.StatusNotFound, "work not found")
		return
	}

	var fields map[string]any
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := queries.UpdateWorkMetadata(h.db, id, fields); err != nil {
		slog.Error("failed to update work", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update work")
		return
	}

	// Re-fetch and return updated work.
	updated, err := queries.GetWorkByID(h.db, id)
	if err != nil {
		slog.Error("failed to re-fetch work", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "work updated but failed to re-fetch")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// Delete removes a work from the database.
func (h *WorksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := queries.DeleteWork(h.db, id); err != nil {
		slog.Error("failed to delete work", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete work")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
