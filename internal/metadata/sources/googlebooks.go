package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const googleBooksBaseURL = "https://www.googleapis.com/books/v1"

// GoogleBooks is a MetadataSource backed by the Google Books Volumes API.
// An API key is optional but recommended to avoid aggressive rate limiting.
type GoogleBooks struct {
	apiKey     string
	httpClient *http.Client
}

// NewGoogleBooks returns a GoogleBooks source. apiKey may be empty for
// unauthenticated requests (lower rate limits). If httpClient is nil,
// a default client with a 15-second timeout is used.
func NewGoogleBooks(apiKey string, httpClient *http.Client) *GoogleBooks {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &GoogleBooks{
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

func (g *GoogleBooks) Name() string { return "google_books" }

// Search queries the Google Books Volumes API. When q.ISBN is set it performs
// an ISBN lookup first; otherwise it uses title + author.
func (g *GoogleBooks) Search(ctx context.Context, q Query) ([]Candidate, error) {
	queryStr := g.buildQuery(q)
	if queryStr == "" {
		return nil, nil
	}

	params := url.Values{}
	params.Set("q", queryStr)
	params.Set("maxResults", "5")
	params.Set("printType", "books")
	if g.apiKey != "" {
		params.Set("key", g.apiKey)
	}

	reqURL := googleBooksBaseURL + "/volumes?" + params.Encode()
	resp, err := g.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("google_books search: %w", err)
	}

	var result gbVolumesResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("google_books search decode: %w", err)
	}

	candidates := make([]Candidate, 0, len(result.Items))
	for _, item := range result.Items {
		c := g.volumeToCandidate(item)
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// FetchByID fetches a single volume by Google Books volumeId or ISBN.
func (g *GoogleBooks) FetchByID(ctx context.Context, idType, idValue string) (*Candidate, error) {
	var reqURL string
	switch idType {
	case "google_books":
		params := url.Values{}
		if g.apiKey != "" {
			params.Set("key", g.apiKey)
		}
		reqURL = googleBooksBaseURL + "/volumes/" + url.PathEscape(idValue) + "?" + params.Encode()
	case "isbn_13", "isbn_10", "isbn":
		params := url.Values{}
		params.Set("q", "isbn:"+idValue)
		params.Set("maxResults", "1")
		if g.apiKey != "" {
			params.Set("key", g.apiKey)
		}
		reqURL = googleBooksBaseURL + "/volumes?" + params.Encode()
	default:
		return nil, nil // unsupported identifier type for this source
	}

	raw, err := g.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("google_books fetch_by_id: %w", err)
	}

	if idType == "google_books" {
		var item gbVolume
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("google_books fetch_by_id decode: %w", err)
		}
		c := g.volumeToCandidate(item)
		return &c, nil
	}

	// ISBN search returns a list
	var result gbVolumesResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("google_books fetch_by_id decode: %w", err)
	}
	if len(result.Items) == 0 {
		return nil, nil
	}
	c := g.volumeToCandidate(result.Items[0])
	return &c, nil
}

// buildQuery constructs the Google Books query string from hints.
func (g *GoogleBooks) buildQuery(q Query) string {
	if q.ISBN != "" {
		return "isbn:" + q.ISBN
	}
	var parts []string
	if q.Title != "" {
		parts = append(parts, "intitle:"+q.Title)
	}
	if q.Author != "" {
		parts = append(parts, "inauthor:"+q.Author)
	}
	return strings.Join(parts, "+")
}

// get performs an HTTP GET and returns the response body.
func (g *GoogleBooks) get(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MiB
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("google_books unexpected status", "status", resp.StatusCode, "url", sanitizeURL(reqURL))
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	return body, nil
}

