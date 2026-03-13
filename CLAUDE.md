# CLAUDE.md — Codex Development Instructions

> This file is read by Claude Code at the start of every session. It contains the rules,
> patterns, and context needed to work on this codebase correctly and safely.
> For full project context (architecture, data model, roadmap), see PROJECT.md.
> For schema details, see schema-spec.md.

-----

## Project Summary

Codex is a self-hosted book and audiobook library server written in Go with a Svelte admin UI.
It ingests media files, enriches them with metadata from external sources, and serves them
via OPDS, KoboSync, KOReader sync, and audiobook streaming protocols. It is a headless
media server — it does NOT render or play media. Reading and listening happen in client apps.

**Tech stack:** Go (server) + SvelteKit static adapter (admin UI, embedded in binary) + SQLite (WAL + FTS5)

**Key architectural rules:**

- Single binary deployment (Svelte compiled and embedded via `go:embed`)
- Single SQLite database file (no external database servers)
- Sidecar files (`metadata.json` + `cover.jpg`) are the source of truth for metadata
- Database is the source of truth for user state (progress, collections, annotations)
- Codex never moves, renames, or reorganizes media files
- All formats of a work live in a single directory

-----

## Project Structure

```
codex/
├── cmd/codex/main.go              ← entry point
├── internal/
│   ├── config/                    ← YAML config loading
│   ├── database/                  ← SQLite connection, migrations, query functions
│   │   ├── database.go
│   │   ├── migrations/
│   │   └── queries/               ← organized by domain (works.go, users.go, etc.)
│   ├── scanner/                   ← directory walking, file detection, metadata extraction
│   ├── metadata/                  ← enrichment engine, fuzzy matching, merge strategy
│   │   └── sources/               ← API clients (googlebooks.go, openlibrary.go, audnexus.go)
│   ├── security/                  ← SafePath, input sanitization
│   ├── auth/                      ← middleware, OIDC, local auth, sessions
│   ├── api/                       ← REST API handlers (the Svelte UI talks to these)
│   ├── protocols/                 ← protocol adapters (each is independent)
│   │   ├── opds/
│   │   ├── kobosync/
│   │   ├── koreader/
│   │   └── audiostream/
│   └── server/                    ← HTTP server setup, graceful shutdown
├── web/                           ← SvelteKit source (compiled output embedded in Go binary)
├── embedded/web.go                ← go:embed directive for Svelte assets
├── Dockerfile
├── PROJECT.md                     ← full architecture and roadmap
├── schema-spec.md                 ← sidecar JSON format and SQLite schema
└── CLAUDE.md                      ← this file
```

### Package Dependency Rules

- Protocol adapters (`internal/protocols/*`) depend ONLY on `internal/database/queries`
- API handlers (`internal/api`) depend on `internal/database/queries`, `internal/scanner`, `internal/metadata`
- Metadata sources (`internal/metadata/sources`) have NO internal dependencies — pure API clients
- NEVER import from `internal/api` or `internal/protocols` into other packages
- ALL file I/O must go through `internal/security.SafePath()`
- ALL database queries live in `internal/database/queries/`, never inline in handlers

-----

## Security Rules (NON-NEGOTIABLE)

These rules apply to every line of code. Violations are not acceptable regardless of context.

### File System Security

1. **NEVER construct a file path from user input without `SafePath()` validation.**
   
   ```go
   // WRONG — path traversal vulnerability
   path := filepath.Join(mediaRoot, req.URL.Query().Get("file"))
   data, _ := os.ReadFile(path)
   
   // CORRECT — SafePath validates and constrains to allowed roots
   path, err := security.SafePath(req.URL.Query().Get("file"), mediaRoot)
   if err != nil {
       http.Error(w, "invalid path", http.StatusBadRequest)
       return
   }
   data, _ := os.ReadFile(path)
   ```
