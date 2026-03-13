# Codex: A Self-Hosted Book & Audiobook Library Server

> **This document is the single source of truth for project architecture, design decisions, and roadmap. Feed this to any LLM before beginning implementation work.**

-----

## Project Identity

**Name:** Codex (working title — subject to change)
**Tagline:** A headless, metadata-first library server for books, audiobooks, and read-alongs.
**License:** TBD (leaning AGPL-3.0 or MIT)

**What Codex IS:**

- A self-hosted media server that ingests, catalogs, enriches, and serves books and audiobooks
- A metadata engine that fetches, merges, and curates rich book data from multiple sources
- A protocol server that speaks OPDS, KoboSync, KOReader sync, and Audiobookshelf-compatible streaming
- A headless service designed to hand off the reading/listening experience to purpose-built client apps

**What Codex is NOT:**

- Not a built-in reader or player (no EPUB rendering, no audio player in the web UI)
- Not a file acquisition tool (no downloading, no torrent integration — that’s what the arr stack is for)
- Not a forced alignment / transcription engine (it serves pre-aligned EPUB3 media overlays created by tools like Storyteller, but does not create them)

-----

## Core Philosophy

1. **Metadata is the product.** The entire value proposition is taking a pile of files and making them *known* — enriched with covers, descriptions, series info, ratings, and identifiers that every downstream client can consume. Every architectural decision serves this goal.
1. **The file system is the source of truth for metadata.** Sidecar files (`metadata.json` + `cover.jpg`) live next to the media files. The SQLite database is a queryable index that can be rebuilt from sidecars at any time. If you delete the database, you lose user state (progress, shelves) but not your metadata curation work.
1. **The database is the source of truth for user state.** Reading progress, listening positions, shelf assignments, annotations, and sync state live exclusively in SQLite. These change frequently and benefit from database performance characteristics.
1. **Accommodate, don’t dictate.** Codex reads whatever folder structure it finds. It does not rename, move, or reorganize files. It writes sidecar files next to the originals and builds a virtual, consistent view through its API. Users who use Readarr, Chaptarr, or manual organization are all equally supported.
1. **Protocols over presentation.** The web admin UI exists for library management and metadata curation. The actual reading/listening experience is delegated to purpose-built apps via standard protocols (OPDS, KoboSync, KOReader sync, audiobook streaming API).
1. **Single binary, single database, single container.** No external database servers, no runtime dependencies beyond the Go binary and the embedded Svelte SPA. The entire application ships as one Docker image.

-----

## Technology Stack

|Layer            |Choice                                                                             |Rationale                                                                                                                                                                                    |
|-----------------|-----------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|**Language**     |Go                                                                                 |Single binary compilation, minimal RAM (20-50MB), built-in HTTP server and concurrency (goroutines), simple error model readable by non-developers, excellent AI-assisted development support|
|**Database**     |SQLite (WAL mode + FTS5)                                                           |Zero operational overhead, single-file backup, handles read-heavy workloads perfectly, full-text search built in. Both Audiobookshelf and Storyteller independently validated this choice.   |
|**SQLite Driver**|`modernc.org/sqlite` (pure Go, no CGO) or `mattn/go-sqlite3` (CGO, slightly faster)|Pure Go preferred for simpler cross-compilation and CI. Performance difference negligible for this workload. Decision deferred to implementation.                                            |
|**Frontend**     |SvelteKit (static adapter, embedded in Go binary)                                  |Reactive UI for metadata review workflows, tiny bundle size, compiled to static files and embedded via Go’s `embed` package. Result is still a single binary.                                |
|**Auth**         |OIDC via `coreos/go-oidc` + local accounts                                         |OIDC for SSO integration (Authentik, Authelia). Local accounts as fallback.                                                                                                                  |
|**Container**    |Single Docker image (Alpine-based)                                                 |One image, one container. No sidecar containers, no external databases.                                                                                                                      |

### Rejected Alternatives (with reasoning)

|Option                      |Why rejected                                                                                                                                                                                      |
|----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|**Rust**                    |Borrow checker hostility for non-developers; AI-assisted Rust development produces significantly more compile errors; async Rust (tokio) is complex; no performance benefit for I/O-bound workload|
|**TypeScript/Node.js**      |Single-threaded event loop blocks on CPU work; npm dependency fragility; larger Docker images; requires runtime                                                                                   |
|**Python**                  |Slow HTTP serving; GIL concurrency issues; messy deployment (venvs); would only be justified if forced alignment were in-scope                                                                    |
|**MariaDB/PostgreSQL**      |Unnecessary operational complexity for a single-server self-hosted app; separate container; connection pooling; backup scripts — all for zero practical benefit at this scale                     |
|**HTMX (instead of Svelte)**|Adequate for forms and tables, but metadata review workflow (comparing sources side-by-side, cover swapping, drag-and-drop shelf curation) benefits from reactive SPA                             |
|**React/Angular/Vue**       |Heavier bundle sizes, more complex build tooling, no advantage over Svelte for this use case                                                                                                      |

-----

## Data Model

### The “Work” Concept

The central entity in Codex is a **Work** — a single intellectual creation that may exist in multiple physical formats. All formats of a book live in one directory:

```
/media/Brandon Sanderson/Stormlight Archive/01 - The Way of Kings/
  ├── The Way of Kings.epub              ← ebook
  ├── The Way of Kings.pdf               ← alternate ebook format
  ├── The Way of Kings.m4b               ← audiobook
  ├── The Way of Kings - Aligned.epub    ← EPUB3 with media overlays (created by Storyteller)
  ├── metadata.json                       ← sidecar (source of truth for metadata)
  ├── cover.jpg                           ← selected cover image
  ├── cover_embedded.jpg                  ← extracted from epub
  └── cover_openlibrary.jpg              ← alternate from Open Library
```

This is the user’s preferred structure: one folder per work, all formats together. Codex also accommodates other structures (separate ebook/audiobook trees, flat directories, Calibre layout) by treating each directory containing media files as a potential work, grouping files by proximity and metadata similarity.

