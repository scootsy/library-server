package config

import (
	"fmt"
	"os"
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

// Load reads a config file at path (optional) and applies environment
// variable overrides. Returns a fully populated Config with defaults applied.
func Load(path string) (*Config, error) {
	cfg := defaults()

	if path != "" {
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

	return cfg, nil
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
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CODEX_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("CODEX_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
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
}
