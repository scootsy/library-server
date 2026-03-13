// Package server sets up the HTTP server, registers routes, and manages
// graceful shutdown.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/scootsy/library-server/internal/config"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	cfg  *config.Config
	http *http.Server
}

// New creates a Server with all routes registered.
// apiHandler is the REST API handler (from internal/api).
// webFS is the embedded Svelte SPA filesystem (may be nil during development).
func New(cfg *config.Config, apiHandler http.Handler, webFS fs.FS) *Server {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("GET /health", s.handleHealth)

	// Mount the REST API
	if apiHandler != nil {
		mux.Handle("/api/", apiHandler)
	}

	// Serve the embedded Svelte SPA for all non-API routes.
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		serveIndex := func(w http.ResponseWriter, r *http.Request) {
			f, err := webFS.Open("index.html")
			if err != nil {
				http.Error(w, "index not found", http.StatusInternalServerError)
				return
			}
			defer f.Close()

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if _, err := io.Copy(w, f); err != nil {
				slog.Error("failed to serve index.html", "error", err)
			}
		}

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// For the root path or SPA client-side routes, serve index.html
			// directly from the embedded FS. We must NOT rewrite r.URL.Path
			// to "/index.html" and pass to http.FileServer, because FileServer
			// has built-in behavior that 301-redirects "/index.html" to "./"
			// (the directory), which resolves back to "/" — causing an infinite
			// redirect loop.
			if path == "/" {
				http.ServeFileFS(w, r, webFS, "index.html")
				return
			}

			// Try to serve the static file directly.
			if !strings.HasPrefix(path, "/api/") {
				// Check if the file exists in the embedded FS.
				f, err := webFS.Open(strings.TrimPrefix(path, "/"))
				if err == nil {
					if stat, statErr := f.Stat(); statErr == nil && stat.IsDir() {
						f.Close()
						serveIndex(w, r)
						return
					}
					f.Close()
					fileServer.ServeHTTP(w, r)
					return
				}
				// File not found — serve index.html for SPA client-side routing.
				// Use http.ServeFileFS to avoid the FileServer redirect loop.
				http.ServeFileFS(w, r, webFS, "index.html")
				return
			}

			http.NotFound(w, r)
		})
	}

	s.http = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Start begins serving HTTP requests. It blocks until the context is cancelled,
// then performs a graceful shutdown with a 30-second deadline.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("binding to %s: %w", s.http.Addr, err)
	}

	slog.Info("server listening", "addr", ln.Addr().String(), "base_url", s.cfg.Server.BaseURL)

	errCh := make(chan error, 1)
	go func() {
		if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		// Drain the error channel to ensure the Serve goroutine exits cleanly.
		if err := <-errCh; err != nil {
			return fmt.Errorf("server error during shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
