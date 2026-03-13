# Code Quality Audit — Codex Library Server

**Date:** 2026-03-13
**Status:** All findings addressed

All 4 high-severity, 11 medium-severity, and 12 low-severity findings from the
2026-03-13 audit have been resolved. See git history for the full original audit
and the commit that addressed all items.

## 2026-03-13 Follow-up Audit (Architecture, Quality, Scalability)

### Scope Reviewed
- Core project intent and roadmap alignment from `CLAUDE.md` and `PROJECT.md`.
- Go backend structure (`cmd/`, `internal/api`, `internal/auth`, `internal/database`, `internal/metadata`, `internal/scanner`, `internal/server`).
- Svelte admin UI source (`web/src`) and embedded build outputs (`embedded/dist`).
- Test baseline via `go test ./...`.

### Executive Summary
The repository has a clean modular direction (config/database/scanner/metadata/api separation), and generally follows its own “single binary + single SQLite + sidecar-first metadata” philosophy. The current risk profile is mostly around **production hardening and long-term consistency**, not foundational design collapse.

Most important issues now:
1. **Test/runtime coupling to SQLite FTS5 build configuration is fragile** in default environments.
2. **Security hardening gaps** (cookie security flags, login/logout token handling edge case, no anti-bruteforce controls).
3. **Query input controls are too permissive** (pagination params can be negative or arbitrarily large).
4. **Vision/implementation drift**: protocol adapter architecture is documented as core, but not yet present in code.
5. **Process consistency gap**: `AUDIT.md` currently says all findings were resolved, but there are still unresolved concerns that should be tracked.

### Detailed Findings & Suggestions

#### 1) FTS5 dependency is not reliably enabled for default test/dev runs (High)
**Observation**
- Running `go test ./...` fails in this environment with repeated `no such module: fts5` migration errors.
- The codebase includes an `fts5` build-tag shim, but default commands do not enforce `-tags fts5`.

**Impact**
- New contributors and CI runners without explicit tag setup will get immediate failures.
- This can hide real regressions behind environment-specific setup noise.

**Suggestions**
- Add a Make target and CI standardization (`go test -tags fts5 ./...`).
- Add a startup/test preflight check that detects missing FTS5 and returns a clearer actionable message.
- Document required build tags in `README.md` quick-start and contributor sections.

#### 2) Session cookie security defaults are incomplete (High)
**Observation**
- Login sets `HttpOnly` and `SameSite=Lax` but does not set `Secure`.

**Impact**
- In HTTPS deployments, absence of `Secure` allows accidental transmission over plaintext HTTP if misconfigured at ingress.

**Suggestions**
- Set `Secure: true` when `server.base_url` is https.
- Optionally add config override for trusted reverse-proxy TLS termination scenarios.

#### 3) Logout token extraction is brittle for Authorization headers (Medium)
**Observation**
- Logout path slices `Authorization` header by index if length > 7, without verifying `Bearer ` prefix.

**Impact**
- Non-standard auth headers may produce incorrect token extraction attempts.
- Not a critical exploit by itself, but a correctness and consistency issue compared to stricter parsing in auth middleware.

**Suggestions**
- Reuse the same Bearer parsing logic as auth middleware (`extractToken`) for consistency.

#### 4) Missing anti-bruteforce and login rate limiting (High)
**Observation**
- Login endpoint checks credentials directly with no visible attempt throttling, IP/user lockout, or backoff.

**Impact**
- Increased risk of credential stuffing and online password guessing, especially with internet-exposed deployments.

**Suggestions**
- Add per-IP and per-username rate limiting with rolling windows.
- Add exponential backoff or temporary lockout after repeated failures.
- Emit structured security audit logs for failed auth patterns.

#### 5) Pagination/query parameter validation is weak (Medium)
**Observation**
- Shared integer query parser accepts negative and unbounded values.
- Many list/search handlers consume this helper.

**Impact**
- Negative offsets/limits can produce inconsistent SQL behavior.
- Very high limits can cause avoidable memory and response-size pressure.

**Suggestions**
- Clamp values globally (e.g., `limit` default 50, min 1, max 200; `offset` min 0).
- Return 400 for invalid values instead of silent fallback where appropriate.

#### 6) Body size limits are not enforced for JSON API endpoints (Medium)
**Observation**
- Handlers decode request JSON directly from `r.Body` with no global/request-level max body guard.

**Impact**
- Risk of oversized payload memory pressure / DoS vector.

**Suggestions**
- Wrap bodies with `http.MaxBytesReader` in a middleware for `/api/*` JSON endpoints.
- Use endpoint-specific tighter limits for known small payloads (auth/settings/etc).

#### 7) Architecture roadmap drift: protocol adapters are defined as central, but absent (Medium)
**Observation**
- Documentation repeatedly defines protocol adapters (`internal/protocols/opds`, `kobosync`, etc.) as key architecture.
- Repository currently has no `internal/protocols` directory.

**Impact**
- New contributors may optimize around abstractions that do not yet exist.
- Planning and implementation priorities can diverge without explicit “not yet implemented” status markers.

**Suggestions**
- Add explicit implementation status table in `PROJECT.md` (Planned/In Progress/Done).
- Either scaffold protocol package boundaries now, or soften docs to indicate staged rollout.

#### 8) Generated frontend artifacts are committed; source-of-truth process should be explicit (Low)
**Observation**
- `embedded/dist` built assets are committed alongside `web/src`.

**Impact**
- Risk of stale embedded assets when source changes are merged without rebuild.
- Larger diffs and noisier reviews over time.

**Suggestions**
- Decide and document one policy:
  - (A) keep committed build artifacts and enforce rebuild checks in CI, or
  - (B) stop committing build output and generate during release build.
- If staying with (A), add a CI verification step that fails on stale dist output.

#### 9) `AUDIT.md` status header is currently misleading (Process, Low)
**Observation**
- File currently states all previous findings are resolved.
- Current codebase still has unresolved risks listed above.

**Impact**
- Can create false confidence for maintainers and external reviewers.

**Suggestions**
- Move to a living audit format with dated sections and open/closed status per finding.
- Keep top-level status synchronized with latest section conclusions.

### Positive Notes
- Security-conscious path handling utilities (`SafePath`, `SafePathParent`) are present and used in scanner/sidecar write paths.
- DB access is centralized under `internal/database/queries`, preserving clear architectural boundaries.
- Metadata engine structure (source abstraction + scoring + queue + cache) is extensible and aligns with project goals.
- Recovery/logging/CORS/content-type middleware layering is clean and readable.

### Recommended Next Sprint Priorities
1. Fix test/CI ergonomics around FTS5 tags and preflight messaging.
2. Harden auth surface (cookie `Secure`, logout parser consistency, brute-force protection).
3. Add shared request safeguards (body size limits + pagination clamping).
4. Reconcile docs vs implementation status for protocol modules.
5. Convert `AUDIT.md` into a durable open-findings tracker.
