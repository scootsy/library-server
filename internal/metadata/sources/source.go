// Package sources provides API clients that implement the MetadataSource
// interface. Each source is a pure API client with no internal dependencies.
package sources

import (
	"context"
	"net/url"
	"time"
)

// MetadataSource is the interface that all external metadata API clients must
// implement. Sources have no internal dependencies — they are pure API clients.
type MetadataSource interface {
	// Name returns the canonical lowercase identifier for this source
	// (e.g. "google_books", "open_library", "audnexus").
	Name() string

	// Search queries the source for candidates matching the supplied hints.
	// Results are returned in decreasing relevance order as determined by
	// the source. Callers apply their own confidence scoring on top.
	Search(ctx context.Context, q Query) ([]Candidate, error)

	// FetchByID retrieves full metadata for a known external identifier.
	// idType is the identifier namespace (e.g. "isbn", "asin", "olid").
	// Returns nil, nil when the source has no record for that ID.
	FetchByID(ctx context.Context, idType, idValue string) (*Candidate, error)
}

// Query holds the hints that drive a metadata lookup. All fields are optional;
// sources use whichever are populated. Providing more fields narrows results.
type Query struct {
	Title  string
	Author string // primary author / narrator
	ISBN   string // ISBN-10 or ISBN-13, digits only
	ASIN   string // Audible / Amazon ASIN
	Year   int    // publication year, 0 = unknown
}

// Candidate is one potential match returned by a source.
type Candidate struct {
	// Source is the Name() of the MetadataSource that produced this result.
	Source string

	// ExternalID is the source-specific record identifier
	// (Google Books volumeId, Open Library /works/OL…W, Audnexus ASIN, etc.).
	ExternalID string

	Title         string
	Subtitle      string
	SortTitle     string
	Authors       []Contributor
	Narrators     []Contributor
	Publisher     string
	PublishDate   string // ISO 8601 date or partial: "2010", "2010-11", "2010-11-17"
	Language      string // BCP-47 language tag (e.g. "en", "fr")
	Description   string
	PageCount     int
	DurationSecs  int               // audiobooks only
	Identifiers   map[string]string // isbn_13, isbn_10, asin, olid, …
	Series        []Series
	Tags          []string // genre / subject tags
	CoverURL      string   // highest-res cover image URL found
	FetchedAt     time.Time

	// RawData holds the parsed API response for storage in source_cache.
	// It must be JSON-serialisable.
	RawData any
}

// Contributor is a person linked to a work.
type Contributor struct {
	Name     string
	SortName string // "Last, First" form; empty = derive from Name
	Role     string // "author", "narrator", "editor", "illustrator", …
}

// Series associates a candidate with a named series.
type Series struct {
	Name     string
	Position float64 // 0 = unknown/not applicable
}

// sanitizeURL strips query parameters that may contain secrets (key, api_key, token)
// before the URL is logged.
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}
	q := u.Query()
	for param := range q {
		switch param {
		case "key", "api_key", "token", "secret", "authorization":
			q.Set(param, "[REDACTED]")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