### Collections (Virtual Libraries)

“Libraries” in Codex are **virtual curated views**, not separate physical storage locations. The user maintains one unified media collection on disk. Collections are filtered, curated subsets created for specific purposes:

- “Daughter’s Kobo” — a collection of age-appropriate ebooks synced to a specific device
- “Currently Reading” — a dynamic collection based on in-progress status
- “Fantasy Shelf” — a manual curation of books by genre
- “Audiobooks for Road Trip” — a temporary collection for a specific use case

Collections can be:

- **Manual** — user adds/removes books explicitly
- **Smart** — auto-populated by filter rules (genre = “Fantasy” AND audience = “young_adult” AND rating > 3.5)
- **Device-linked** — associated with a sync device, so KoboSync serves only books in this collection to that device

The physical mount point is a **Media Root** — the top-level directory that Codex scans. A user can have multiple media roots (e.g., one on local storage, one on NAS), but this is about storage location, not curation.

### Key Relationships

```
Media Root (physical path)
  └── has many → Works (directories with media files)
       ├── has many → Files (epub, pdf, m4b, etc.)
       ├── has many → Contributors (with roles: author, narrator, editor, etc.)
       ├── has many → Series memberships (with position)
       ├── has many → Tags/Genres
       ├── has many → Identifiers (ISBN, ASIN, etc.)
       ├── has many → Ratings (per-source)
       ├── has many → Covers (per-source, one selected)
       ├── has one  → Audiobook chapter map (if audiobook files exist)
       └── has one  → Media overlay reference (if aligned EPUB3 exists)

User
  ├── has many → Collections (virtual shelves/libraries)
  │    └── has many → Works (junction with position)
  ├── has many → Progress records (one per work, tracks both ebook + audio position)
  ├── has many → Annotations (bookmarks, highlights, notes)
  └── has many → Sync devices (Kobo, KOReader, etc.)
       └── has many → Sync state records (tracks what was sent to each device)
```

-----

## Sidecar Format (`metadata.json`)

The sidecar is a JSON file living next to the media files. It is the source of truth for all metadata about a work. See the full schema specification in `schema-spec.md`.

Key design decisions:

- **`schema_version`** field enables future migration without breaking existing sidecars
- **`identifiers`** object covers every major ecosystem (ISBN, ASIN, Google Books, Open Library, Goodreads legacy, Hardcover, BookBrainz, Audible, Calibre UUID, LCCN, OCLC, LibraryThing, StoryGraph)
- **`contributors`** uses a single array with role fields instead of separate author/narrator fields (handles edge cases like author-narrated audiobooks, translators, illustrators)
- **`series`** is an array (a book can belong to both “Stormlight Archive” and “Cosmere”); position is a float for novellas between numbered entries
- **`ratings`** are per-source, never averaged (Google Books: 4.0, Open Library: 4.28 — both preserved with count and fetch timestamp)
- **`covers`** stores every source with a `selected` field; user can swap covers without re-fetching
- **`metadata.locked_fields`** prevents auto-refresh from overwriting manual corrections
- **`metadata.match_confidence`** and `needs_review` flag the human-in-the-loop workflow

-----

## SQLite Database Schema

The database indexes sidecar data for fast querying and stores user state. See the full schema in `schema-spec.md`.

Key design decisions:

- **UUIDs as TEXT primary keys** — globally unique, no insertion-order dependency, safe for offline sync
- **FTS5 virtual table** for full-text search across titles, contributors, series, tags, descriptions
- **`book_series.position` is REAL** — supports novellas at position 1.5 between books 1 and 2
- **Single `user_progress` row per user-per-work** — tracks both ebook (CFI) and audio position
- **`metadata_tasks` table** — work queue for the metadata engine (pending → running → completed/review/failed)
- **`source_cache` table** — optional retention of raw API responses (configurable, default 90 days)
- **Database can be rebuilt from sidecars** — loses user state only (mitigated by nightly backups and JSON export)

### Schema Change: Libraries → Media Roots + Collections

The original schema had a `libraries` table with type constraints. This is now revised:

```sql
-- Physical storage locations that Codex scans
CREATE TABLE media_roots (
    id          TEXT    PRIMARY KEY,
    name        TEXT    NOT NULL,
    root_path   TEXT    NOT NULL UNIQUE,
    scan_config TEXT    NOT NULL DEFAULT '{}',
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Virtual curated views (replaces the old "libraries" concept)
CREATE TABLE collections (
    id              TEXT    PRIMARY KEY,
    user_id         TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT    NOT NULL,
    description     TEXT,
    collection_type TEXT    NOT NULL DEFAULT 'manual'
                    CHECK (collection_type IN ('manual', 'smart', 'device')),
    -- For smart collections: JSON filter rules
    -- Example: {"genres": ["Fantasy"], "audience": "young_adult", "min_rating": 3.5}
    smart_filter    TEXT,
    -- For device collections: linked sync device
    device_id       TEXT    REFERENCES sync_devices(id) ON DELETE SET NULL,
    is_public       INTEGER NOT NULL DEFAULT 0,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, name)
);

CREATE TABLE collection_works (
    collection_id   TEXT    NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    work_id         TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL DEFAULT 0,
    added_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (collection_id, work_id)
);

-- The "books" table is renamed to "works" to reflect the unified model
-- A work is an intellectual creation; files are physical manifestations
-- The rest of the schema remains the same, with "book_id" → "work_id" throughout
```

-----

## Metadata Engine

### Source Priority (at launch)

1. **Google Books API** — free, reliable, decent metadata and covers. Primary source for ISBNs.
1. **Open Library API** — free, community-driven, good coverage for popular titles. Excellent for work/edition relationships.
1. **Audnexus API** — community API for Audible metadata (narrators, chapters, duration, series). Critical for audiobook enrichment.

### Future Sources (post-launch)

