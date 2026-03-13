# Codex — Work Detail Redesign, Hardcover API Integration & Metadata Selector

## Context

This is the `library-server` repo: https://github.com/scootsy/library-server/  
Stack: **Go backend** (`cmd/codex`, `internal/`) + **Svelte frontend** (`web/`) + SQLite/embedded DB. Docker-deployed on Unraid.

Read `CLAUDE.md`, `PROJECT.md`, and `schema-spec.md` before starting. Familiarize yourself with the existing metadata provider pattern in `internal/` (look for how the current metadata sources like Open Library are implemented — search, fetch, and auto_match task handlers). Study the existing Svelte components in `web/` — particularly the current work/book detail page and any shared UI components.

---

## Task 1: Redesign the Work Detail Page

The current work detail view is a sparse grid of labeled boxes (METADATA, CONTRIBUTORS, SERIES, TAGS, FILES, IDENTIFIERS, COVERS, RATINGS, METADATA TASKS). It's ugly and hard to scan. Redesign it to look like a **product listing page** — dense, informational, visually anchored by the cover image.

### Target Layout (top-to-bottom)

**Hero Section (top of page):**
- **Left column (~250px):** Large cover image with rounded corners and subtle shadow. If no cover exists, show a styled placeholder with the title text. Below the cover: format badges (m4b, epub, etc.) derived from the files associated with this work.
- **Right column (remainder):** 
  - **Series line** (small, muted, above title): e.g. "Doctor Dolittle #1" — linked if series exists, hidden if not.
  - **Title** (large, bold heading, h1-equivalent).
  - **Author(s)** as a clickable link row beneath the title. Show all contributors, primary author first.
  - **Format badges row:** File format tags (m4b, epub, pdf, etc.) with a "PRIMARY" badge on the primary file format, similar to how Booklore does it.
  - **Star rating row:** If ratings exist, show stars. If external ratings exist from metadata sources, show small source icons (Hardcover logo, etc.) with their score.
  - **Genre/Tag pills:** Horizontal wrapping row of rounded pill badges for all tags. Use a subtle colored background (e.g. muted blue/teal pills on dark background).
  - **Metadata grid (2-3 columns, compact key:value pairs):**
    - Publisher: value
    - Published: date
    - Language: value  
    - Page Count: value (if available)
    - ISBN: value (if identifiers exist)
    - File Size: computed from files
    - Narrator: value (for audiobooks, from contributors)
    - Duration: value (for audiobooks, computed from file metadata if available)
  - **Action buttons row:**
    - "Fetch Metadata" button (primary action, accent color) — triggers the metadata fetch + selector flow (Task 3).
    - "Download" button with file size shown (e.g. "m4b · 426.8 MB") — downloads the primary file.
    - Overflow menu (three dots) for: Refresh Metadata, Edit Metadata, Delete Work.

**Description Section (below hero):**
- Full-width card/panel with the work's description text. If no description, show "No description available" in muted text with a prompt to fetch metadata.

**Tabbed Section (below description):**
- Tabs: **Files** | **Notes** (future) | **Similar** (future)
- **Files tab (default):**
  - "Primary File" section with the primary file shown as a row: format badge, filename, size, download icon, delete icon.
  - "Alternative Formats" section listing other associated files the same way.

**Metadata Tasks Section (bottom, collapsible or subtle):**
- Small table of metadata tasks (refresh, auto_match) with status badges and timestamps — keep this but make it less prominent than it currently is. Consider a collapsible accordion.

### Design Guidelines
- Dark theme consistent with the existing Codex UI (dark background ~`#0f1219`, card backgrounds ~`#1a1f2e`, muted borders).
- Use the existing Svelte component patterns and CSS conventions already in the project.
- Responsive: the hero section should stack vertically on narrow viewports (cover on top, metadata below).
- All data should come from the existing work API endpoint. No new API endpoints needed for this task — just better frontend presentation of existing data.

---

## Task 2: Add Hardcover as a Metadata Source