// volumeToCandidate maps a Google Books volume to a Candidate.
func (g *GoogleBooks) volumeToCandidate(v gbVolume) Candidate {
	vi := v.VolumeInfo
	c := Candidate{
		Source:      g.Name(),
		ExternalID:  v.ID,
		Title:       vi.Title,
		Subtitle:    vi.Subtitle,
		Publisher:   vi.Publisher,
		PublishDate: vi.PublishedDate,
		Language:    vi.Language,
		Description: vi.Description,
		PageCount:   vi.PageCount,
		Identifiers: make(map[string]string),
		FetchedAt:   time.Now().UTC(),
		RawData:     v,
	}

	for _, id := range vi.IndustryIdentifiers {
		switch id.Type {
		case "ISBN_13":
			c.Identifiers["isbn_13"] = id.Identifier
		case "ISBN_10":
			c.Identifiers["isbn_10"] = id.Identifier
		}
	}
	c.Identifiers["google_books"] = v.ID

	for _, name := range vi.Authors {
		c.Authors = append(c.Authors, Contributor{
			Name: name,
			Role: "author",
		})
	}

	// Cover: prefer the largest available thumbnail
	if vi.ImageLinks.ExtraLarge != "" {
		c.CoverURL = vi.ImageLinks.ExtraLarge
	} else if vi.ImageLinks.Large != "" {
		c.CoverURL = vi.ImageLinks.Large
	} else if vi.ImageLinks.Medium != "" {
		c.CoverURL = vi.ImageLinks.Medium
	} else if vi.ImageLinks.Thumbnail != "" {
		c.CoverURL = vi.ImageLinks.Thumbnail
	}
	// Google Books thumbnails use HTTP by default; upgrade to HTTPS
	if c.CoverURL != "" {
		c.CoverURL = strings.Replace(c.CoverURL, "http://", "https://", 1)
	}

	// Parse series from subtitle hints like "Book 3 of the Stormlight Archive"
	if series, pos := parseSeriesFromSubtitle(vi.Subtitle); series != "" {
		c.Series = []Series{{Name: series, Position: pos}}
	}

	return c
}

// parseSeriesFromSubtitle extracts a series name and position from strings
// like "Book 3 of the Stormlight Archive" or "Stormlight Archive, Book 3".
var (
	reBookOfSeries  = regexp.MustCompile(`(?i)book\s+(\d+(?:\.\d+)?)\s+of\s+(?:the\s+)?(.+)`)
	reSeriesBookNum = regexp.MustCompile(`(?i)(.+),\s+book\s+(\d+(?:\.\d+)?)`)
)

func parseSeriesFromSubtitle(subtitle string) (name string, pos float64) {
	if m := reBookOfSeries.FindStringSubmatch(subtitle); len(m) == 3 {
		// ParseFloat is safe here: the regex only captures digits and optional decimal.
		p, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return strings.TrimSpace(m[2]), 0
		}
		return strings.TrimSpace(m[2]), p
	}
	if m := reSeriesBookNum.FindStringSubmatch(subtitle); len(m) == 3 {
		p, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return strings.TrimSpace(m[1]), 0
		}
		return strings.TrimSpace(m[1]), p
	}
	return "", 0
}

// ── Google Books API response types ──────────────────────────────────────────

type gbVolumesResponse struct {
	TotalItems int        `json:"totalItems"`
	Items      []gbVolume `json:"items"`
}

type gbVolume struct {
	ID         string       `json:"id"`
	VolumeInfo gbVolumeInfo `json:"volumeInfo"`
}

type gbVolumeInfo struct {
	Title               string              `json:"title"`
	Subtitle            string              `json:"subtitle"`
	Authors             []string            `json:"authors"`
	Publisher           string              `json:"publisher"`
	PublishedDate       string              `json:"publishedDate"`
	Description         string              `json:"description"`
	IndustryIdentifiers []gbIdentifier      `json:"industryIdentifiers"`
	PageCount           int                 `json:"pageCount"`
	Categories          []string            `json:"categories"`
	Language            string              `json:"language"`
	ImageLinks          gbImageLinks        `json:"imageLinks"`
}

type gbIdentifier struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

type gbImageLinks struct {
	SmallThumbnail string `json:"smallThumbnail"`
	Thumbnail      string `json:"thumbnail"`
	Small          string `json:"small"`
	Medium         string `json:"medium"`
	Large          string `json:"large"`
	ExtraLarge     string `json:"extraLarge"`
}
