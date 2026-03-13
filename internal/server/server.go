// Package server sets up the HTTP server, registers routes, and manages
// graceful shutdown.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/scootsy/library-server/internal/config"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	cfg    *config.Config
	http   *http.Server
}

// New creates a Server with all routes registered.
func New(cfg *config.Config) *Server {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("GET /health", s.handleHealth)

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
