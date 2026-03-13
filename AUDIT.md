# Code Quality Audit — Codex Library Server

**Date:** 2026-03-13
**Scope:** Full codebase audit (~30 Go source files, ~4,500 lines)

---

## Executive Summary

The codebase is well-structured with clean separation of concerns, consistent use of
parameterized SQL queries, and good HTTP client hygiene in metadata sources. However,
the audit identified **4 high-severity issues**, **11 medium-severity issues**, and
**8 low-severity items** across security, error handling, consistency, and test coverage.

No SQL injection or TLS verification vulnerabilities were found. The most critical
findings relate to incomplete SafePath enforcement in the scanner, discarded errors
violating project conventions, stub functions masquerading as implementations, and
a missing log sanitizer for secrets.

---

## Critical & High Severity Findings

### H1. Scanner constructs file paths from directory entries without SafePath validation

**Files:** `internal/scanner/scanner.go:396,410`

```go
epubPath := filepath.Join(absDir, f.name)   // line 396
audioPath := filepath.Join(absDir, f.name)  // line 410
```

While `absDir` is validated via SafePath on line 79, `f.name` comes from `os.ReadDir()`
and is used directly in `filepath.Join`. A malicious filename containing path separators
(e.g., `../../../etc/passwd.epub`) would escape the validated root. The result is passed
to `os.Open` inside `ExtractEPUBMeta` and `ExtractAudioMeta`.

**Risk:** Path traversal — reading arbitrary files on the host.
**Fix:** Validate that `f.name` contains no path separators, or re-validate the joined
path through SafePath before use.

---

### H2. `ReadSidecar()` and `WriteSidecar()` skip SafePath validation

**Files:** `internal/scanner/sidecar.go:160,191`

Both functions accept a `dir` string and use `filepath.Join(dir, sidecarFilename)` to
construct a path for `os.ReadFile` / `os.WriteFile`. A comment claims `dir` is
"pre-validated", but no runtime enforcement exists. Only `HashSidecar()` (line 210)
actually calls `security.SafePath`.

**Risk:** If a future caller passes an unvalidated directory, file reads/writes could
escape the media root.
**Fix:** Either add SafePath validation inside these functions (accepting `mediaRoot` as
a parameter), or document the precondition with a build-tag-checked assertion.

---

### H3. Discarded errors throughout the codebase (CLAUDE.md violation)

CLAUDE.md states: *"Always handle errors — never use `_` to discard them in production
code."* The following locations violate this rule:

| File | Line(s) | Discarded Error |
|------|---------|-----------------|
| `internal/scanner/scanner.go` | 156 | `hashFile()` error |
| `internal/scanner/scanner.go` | 423 | `filepath.Rel()` error |
| `internal/database/queries/works.go` | 237-239 | `time.Parse()` x3 |
| `internal/database/queries/metadata.go` | 75,156,218 | `time.Parse()` x3 |
| `internal/database/queries/media_roots.go` | 73-74,90-91 | `time.Parse()` x4 |
| `internal/scanner/sidecar.go` | 198 | `os.Remove()` on temp file |
| `internal/metadata/sources/googlebooks.go` | 234,238 | `strconv.ParseFloat()` x2 |
| `internal/metadata/sources/audnexus.go` | 198 | `fmt.Sscanf()` |
| `internal/scanner/mp4parser.go` | 268,351 | Assigned-then-discarded vars |

**Impact:** Corrupted timestamps silently become zero values. Failed file operations
go unnoticed. Parse errors in metadata silently default to 0.
**Fix:** Handle each error — either return it, log a warning, or add a comment
explaining why it is safe to ignore.

---

### H4. Missing log sanitizer for secrets

**File:** `cmd/codex/main.go:125-146`

CLAUDE.md requires: *"The structured logger must have a sanitizer that redacts fields
named `password`, `secret`, `token`, `key`, `authorization`."* The `setupLogger()`
function creates a plain `slog.TextHandler` or `slog.JSONHandler` with no
`ReplaceAttr` function to redact sensitive fields.

Meanwhile, `internal/metadata/sources/googlebooks.go:160` logs the full request URL
(which may contain `?key=API_KEY`) in a warning message.

**Risk:** API keys and other secrets could appear in log output.
**Fix:** Add a `ReplaceAttr` function to the slog handler that redacts values for
keys matching `password`, `secret`, `token`, `key`, `authorization`, `api_key`.

---

## Medium Severity Findings

### M1. `applyRoleRefinement()` is a no-op stub

**File:** `internal/scanner/epub.go:365-373`

