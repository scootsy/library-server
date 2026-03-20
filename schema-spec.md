# Codex Schema & Sidecar Specification

## Design Principles

1. **Sidecar files are the metadata source of truth.** The database is a queryable index that can be rebuilt from sidecars at any time.
1. **The database is the source of truth for user state** — reading progress, collection assignments, sync positions, and user accounts.
1. **Wide compatibility.** Identifiers map to every major ecosystem (Calibre, OPDS, Audiobookshelf, Kobo, Goodreads legacy, Hardcover, Bookbrainz). The sidecar format can round-trip to OPF without data loss.
1. **Rich metadata.** Per-source ratings, multiple covers, full series/collection modeling, narrator and contributor roles, audiobook chapter maps, and media overlay references.
1. **Multi-format works.** A single “work” record can have multiple files (EPUB, PDF, M4B, EPUB3 with media overlays). The work is the intellectual creation; files are physical manifestations. All formats of a work live in a single directory.

-----

## Part 1: Sidecar Format (`metadata.json`)

The sidecar lives alongside the book’s media files in whatever directory structure exists. For a book at `/library/Brandon Sanderson/Stormlight Archive/01 - The Way of Kings/The Way of Kings.epub`, the sidecar would be at `/library/Brandon Sanderson/Stormlight Archive/01 - The Way of Kings/metadata.json`, with an optional `cover.jpg` (or `cover.png`) in the same directory.

### Complete Sidecar Schema