1. **Hardcover API** — GraphQL, community-driven, growing catalog. The Goodreads successor.
1. **BookBrainz** — MusicBrainz sister project for books. Small but architecturally solid.

### Sources NOT Supported (with reasoning)

- **Goodreads** — API killed December 2020. Scraping is TOS-violating and constantly breaks. Dead end.
- **Amazon** — No public metadata API. Product Advertising API requires affiliate revenue. Scraping is cat-and-mouse.

### Matching Strategy

1. **ISBN match** (highest confidence): If the file or folder name contains an ISBN, query Google Books and Open Library by ISBN. Confidence: 0.95+.
1. **ASIN match** (high confidence): If an ASIN is found (common for audiobooks), query Audnexus. Confidence: 0.90+.
1. **Title + Author match** (medium confidence): Parse title and author from folder structure and/or embedded metadata. Query sources with both fields. Apply fuzzy matching (Levenshtein distance, normalized scoring). Confidence: 0.60-0.90 depending on match quality.
1. **Title-only match** (low confidence): Fallback when author is unknown. Multiple candidates likely. Confidence: 0.30-0.60. Always goes to review queue.

### Confidence Thresholds

- **≥ 0.85**: Auto-apply best match. Write sidecar. Mark `needs_review = false`.
- **0.50 - 0.84**: Auto-apply best match as provisional. Mark `needs_review = true`. User sees it in review queue.
- **< 0.50**: Do not auto-apply. Store candidates in metadata_tasks. Mark `needs_review = true`. User must select.

### Merge Strategy

When multiple sources return data for the same work:

1. Use **source priority order** (Google Books → Open Library → Audnexus) for each field.
1. Take the **first non-null, non-empty value** for each field.
1. **Exception: covers** — store all source covers, select the highest resolution by default.
1. **Exception: ratings** — store all per-source ratings, never merge.
1. **Exception: identifiers** — merge all (take the union across sources).
1. **Exception: contributors** — prefer the source with the most complete contributor list (e.g., Audnexus has narrators that Google Books lacks).
1. User can override any field; overridden fields are added to `locked_fields` to prevent future refresh from reverting them.

### Folder Structure Parsing

Codex does not mandate a folder structure but recognizes common patterns:

```
# Pattern 1: Author / Series / Position - Title (user's preferred)
/media/Brandon Sanderson/Stormlight Archive/01 - The Way of Kings/

# Pattern 2: Author / Title
/media/Brandon Sanderson/The Way of Kings/

# Pattern 3: Flat (everything in one directory)
/media/The Way of Kings/

# Pattern 4: Calibre style
/media/Brandon Sanderson/The Way of Kings (1234)/

# Pattern 5: Audiobookshelf style
/media/Brandon Sanderson/The Stormlight Archive/The Way of Kings/

# Pattern 6: Chaptarr style with year
/media/Brandon Sanderson/The Way of Kings (2010)/
```

The scanner uses a priority system:

1. If a `metadata.json` sidecar exists, use it (already cataloged).
1. Extract embedded metadata from files (OPF in EPUBs, ID3/M4B chapters in audiobooks).
1. Parse folder structure for hints (author, series, position, title, year).
1. Filename parsing as final fallback.

All extracted hints are passed to the metadata engine as the starting point for external lookups.

-----

## Protocol Adapters

Each protocol is implemented as an independent Go package that reads from the shared SQLite database and exposes HTTP endpoints. Adapters can be enabled/disabled in configuration.

### OPDS 1.2

Standard Atom XML feeds for ebook catalog browsing. Consumed by apps like KyBook, Panels, Cantook, Moon+ Reader, FBReader.

```
GET /opds/v1.2/catalog                    → root navigation feed (lists collections + browse options)
GET /opds/v1.2/recent                     → recently added works
GET /opds/v1.2/collection/{id}            → works in a collection
GET /opds/v1.2/author/{id}                → works by contributor
GET /opds/v1.2/series/{id}                → works in series (ordered by position)
GET /opds/v1.2/search?q={query}           → FTS5 search
GET /opds/v1.2/work/{id}                  → single work with acquisition links per format
GET /opds/v1.2/work/{id}/file/{file_id}   → file download
GET /opds/v1.2/work/{id}/cover            → cover image
```

### KoboSync

REST API mimicking Kobo’s cloud server. Allows Kobo e-readers to sync with Codex instead of Kobo’s servers. Protocol is well-documented via Calibre-Web and KOReader reverse engineering.

Key endpoints:

```
GET  /kobo/v1/initialization
GET  /kobo/v1/library/sync
PUT  /kobo/v1/library/{book_id}/state
GET  /kobo/v1/library/{book_id}/content
POST /kobo/v1/library/{book_id}/bookmark
```

A device-linked collection controls which works are visible to each Kobo device.

### KOReader Sync

Simple REST API for syncing reading progress and highlights with KOReader devices.

```
PUT    /koreader/syncs/progress
GET    /koreader/syncs/progress/{document}
PUT    /koreader/syncs/activity
```

### Audiobook Streaming

REST API serving audiobook content and metadata for streaming playback. Designed for compatibility with Audiobookshelf mobile apps where possible.

```
GET /api/v1/works/{id}/audiobook                → audiobook manifest (chapters, duration, cover)
GET /api/v1/works/{id}/audiobook/chapters       → chapter list
GET /api/v1/works/{id}/audiobook/stream         → audio stream (supports range requests)
GET /api/v1/works/{id}/audiobook/stream/{chapter} → stream specific chapter
PUT /api/v1/works/{id}/audiobook/progress       → update listening position
```

### EPUB3 Media Overlay Serving

For pre-aligned books (created by Storyteller or similar tools), Codex serves the EPUB3 with media overlays via OPDS. The EPUB file already contains the SMIL timing data and audio references. Client apps that support EPUB Media Overlays (Storyteller mobile app, Thorium Reader, Apple Books) handle the synchronized playback. Codex’s role is purely serving the file and tracking progress.

-----

## Admin Web UI (Svelte)

