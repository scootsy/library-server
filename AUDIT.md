# Code Quality Audit — Codex Library Server

**Date:** 2026-03-13
**Status:** Follow-up audit in progress — see finding statuses below

All 4 high-severity, 11 medium-severity, and 12 low-severity findings from the
initial 2026-03-13 audit have been resolved. See git history for the full original
audit and the commit that addressed all items.

## 2026-03-13 Follow-up Audit (Architecture, Quality, Scalability)

### Scope Reviewed
- Core project intent and roadmap alignment from `CLAUDE.md` and `PROJECT.md`.
- Go backend structure (`cmd/`, `internal/api`, `internal/auth`, `internal/database`, `internal/metadata`, `internal/scanner`, `internal/server`).
- Svelte admin UI source (`web/src`) and embedded build outputs (`embedded/dist`).
- Test baseline via `go test ./...`.

### Executive Summary
The repository has a clean modular direction (config/database/scanner/metadata/api separation), and generally follows its own "single binary + single SQLite + sidecar-first metadata" philosophy. The current risk profile is mostly around **production hardening and long-term consistency**, not foundational design collapse.

### Detailed Findings & Resolution Status

#### 1) FTS5 dependency is not reliably enabled for default test/dev runs (High) — RESOLVED
**Observation:** Running `go test ./...` fails without explicit `-tags fts5`.
**Resolution:** Added a `Makefile` with standardized targets (`make test`, `make build`, etc.) that include `-tags fts5`. Contributors should use `make test` instead of bare `go test`.

#### 2) Session cookie security defaults are incomplete (High) — RESOLVED (pre-existing fix)
**Observation:** Audit claimed `Secure` flag was missing from session cookies.
**Resolution:** Already implemented correctly. `internal/api/auth.go` derives the `Secure` flag from `server.base_url`:
```go
secureCookie := strings.HasPrefix(h.config.Server.BaseURL, "https://")
```
No code change was needed — the audit finding was stale.

#### 3) Logout token extraction is brittle for Authorization headers (Medium) — RESOLVED
**Observation:** Logout handler used `authHeader[7:]` without verifying `Bearer ` prefix.
**Resolution:** Replaced with proper `strings.HasPrefix(authHeader, "Bearer ")` check, consistent with the auth middleware's `extractToken` function.

#### 4) Missing anti-bruteforce and login rate limiting (High) — RESOLVED
**Observation:** Login endpoint had no rate limiting or brute-force protection.
**Resolution:** Added per-IP rate limiting middleware (`internal/api/ratelimit.go`) applied to the login endpoint. Uses a sliding window of 10 requests/minute per IP with structured logging on rate limit events.

#### 5) Pagination/query parameter validation is weak (Medium) — RESOLVED
**Observation:** `parseIntParam` accepted negative values for offset/limit.
**Resolution:** Added negative value clamping in `parseIntParam` — negative values now fall back to the default. The query layer already clamped limits (1–100 for works, 1–200 for contributors).

#### 6) Body size limits are not enforced for JSON API endpoints (Medium) — RESOLVED
**Observation:** No `MaxBytesReader` guard on request bodies.
**Resolution:** Added `maxBodyMiddleware` in the middleware chain that wraps `r.Body` with `http.MaxBytesReader` (1 MB limit) for all `/api/*` routes.

#### 7) Architecture roadmap drift: protocol adapters are defined as central, but absent (Medium) — RESOLVED
**Observation:** `internal/protocols/` directory doesn't exist despite being documented as core architecture.
**Resolution:** Added implementation status table to `PROJECT.md` marking protocol adapters as "Planned — not yet implemented".

#### 8) Generated frontend artifacts are committed; source-of-truth process should be explicit (Low) — ACKNOWLEDGED
**Observation:** `embedded/dist/` build artifacts are committed alongside source.
**Status:** This is an intentional design decision for the single-binary deployment model. Build artifacts are committed so that `go build` works without a Node.js toolchain. A CI staleness check is recommended as a future improvement but is not blocking.

#### 9) `AUDIT.md` status header is currently misleading (Process, Low) — RESOLVED
**Observation:** Header said "All findings addressed" while listing 9 open findings.
**Resolution:** Updated this file to use a living audit format with per-finding status tracking.

### Positive Notes
- Security-conscious path handling utilities (`SafePath`, `SafePathParent`) are present and used in scanner/sidecar write paths.
- DB access is centralized under `internal/database/queries`, preserving clear architectural boundaries.
- Metadata engine structure (source abstraction + scoring + queue + cache) is extensible and aligns with project goals.
- Recovery/logging/CORS/content-type middleware layering is clean and readable.
