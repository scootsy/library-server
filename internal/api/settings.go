package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/database/queries"
)

// SettingsHandler handles REST endpoints for app settings.
type SettingsHandler struct {
	db *sql.DB
}

// List returns all app settings.
func (h *SettingsHandler) List(w http.ResponseWriter, r *http.Request) {
	settings, err := queries.GetAllSettings(h.db)
	if err != nil {
		slog.Error("failed to list settings", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

// Update sets one or more app settings.
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: expected {key: value, ...}")
		return
	}

	for key, value := range settings {
		if err := queries.SetSetting(h.db, key, value); err != nil {
			slog.Error("failed to set setting", "key", key, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update settings")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
