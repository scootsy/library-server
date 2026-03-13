package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/database"
	"github.com/scootsy/library-server/internal/database/queries"
	"github.com/scootsy/library-server/internal/scanner"
	"github.com/scootsy/library-server/internal/server"
)

func main() {
	os.Exit(run())
}

func run() int {
	// ── Flags ────────────────────────────────────────────────────────────────
	var (
		configPath = flag.String("config", "", "path to config.yaml (optional)")
		scanOnly   = flag.Bool("scan", false, "run a library scan and exit")
		migrateOnly = flag.Bool("migrate", false, "run database migrations and exit")
	)
	flag.Parse()

	// ── Config ───────────────────────────────────────────────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		return 1
	}

	// ── Logging ──────────────────────────────────────────────────────────────
	setupLogger(cfg)
	slog.Info("codex starting")

	// ── Database ─────────────────────────────────────────────────────────────
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		slog.Error("failed to open database", "path", cfg.Database.Path, "error", err)
		return 1
	}
	defer db.Close()

	if *migrateOnly {
		slog.Info("migrations complete")
		return 0
	}

	// ── Register configured media roots ──────────────────────────────────────
	if err := ensureMediaRoots(db, cfg); err != nil {
		slog.Error("registering media roots", "error", err)
		return 1
	}

	// ── Scan mode ────────────────────────────────────────────────────────────
	if *scanOnly {
		if err := runScan(db, cfg); err != nil {
			slog.Error("scan failed", "error", err)
			return 1
		}
		return 0
	}

	// ── HTTP server ──────────────────────────────────────────────────────────
	srv := server.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(ctx); err != nil {
		slog.Error("server error", "error", err)
		return 1
	}

	slog.Info("codex stopped")
	return 0
}

// ensureMediaRoots inserts or updates all media roots from the config.
func ensureMediaRoots(db *sql.DB, cfg *config.Config) error {
	for _, root := range cfg.Media.Roots {
		existing, err := queries.GetMediaRootByPath(db, root.Path)
		if err != nil {
			return fmt.Errorf("checking media root %q: %w", root.Path, err)
		}
		rootID := uuid.NewString()
		if existing != nil {
			rootID = existing.ID
		}
		if err := queries.UpsertMediaRoot(db, rootID, root.Name, root.Path); err != nil {
			return fmt.Errorf("upserting media root %q: %w", root.Path, err)
		}
	}
	return nil
}

// runScan scans all configured media roots.
func runScan(db *sql.DB, cfg *config.Config) error {
	for _, root := range cfg.Media.Roots {
		mediaRoot, err := queries.GetMediaRootByPath(db, root.Path)
		if err != nil {
			return fmt.Errorf("looking up media root %q: %w", root.Path, err)
		}
		if mediaRoot == nil {
			slog.Warn("media root not in database, skipping", "path", root.Path)
			continue
		}
		s := scanner.New(db, mediaRoot)
		if err := s.Scan(); err != nil {
			return fmt.Errorf("scanning %q: %w", root.Path, err)
		}
	}
	return nil
}

// sensitiveKeys contains log attribute key names whose values must be redacted.
var sensitiveKeys = map[string]bool{
	"password":      true,
	"secret":        true,
	"token":         true,
	"key":           true,
	"authorization": true,
	"api_key":       true,
}

// redactSensitiveAttrs is a ReplaceAttr function for slog that redacts values
// of attributes whose keys match known sensitive field names.
func redactSensitiveAttrs(_ []string, a slog.Attr) slog.Attr {
	if sensitiveKeys[strings.ToLower(a.Key)] {
		a.Value = slog.StringValue("[REDACTED]")
	}
	return a
}

func setupLogger(cfg *config.Config) {
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redactSensitiveAttrs,
	}
	var handler slog.Handler
	if cfg.Log.Format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}