This function is called from the EPUB parser (line 279) but its body does nothing —
it assigns loop variables to `_` blanks. EPUB3 role refinements are silently ignored,
meaning author/narrator role metadata from OPF3 files is lost.

**Fix:** Either implement the function or remove the call and add a TODO with a
tracking issue.

---

### M2. `applyFileAsRefinement()` has a logic bug

**File:** `internal/scanner/epub.go:375-382`

```go
if m.Authors[i].Role == id {  // compares role ("author") to creator ID ("#creator01")
```

This comparison will never match because it compares a role string against a creator
ID. The `SortName` (file-as) refinement is never applied.

**Fix:** Match on the creator ID, not the role field.

---

### M3. `ExtractAudioMeta()` opens the file twice

**File:** `internal/scanner/audiobook.go:36-54`

The function opens `filePath` on line 38 (for format detection via extension), then
the `extractMP4Meta()` / `extractID3Meta()` calls open the same file again. The first
file handle is never read — only `defer f.Close()` keeps it from leaking.

**Fix:** Remove the unnecessary initial open, or pass the already-open handle to the
format-specific extractors.

---

### M4. `extractMP4Meta()` silently swallows parse errors

**File:** `internal/scanner/mp4parser.go:60-72`

```go
if err := parser.parse(); err != nil {
    return &AudioMeta{}, nil  // error lost
}
```

Callers cannot distinguish "file had no metadata" from "file was corrupt". This should
at minimum log a warning with the file path and error.

---

### M5. `findMediaFiles()` silently returns nil on directory read failure

**File:** `internal/scanner/scanner.go:651-678`

If `os.ReadDir()` fails (permissions, deleted directory), the function returns `nil`
with no logging. Callers interpret this as "directory has no media files" rather than
"an error occurred".

**Fix:** Log a warning with the directory path and error.

---

### M6. Inconsistent `ON CONFLICT` clause syntax in database queries

**Files:** `internal/database/queries/tags.go:13` vs `tags.go:32`, `metadata.go:31`

Some queries specify explicit conflict columns (`ON CONFLICT(name, type) DO NOTHING`),
others use bare `ON CONFLICT DO NOTHING`. The explicit form is safer against schema
changes adding additional unique constraints.

**Fix:** Standardize on explicit column specifications.

---

### M7. No test coverage for metadata source implementations

**Files:** `internal/metadata/sources/googlebooks.go`, `openlibrary.go`, `audnexus.go`

Zero `_test.go` files exist for the sources package. Response parsing, error handling,
HTTP status code handling, and edge cases in data mapping are all untested.

**Fix:** Add tests with mocked HTTP responses (`httptest.NewServer`) for each source.

---

### M8. Redundant transaction rollback in `DequeueMetadataTask()`

**File:** `internal/database/queries/metadata.go:41-92`

An explicit `tx.Rollback()` on line 69 (for `sql.ErrNoRows`) is redundant with the
deferred rollback closure at line 48. The dual handling is confusing.

**Fix:** Remove the explicit rollback and rely on the defer.

---

### M9. Each metadata source creates its own `http.Client`

**Files:** All three source constructors (`googlebooks.go:31`, `openlibrary.go:27`,
`audnexus.go:28`)

Each source creates a separate `http.Client{Timeout: 15 * time.Second}` with default
transport settings. This prevents connection pooling across sources and could lead to
file descriptor exhaustion under heavy metadata enrichment loads.

**Fix:** Accept a shared `*http.Client` as a constructor parameter, configured once
at the engine level with proper transport limits.

---

### M10. Config file permissions not checked

**File:** `internal/config/config.go:101-118`

CLAUDE.md requires: *"Config file containing secrets must have restrictive permissions
(0600). Log a warning on startup if permissions are too open."* The `Load()` function
reads the config file but never checks its permissions. While API keys are env-var-only,
the database path and other settings could be sensitive.

**Fix:** `os.Stat()` the config file and warn if mode is more permissive than 0600.

---

### M11. No config validation

**File:** `internal/config/config.go`

No validation is performed after loading. Invalid configurations (empty database path,
port 0, negative confidence thresholds, no media roots, overlapping media root paths)
are silently accepted and will cause failures later at runtime.

**Fix:** Add a `Validate()` method that checks invariants and returns clear errors.

---

## Low Severity Findings

### L1. SafePath test coverage is thin

**File:** `internal/security/safepath_test.go`

Only 4 test cases. Missing coverage for: symlink traversal, relative paths (e.g.,
`../../etc/passwd`), empty string input, very long paths, Unicode path components,
multiple allowed roots, and non-existent paths.

---

