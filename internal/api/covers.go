package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/database/queries"
)

// CoversHandler handles REST endpoints for cover management.
type CoversHandler struct {
	db     *sql.DB
	config *config.Config
}

// List returns all available covers for a work.
func (h *CoversHandler) List(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	covers, err := queries.GetWorkCovers(h.db, id)
	if err != nil {
		slog.Error("failed to list covers", "work_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list covers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": covers})
}

type selectCoverRequest struct {
	Source string `json:"source"`
}

// Select sets the selected cover for a work.
func (h *CoversHandler) Select(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req selectCoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}

	if err := queries.SelectCover(h.db, id, req.Source); err != nil {
		slog.Error("failed to select cover", "work_id", id, "source", req.Source, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to select cover")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "selected"})
}