```json
{
  "$schema": "https://your-app.dev/schemas/metadata-v1.json",
  "schema_version": 1,

  // === IDENTITY ===
  "title": "The Way of Kings",
  "subtitle": "Book One of The Stormlight Archive",
  "sort_title": "Way of Kings, The",
  "original_title": null,
  "language": "en",
  "languages": ["en"],

  // === IDENTIFIERS ===
  // Every known identifier for cross-referencing. All optional.
  "identifiers": {
    "isbn_13": "9780765326355",
    "isbn_10": "0765326353",
    "asin": "B003P2WO5E",
    "google_books": "QaBFDwAAQBAJ",
    "open_library_work": "/works/OL15358691W",
    "open_library_edition": "/books/OL24218498M",
    "goodreads": "7235533",
    "hardcover": null,
    "bookbrainz": null,
    "audible_asin": "B003ZWFO7E",
    "audnexus": null,
    "calibre_id": null,
    "calibre_uuid": null,
    "lccn": null,
    "oclc": null,
    "doi": null,
    "librarything": null,
    "storygraph": null
  },

  // === CONTRIBUTORS ===
  // Ordered array. Roles follow MARC relator codes where possible.
  "contributors": [
    {
      "name": "Brandon Sanderson",
      "sort_name": "Sanderson, Brandon",
      "roles": ["author"],
      "identifiers": {
        "open_library": "/authors/OL1394865A",
        "goodreads": "38550",
        "hardcover": null
      }
    },
    {
      "name": "Michael Kramer",
      "sort_name": "Kramer, Michael",
      "roles": ["narrator"],
      "identifiers": {
        "audible": "B000FECNYY"
      }
    },
    {
      "name": "Kate Reading",
      "sort_name": "Reading, Kate",
      "roles": ["narrator"],
      "identifiers": {
        "audible": "B000FEC6SU"
      }
    }
  ],

  // === SERIES ===
  // A book can belong to multiple series/collections.
  "series": [
    {
      "name": "The Stormlight Archive",
      "position": 1,
      "total_in_series": null,
      "identifiers": {
        "goodreads": "49075",
        "open_library": null
      }
    },
    {
      "name": "Cosmere",
      "position": null,
      "total_in_series": null,
      "identifiers": {}
    }
  ],

  // === PUBLICATION ===
  "publisher": "Tor Books",
  "publish_date": "2010-08-31",
  "edition": "First Edition",
  "page_count": 1007,
  "word_count": null,

  // === DESCRIPTION ===
  "description": "Roshar is a world of stone and storms...",
  "description_format": "plain",

  // === CLASSIFICATION ===
  "genres": ["Fantasy", "Epic Fantasy", "High Fantasy"],
  "tags": ["magic systems", "war", "politics", "multiple POV"],
  "subjects": ["Fiction / Fantasy / Epic"],
  "audience": "adult",
  "content_rating": null,

  // === RATINGS ===
  // Per-source ratings preserved individually. Never averaged.
  "ratings": {
    "google_books": {
      "score": 4.0,
      "max": 5,
      "count": 12847,
      "fetched_at": "2026-03-12T00:00:00Z"
    },
    "open_library": {
      "score": 4.28,
      "max": 5,
      "count": 342,
      "fetched_at": "2026-03-12T00:00:00Z"
    }
  },

  // === COVERS ===
  // Multiple cover sources. "selected" marks which one is active.
  "covers": {
    "selected": "google_books",
    "sources": {
      "embedded": {
        "filename": "cover_embedded.jpg",
        "width": 600,
        "height": 900,
        "source_note": "Extracted from EPUB"
      },
      "google_books": {
        "filename": "cover.jpg",
        "width": 800,
        "height": 1200,
        "url": "https://books.google.com/books/content?id=QaBFDwAAQBAJ&printsec=frontcover&img=1&zoom=1",
        "fetched_at": "2026-03-12T00:00:00Z"
      },
      "open_library": {
        "filename": "cover_openlibrary.jpg",
        "width": 600,
        "height": 900,
        "url": "https://covers.openlibrary.org/b/isbn/9780765326355-L.jpg",
        "fetched_at": "2026-03-12T00:00:00Z"
      }
    }
  },

  // === FILES ===
  // All media files associated with this book in this directory.
  "files": [
    {
      "filename": "The Way of Kings.epub",
      "format": "epub",
      "size_bytes": 4523891,
      "checksum_sha256": "a1b2c3...",
      "has_media_overlay": false,
      "added_at": "2026-01-15T12:00:00Z"
    },
    {
      "filename": "The Way of Kings.m4b",
      "format": "m4b",
      "size_bytes": 1893456000,
      "checksum_sha256": "d4e5f6...",
      "has_media_overlay": false,
      "duration_seconds": 187522,
      "bitrate_kbps": 128,
      "codec": "aac",
      "added_at": "2026-01-15T12:00:00Z"
    }
  ],

  // === AUDIOBOOK-SPECIFIC ===
  // Only present when audiobook files exist.
  "audiobook": {
    "duration_seconds": 187522,
    "chapters": [
      {
        "title": "Prelude to the Stormlight Archive",
        "start_seconds": 0,
        "end_seconds": 1847,
        "index": 0
      },
      {
        "title": "Prologue: To Kill",
        "start_seconds": 1847,
        "end_seconds": 3294,
        "index": 1
      }
    ],
    "abridged": false
  },

  // === MEDIA OVERLAY ===
  // Only present when an EPUB3 with media overlays exists.
  // This references a pre-aligned file (created externally by Storyteller, Readaloud, etc.).
  // The file is a standard EPUB3 whose spine items each have a SMIL media overlay document
  // that synchronises text fragments with audio clips — no separate audiobook file is needed.
  "media_overlay": {
    "aligned_epub_filename": "The Way of Kings - Aligned.epub",
    "alignment_tool": "storyteller",
    "alignment_version": "0.14.0",
    "aligned_at": "2026-02-20T00:00:00Z",

    // Narrator identity for the overlay audio.  May differ from contributors[].narrator
    // when the commercial narrator and the overlay narrator are different people.
    "overlay_narrator_name": "Michael Kramer",
    "overlay_narrator_language": "en-US",   // BCP-47
    "overlay_duration_seconds": 187522,

    // SMIL details — used by reading systems to select the correct highlight class
    // and to report sync fidelity (word vs. sentence level).
    "smil_version": "3.0",
    "sync_granularity": "sentence",         // "word" | "sentence" | "paragraph"
    "active_class": "-epub-media-overlay-active",
    "playback_active_class": "-epub-media-overlay-playing"
  },

  // === ACCESSIBILITY ===
  // EPUB Accessibility 1.1 metadata required by the EU Accessibility Act
  // (EAA, Directive 2019/882) for ebooks published after 28 June 2025.
  //
  // Field names map directly to the schema.org / a11y vocabulary used in EPUB
  // package documents and surfaced by the Readium AccessibilityMetadataDisplayGuide
  // (eight display categories: WaysOfReading, Navigation, RichContent,
  // AdditionalInformation, Hazards, Conformance, Legal, AccessibilitySummary).
  //
  // For aligned readalouds the minimum recommended set is:
  //   access_modes:           ["textual", "visual", "auditory"]
  //   access_modes_sufficient: [["textual"], ["auditory", "textual"]]
  //   features:               [..., "synchronizedAudioText", "readingOrder",
  //                            "structuralNavigation", "tableOfContents"]
  "accessibility": {
    // schema:accessMode — sensory modalities required to consume the content.
    // Values: "textual", "visual", "auditory", "tactile", "colorDependent",
    //         "chartOnVisual", "chemOnVisual", "diagramOnVisual", "mathOnVisual",
    //         "musicOnVisual", "textOnVisual".
    "access_modes": ["textual", "visual", "auditory"],

    // schema:accessModeSufficient — sets of modes each sufficient for full access.
    "access_modes_sufficient": [
      ["textual"],
      ["auditory", "textual"]
    ],

    // schema:accessibilityFeature — specific features that aid accessibility.
    // Key tokens for aligned readalouds:
    //   "synchronizedAudioText"   — SMIL media overlays align audio with text
    //   "readingOrder"            — logical reading order is defined
    //   "structuralNavigation"    — headings and landmarks are present
    //   "tableOfContents"         — navigable TOC exists
    //   "alternativeText"         — images carry alt text
    //   "displayTransformability" — reflowable; font size/spacing adjustable
    //   "printPageNumbers"        — page-break markers match the print edition
    //   "SSML"                    — SSML attributes enhance TTS pronunciation
    //   "ttsMarkup"               — general TTS markup present
    //   "rubyAnnotations"         — ruby for CJK pronunciation
    //   "longDescription"         — complex images have extended descriptions
    //   "MathML"                  — math in MathML
    //   "describedMath"           — math formulas described in prose
    //   "transcript"              — text transcript for audio/video
    //   "captions"                — captions for audio/video
    "features": [
      "synchronizedAudioText",
      "readingOrder",
      "structuralNavigation",
      "tableOfContents",
      "alternativeText",
      "displayTransformability",
      "printPageNumbers"
    ],

    // schema:accessibilityHazard — known hazards in the content.
    // Values: "flashing", "motionSimulation", "sound",
    //         "noFlashingHazard", "noMotionSimulationHazard", "noSoundHazard",
    //         "none", "unknown".
    "hazards": ["noFlashingHazard", "noMotionSimulationHazard", "noSoundHazard"],

    // schema:accessibilitySummary — required human-readable prose that
    // complements but does not duplicate the structured fields above.
    // EU Accessibility Act and EPUB A11y 1.1 §4.1.4 both require this.
    "summary": "This publication includes synchronised audio narration aligned to the text using EPUB3 Media Overlays (SMIL). All images carry alternative text. The text is fully reflowable with adjustable font size and spacing. Print page numbers are preserved for cross-format reference.",

    // Conformance claim — required under EAA for books published after 28 June 2025.
    // WCAG Level AA is the minimum level demanded by the EAA.
    "conformance": {
      // dcterms:conformsTo — full standard identifier string.
      "standard": "EPUB Accessibility 1.1 - WCAG 2.1 Level AA",
      "wcag_level": "AA",       // "A" | "AA" | "AAA"
      "wcag_version": "2.1",    // "2.0" | "2.1" | "2.2"

      // a11y:certifiedBy — name of the certifying organisation or individual.
      "certifier": null,

      // a11y:certifierCredential — URL of the certifier's accreditation.
      "certifier_credential": null,

      // a11y:certifierReport — URL of the full accessibility evaluation report.
      "certifier_report": null,

      // ISO 8601 date of certification.
      "certification_date": null
    },

    // Optional: EAA Article 14 / national exemptions (e.g. micro-enterprise exemption).
    // Free-text; not machine-actionable.
    "legal_exemptions": []
  },

  // === LINKS ===
  // External links for reference. Not used programmatically.
  "links": [
    {
      "type": "author_website",
      "url": "https://www.brandonsanderson.com/"
    }
  ],

  // === METADATA PROVENANCE ===
  "metadata": {
    "created_at": "2026-01-15T12:00:00Z",
    "updated_at": "2026-03-12T00:00:00Z",
    "match_confidence": 0.97,
    "match_method": "isbn",
    "primary_source": "google_books",
    "needs_review": false,
    "locked_fields": ["title", "contributors"],
    "raw_sources": {
      "google_books": {
        "fetched_at": "2026-03-12T00:00:00Z",
        "query_used": "isbn:9780765326355",
        "response_hash": "sha256:abc123..."
      },
      "open_library": {
        "fetched_at": "2026-03-12T00:00:00Z",
        "query_used": "isbn/9780765326355",
        "response_hash": "sha256:def456..."
      },
      "audnexus": {
        "fetched_at": "2026-03-12T00:00:00Z",
        "query_used": "asin/B003ZWFO7E",
        "response_hash": "sha256:ghi789..."
      }
    }
  }
}
```