The Svelte SPA is the control panel for library management. It is NOT a reading/listening interface.

### Core Views

1. **Dashboard** — scan status, recent additions, items needing review, collection overview
1. **Browse** — filterable/searchable grid or list of all works (by title, author, series, genre, format, rating)
1. **Work Detail** — full metadata display for a single work with:
- All metadata fields, editable inline
- Side-by-side comparison of metadata from different sources
- Cover picker (thumbnail grid of all available covers)
- File list with format/size/duration
- Chapter list (for audiobooks)
- Rating breakdown by source
- Identifier cross-references
- Collection membership
1. **Review Queue** — works needing human attention (low-confidence matches, no match found, multiple candidates)
- Shows match candidates with confidence scores
- One-click accept, or manual search for better match
1. **Collections Manager** — create/edit manual and smart collections, link to sync devices
1. **Users & Devices** — user management, OIDC configuration, registered sync devices
1. **Settings** — media roots, scan scheduling, metadata source configuration, backup/restore

### UI Principles

- Functional over beautiful. Tables, forms, and grids. Not a showcase.
- Every metadata field is editable. Edited fields are locked from auto-refresh.
- Batch operations: select multiple works, apply genre/tag/collection, trigger metadata refresh.
- Responsive but not mobile-optimized (admin tasks are done on desktop).

-----

## Go Project Layout

```
codex/
├── cmd/
│   └── codex/
│       └── main.go                    ← entry point, CLI flags, startup
│
├── internal/
│   ├── config/
│   │   └── config.go                  ← YAML config loading, defaults
│   │
│   ├── database/
│   │   ├── database.go                ← SQLite connection, WAL setup, migrations
│   │   ├── migrations/                ← numbered migration files (Go code, not SQL files)
│   │   └── queries/                   ← query functions organized by domain
│   │       ├── works.go
│   │       ├── contributors.go
│   │       ├── collections.go
│   │       ├── users.go
│   │       ├── progress.go
│   │       └── tasks.go
│   │
│   ├── scanner/
│   │   ├── scanner.go                 ← directory walking, file detection
│   │   ├── parser.go                  ← folder structure pattern recognition
│   │   ├── epub.go                    ← EPUB metadata extraction (OPF parsing)
│   │   ├── audiobook.go               ← M4B/MP3 metadata + chapter extraction
│   │   ├── comic.go                   ← CBZ/CBR metadata extraction
│   │   └── sidecar.go                 ← metadata.json read/write
│   │
│   ├── metadata/
│   │   ├── engine.go                  ← orchestrator: queue processing, merge strategy
│   │   ├── matcher.go                 ← fuzzy matching, confidence scoring
│   │   ├── sources/
│   │   │   ├── source.go              ← interface definition
│   │   │   ├── googlebooks.go         ← Google Books API client
│   │   │   ├── openlibrary.go         ← Open Library API client
│   │   │   └── audnexus.go            ← Audnexus API client
│   │   └── sidecar_writer.go          ← merge results → write metadata.json + covers
│   │
│   ├── auth/
│   │   ├── auth.go                    ← middleware, session management
│   │   ├── oidc.go                    ← OIDC provider integration
│   │   └── local.go                   ← local username/password auth
│   │
│   ├── api/
│   │   ├── router.go                  ← HTTP router setup, middleware chain
│   │   ├── works.go                   ← REST endpoints for works CRUD
│   │   ├── collections.go             ← REST endpoints for collections
│   │   ├── users.go                   ← REST endpoints for user management
│   │   ├── metadata.go                ← REST endpoints for metadata operations
│   │   ├── scan.go                    ← REST endpoints for scan triggers
│   │   └── settings.go               ← REST endpoints for app configuration
│   │
│   ├── protocols/
│   │   ├── opds/
│   │   │   ├── handler.go             ← OPDS feed generation
│   │   │   ├── feed.go                ← Atom XML construction
│   │   │   └── search.go              ← OPDS search descriptor
│   │   ├── kobosync/
│   │   │   ├── handler.go             ← KoboSync API endpoints
│   │   │   └── sync.go                ← Sync state management
│   │   ├── koreader/
│   │   │   └── handler.go             ← KOReader sync endpoints
│   │   └── audiostream/
│   │       ├── handler.go             ← Audiobook streaming endpoints
│   │       └── manifest.go            ← Chapter manifest generation
│   │
│   └── server/
│       └── server.go                  ← HTTP server setup, graceful shutdown
│
├── web/                               ← Svelte SPA source
│   ├── src/
│   │   ├── routes/                    ← SvelteKit routes
│   │   ├── lib/
│   │   │   ├── api/                   ← API client functions
│   │   │   └── components/            ← Reusable Svelte components
│   │   └── app.html
│   ├── static/
│   ├── svelte.config.js
│   ├── package.json
│   └── vite.config.js
│
├── embedded/
│   └── web.go                         ← go:embed directive for built Svelte assets
│
├── Dockerfile
├── docker-compose.yml                 ← example deployment
├── config.example.yaml
├── CLAUDE.md                          ← this document (or a condensed version for Claude Code)
├── go.mod
└── go.sum
```

### Package Dependency Rules

- `cmd/codex` depends on `internal/server`, `internal/config`, `internal/database`
- `internal/server` wires together `internal/api`, `internal/protocols/*`, `internal/auth`, `embedded`
- `internal/api` depends on `internal/database/queries`, `internal/scanner`, `internal/metadata`
- `internal/protocols/*` each depend only on `internal/database/queries` (read from shared DB)
- `internal/scanner` depends on `internal/database/queries` (write scan results)
- `internal/metadata` depends on `internal/database/queries`, `internal/metadata/sources` (read tasks, write results)
- **No package imports from `internal/api` or `internal/protocols`** — these are leaf packages
- **`internal/metadata/sources` has no internal dependencies** — pure API clients returning structured data

This dependency structure means protocol adapters, metadata sources, and the scanner can all be developed and tested independently.

-----

## Build & Deployment