Integrate Hardcover (https://hardcover.app) as a new metadata provider alongside any existing ones.

### Hardcover API Details
- **Type:** GraphQL API
- **Endpoint:** `https://api.hardcover.app/v1/graphql`
- **Auth:** Bearer token in `authorization` header (user provides their API key)
- **Rate limit:** 60 requests/minute, 30-second timeout per query
- **Docs:** https://docs.hardcover.app/api/getting-started/

### Configuration
Add a `hardcover` section to `config.yaml`:
```yaml
metadata:
  hardcover:
    enabled: true
    api_key: "your-hardcover-api-token"
    priority: 1  # lower = higher priority when multiple sources return data
```

### Backend Implementation

Create a new metadata provider in `internal/` following the same pattern as existing providers. The provider needs to implement:

**1. Search by title/author:**
```graphql
query SearchBooks($query: String!) {
  search(
    query: $query,
    query_type: "Book",
    per_page: 10
  ) {
    results {
      ... on SearchBook {
        id: document_id
        title
        author_names
        cover_url: image
        release_year
        isbns
        series_names
      }
    }
  }
}
```

**2. Search by ISBN:**
Use the same search endpoint with the ISBN as the query string, and set `fields: "isbns"` and `weights: "5"` to prioritize ISBN matches.

**3. Get full book details (after identifying a match):**
```graphql
query GetBookDetails($id: Int!) {
  books_by_pk(id: $id) {
    id
    title
    subtitle
    description
    release_date
    pages
    slug
    cached_contributors  # JSON array with author info
    cached_tags          # JSON with genre/tag info
    rating               # aggregate rating
    ratings_count
    users_read_count
    default_physical_edition {
      isbn_10
      isbn_13
      pages
      publisher {
        name
      }
      language {
        language
      }
      image {
        url
      }
      release_date
    }
    default_audio_edition {
      isbn_10
      isbn_13
      pages
      publisher {
        name
      }
      release_date
      audio_seconds
      image {
        url
      }
    }
    book_series {
      position
      series {
        id
        name
      }
    }
  }
}
```

**4. Map Hardcover fields → Codex schema:**
- `title` → work title
- `subtitle` → work subtitle
- `description` → work description
- `cached_contributors[].author.name` → contributors (check `contribution` field: "Author", "Narrator", etc.)
- `cached_tags` → tags
- `default_physical_edition.isbn_13` or `isbn_10` → identifiers
- `default_physical_edition.publisher.name` → publisher
- `default_physical_edition.language.language` → language
- `release_date` → published date
- `pages` → page count
- `default_audio_edition.audio_seconds` → duration (for audiobooks)
- `book_series[].series.name` + `position` → series + series position
- `rating` → rating (store source as "hardcover")
- `default_physical_edition.image.url` or `default_audio_edition.image.url` → cover image URL (download and store locally)

**5. Cover image handling:**
When Hardcover returns an image URL, download it and store it in the covers directory following the existing pattern. Don't hotlink to Hardcover.

**6. Auto-match integration:**
Wire the Hardcover provider into the existing `auto_match` task system. When a library scan finds a new work, the auto_match task should query Hardcover using the filename-derived title and author. Use ISBN if available from file metadata (e.g., embedded in epub/m4b metadata).

### Error Handling
- If the API key is missing or invalid, log a warning and skip Hardcover (don't crash).
- If rate-limited (HTTP 429), implement exponential backoff with a max of 3 retries.
- If a query times out, log and move on — don't block the auto_match pipeline.

---

## Task 3: Metadata Selector / Review UI

When a user clicks "Fetch Metadata" on a work, instead of blindly overwriting all fields, show a **side-by-side comparison and selection UI** so the user can cherry-pick which fields to apply.

### Flow

1. User clicks "Fetch Metadata" on a work detail page.
2. Frontend sends a request to a new API endpoint: `POST /api/works/{id}/metadata/fetch`
   - Backend queries all enabled metadata sources (Hardcover, Open Library, etc.) in parallel.
   - Returns a combined response with the current work metadata + all source results:
     ```json
     {
       "current": { "title": "James (Unabridged)", "author": "", "description": "When the enslaved Jim...", ... },
       "sources": {
         "hardcover": { "title": "James: A Novel", "author": "Percival Everett", "description": "...", "cover_url": "...", ... },
         "openlibrary": { "title": "James", "author": "Percival Everett", "description": "...", ... }
       }
     }
     ```
3. Frontend shows a **Metadata Selector Modal/Panel** (full-screen overlay or slide-in panel).

### Metadata Selector UI

**Layout: Field-by-field comparison table.**

For each metadata field (title, subtitle, author, description, publisher, published date, language, ISBN, page count, series, series position, tags, cover image, narrator, duration):

- **Row layout:** 
  - Field name label on the left.
  - Current value shown with a radio button or checkbox (pre-selected if no better match exists).
  - Each source's value shown with its own radio button, labeled with the source name (e.g., "Hardcover", "Open Library").
  - If a source didn't return data for that field, show "—" grayed out and unselectable.

- **Cover image row:** Show thumbnail previews of each available cover side-by-side so the user can visually compare.

- **Description row:** Since descriptions can be long, show a truncated preview (2-3 lines) with an expand toggle for each source.

- **Tags row:** Show tag pills from each source. Let the user select which source's tag set to use, or potentially merge them (checkbox: "Merge all tags").

**Top of the selector:**
- Source summary bar showing which sources returned results and how many fields each populated.
- "Select All from [Source]" quick buttons — e.g., clicking "Use all Hardcover" selects Hardcover's value for every field where it has data.
- "Keep All Current" button to deselect everything and close.

**Bottom of the selector:**
- "Apply Selected" button (primary) — sends the chosen field values to: `PATCH /api/works/{id}/metadata`
- "Cancel" button.

### Backend for Metadata Selector

**New endpoints:**

1. `POST /api/works/{id}/metadata/fetch` — Triggers parallel metadata fetch from all enabled sources. Returns the structure shown above. Cache results briefly (5 min) so the user can re-open the selector without re-fetching.

2. `PATCH /api/works/{id}/metadata` — Accepts a partial update payload with only the fields the user chose to apply:
   ```json
   {
     "title": "James: A Novel",
     "author": "Percival Everett",
     "description": "...",
     "cover_source": "hardcover",
     "cover_url": "https://...",
     "tags": ["Historical Fiction", "Literary Fiction"],
     "series": "...",
     "series_position": 1
   }
   ```
   The handler applies each provided field, downloads the cover if `cover_url` is provided, and updates the database.

### Design Guidelines for the Selector
- Use the same dark theme as the rest of Codex.
- The selector should feel like a diff viewer — make it very clear what's "current" vs what each source offers.
- Highlight fields where sources disagree (e.g., different titles) with a subtle accent color.
- Pre-select the "best" option intelligently: if current field is empty/blank and a source has data, pre-select the source. If current field already has data, keep it selected by default.
- The user should be able to complete the entire review and selection without navigating away from the work detail page.

---

## Implementation Order

1. **Task 2 first (Hardcover provider)** — Backend only. Get the provider working, test search and detail fetch via CLI or API calls. Wire into auto_match.
2. **Task 1 second (Work detail redesign)** — Frontend only. Redesign the Svelte component using existing API data. This gives you a much better view to work with.
3. **Task 3 last (Metadata selector)** — Full-stack. New endpoints + new Svelte modal/panel. This builds on top of both the provider (Task 2) and the redesigned detail page (Task 1).

## Testing Approach
- For Task 2: Test with known books — search for "James" by Percival Everett, "The Story of Doctor Dolittle" by Hugh Lofting. Verify field mapping is correct.
- For Task 1: Compare the redesigned page against the reference screenshot (Booklore-style). Verify all existing data renders correctly.
- For Task 3: Test with a work that has incomplete metadata. Verify the selector shows current vs source data correctly, that field selection works, and that "Apply Selected" writes only the chosen fields.