### Sidecar Design Notes

**`schema_version`**: Integer, incremented on breaking changes. The app handles migration between versions. Allows the format to evolve without breaking existing sidecars.

**`identifiers`**: Deliberately comprehensive. Not every book will have every identifier, but when one exists, storing it enables cross-referencing with any future metadata source. The Calibre UUID field supports importing from/exporting to Calibre libraries.

**`contributors` instead of separate `authors`/`narrators`**: A single ordered list with roles is more flexible than separate fields. Roles use MARC relator codes where applicable: “author” (aut), “narrator” (nrt), “editor” (edt), “translator” (trl), “illustrator” (ill). This handles edge cases like author-narrated audiobooks cleanly.

**`series` as an array**: A book can belong to both a specific series (Stormlight Archive #1) and a universe/collection (Cosmere) simultaneously. Position is nullable for collections where order doesn’t apply.

**`ratings` kept per-source**: Never averaged or combined. Different communities rate differently. The UI can display them however the user prefers, but the sidecar preserves the original data.

**`covers` with `selected`**: Store every cover found, mark which is active. The user can switch covers in the admin UI without re-fetching.

**`metadata.locked_fields`**: When a user manually edits a field, it’s added to `locked_fields` so the next metadata refresh doesn’t overwrite their correction.

**`metadata.raw_sources`**: We store enough to know *what was fetched and when*, but NOT the raw API response itself (that would bloat the sidecar). The response hash lets us detect if re-fetching would yield new data. Full raw responses can optionally be cached in the database for a configurable retention period.

**`description_format`**: Either “plain” or “html”. Some sources return HTML descriptions. We preserve the format rather than lossy-converting.

-----

## Part 2: SQLite Schema

### Migration Strategy

All schema changes are versioned migrations stored in Go code (not external SQL files). The database has a `schema_version` table that tracks the current version. On startup, the app runs any pending migrations in order.

### Core Schema

```sql
-- ============================================================
-- SYSTEM
-- ============================================================

CREATE TABLE schema_version (
    version     INTEGER NOT NULL,
    applied_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE app_settings (
    key         TEXT    PRIMARY KEY,
    value       TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- MEDIA ROOTS (physical storage locations)
-- ============================================================

CREATE TABLE media_roots (
    id          TEXT    PRIMARY KEY,  -- UUID
    name        TEXT    NOT NULL,
    root_path   TEXT    NOT NULL UNIQUE,
    scan_config TEXT    NOT NULL DEFAULT '{}',  -- JSON: folder parsing hints, exclusion patterns
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- WORKS (core entity, indexed from sidecars)
-- A "work" is a single intellectual creation in any/all formats.
-- All formats (epub, pdf, m4b, aligned epub3) live in one directory.
-- ============================================================

CREATE TABLE works (
    id              TEXT    PRIMARY KEY,  -- UUID
    media_root_id   TEXT    NOT NULL REFERENCES media_roots(id) ON DELETE CASCADE,
    directory_path  TEXT    NOT NULL,      -- relative to media_root root_path
    
    -- Denormalized from sidecar for fast querying
    title           TEXT    NOT NULL,
    sort_title      TEXT    NOT NULL,
    subtitle        TEXT,
    language        TEXT,
    publisher       TEXT,
    publish_date    TEXT,
    description     TEXT,
    description_format TEXT DEFAULT 'plain',
    page_count      INTEGER,
    
    -- Audiobook summary fields (null for non-audiobooks)
    duration_seconds    INTEGER,
    is_abridged         INTEGER DEFAULT 0,
    
    -- Media overlay indicator
    has_media_overlay   INTEGER DEFAULT 0,
    
    -- Metadata state
    match_confidence    REAL,
    match_method        TEXT,
    primary_source      TEXT,
    needs_review        INTEGER DEFAULT 1,
    sidecar_hash        TEXT,   -- SHA-256 of metadata.json, used to detect external edits
    
    -- Timestamps
    added_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    last_scanned_at TEXT    NOT NULL DEFAULT (datetime('now')),
    
    UNIQUE (media_root_id, directory_path)
);

CREATE INDEX idx_works_media_root    ON works(media_root_id);
CREATE INDEX idx_works_title         ON works(sort_title COLLATE NOCASE);
CREATE INDEX idx_works_publish_date  ON works(publish_date);
CREATE INDEX idx_works_needs_review  ON works(needs_review) WHERE needs_review = 1;
CREATE INDEX idx_works_added_at      ON works(added_at);

-- ============================================================
-- FULL-TEXT SEARCH (FTS5)
-- ============================================================

CREATE VIRTUAL TABLE works_fts USING fts5(
    title,
    subtitle,
    description,
    contributors,  -- denormalized "Author Name, Narrator Name" for search
    series,         -- denormalized "Series Name" for search
    publisher,
    tags,           -- denormalized "tag1, tag2, tag3" for search
    content=works,
    content_rowid=rowid
);

-- Triggers to keep FTS index in sync
CREATE TRIGGER works_fts_insert AFTER INSERT ON works BEGIN
    INSERT INTO works_fts(rowid, title, subtitle, description, contributors, series, publisher, tags)
    VALUES (new.rowid, new.title, new.subtitle, new.description, '', '', new.publisher, '');
END;

CREATE TRIGGER works_fts_delete AFTER DELETE ON works BEGIN
    INSERT INTO works_fts(books_fts, rowid, title, subtitle, description, contributors, series, publisher, tags)
    VALUES ('delete', old.rowid, old.title, old.subtitle, old.description, '', '', old.publisher, '');
END;

CREATE TRIGGER works_fts_update AFTER UPDATE ON works BEGIN
    INSERT INTO works_fts(books_fts, rowid, title, subtitle, description, contributors, series, publisher, tags)
    VALUES ('delete', old.rowid, old.title, old.subtitle, old.description, '', '', old.publisher, '');
    INSERT INTO works_fts(rowid, title, subtitle, description, contributors, series, publisher, tags)
    VALUES (new.rowid, new.title, new.subtitle, new.description, '', '', new.publisher, '');
END;
-- NOTE: The denormalized FTS fields (contributors, series, tags) are populated
-- by the application after inserting/updating the junction tables. A dedicated
-- function rebuilds the FTS row with current denormalized values. This is simpler
-- and more maintainable than complex multi-table triggers.

-- ============================================================
-- IDENTIFIERS
-- ============================================================

CREATE TABLE identifiers (
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    type        TEXT    NOT NULL,  -- 'isbn_13', 'asin', 'google_books', etc.
    value       TEXT    NOT NULL,
    PRIMARY KEY (work_id, type)
);

CREATE INDEX idx_identifiers_lookup ON identifiers(type, value);

-- ============================================================
-- CONTRIBUTORS (normalized)
-- ============================================================

CREATE TABLE contributors (
    id          TEXT    PRIMARY KEY,  -- UUID
    name        TEXT    NOT NULL,
    sort_name   TEXT    NOT NULL,
    
    -- Optional external identifiers for the person
    identifiers TEXT    DEFAULT '{}',  -- JSON: {"open_library": "/authors/...", "goodreads": "123"}
    
    UNIQUE (name, sort_name)
);

CREATE INDEX idx_contributors_name ON contributors(sort_name COLLATE NOCASE);

CREATE TABLE work_contributors (
    work_id         TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    contributor_id  TEXT    NOT NULL REFERENCES contributors(id) ON DELETE CASCADE,
    role            TEXT    NOT NULL DEFAULT 'author',  -- author, narrator, editor, translator, illustrator
    position        INTEGER NOT NULL DEFAULT 0,          -- display order
    PRIMARY KEY (work_id, contributor_id, role)
);

CREATE INDEX idx_work_contributors_work        ON work_contributors(work_id);
CREATE INDEX idx_work_contributors_contributor  ON work_contributors(work_id, contributor_id);
CREATE INDEX idx_work_contributors_role         ON work_contributors(role);

-- ============================================================
-- SERIES (normalized)
-- ============================================================

CREATE TABLE series (
    id      TEXT    PRIMARY KEY,  -- UUID
    name    TEXT    NOT NULL UNIQUE,
    identifiers TEXT DEFAULT '{}' -- JSON
);

CREATE INDEX idx_series_name ON series(name COLLATE NOCASE);

CREATE TABLE work_series (
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    series_id   TEXT    NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    position    REAL,   -- REAL allows 1.5 for "between book 1 and 2" novellas
    PRIMARY KEY (work_id, series_id)
);

CREATE INDEX idx_work_series_series ON work_series(series_id, position);

-- ============================================================
-- TAGS & GENRES
-- ============================================================

CREATE TABLE tags (
    id      TEXT    PRIMARY KEY,  -- UUID
    name    TEXT    NOT NULL,
    type    TEXT    NOT NULL DEFAULT 'tag',  -- 'tag', 'genre', 'subject'
    UNIQUE (name, type)
);

CREATE INDEX idx_tags_name ON tags(name COLLATE NOCASE);

CREATE TABLE work_tags (
    work_id TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    tag_id  TEXT    NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (work_id, tag_id)
);

CREATE INDEX idx_work_tags_tag ON work_tags(tag_id);

-- ============================================================
-- FILES
-- ============================================================

CREATE TABLE work_files (
    id              TEXT    PRIMARY KEY,  -- UUID
    work_id         TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    filename        TEXT    NOT NULL,
    format          TEXT    NOT NULL,  -- epub, pdf, m4b, mp3, cbz, cbr, epub3_overlay
    size_bytes      INTEGER NOT NULL,
    checksum_sha256 TEXT,
    
    -- Audio-specific
    duration_seconds    INTEGER,
    bitrate_kbps        INTEGER,
    codec               TEXT,
    
    -- Media overlay
    has_media_overlay   INTEGER DEFAULT 0,
    
    added_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    
    UNIQUE (work_id, filename)
);

CREATE INDEX idx_work_files_work   ON work_files(work_id);
CREATE INDEX idx_work_files_format ON work_files(format);

-- ============================================================
-- AUDIOBOOK CHAPTERS
-- ============================================================

CREATE TABLE audiobook_chapters (
    id              TEXT    PRIMARY KEY,  -- UUID
    work_id         TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    title           TEXT    NOT NULL,
    start_seconds   REAL    NOT NULL,
    end_seconds     REAL    NOT NULL,
    index_position  INTEGER NOT NULL,
    
    UNIQUE (work_id, index_position)
);

CREATE INDEX idx_chapters_work ON audiobook_chapters(work_id, index_position);

-- ============================================================
-- WORK ACCESSIBILITY
-- EPUB Accessibility 1.1 metadata (EU Accessibility Act, EAA Directive 2019/882).
-- One row per work. Slice fields are stored as JSON TEXT arrays.
-- ============================================================

CREATE TABLE work_accessibility (
    work_id TEXT PRIMARY KEY REFERENCES works(id) ON DELETE CASCADE,

    -- schema:accessMode — JSON array, e.g. '["textual","visual","auditory"]'
    access_modes TEXT,

    -- schema:accessModeSufficient — JSON array of arrays
    -- e.g. '[["textual"],["auditory","textual"]]'
    access_modes_sufficient TEXT,

    -- schema:accessibilityFeature — JSON array of feature tokens.
    -- Key token for aligned readalouds: "synchronizedAudioText".
    features TEXT,

    -- schema:accessibilityHazard — JSON array of hazard tokens.
    hazards TEXT,

    -- schema:accessibilitySummary — required human-readable prose.
    summary TEXT,

    -- Conformance claim (dcterms:conformsTo / a11y: vocabulary).
    -- Required under the EAA for ebooks published after 28 June 2025.
    conformance_standard     TEXT,  -- e.g. "EPUB Accessibility 1.1 - WCAG 2.1 Level AA"
    wcag_level               TEXT,  -- "A" | "AA" | "AAA"  (EAA minimum is "AA")
    wcag_version             TEXT,  -- "2.0" | "2.1" | "2.2"
    certifier                TEXT,  -- a11y:certifiedBy
    certifier_credential     TEXT,  -- a11y:certifierCredential URL
    certifier_report         TEXT,  -- a11y:certifierReport URL
    certification_date       TEXT,  -- YYYY-MM-DD

    -- Media overlay / aligned readaloud specifics.
    overlay_narrator_name     TEXT,
    overlay_narrator_language TEXT,  -- BCP-47, e.g. "en-US"
    overlay_duration_seconds  INTEGER,
    smil_version              TEXT,  -- e.g. "3.0"
    sync_granularity          TEXT,  -- "word" | "sentence" | "paragraph"
    active_class              TEXT,  -- CSS class applied to the active text element
    playback_active_class     TEXT   -- CSS class applied while the overlay is playing
);

CREATE INDEX idx_work_accessibility_wcag ON work_accessibility(wcag_level);
CREATE INDEX idx_work_accessibility_smil ON work_accessibility(sync_granularity)
    WHERE sync_granularity IS NOT NULL;

-- ============================================================
-- RATINGS (per-source)
-- ============================================================

CREATE TABLE ratings (
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    source      TEXT    NOT NULL,  -- 'google_books', 'open_library', etc.
    score       REAL    NOT NULL,
    max_score   REAL    NOT NULL DEFAULT 5.0,
    count       INTEGER,
    fetched_at  TEXT    NOT NULL,
    PRIMARY KEY (work_id, source)
);

-- ============================================================
-- COVERS
-- ============================================================

CREATE TABLE covers (
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    source      TEXT    NOT NULL,  -- 'embedded', 'google_books', 'open_library'
    filename    TEXT    NOT NULL,
    width       INTEGER,
    height      INTEGER,
    is_selected INTEGER NOT NULL DEFAULT 0,
    fetched_at  TEXT,
    PRIMARY KEY (work_id, source)
);

-- ============================================================
-- USERS & AUTH
-- ============================================================

CREATE TABLE users (
    id              TEXT    PRIMARY KEY,  -- UUID
    username        TEXT    NOT NULL UNIQUE,
    display_name    TEXT,
    email           TEXT    UNIQUE,
    password_hash   TEXT,              -- NULL for OIDC-only users
    oidc_subject    TEXT    UNIQUE,    -- OIDC sub claim
    oidc_issuer     TEXT,
    role            TEXT    NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user', 'guest')),
    is_active       INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    last_login_at   TEXT
);

CREATE TABLE sessions (
    id          TEXT    PRIMARY KEY,  -- opaque token
    user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name TEXT,
    device_type TEXT,   -- 'web', 'kobo', 'koreader', 'opds_client', 'abs_client'
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT    NOT NULL,
    last_used_at TEXT
);

CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- ============================================================
-- USER STATE (source of truth — NOT in sidecars)
-- ============================================================

-- Reading/listening progress per user per work
CREATE TABLE user_progress (
    user_id         TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    work_id         TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    
    -- For ebooks: position within the book
    ebook_cfi       TEXT,       -- EPUB CFI position string
    ebook_percent   REAL,       -- 0.0 to 1.0
    ebook_chapter   TEXT,       -- current chapter title/identifier
    
    -- For audiobooks: playback position
    audio_position_seconds  REAL,
    audio_chapter_index     INTEGER,
    
    -- General
    is_finished     INTEGER NOT NULL DEFAULT 0,
    started_at      TEXT,
    finished_at     TEXT,
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    
    -- Device that last updated this progress
    last_device_id  TEXT,
    
    PRIMARY KEY (user_id, work_id)
);

CREATE INDEX idx_progress_user ON user_progress(user_id);

-- Bookmarks and highlights
CREATE TABLE user_annotations (
    id          TEXT    PRIMARY KEY,  -- UUID
    user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    type        TEXT    NOT NULL CHECK (type IN ('bookmark', 'highlight', 'note')),
    
    -- Position (format depends on media type)
    ebook_cfi   TEXT,
    audio_position_seconds REAL,
    
    -- Content
    text        TEXT,       -- highlighted text or note content
    color       TEXT,       -- highlight color
    
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_annotations_user_work ON user_annotations(user_id, work_id);

-- Virtual curated views (manual, smart, or device-linked)
CREATE TABLE collections (
    id              TEXT    PRIMARY KEY,  -- UUID
    user_id         TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT    NOT NULL,
    description     TEXT,
    collection_type TEXT    NOT NULL DEFAULT 'manual'
                    CHECK (collection_type IN ('manual', 'smart', 'device')),
    smart_filter    TEXT,   -- JSON filter rules for smart collections
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

-- ============================================================
-- SYNC STATE (KoboSync, KOReader)
-- ============================================================

CREATE TABLE sync_devices (
    id          TEXT    PRIMARY KEY,  -- UUID
    user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name TEXT    NOT NULL,
    device_type TEXT    NOT NULL,  -- 'kobo', 'koreader', 'abs_app'
    device_id   TEXT,              -- device-reported identifier
    last_sync_at TEXT,
    settings    TEXT    DEFAULT '{}',  -- JSON: per-device sync preferences
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_sync_devices_user ON sync_devices(user_id);

-- Tracks which works have been synced to which devices
-- and what state they were in at sync time
CREATE TABLE sync_state (
    device_id   TEXT    NOT NULL REFERENCES sync_devices(id) ON DELETE CASCADE,
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    last_synced_hash TEXT,   -- hash of work metadata at sync time, to detect changes
    last_synced_at   TEXT    NOT NULL,
    PRIMARY KEY (device_id, work_id)
);

-- ============================================================
-- METADATA TASKS (fetch queue)
-- ============================================================

CREATE TABLE metadata_tasks (
    id          TEXT    PRIMARY KEY,  -- UUID
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    status      TEXT    NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'review')),
    task_type   TEXT    NOT NULL DEFAULT 'auto_match'
                        CHECK (task_type IN ('auto_match', 'refresh', 'manual_search')),
    priority    INTEGER NOT NULL DEFAULT 0,
    
    -- Results
    candidates  TEXT,   -- JSON array of match candidates with confidence scores
    selected    INTEGER, -- index into candidates array that was auto-selected or user-picked
    error       TEXT,
    
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    started_at  TEXT,
    completed_at TEXT
);

CREATE INDEX idx_tasks_status   ON metadata_tasks(status, priority DESC);
CREATE INDEX idx_tasks_work     ON metadata_tasks(work_id);

-- ============================================================
-- RAW SOURCE CACHE (optional, configurable retention)
-- ============================================================

CREATE TABLE source_cache (
    work_id     TEXT    NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    source      TEXT    NOT NULL,
    query_used  TEXT    NOT NULL,
    response    TEXT    NOT NULL,  -- full JSON response from the API
    fetched_at  TEXT    NOT NULL,
    PRIMARY KEY (work_id, source)
);
```

### Schema Design Notes

**UUIDs as primary keys (stored as TEXT).** SQLite handles TEXT PKs efficiently, and UUIDs mean IDs are globally unique and not tied to insertion order. This matters for sync — if two devices create records offline, there are no ID collisions. Go’s `github.com/google/uuid` generates them.

**`work_series.position` is REAL, not INTEGER.** This allows novellas between numbered entries (e.g., position 1.5 for a novella between books 1 and 2). This is how Calibre and most library tools handle it.

**FTS5 with denormalized fields.** The FTS table includes `contributors`, `series`, `tags`, and `publisher` as searchable text, but these are populated by the application layer (not triggers on junction tables, which would be fragile). After any change to a book’s contributors, series, or tags, the app calls a function that rebuilds the FTS row with current values.

**`user_progress` stores both ebook and audio positions.** A single row per user-per-book, with nullable fields for each media type. This means if a user reads the ebook and also listens to the audiobook, both positions are tracked in one record. The alternative (separate tables for ebook progress vs audio progress) creates unnecessary joins for the common case.

**`ebook_cfi`**: EPUB Canonical Fragment Identifier. This is the standard way to identify a position within an EPUB and is what OPDS readers, KOReader, and Kobo devices use. Storing the raw CFI string ensures compatibility with any client.

**`metadata_tasks` as a work queue.** When a book is scanned and needs metadata, a task is created. The metadata engine processes the queue, writes results, and either auto-selects a high-confidence match or sets status to ‘review’ for human triage. The `candidates` field stores the JSON array of all match options so the user can pick from them in the admin UI.

**`source_cache` is separate from sidecars.** The sidecar stores the *merged, canonical* metadata. The source cache stores the *raw API responses* for potential re-processing. The cache has configurable retention (default: 90 days) and can be purged without affecting the sidecars.

**`sync_state.last_synced_hash`**: When a book’s metadata changes after it was last synced to a device, comparing hashes tells the sync adapter that it needs to push an update. This is much cheaper than diffing full metadata records.

-----

## Part 3: Compatibility Matrix

### How existing tools map to this schema

|External Tool                             |What it uses                            |Our mapping                                                                |
|------------------------------------------|----------------------------------------|---------------------------------------------------------------------------|
|**Calibre**                               |`metadata.opf` (OPF/Dublin Core XML)    |Round-trip via identifiers.calibre_uuid. Export generates OPF from sidecar.|
|**OPDS clients** (KyBook, Panels, Cantook)|Atom XML feeds with Dublin Core metadata|Generated from works + identifiers + work_files                            |
|**KoboSync** (Kobo e-readers)             |REST API mimicking Kobo cloud           |books + user_progress (ebook_cfi) + sync_state                             |
|**KOReader**                              |Sync progress API                       |user_progress.ebook_cfi + user_progress.ebook_percent                      |
|**Audiobookshelf apps**                   |REST API with chapter manifests         |works + audiobook_chapters + work_files + user_progress (audio)            |
|**Storyteller**                           |EPUB3 with Media Overlays               |work_files (format=epub3_overlay) + media_overlay sidecar fields           |
|**Readarr / Chaptarr**                    |Folder structure + filenames            |Scanner reads their output; identifiers cross-reference                    |

### OPDS Feed Generation (from schema)

```
OPDS Catalog Root
├── /opds/v1.2/catalog           → collections (one navigation entry per collection)
├── /opds/v1.2/library/{id}      → books in library (paginated acquisition feed)
├── /opds/v1.2/search?q=         → books_fts query
├── /opds/v1.2/series/{id}       → works in series (ordered by work_series.position)
├── /opds/v1.2/author/{id}       → books by contributor
├── /opds/v1.2/book/{id}         → single book entry with download links per format
└── /opds/v1.2/collection/{id}        → user collection contents (auth required)
```

Each book in an OPDS acquisition feed maps to:

- `<title>` ← books.title
- `<author>` ← work_contributors WHERE role=‘author’ → contributors.name
- `<dc:identifier>` ← identifiers (one per identifier type)
- `<link rel="http://opds-spec.org/acquisition">` ← work_files (one per format)
- `<link rel="http://opds-spec.org/image">` ← covers WHERE is_selected=1
- `<category>` ← work_tags → tags (WHERE type=‘genre’)
- `<summary>` ← books.description
- `<series>` ← work_series → series (uses OPDS Series extension)

-----

## Part 4: File System Layout

### Container Mount Points

```
/config                     ← app configuration + SQLite database
  ├── library.db            ← main SQLite database
  ├── library.db-wal        ← WAL file
  ├── library.db-shm        ← shared memory
  └── config.yaml           ← app configuration

/media                      ← bind mount to user's unified media collection (read-write)
  └── (user's folder structure — Codex reads whatever exists)

/cache                      ← optional, for thumbnails and temp files
  ├── thumbnails/           ← resized cover images for API responses
  └── tmp/                  ← processing temp space
```

### Sidecar Files In-Place (unified mixed-format directory)

```
/media/Brandon Sanderson/Stormlight Archive/01 - The Way of Kings/
  ├── The Way of Kings.epub              ← ebook
  ├── The Way of Kings.pdf               ← alternate ebook format
  ├── The Way of Kings.m4b               ← audiobook
  ├── The Way of Kings - Aligned.epub    ← EPUB3 with media overlays
  ├── metadata.json                       ← sidecar (source of truth)
  ├── cover.jpg                           ← selected cover image
  ├── cover_embedded.jpg                  ← extracted from epub
  └── cover_openlibrary.jpg              ← alternate cover source
```

-----

## Part 5: Database Rebuild Procedure

Because sidecars are the metadata source of truth, the database can be rebuilt from scratch:

1. Delete `library.db` (preserving a backup).
1. On next startup, the app creates a fresh database with the schema above.
1. For each library, the scanner walks the directory tree.
1. For each directory containing a `metadata.json`, the sidecar is parsed and the book + all related records (contributors, series, tags, identifiers, files, chapters, ratings, covers) are inserted.
1. For directories *without* a `metadata.json`, a new book record is created with `needs_review=1` and a metadata task is queued.
1. **User state is lost** (progress, shelves, annotations, sync state). This is the one thing that lives only in the database.

### Mitigating User State Loss

The app should include an automatic nightly backup of the SQLite database (compressed, with configurable retention). A manual “Export User Data” feature can dump user progress, shelves, and annotations to a JSON file that can be re-imported after a rebuild.