### Build Pipeline

```bash
# 1. Build Svelte SPA
cd web && npm run build   # outputs to web/build/

# 2. Build Go binary (embeds Svelte output)
cd .. && go build -o codex ./cmd/codex

# Result: single binary "codex" containing everything
```

### Dockerfile

```dockerfile
# Stage 1: Build Svelte
FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go
FROM golang:1.22-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/build ./web/build
RUN CGO_ENABLED=0 go build -o codex ./cmd/codex

# Stage 3: Runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend /app/codex /usr/local/bin/codex
EXPOSE 6060
VOLUME ["/config", "/media", "/cache"]
ENTRYPOINT ["codex"]
```

### Docker Compose (example deployment)

```yaml
services:
  codex:
    image: codex:latest
    container_name: codex
    environment:
      - TZ=America/Denver
      - CODEX_SECRET_KEY=${SECRET_KEY}  # for session signing
    ports:
      - "6060:6060"
    volumes:
      - ./config:/config               # database + config.yaml
      - /mnt/user/media/books:/media:rw # unified media library
      - ./cache:/cache                  # thumbnails, temp files
    restart: unless-stopped
```

### Configuration (`config.yaml`)

```yaml
server:
  port: 6060
  base_url: "https://books.example.com"  # for generating external links

database:
  path: "/config/library.db"
  wal_mode: true
  busy_timeout_ms: 5000

media_roots:
  - name: "Main Library"
    path: "/media"

scanner:
  watch: true                    # filesystem watcher (inotify)
  scan_interval_hours: 24        # full rescan interval
  ignore_patterns:
    - "*.tmp"
    - ".DS_Store"
    - "@eaDir"                   # Synology thumbnail dirs

metadata:
  auto_match_threshold: 0.85     # above this: auto-apply
  review_threshold: 0.50         # below this: don't auto-apply at all
  source_cache_days: 90          # how long to keep raw API responses
  sources:
    google_books:
      enabled: true
    open_library:
      enabled: true
    audnexus:
      enabled: true

auth:
  local_enabled: true
  oidc:
    enabled: false
    issuer: ""
    client_id: ""
    client_secret: ""
    redirect_url: ""

protocols:
  opds:
    enabled: true
    path_prefix: "/opds"
  kobosync:
    enabled: true
    path_prefix: "/kobo"
  koreader:
    enabled: true
    path_prefix: "/koreader"
  audiostream:
    enabled: true
    path_prefix: "/api/v1"

backup:
  enabled: true
  schedule: "0 3 * * *"         # 3 AM daily
  keep_count: 7                  # keep last 7 backups
  path: "/config/backups"
```

-----

## Development Roadmap

### Phase 1: Foundation (Weeks 1-3)

**Goal:** Go project scaffolding, SQLite database, file scanner, sidecar read/write. At the end of this phase, the app can scan a directory tree, detect media files, and create work records in the database.

- [ ] Go project structure with module layout above
- [ ] SQLite database initialization with full schema and migration system
- [ ] Configuration loading (YAML + env var overrides)
- [ ] File scanner: walk directory tree, detect media files by extension
- [ ] Folder structure parser: extract author/series/title/position hints from common patterns
- [ ] EPUB metadata extractor: parse OPF for title, author, ISBN, description, cover
- [ ] Audiobook metadata extractor: parse M4B chapters, ID3 tags
- [ ] Sidecar reader: load existing `metadata.json` files
- [ ] Sidecar writer: create/update `metadata.json` files
- [ ] Database indexing: insert works with all related records from sidecar data
- [ ] Basic HTTP server with health check endpoint
- [ ] Docker build pipeline

### Phase 2: Metadata Engine (Weeks 4-6)

**Goal:** External metadata fetching, fuzzy matching, confidence scoring, and the merge/enrichment pipeline. At the end of this phase, scanning a directory triggers automatic metadata enrichment.

- [ ] Metadata source interface definition
- [ ] Google Books API client
- [ ] Open Library API client
- [ ] Audnexus API client
- [ ] Fuzzy matching: Levenshtein distance, normalized scoring, title/author comparison
- [ ] Confidence scoring with configurable thresholds
- [ ] Merge strategy: source priority, field-level merge, cover collection
- [ ] Metadata task queue: pending → running → completed/review/failed
- [ ] Sidecar writing with merged results
- [ ] Cover download and storage (multiple sources per work)
- [ ] Locked fields support (user edits survive refresh)
- [ ] Source cache with configurable retention

### Phase 3: REST API + Admin UI (Weeks 7-10)

**Goal:** Complete REST API and Svelte admin interface. At the end of this phase, you can browse your library, review metadata matches, edit metadata, and manage collections through a web browser.

- [ ] REST API: works CRUD (list, get, update, search)
- [ ] REST API: full-text search via FTS5
- [ ] REST API: contributors, series, tags endpoints
- [ ] REST API: collections CRUD (manual + smart filters)
- [ ] REST API: metadata operations (trigger refresh, review candidates, apply match)
- [ ] REST API: scan triggers (manual scan, scan status)
- [ ] REST API: cover management (list sources, select cover)
- [ ] REST API: settings management
- [ ] Svelte SPA: project setup with SvelteKit static adapter
- [ ] Svelte SPA: dashboard view
- [ ] Svelte SPA: browse view (filterable grid/list)
- [ ] Svelte SPA: work detail view with inline editing
- [ ] Svelte SPA: metadata review queue
- [ ] Svelte SPA: collection manager
- [ ] Svelte SPA: settings page
- [ ] Go embed integration (bake Svelte into binary)

### Phase 4: Auth & Users (Weeks 11-12)

**Goal:** Multi-user support with local accounts and OIDC. At the end of this phase, multiple users can log in, each with their own progress and collections.

- [ ] Local auth: username/password with bcrypt hashing
- [ ] Session management with secure tokens
- [ ] OIDC integration via `coreos/go-oidc`
- [ ] User roles: admin, user, guest
- [ ] Per-user progress tracking
- [ ] Per-user collections/shelves
- [ ] Per-user annotations (bookmarks, highlights, notes)
- [ ] Svelte SPA: login flow, user management (admin)

