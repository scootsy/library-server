package api

import (
	"database/sql"
	"net/http"

	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/metadata"
)

// ScanHandler handles REST endpoints for scan operations.
type ScanHandler struct {
	db      *sql.DB
	config  *config.Config
	engine  *metadata.Engine
	scanMgr *ScanManager
}

// TriggerScan starts a library scan.
func (h *ScanHandler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	if h.scanMgr.IsRunning() {
		writeJSON(w, http.StatusConflict, map[string]string{
			"status":  "already_running",
			"message": "a scan is already in progress",
		})
		return
	}

	h.scanMgr.RunScan()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// Status returns the current scan status.
func (h *ScanHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := "idle"
	if h.scanMgr.IsRunning() {
		status = "running"
	}

	resp := map[string]any{
		"status": status,
	}
	if !h.scanMgr.LastRun().IsZero() {
		resp["last_run"] = h.scanMgr.LastRun()
	}
	if h.scanMgr.LastError() != nil {
		resp["last_error"] = h.scanMgr.LastError().Error()
	}

	writeJSON(w, http.StatusOK, resp)
}
