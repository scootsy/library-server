package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Media    MediaConfig    `yaml:"media"`
	Log      LogConfig      `yaml:"log"`
	Metadata MetadataConfig `yaml:"metadata"`
	Auth     AuthConfig     `yaml:"auth"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	// InitialAdminPassword is the password set for the initial "admin" account
	// when the database is first created. If empty, defaults to "admin".
	// Override via CODEX_ADMIN_PASSWORD environment variable.
	InitialAdminPassword string `yaml:"-"` // env-var only, never in config file

	// SessionLifetimeDays is how long sessions remain valid. Default: 30.
	SessionLifetimeDays int `yaml:"session_lifetime_days"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	BaseURL string `yaml:"base_url"`
}

// DatabaseConfig holds database settings.
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// MediaConfig holds media library settings.
type MediaConfig struct {
	Roots []MediaRoot `yaml:"roots"`
}

// MediaRoot defines a directory tree that Codex will scan.
type MediaRoot struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

// MetadataConfig holds settings for the external metadata enrichment engine.
type MetadataConfig struct {
	// AutoEnrich controls whether scanning automatically queues metadata tasks.
	AutoEnrich bool `yaml:"auto_enrich"`

	// ConfidenceAutoApply is the minimum score to auto-apply without review.
	// Default: 0.85
	ConfidenceAutoApply float64 `yaml:"confidence_auto_apply"`

	// ConfidenceMinMatch is the minimum score to store a candidate at all.
	// Below this the result is discarded and the work stays in review.
	// Default: 0.50
	ConfidenceMinMatch float64 `yaml:"confidence_min_match"`

	// SourceCacheRetentionDays is how long raw API responses are kept in the
	// source_cache table before being purged. Default: 90.
	SourceCacheRetentionDays int `yaml:"source_cache_retention_days"`

	// GoogleBooks holds Google Books API settings.
	GoogleBooks GoogleBooksConfig `yaml:"google_books"`

	// OpenLibrary holds Open Library settings (no auth required).
	OpenLibrary OpenLibraryConfig `yaml:"open_library"`

	// Audnexus holds Audnexus settings (no auth required).
	Audnexus AudnexusConfig `yaml:"audnexus"`
}

// GoogleBooksConfig holds Google Books API settings.
type GoogleBooksConfig struct {
	// Enabled controls whether this source is used. Default: true.
	Enabled bool `yaml:"enabled"`
	// APIKey is optional; omitting it uses anonymous quota (lower rate limit).
	// Load from environment variable CODEX_GOOGLE_BOOKS_API_KEY.
	APIKey string `yaml:"-"` // never read from config file; env-var only
}

// OpenLibraryConfig holds Open Library settings.
type OpenLibraryConfig struct {
	// Enabled controls whether this source is used. Default: true.
	Enabled bool `yaml:"enabled"`
}

// AudnexusConfig holds Audnexus settings.
type AudnexusConfig struct {
	// Enabled controls whether this source is used. Default: true.
	Enabled bool `yaml:"enabled"`
}

// Load reads a config file at path (optional) and applies environment
// variable overrides. Returns a fully populated Config with defaults applied.
func Load(path string) (*Config, error) {
	cfg := defaults()

	if path != "" {
		checkConfigFilePermissions(path)

		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config file %q: %w", path, err)
		}
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parsing config file %q: %w", path, err)
			}
		}
	}

	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// checkConfigFilePermissions warns if the config file has overly permissive
// permissions (more open than 0600).
func checkConfigFilePermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return // file may not exist yet; Load handles that
	}
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		slog.Warn("config file has overly permissive permissions",
			"path", path,
			"mode", fmt.Sprintf("%04o", mode),
			"recommended", "0600")
	}
}

// Validate checks config invariants and returns an error if any are violated.
func (c *Config) Validate() error {
	if c.Database.Path == "" {
		return fmt.Errorf("database path must not be empty")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Metadata.ConfidenceAutoApply < 0 || c.Metadata.ConfidenceAutoApply > 1 {
		return fmt.Errorf("confidence_auto_apply must be between 0 and 1, got %f", c.Metadata.ConfidenceAutoApply)
	}
	if c.Metadata.ConfidenceMinMatch < 0 || c.Metadata.ConfidenceMinMatch > 1 {
		return fmt.Errorf("confidence_min_match must be between 0 and 1, got %f", c.Metadata.ConfidenceMinMatch)
	}
	if c.Metadata.ConfidenceMinMatch > c.Metadata.ConfidenceAutoApply {
		return fmt.Errorf("confidence_min_match (%f) must not exceed confidence_auto_apply (%f)",
			c.Metadata.ConfidenceMinMatch, c.Metadata.ConfidenceAutoApply)
	}
	// Check for overlapping media root paths
	sep := string(filepath.Separator)
	for i, r1 := range c.Media.Roots {
		if r1.Path == "" {
			return fmt.Errorf("media root %d: path must not be empty", i)
		}
		abs1, err := filepath.Abs(r1.Path)
		if err != nil {
			return fmt.Errorf("media root %d: cannot resolve path %q: %w", i, r1.Path, err)
		}
		for j, r2 := range c.Media.Roots {
			if i == j {
				continue
			}
			abs2, err := filepath.Abs(r2.Path)
			if err != nil {
				return fmt.Errorf("media root %d: cannot resolve path %q: %w", j, r2.Path, err)
			}
			if strings.HasPrefix(abs1+sep, abs2+sep) || strings.HasPrefix(abs2+sep, abs1+sep) {
				return fmt.Errorf("media roots %q and %q overlap", r1.Path, r2.Path)
			}
		}
	}
	return nil
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8080,
			BaseURL: "http://localhost:8080",
		},
		Database: DatabaseConfig{
			Path: "/config/library.db",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Auth: AuthConfig{
			InitialAdminPassword: "admin",
			SessionLifetimeDays:  30,
		},
		Metadata: MetadataConfig{
			AutoEnrich:               true,
			ConfidenceAutoApply:      0.85,
			ConfidenceMinMatch:       0.50,
			SourceCacheRetentionDays: 90,
			GoogleBooks: GoogleBooksConfig{
				Enabled: true,
			},
			OpenLibrary: OpenLibraryConfig{
				Enabled: true,
			},
			Audnexus: AudnexusConfig{
				Enabled: true,
			},
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CODEX_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("CODEX_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 {
			slog.Warn("invalid CODEX_PORT env var, using default", "value", v)
		} else {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("CODEX_BASE_URL"); v != "" {
		cfg.Server.BaseURL = v
	}
	if v := os.Getenv("CODEX_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("CODEX_LOG_LEVEL"); v != "" {
		cfg.Log.Level = strings.ToLower(v)
	}
	if v := os.Getenv("CODEX_LOG_FORMAT"); v != "" {
		cfg.Log.Format = strings.ToLower(v)
	}
	// Metadata API keys are NEVER read from config files — env-var only.
	if v := os.Getenv("CODEX_GOOGLE_BOOKS_API_KEY"); v != "" {
		cfg.Metadata.GoogleBooks.APIKey = v
	}
	// Initial admin password — env-var only, never in config file.
	if v := os.Getenv("CODEX_ADMIN_PASSWORD"); v != "" {
		cfg.Auth.InitialAdminPassword = v
	}
}