### Phase 5: OPDS (Weeks 13-14)

**Goal:** OPDS 1.2 catalog feed. At the end of this phase, any OPDS reader can browse and download books from Codex.

- [ ] OPDS Atom XML feed generation
- [ ] Navigation feeds: root catalog, by author, by series, by collection
- [ ] Acquisition feeds: work entries with download links per format
- [ ] OPDS search (OpenSearch descriptor + FTS5 backend)
- [ ] Cover image serving (thumbnails for feed, full-res on request)
- [ ] Authentication for OPDS (HTTP Basic over HTTPS, compatible with most OPDS clients)
- [ ] Pagination for large libraries

### Phase 6: KoboSync (Weeks 15-17)

**Goal:** Kobo e-readers can sync with Codex. At the end of this phase, a Kobo pointed at Codex can browse, download, and sync reading progress.

- [ ] Study Calibre-Web and KOReader KoboSync implementations
- [ ] KoboSync API endpoints (initialization, library sync, state, content, bookmarks)
- [ ] Device-linked collections (control which works a specific Kobo sees)
- [ ] Reading progress sync (Kobo → Codex and Codex → Kobo)
- [ ] Bookmark/highlight sync

### Phase 7: KOReader Sync (Week 18)

**Goal:** KOReader devices sync progress and highlights with Codex.

- [ ] KOReader sync API endpoints (progress, activity)
- [ ] Position mapping (KOReader’s document identifiers → Codex work IDs)
- [ ] Bidirectional progress sync

### Phase 8: Audiobook Streaming (Weeks 19-21)

**Goal:** Audiobook streaming API for mobile app consumption. At the end of this phase, an Audiobookshelf-compatible app (or similar) can stream audiobooks from Codex.

- [ ] Audiobook manifest endpoint (JSON: chapters, duration, cover URL)
- [ ] Audio streaming with HTTP range request support
- [ ] Per-chapter streaming
- [ ] Listening progress sync (update/retrieve position)
- [ ] Transcode on-the-fly (optional, if source format isn’t broadly compatible)

### Phase 9: Polish & Operations (Weeks 22-24)

**Goal:** Production hardening, backup/restore, monitoring.

- [ ] Automatic nightly SQLite backup (compressed, configurable retention)
- [ ] User data export/import (progress, collections, annotations as JSON)
- [ ] Filesystem watcher for real-time scan (inotify via `fsnotify`)
- [ ] Graceful shutdown with in-flight request completion
- [ ] Structured logging (JSON logs, configurable level)
- [ ] Metrics endpoint (Prometheus-compatible, optional)
- [ ] Rate limiting on metadata API calls (respect source API limits)
- [ ] Thumbnail generation and caching for cover images
- [ ] Database integrity checks on startup
- [ ] Documentation: user guide, API reference, deployment guide

-----

## Security Architecture

### Threat Model

Codex is a self-hosted server, typically behind a reverse proxy and potentially behind SSO (Authentik/Authelia). It is not a public SaaS product. The primary threats are: unauthorized access from within the local network, compromised user accounts, and automated scanners hitting exposed ports. The goal is not “unhackable” but “no obvious holes and limited blast radius.”

### Vulnerability Priority (by likelihood and impact)

1. **Path traversal** — Codex serves files from disk. Unsanitized path parameters could expose `../../etc/passwd` or the SQLite database. This is the most common vulnerability in file-serving apps and the one AI-generated code is most likely to introduce.
1. **SQL injection** — User input concatenated into SQL strings instead of parameterized queries.
1. **Authentication bypass** — Predictable session tokens, incomplete OIDC validation, or endpoints missing auth middleware.
1. **Cross-site scripting (XSS)** — Malicious metadata (book titles, descriptions) rendered without sanitization in the Svelte UI.
1. **Dependency supply chain** — Compromised Go module or npm package.

### Architectural Mitigations (built into the design)

**Non-root container.** The Dockerfile creates a dedicated `codex` user. The entrypoint runs as this user. A compromised process cannot escalate to root.

```dockerfile
RUN addgroup -S codex && adduser -S codex -G codex
USER codex
```

**Centralized path validation.** A single `SafePath()` function in `internal/security/safepath.go` is the ONLY way to resolve a user-influenced file path. It resolves symlinks, normalizes the path, strips null bytes and path separators, and confirms the result falls within a permitted media root or cache directory. Every file I/O operation must call this function. There are no exceptions.

**Parameterized queries only.** All database access goes through `internal/database/queries/` which uses `?` placeholders exclusively. No SQL string concatenation anywhere in the codebase.

**Middleware-based authentication.** The HTTP router applies auth middleware to entire route groups. Individual handlers never check auth — if they execute, auth has already passed. Public endpoints (health check, OPDS catalog root if configured as public) are explicitly registered outside the auth group.

**CORS locked to origin.** The Access-Control-Allow-Origin header is set to the configured `base_url`, never `*`.

**Session tokens from crypto/rand.** Session IDs are generated with `crypto/rand`, never `math/rand`. Tokens are 256-bit minimum.

**Bcrypt for passwords.** Local account passwords are hashed with `golang.org/x/crypto/bcrypt` at cost factor 12 minimum.

**Rate limiting on auth endpoints.** Login, token refresh, and account creation endpoints are rate-limited per IP to prevent brute force.

**No secrets in code or logs.** Secrets are loaded from environment variables or config file (0600 permissions). The structured logger has a sanitizer that redacts any field named `password`, `secret`, `token`, or `key`.

**Svelte XSS protection.** Svelte escapes all interpolated values by default. The `{@html}` directive is used ONLY for book descriptions and ONLY after sanitization through DOMPurify. A `CLAUDE.md` rule enforces this.

### Security Tooling Pipeline

These tools run as part of the development workflow. They require no security expertise — they produce actionable reports.