1. **NEVER use `os.Open`, `os.ReadFile`, `os.Stat`, or any file I/O with a path derived from user input** (query params, URL path segments, POST body fields, sidecar content) without first passing it through `SafePath()`.
1. **Sanitize all filenames** from user input: strip path separators (`/`, `\`), null bytes (`\x00`), and non-printable characters before using them in any file operation.

### Database Security

1. **NEVER use `fmt.Sprintf`, string concatenation, or template strings to build SQL queries.**
   
   ```go
   // WRONG — SQL injection vulnerability
   query := fmt.Sprintf("SELECT * FROM works WHERE title = '%s'", userInput)
   db.Query(query)
   
   // CORRECT — parameterized query
   db.Query("SELECT * FROM works WHERE title = ?", userInput)
   ```
1. **ALL database queries must use `?` placeholders.** No exceptions, even for “safe” values.
1. **ALL query functions live in `internal/database/queries/`.** Never write inline SQL in handlers.

### Authentication & Sessions

1. **ALL new HTTP handlers MUST be registered within an authenticated route group** unless they are explicitly public. Public endpoints are limited to:
- `GET /health` (health check)
- `GET /opds/*` (if configured as public in settings)
- OIDC callback endpoints
- Static asset serving (embedded Svelte SPA)
1. **Session tokens MUST be generated with `crypto/rand`**, never `math/rand`.
   
   ```go
   // WRONG — predictable tokens
   token := fmt.Sprintf("%d", rand.Int63())
   
   // CORRECT — cryptographically random
   b := make([]byte, 32)
   crypto_rand.Read(b)
   token := base64.URLEncoding.EncodeToString(b)
   ```
1. **Password hashing MUST use bcrypt with cost factor ≥ 12.**
   
   ```go
   hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
   ```
1. **NEVER log secrets, tokens, passwords, session IDs, or API keys.** The structured logger must have a sanitizer that redacts fields named `password`, `secret`, `token`, `key`, `authorization`.

### Frontend Security (Svelte)

1. **NEVER use `{@html}` in Svelte without DOMPurify sanitization.**
   
   ```svelte
   <!-- WRONG — XSS vulnerability -->
   {@html work.description}
   
   <!-- CORRECT — sanitized -->
   {@html DOMPurify.sanitize(work.description)}
   ```
1. **API responses that include user-controllable strings** (book titles, descriptions, contributor names) must not be trusted as safe HTML on the frontend. Always escape or sanitize.

### Network & Crypto

1. **NEVER disable TLS certificate verification** in HTTP clients (for metadata API calls, OIDC, etc.).
   
   ```go
   // NEVER DO THIS
   &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
   ```
1. **CORS must be locked to the configured base_url origin**, never `Access-Control-Allow-Origin: *`.
1. **Rate limit authentication endpoints** (login, token refresh, account creation) to prevent brute force.

### Secrets Management

1. **NEVER hardcode secrets, API keys, or credentials in source code.** Load from environment variables or config file.
1. **Config file containing secrets must have restrictive permissions** (0600). Log a warning on startup if permissions are too open.

-----

## Coding Patterns

### Error Handling

Use Go’s explicit error handling. Always handle errors — never use `_` to discard them in production code (test code is acceptable).

```go
// CORRECT
result, err := db.Query("SELECT * FROM works WHERE id = ?", id)
if err != nil {
    slog.Error("failed to query work", "id", id, "error", err)
    return fmt.Errorf("querying work %s: %w", id, err)
}

// WRONG — swallowed error
result, _ := db.Query("SELECT * FROM works WHERE id = ?", id)
```

Wrap errors with context using `fmt.Errorf("context: %w", err)` so stack traces are useful.

### Logging

Use Go’s `slog` structured logger (standard library, Go 1.21+).

```go
slog.Info("scan completed", "media_root", rootPath, "works_found", count, "duration_ms", elapsed)
slog.Error("metadata fetch failed", "work_id", workID, "source", "google_books", "error", err)
slog.Warn("low confidence match", "work_id", workID, "confidence", score, "title", candidate.Title)
```

Never use `fmt.Println` or `log.Println` for application logging.

### Database Access Pattern

All queries go through typed functions in `internal/database/queries/`:

```go
// internal/database/queries/works.go
func GetWorkByID(db *sql.DB, id string) (*models.Work, error) {
    row := db.QueryRow("SELECT id, title, sort_title, ... FROM works WHERE id = ?", id)
    var w models.Work
    err := row.Scan(&w.ID, &w.Title, &w.SortTitle, ...)
    if err == sql.ErrNoRows {
        return nil, nil  // not found is not an error
    }
    return &w, err
}
```

Handlers call these functions, never execute SQL directly:

```go
// internal/api/works.go
func (h *WorksHandler) GetWork(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    work, err := queries.GetWorkByID(h.db, id)
    if err != nil {
        slog.Error("database error", "error", err)
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    if work == nil {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    json.NewEncoder(w).Encode(work)
}
```

### HTTP Handler Pattern

Handlers are methods on a struct that holds dependencies (database, config, etc.):

```go
type WorksHandler struct {
    db     *sql.DB
    config *config.Config
}

func NewWorksHandler(db *sql.DB, cfg *config.Config) *WorksHandler {
    return &WorksHandler{db: db, config: cfg}
}
```

### SafePath Implementation Pattern

```go
// internal/security/safepath.go
package security

import (
    "fmt"
    "path/filepath"
    "strings"
)

// SafePath resolves a user-influenced path and validates it falls within
// one of the allowed root directories. Returns the cleaned absolute path
// or an error if validation fails.
func SafePath(userPath string, allowedRoots ...string) (string, error) {
    // Strip null bytes
    if strings.ContainsRune(userPath, 0) {
        return "", fmt.Errorf("path contains null byte")
    }

    // Resolve to absolute, following symlinks
    resolved, err := filepath.Abs(userPath)
    if err != nil {
        return "", fmt.Errorf("resolving path: %w", err)
    }
    resolved, err = filepath.EvalSymlinks(resolved)
    if err != nil {
        return "", fmt.Errorf("evaluating symlinks: %w", err)
    }

    // Check against allowed roots
    for _, root := range allowedRoots {
        absRoot, err := filepath.Abs(root)
        if err != nil {
            continue
        }
        absRoot, err = filepath.EvalSymlinks(absRoot)
        if err != nil {
            continue
        }
        // Ensure the resolved path is within or equal to the root
        if strings.HasPrefix(resolved, absRoot+string(filepath.Separator)) || resolved == absRoot {
            return resolved, nil
        }
    }

    return "", fmt.Errorf("path %q is outside allowed roots", userPath)
}
```

-----

## Testing

### Running Tests

```bash
go test ./...                    # all tests
go test ./internal/scanner/...   # specific package
go test -v -run TestSafePath     # specific test
go test -race ./...              # race condition detection
```

### Test Patterns

Use table-driven tests (Go convention):

```go
func TestSafePath(t *testing.T) {
    tests := []struct {
        name        string
        userPath    string
        roots       []string
        wantErr     bool
    }{
        {"valid path", "/media/book.epub", []string{"/media"}, false},
        {"traversal attack", "/media/../etc/passwd", []string{"/media"}, true},
        {"null byte", "/media/book\x00.epub", []string{"/media"}, true},
        {"outside root", "/tmp/evil.epub", []string{"/media"}, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := SafePath(tt.userPath, tt.roots...)
            if (err != nil) != tt.wantErr {
                t.Errorf("SafePath() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

Every new query function in `internal/database/queries/` should have a corresponding test using an in-memory SQLite database.

-----

## Security Scanning (run after every session)

```bash
# Go security analysis
gosec ./...

# Dependency vulnerability check
govulncheck ./...

# Code quality and correctness
staticcheck ./...

# Frontend dependencies (if web/ changed)
cd web && npm audit && cd ..

# Container scan (if Docker image rebuilt)
docker build -t codex:latest .
docker run --rm aquasec/trivy image codex:latest
```

**Address all HIGH and MEDIUM findings before committing.** LOW findings should be reviewed but may be deferred with a comment explaining why.

-----

## Build Commands

```bash
# Development: build and run Go server (serves Svelte dev server via proxy)
go run ./cmd/codex --config config.yaml

# Development: run Svelte dev server
cd web && npm run dev

# Production: build Svelte, then build Go with embedded assets
cd web && npm run build && cd ..
go build -o codex ./cmd/codex

# Docker: build complete image
docker build -t codex:latest .

# Database: run migrations (happens automatically on startup, but can force)
./codex migrate

# Scanner: trigger manual scan
./codex scan --root /media
```

-----

## Key Files Reference

|File                                 |Purpose                                                 |When to read                   |
|-------------------------------------|--------------------------------------------------------|-------------------------------|
|`PROJECT.md`                         |Full architecture, data model, roadmap, design decisions|Start of any major feature work|
|`schema-spec.md`                     |Sidecar JSON format and SQLite schema with all tables   |Any database or sidecar work   |
|`CLAUDE.md`                          |This file — coding rules, patterns, security constraints|Every session (auto-read)      |
|`internal/security/safepath.go`      |Path validation — MUST be used for all file I/O         |Any file-serving code          |
|`internal/database/database.go`      |DB connection setup, WAL config, migration runner       |Database changes               |
|`internal/database/queries/`         |All SQL queries organized by domain                     |Any data access                |
|`internal/metadata/sources/source.go`|Interface that all metadata sources implement           |Adding new metadata sources    |
|`internal/api/router.go`             |HTTP route registration and middleware chain            |Adding new endpoints           |
|`config.example.yaml`                |All configuration options with defaults                 |Config changes                 |

-----

## Common Tasks

### Adding a new REST API endpoint

1. Add the query function(s) in `internal/database/queries/`
1. Add the handler method in the appropriate `internal/api/*.go` file
1. Register the route in `internal/api/router.go` within the correct auth group
1. Add tests for both the query and the handler
1. Run `gosec ./...` and `staticcheck ./...`

### Adding a new metadata source

1. Create `internal/metadata/sources/newsource.go`
1. Implement the `MetadataSource` interface defined in `source.go`
1. Register the source in `internal/metadata/engine.go`
1. Add configuration options in `internal/config/config.go`
1. Update `config.example.yaml`

### Adding a new protocol adapter

1. Create `internal/protocols/newprotocol/handler.go`
1. Depend ONLY on `internal/database/queries` — no imports from `api` or other protocols
1. Register routes in `internal/server/server.go`
1. Add enable/disable config in `internal/config/config.go`
1. Update `config.example.yaml`

### Modifying the database schema

1. Create a new numbered migration in `internal/database/migrations/`
1. Migrations are Go code, not SQL files — they run within a transaction
1. Update the corresponding query functions in `internal/database/queries/`
1. Update `schema-spec.md` to reflect the change
1. Test with both fresh database creation and migration from previous version

### Modifying the sidecar format

1. Increment `schema_version` in the sidecar spec
1. Add migration logic in `internal/scanner/sidecar.go` to handle old versions
1. Update `schema-spec.md`
1. Ensure backward compatibility — old sidecars must still be readable

-----

## Things Claude Code Should NEVER Do

- Generate code that uses `fmt.Sprintf` or `+` to build SQL queries
- Generate code that constructs file paths from user input without `SafePath()`
- Generate code that uses `math/rand` for security-sensitive values
- Generate code that disables TLS verification
- Generate code that hardcodes secrets or credentials
- Generate Svelte code that uses `{@html}` without DOMPurify
- Log passwords, tokens, session IDs, or API keys
- Import from `internal/api` or `internal/protocols` into other internal packages
- Write inline SQL in handler functions (queries belong in `internal/database/queries/`)
- Skip error handling with `_` in production code
- Create new public HTTP endpoints without explicit documentation of why they’re public
- Use `os.Open` / `os.ReadFile` / `os.Stat` with user-influenced paths without `SafePath()`
