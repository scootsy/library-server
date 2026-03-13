// Package api provides the REST API handlers for the Codex admin interface.
package api

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/scootsy/library-server/internal/auth"
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

	authH := &AuthHandler{db: deps.DB, config: deps.Config}
	usersH := &UsersHandler{db: deps.DB}
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

	// Auth middleware
	requireAuth := auth.Middleware(deps.DB)
	requireAdmin := auth.RequireRole("admin")

	// Rate limiter for authentication endpoints: 10 requests per minute per IP.
	authLimiter := newIPRateLimiter(10, time.Minute)

	// ── Public auth endpoints (no session required) ─────────────────────
	mux.Handle("POST /api/auth/login", rateLimitMiddleware(authLimiter)(http.HandlerFunc(authH.Login)))

	// ── Authenticated routes ────────────────────────────────────────────
	authed := http.NewServeMux()

	// Auth
	authed.HandleFunc("POST /api/auth/logout", authH.Logout)
	authed.HandleFunc("GET /api/auth/me", authH.Me)
	authed.HandleFunc("PUT /api/auth/password", authH.ChangePassword)

	// Works
	authed.HandleFunc("GET /api/works", works.List)
	authed.HandleFunc("GET /api/works/search", works.Search)
	authed.HandleFunc("GET /api/works/{id}", works.Get)
	authed.HandleFunc("PUT /api/works/{id}", works.Update)
	authed.HandleFunc("DELETE /api/works/{id}", works.Delete)

	// Contributors
	authed.HandleFunc("GET /api/contributors", contribs.List)
	authed.HandleFunc("GET /api/contributors/{id}", contribs.Get)

	// Series
	authed.HandleFunc("GET /api/series", seriesH.List)
	authed.HandleFunc("GET /api/series/{id}", seriesH.Get)

	// Tags
	authed.HandleFunc("GET /api/tags", tagsH.List)
	authed.HandleFunc("GET /api/tags/{id}", tagsH.Get)

	// Collections
	authed.HandleFunc("GET /api/collections", collections.List)
	authed.HandleFunc("POST /api/collections", collections.Create)
	authed.HandleFunc("GET /api/collections/{id}", collections.Get)
	authed.HandleFunc("PUT /api/collections/{id}", collections.Update)
	authed.HandleFunc("DELETE /api/collections/{id}", collections.Delete)
	authed.HandleFunc("POST /api/collections/{id}/works", collections.AddWork)
	authed.HandleFunc("DELETE /api/collections/{id}/works/{workID}", collections.RemoveWork)

	// Metadata
	authed.HandleFunc("POST /api/metadata/refresh/{workID}", meta.Refresh)
	authed.HandleFunc("GET /api/metadata/tasks/{workID}", meta.GetTasks)
	authed.HandleFunc("POST /api/metadata/apply/{taskID}", meta.ApplyCandidate)
	authed.HandleFunc("GET /api/metadata/review", meta.ReviewQueue)

	// Scan
	authed.HandleFunc("POST /api/scan", scanH.TriggerScan)
	authed.HandleFunc("GET /api/scan/status", scanH.Status)

	// Covers
	authed.HandleFunc("GET /api/works/{id}/covers", covers.List)
	authed.HandleFunc("PUT /api/works/{id}/covers/select", covers.Select)

	// Settings
	authed.HandleFunc("GET /api/settings", settings.List)
	authed.HandleFunc("PUT /api/settings", settings.Update)

	// Dashboard
	authed.HandleFunc("GET /api/dashboard", dashboard.Get)

	// Mount authenticated routes
	mux.Handle("/api/", requireAuth(authed))

	// ── Admin-only routes ───────────────────────────────────────────────
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /api/admin/users", usersH.List)
	adminMux.HandleFunc("POST /api/admin/users", usersH.Create)
	adminMux.HandleFunc("GET /api/admin/users/{id}", usersH.Get)
	adminMux.HandleFunc("PUT /api/admin/users/{id}", usersH.Update)
	adminMux.HandleFunc("DELETE /api/admin/users/{id}", usersH.Delete)

	mux.Handle("/api/admin/", requireAuth(requireAdmin(adminMux)))

	// Wrap with middleware
	var handler http.Handler = mux
	handler = corsMiddleware(deps.Config.Server.BaseURL)(handler)
	handler = maxBodyMiddleware(1 << 20)(handler) // 1 MB body limit for API requests
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
	mu      sync.RWMutex
	running bool
	lastErr error
	lastRun time.Time
}

// NewScanManager creates a ScanManager.
func NewScanManager(db *sql.DB, cfg *config.Config, engine *metadata.Engine) *ScanManager {
	return &ScanManager{db: db, config: cfg, engine: engine}
}

// IsRunning returns whether a scan is currently in progress.
func (sm *ScanManager) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.running
}

// LastRun returns the time of the last completed scan.
func (sm *ScanManager) LastRun() time.Time {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.lastRun
}

// LastError returns the last scan error, if any.
func (sm *ScanManager) LastError() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.lastErr
}

// RunScan executes a full library scan in the background.
func (sm *ScanManager) RunScan() {
	sm.mu.Lock()
	if sm.running {
		sm.mu.Unlock()
		return
	}
	sm.running = true
	sm.mu.Unlock()

	go func() {
		defer func() {
			sm.mu.Lock()
			sm.running = false
			sm.lastRun = time.Now()
			sm.mu.Unlock()
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
				sm.mu.Lock()
				sm.lastErr = err
				sm.mu.Unlock()
				return
			}
		}
		sm.mu.Lock()
		sm.lastErr = nil
		sm.mu.Unlock()
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
		// Refuse to allow wildcard CORS; fall back to rejecting cross-origin.
		slog.Warn("server.base_url is not configured; CORS will reject cross-origin requests")
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
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

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
			if rec := recover(); rec != nil {
				slog.Error("panic recovered in HTTP handler",
					"panic", fmt.Sprintf("%v", rec),
					"path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// maxBodyMiddleware limits the size of incoming request bodies for API routes.
func maxBodyMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
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