|Tool           |What it catches                                                                                                                  |When to run                                       |Install                                                   |
|---------------|---------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------|----------------------------------------------------------|
|**gosec**      |Go-specific security issues: hardcoded credentials, weak crypto, SQL injection patterns, unvalidated paths, insecure HTTP configs|After every Claude Code session, before committing|`go install github.com/securego/gosec/v2/cmd/gosec@latest`|
|**govulncheck**|Known CVEs in Go dependencies                                                                                                    |After adding/updating dependencies, weekly        |`go install golang.org/x/vuln/cmd/govulncheck@latest`     |
|**npm audit**  |Known CVEs in npm dependencies (Svelte frontend)                                                                                 |After `npm install`, before building frontend     |Built into npm                                            |
|**trivy**      |OS package vulns, language dependency vulns, Dockerfile misconfigs in the built container image                                  |After building Docker image, before deploying     |`docker run aquasec/trivy image codex:latest`             |
|**staticcheck**|General Go code quality, correctness issues, some security-adjacent bugs                                                         |After every Claude Code session                   |`go install honnef.co/go/tools/cmd/staticcheck@latest`    |

**Mandatory workflow after any Claude Code implementation session:**

```bash
# 1. Go security scan
gosec ./...

# 2. Dependency vulnerability check
govulncheck ./...

# 3. General code quality
staticcheck ./...

# 4. Frontend dependency check (if frontend changed)
cd web && npm audit && cd ..

# 5. Container scan (if Docker image rebuilt)
docker build -t codex:latest .
docker run --rm aquasec/trivy image codex:latest

# Address all HIGH and MEDIUM findings before committing.
# LOW findings should be reviewed but may be acceptable with justification.
```

### Security Review Checkpoints

At certain project milestones, perform a focused security review by creating a dedicated Claude conversation with the prompt: “You are a security auditor reviewing a self-hosted Go web application. Review the following code for vulnerabilities, with particular attention to path traversal, SQL injection, authentication bypass, and XSS.” Then paste in the relevant source files.

|Checkpoint   |Files to review                                                 |When                             |
|-------------|----------------------------------------------------------------|---------------------------------|
|After Phase 1|`safepath.go`, scanner file I/O, sidecar read/write             |Before proceeding to Phase 2     |
|After Phase 3|All REST API handlers, router middleware chain                  |Before adding auth in Phase 4    |
|After Phase 4|Auth middleware, session management, OIDC flow, password hashing|Before exposing outside localhost|
|After Phase 6|KoboSync handlers (device auth is different from user auth)     |Before connecting real devices   |
|After Phase 8|Audio streaming handlers (range requests, file serving)         |Before production use            |

-----

## Code Reuse Strategy

### Principle: Borrow knowledge, build implementations

Direct code copy from existing projects is impractical (different languages) and undesirable (different architectural assumptions). What IS valuable from existing projects is protocol knowledge, API contracts, and battle-tested patterns.

### What to study from each project

|Project                               |What to extract                                                                              |How to use it                                                                            |
|--------------------------------------|---------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|
|**Calibre-Web** (`kobo.py`)           |KoboSync protocol: exact endpoints, request/response shapes, device handshake flow           |Read the code to understand the protocol contract, then implement fresh in Go            |
|**KOReader** (sync wiki + source)     |KOReader sync API: progress endpoints, document identification                               |Read their API docs and sync code, implement the same contract                           |
|**Audiobookshelf** (API wiki + source)|Audiobook streaming API: chapter manifest format, streaming endpoints, progress sync         |Study their API to understand the contract; consider compatibility with their mobile apps|
|**Storyteller** (EPUB3 output format) |EPUB3 Media Overlay structure: how SMIL files reference audio, how aligned EPUBs are packaged|Understand the file format so Codex can correctly parse and serve aligned EPUBs          |
|**Calibre** (`metadata.opf` format)   |OPF metadata format for Calibre interop                                                      |Support reading/writing OPF sidecars as an import/export option                          |
|**OPDS 1.2 Specification**            |Feed format, acquisition links, search, pagination                                           |Implement directly from the published spec (no need to reference other implementations)  |

### Go dependencies to use (not reinvent)

|Need              |Recommended package                                         |Why                                                                               |
|------------------|------------------------------------------------------------|----------------------------------------------------------------------------------|
|SQLite driver     |`modernc.org/sqlite` or `mattn/go-sqlite3`                  |Battle-tested, actively maintained                                                |
|HTTP router       |`go-chi/chi` or standard `net/http` (Go 1.22+)              |Chi adds minimal overhead; std lib is sufficient with Go 1.22 route patterns      |
|OIDC              |`coreos/go-oidc/v3`                                         |Maintained by the team that built the OIDC spec                                   |
|Password hashing  |`golang.org/x/crypto/bcrypt`                                |Standard Go extended library                                                      |
|UUID generation   |`github.com/google/uuid`                                    |Canonical Go UUID library                                                         |
|EPUB parsing      |`github.com/taylorskalyo/goreader` or custom OPF XML parsing|Evaluate at implementation time; OPF is simple enough to parse with `encoding/xml`|
|Audio metadata    |`github.com/dhowden/tag`                                    |Reads ID3, MP4 (M4B), FLAC, OGG tags                                              |
|YAML config       |`gopkg.in/yaml.v3`                                          |Standard Go YAML library                                                          |
|File watching     |`github.com/fsnotify/fsnotify`                              |Cross-platform filesystem notifications                                           |
|Structured logging|`log/slog` (Go 1.21+ standard library)                      |Built-in, zero dependencies                                                       |
|Fuzzy matching    |`github.com/agnivade/levenshtein` or custom                 |Evaluate at implementation time                                                   |
|Image resizing    |`golang.org/x/image` or `github.com/disintegration/imaging` |For cover thumbnail generation                                                    |
|CORS middleware   |`github.com/go-chi/cors` (if using chi)                     |Simple, configurable                                                              |

### What to build from scratch