### L2. SafePath does not handle non-existent paths

**File:** `internal/security/safepath.go:23`

`filepath.EvalSymlinks()` returns an error for paths that don't exist. This means
SafePath cannot validate paths before the file is created (e.g., for new sidecar
writes). This may need a variant that validates the parent directory instead.

---

### L3. Duplicated helper functions across query files

**File:** `internal/database/queries/works.go`

`nullableString()`, `nullableInt()`, `nullableFloat()`, `boolToInt()` are defined in
`works.go` but used from `files.go`, `covers.go`, and `metadata.go`. Should be
extracted to a `helpers.go` file in the same package.

---

### L4. Duplicated time-parsing pattern

**Files:** `queries/works.go`, `queries/metadata.go`, `queries/media_roots.go`

The pattern `field, _ = time.Parse(time.DateTime, dbString)` appears 10 times. A
shared `parseDBTime(s string) time.Time` helper would centralize error handling
(once H3 is fixed) and reduce duplication.

---

### L5. Format detection is scattered across multiple files

**Files:** `scanner.go` (supportedFormats map), `audiobook.go` (switch on extension),
`scanner.go:extractFromFiles()` (format checks)

Format classification logic exists in three places. Adding a new format requires
changes in multiple locations, risking inconsistency.

---

### L6. `SortTitle()` is in `sidecar.go` but is a general utility

**File:** `internal/scanner/sidecar.go:234-244`

Called from `epub.go`, `audiobook.go`, and `sidecar.go`. It's a string utility that
doesn't belong in the sidecar module. Consider a `stringutil` package or a
`scanner/util.go` file.

---

### L7. Unused variable assignments in mp4parser

**File:** `internal/scanner/mp4parser.go:268,351`

```go
_ = cur  // line 268
_ = ver  // line 351
```

Variables are assigned and immediately discarded. Either use them or remove the
assignments.

---

### L8. `server.New()` does not receive a `*sql.DB`

**File:** `internal/server/server.go:24`

The server constructor only takes `*config.Config`. To add any authenticated routes or
API handlers, it will need access to the database. This will require a signature change
when the server grows beyond the health endpoint.

---

## Architecture & Structural Observations

### What's Working Well

1. **Clean package boundaries** — Protocol adapters, API, metadata sources, and database
   queries are properly isolated. No circular imports detected.
2. **SQL injection prevention** — All 100+ SQL statements use parameterized queries.
   No string concatenation in SQL anywhere in the codebase.
3. **HTTP client hygiene** — All metadata sources properly close response bodies, limit
   response sizes to 2 MiB, use context propagation, and have timeouts.
4. **TLS security** — No `InsecureSkipVerify` anywhere in the codebase.
5. **Graceful shutdown** — Server properly drains connections with a 30-second deadline.
6. **Secrets handling** — API keys are env-var-only (`yaml:"-"` tag on GoogleBooksConfig).
7. **Dependency footprint** — Only 3 direct dependencies (`uuid`, `go-sqlite3`, `yaml.v3`).
   Minimal attack surface.

### Structural Risks for Growth

1. **No middleware chain** — `server.go` uses raw `http.NewServeMux()` with no middleware
   for auth, CORS, rate limiting, or request logging. These will be needed before any
   authenticated endpoints are added.
2. **No API handler registration** — The server only has a health endpoint. The router
   architecture described in CLAUDE.md (`internal/api/router.go`) doesn't exist yet.
   When it's built, ensure the patterns established in CLAUDE.md are followed.
3. **Scanner is synchronous** — `Scan()` processes directories sequentially. For large
   libraries (10k+ works), this could be slow. Consider a worker pool for file I/O and
   metadata extraction. Not urgent but worth designing for.

---

## Recommended Priority Actions

### Immediate (before next feature work)

1. Fix path traversal risk in scanner — validate filenames from `os.ReadDir` (H1)
2. Add SafePath enforcement to `ReadSidecar`/`WriteSidecar` (H2)
3. Add log sanitizer `ReplaceAttr` for secrets (H4)
4. Fix `applyFileAsRefinement()` logic bug (M2)

### Short-term (next sprint)

5. Address all discarded errors (H3) — either handle or document
6. Implement or remove `applyRoleRefinement()` stub (M1)
7. Add metadata source tests with httptest mocks (M7)
8. Add config validation (M11)
9. Check config file permissions (M10)

### When convenient

10. Consolidate helper functions and time-parsing (L3, L4)
11. Expand SafePath test coverage (L1)
12. Centralize format detection (L5)
13. Share `http.Client` across metadata sources (M9)
