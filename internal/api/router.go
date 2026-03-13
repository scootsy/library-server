// Package api provides the REST API handlers for the Codex admin interface.
package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/database/queries"
	"github.com/scootsy/library-server/internal/metadata"
	"github.com/scootsy/library-server/internal/scanner"
)

// Dependencies holds shared dependencies injected into all API handlers.
type Dependencies struct {
	DB      *sql.DB
	Config  *config.Config
	Engine  *metadata.Engine
	Scanner *ScanManager
}

// NewRouter returns an http.Handler with all API routes registered.
func NewRouter(deps *Dependencies) http.Handler {
	mux := http.NewServeMux()

	works := &WorksHandler{db: deps.DB, config: deps.Config}
	contribs := &ContributorsHandler{db: deps.DB}
	seriesH := &SeriesHandler{db: deps.DB}
	tagsH := &TagsHandler{db: deps.DB}
	collections := &CollectionsHandler{db: deps.DB}
	meta := &MetadataHandler{db: deps.DB, engine: deps.Engine}
	scanH := &ScanHandler{db: deps.DB, config: deps.Config, engine: deps.Engine, scanMgr: deps.Scanner}
	covers := &CoversHandler{db: deps.DB, config: deps.Config}
	settings := &SettingsHandler{db: deps.DB}
	dashboard := &DashboardHandler{db: deps.DB}

	// ── Works ────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/works", works.List)
	mux.HandleFunc("GET /api/works/search", works.Search)
	mux.HandleFunc("GET /api/works/{id}", works.Get)
	mux.HandleFunc("PUT /api/works/{id}", works.Update)
	mux.HandleFunc("DELETE /api/works/{id}", works.Delete)

	// ── Contributors ─────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/contributors", contribs.List)
	mux.HandleFunc("GET /api/contributors/{id}", contribs.Get)

	// ── Series ───────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/series", seriesH.List)
	mux.HandleFunc("GET /api/series/{id}", seriesH.Get)

	// ── Tags ─────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/tags", tagsH.List)
	mux.HandleFunc("GET /api/tags/{id}", tagsH.Get)

	// ── Collections ──────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/collections", collections.List)
	mux.HandleFunc("POST /api/collections", collections.Create)
	mux.HandleFunc("GET /api/collections/{id}", collections.Get)
	mux.HandleFunc("PUT /api/collections/{id}", collections.Update)
	mux.HandleFunc("DELETE /api/collections/{id}", collections.Delete)
	mux.HandleFunc("POST /api/collections/{id}/works", collections.AddWork)
	mux.HandleFunc("DELETE /api/collections/{id}/works/{workID}", collections.RemoveWork)

	// ── Metadata ─────────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/metadata/refresh/{workID}", meta.Refresh)
	mux.HandleFunc("GET /api/metadata/tasks/{workID}", meta.GetTasks)
	mux.HandleFunc("POST /api/metadata/apply/{taskID}", meta.ApplyCandidate)
	mux.HandleFunc("GET /api/metadata/review", meta.ReviewQueue)

	// ── Scan ─────────────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/scan", scanH.TriggerScan)
	mux.HandleFunc("GET /api/scan/status", scanH.Status)

	// ── Covers ───────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/works/{id}/covers", covers.List)
	mux.HandleFunc("PUT /api/works/{id}/covers/select", covers.Select)

	// ── Settings ─────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/settings", settings.List)
	mux.HandleFunc("PUT /api/settings", settings.Update)

	// ── Dashboard ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/dashboard", dashboard.Get)

	// Wrap with middleware
	var handler http.Handler = mux
	handler = corsMiddleware(deps.Config.Server.BaseURL)(handler)
	handler = contentTypeMiddleware(handler)
	handler = loggingMiddleware(handler)
	handler = recoveryMiddleware(handler)

	return handler
}

// ScanManager tracks the state of library scans.
type ScanManager struct {
	db      *sql.DB
	config  *config.Config
	engine  *metadata.Engine
	running bool
	lastErr error
	lastRun time.Time
}

// NewScanManager creates a ScanManager.
func NewScanManager(db *sql.DB, cfg *config.Config, engine *metadata.Engine) *ScanManager {
	return &ScanManager{db: db, config: cfg, engine: engine}
}

// IsRunning returns whether a scan is currently in progress.
func (sm *ScanManager) IsRunning() bool { return sm.running }

// LastRun returns the time of the last completed scan.
func (sm *ScanManager) LastRun() time.Time { return sm.lastRun }

// LastError returns the last scan error, if any.
func (sm *ScanManager) LastError() error { return sm.lastErr }

// RunScan executes a full library scan in the background.
func (sm *ScanManager) RunScan() {
	if sm.running {
		return
	}
	sm.running = true
	go func() {
		defer func() {
			sm.running = false
			sm.lastRun = time.Now()
		}()

		for _, root := range sm.config.Media.Roots {
			mr, err := getMediaRootByPath(sm.db, root.Path)
			if err != nil || mr == nil {
				slog.Warn("scan: media root not found", "path", root.Path, "error", err)
				continue
			}
			s := scanner.New(sm.db, mr)
			s.SetOnWorkIndexed(func(workID string, isNew bool) {
				if !isNew {
					return
				}
				if err := sm.engine.EnqueueWork(workID, "auto_match", 0); err != nil {
					slog.Warn("failed to enqueue metadata task", "work_id", workID, "error", err)
				}
			})
			if err := s.Scan(); err != nil {
				slog.Error("scan failed", "path", root.Path, "error", err)
				sm.lastErr = err
				return
			}
		}
		sm.lastErr = nil
		slog.Info("library scan completed")
	}()
}

// getMediaRootByPath delegates to the queries package.
func getMediaRootByPath(db *sql.DB, path string) (*queries.MediaRoot, error) {
	return queries.GetMediaRootByPath(db, path)
}

// ── Middleware ────────────────────────────────────────────────────────────────

func corsMiddleware(baseURL string) func(http.Handler) http.Handler {
	// Extract origin from base URL.
	origin := baseURL
	if origin == "" {
		origin = "*"
	}
	// Strip trailing path from base URL to get just the origin.
	if idx := strings.Index(origin, "://"); idx != -1 {
		rest := origin[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			origin = origin[:idx+3+slashIdx]
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered in HTTP handler", "error", err, "path", r.URL.Path)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