|Component                                                    |Why not reuse                                                                 |
|-------------------------------------------------------------|------------------------------------------------------------------------------|
|Metadata engine (merge strategy, confidence scoring)         |Core differentiator; no existing implementation matches our design            |
|Scanner (folder parsing, work detection)                     |Our flexible multi-pattern recognition is unique                              |
|Sidecar format (metadata.json read/write)                    |Our schema is custom                                                          |
|Collections system (manual, smart, device-linked)            |Novel concept not present in other projects                                   |
|Admin UI (Svelte)                                            |Tailored to our metadata review workflow                                      |
|All protocol adapters (OPDS, KoboSync, KOReader, audiostream)|Must be Go; protocol logic is well-documented and straightforward to implement|

-----

## Design Decisions Log

This section records key decisions and their rationale, so future development doesn’t revisit settled questions.

|# |Decision                                                          |Rationale                                                                                                                                                         |Date      |
|--|------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|
|1 |Go as primary language                                            |Single binary, minimal RAM, built-in HTTP/concurrency, best AI-assisted dev experience for non-developers                                                         |2026-03-12|
|2 |SQLite (WAL + FTS5) as database                                   |Zero ops overhead, single-file backup, validated by Audiobookshelf and Storyteller. FTS5 eliminates need for external search.                                     |2026-03-12|
|3 |Svelte for admin UI (embedded in Go binary)                       |Reactive UI for metadata review; static adapter compiles to files embedded via `go:embed`; single binary preserved                                                |2026-03-12|
|4 |Sidecar JSON as metadata source of truth                          |Portable, human-readable, survives database rebuild. Database is a queryable cache.                                                                               |2026-03-12|
|5 |Virtual collections instead of physical library separation        |User wants one unified media tree; “libraries” are curated views for device sync and organization                                                                 |2026-03-12|
|6 |Multi-format works in single directory                            |All formats of a book (epub, pdf, m4b, aligned epub3) live together. The “work” is the central entity.                                                            |2026-03-12|
|7 |No file reorganization                                            |Codex never moves/renames files. Reads whatever structure exists. Compatible with arr stack and manual organization.                                              |2026-03-12|
|8 |Per-source ratings, never averaged                                |Different communities rate differently. Preserve original data, let the UI present it.                                                                            |2026-03-12|
|9 |Contributors with roles instead of separate author/narrator fields|Handles author-narrated audiobooks, translators, illustrators, editors cleanly. Uses MARC relator codes.                                                          |2026-03-12|
|10|Series position as REAL (float)                                   |Supports novellas between numbered entries (e.g., 1.5 for a story between books 1 and 2)                                                                          |2026-03-12|
|11|Serve but don’t create EPUB3 media overlays                       |Eliminates Python/ffmpeg dependencies, removes heaviest computational workload. Users create alignments with Storyteller or similar tools.                        |2026-03-12|
|12|Goodreads and Amazon not supported as metadata sources            |Goodreads API dead since 2020. Amazon has no public metadata API. Both require scraping which is fragile and TOS-violating.                                       |2026-03-12|
|13|Metadata task queue with human-in-the-loop                        |High-confidence matches auto-apply; low-confidence goes to review queue. Avoids silent wrong merges.                                                              |2026-03-12|
|14|Locked fields prevent auto-refresh overwriting user edits         |When user manually corrects metadata, that field is protected from future automated updates.                                                                      |2026-03-12|
|15|Centralized SafePath() for all file I/O                           |Single function validates all user-influenced paths against allowed roots. Eliminates path traversal class entirely.                                              |2026-03-12|
|16|Middleware-based auth, not per-handler                            |Auth checks in router middleware, not individual handlers. Eliminates “forgot to check auth” class of bugs.                                                       |2026-03-12|
|17|Non-root container with dedicated user                            |Limits blast radius of any compromise. Container runs as `codex` user.                                                                                            |2026-03-12|
|18|Automated security scanning in dev workflow                       |gosec, govulncheck, staticcheck, npm audit, trivy run after every implementation session. No security expertise required.                                         |2026-03-12|
|19|Protocol knowledge reuse, not code reuse                          |Study KoboSync/KOReader/OPDS/ABS protocols from existing projects, implement fresh in Go. Use established Go libraries for foundation (crypto, parsing, database).|2026-03-12|

-----

## Glossary

|Term                 |Definition                                                                                                                                                                       |
|---------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|**Work**             |A single intellectual creation (a book, audiobook, etc.) that may exist in multiple file formats. The central entity in Codex. Equivalent to a “book record” but format-agnostic.|
|**Media Root**       |A physical directory path mounted into the container that Codex scans for media files. A user might have one (/media) or several (/local-ssd, /nas-books).                       |
|**Collection**       |A virtual curated view — a subset of works organized for a purpose. Can be manual, smart (auto-filtered), or device-linked. Replaces the traditional “library” concept.          |
|**Sidecar**          |A `metadata.json` file (and associated cover images) stored next to media files. The source of truth for a work’s metadata.                                                      |
|**Contributor**      |A person associated with a work in any role (author, narrator, editor, translator, illustrator).                                                                                 |
|**Confidence Score** |A 0.0-1.0 value indicating how certain the metadata engine is that a match is correct. Drives auto-apply vs. review queue routing.                                               |
|**Locked Field**     |A metadata field that the user has manually edited. Protected from being overwritten by automated metadata refresh.                                                              |
|**Smart Collection** |A collection whose membership is defined by filter rules (genre, audience, rating, etc.) and automatically updates as the library changes.                                       |
|**Device Collection**|A collection linked to a specific sync device (e.g., a Kobo). Controls which works are visible to that device during sync.                                                       |
|**Media Overlay**    |EPUB3 specification for synchronized text and audio playback (SMIL files). Created by external tools like Storyteller, served by Codex.                                          |
|**CFI**              |EPUB Canonical Fragment Identifier. A string that uniquely identifies a position within an EPUB document. Used for reading progress sync.                                        |
