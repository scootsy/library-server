package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const audnexusBaseURL = "https://api.audnex.us"

// Audnexus is a MetadataSource backed by the Audnexus community API.
// Audnexus provides high-quality audiobook metadata including narrators,
// chapters, and series information sourced from Audible.
// No authentication is required.
type Audnexus struct {
	httpClient *http.Client
}

// NewAudnexus returns an Audnexus source. If httpClient is nil,
// a default client with a 15-second timeout is used.
func NewAudnexus(httpClient *http.Client) *Audnexus {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Audnexus{
		httpClient: httpClient,
	}
}

func (a *Audnexus) Name() string { return "audnexus" }

// Search queries Audnexus by ASIN (highest-confidence) or title+author.
// Audnexus has no general search endpoint; title+author search is approximated
// by querying books with a title filter parameter.
func (a *Audnexus) Search(ctx context.Context, q Query) ([]Candidate, error) {
	if q.ASIN != "" {
		c, err := a.FetchByID(ctx, "asin", q.ASIN)
		if err != nil {
			return nil, err
		}
		if c != nil {
			return []Candidate{*c}, nil
		}
		return nil, nil
	}

	// Audnexus supports a title query parameter for approximate title search.
	if q.Title == "" {
		return nil, nil
	}

	params := url.Values{}
	params.Set("title", q.Title)
	if q.Author != "" {
		params.Set("author", q.Author)
	}

	reqURL := audnexusBaseURL + "/books?" + params.Encode()
	raw, err := a.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("audnexus search: %w", err)
	}
	if raw == nil {
		return nil, nil
	}

	var results []audnexusBook
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, fmt.Errorf("audnexus search decode: %w", err)
	}

	candidates := make([]Candidate, 0, len(results))
	for _, b := range results {
		c := a.bookToCandidate(b)
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// FetchByID retrieves a single Audnexus record by ASIN.
func (a *Audnexus) FetchByID(ctx context.Context, idType, idValue string) (*Candidate, error) {
	if idType != "asin" {
		return nil, nil // Audnexus only indexes by ASIN
	}

	reqURL := audnexusBaseURL + "/books/" + url.PathEscape(idValue)
	raw, err := a.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("audnexus fetch_by_id: %w", err)
	}
	if raw == nil {
		return nil, nil
	}

	var book audnexusBook
	if err := json.Unmarshal(raw, &book); err != nil {
		return nil, fmt.Errorf("audnexus fetch_by_id decode: %w", err)
	}

	// Attempt to also fetch chapter data
	chapters, err := a.fetchChapters(ctx, idValue)
	if err != nil {
		slog.Warn("audnexus failed to fetch chapters", "asin", idValue, "error", err)
	}
	book.Chapters = chapters

	c := a.bookToCandidate(book)
	return &c, nil
}

func (a *Audnexus) fetchChapters(ctx context.Context, asin string) ([]audnexusChapter, error) {
	reqURL := audnexusBaseURL + "/books/" + url.PathEscape(asin) + "/chapters"
	raw, err := a.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("fetching chapters: %w", err)
	}
	if raw == nil {
		return nil, nil
	}

	var result audnexusChaptersResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decoding chapters: %w", err)
	}
	return result.Chapters, nil
}

// get performs an HTTP GET and returns the body, or nil for 404.
func (a *Audnexus) get(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
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
		slog.Warn("audnexus unexpected status", "status", resp.StatusCode, "url", reqURL)
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	return body, nil
}

// bookToCandidate maps an Audnexus book record to a Candidate.
func (a *Audnexus) bookToCandidate(b audnexusBook) Candidate {
	c := Candidate{
		Source:      a.Name(),
		ExternalID:  b.ASIN,
		Title:       b.Title,
		Publisher:   b.PublisherName,
		PublishDate: b.ReleaseDate,
		Language:    b.Language,
		Description: b.Summary,
		DurationSecs: b.RuntimeLengthMin * 60,
		Identifiers: map[string]string{"asin": b.ASIN},
		FetchedAt:   time.Now().UTC(),
		RawData:     b,
	}

	// Authors
	for _, author := range b.Authors {
		c.Authors = append(c.Authors, Contributor{
			Name: author.Name,
			Role: "author",
		})
	}

	// Narrators
	for _, narrator := range b.Narrators {
		c.Narrators = append(c.Narrators, Contributor{
			Name: narrator.Name,
			Role: "narrator",
		})
	}

	// Series
	for _, s := range b.SeriesPrimary {
		pos := 0.0
		if s.Position != "" {
			if _, err := fmt.Sscanf(s.Position, "%f", &pos); err != nil {
				slog.Debug("failed to parse series position", "position", s.Position, "error", err)
			}
		}
		c.Series = append(c.Series, Series{Name: s.Name, Position: pos})
	}
	for _, s := range b.SeriesSecondary {
		c.Series = append(c.Series, Series{Name: s.Name})
	}

	// Genres/tags from Audnexus category ladders
	for _, g := range b.Genres {
		if g.Type == "genre" {
			c.Tags = append(c.Tags, g.Name)
		}
	}

	// Cover — Audnexus returns a cover URL, typically high-res
	if b.Image != "" {
		// Request largest size by appending size parameter
		c.CoverURL = strings.Replace(b.Image, "._SL500_", "._SL800_", 1)
		if c.CoverURL == b.Image {
			c.CoverURL = b.Image // no substitution found, use as-is
		}
	}

	return c
}

// ── Audnexus API response types ───────────────────────────────────────────────

type audnexusBook struct {
	ASIN            string             `json:"asin"`
	Title           string             `json:"title"`
	Authors         []audnexusPerson   `json:"authors"`
	Narrators       []audnexusPerson   `json:"narrators"`
	PublisherName   string             `json:"publisherName"`
	ReleaseDate     string             `json:"releaseDate"`
	Language        string             `json:"language"`
	Summary         string             `json:"summary"`
	RuntimeLengthMin int               `json:"runtimeLengthMin"`
	Image           string             `json:"image"`
	Genres          []audnexusGenre    `json:"genres"`
	SeriesPrimary   []audnexusSeries   `json:"seriesPrimary"`
	SeriesSecondary []audnexusSeries   `json:"seriesSecondary"`
	Chapters        []audnexusChapter  `json:"-"` // populated separately
}

type audnexusPerson struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
}

type audnexusGenre struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
	Type string `json:"type"` // "genre" or "tag"
}

type audnexusSeries struct {
	ASIN     string `json:"asin"`
	Name     string `json:"name"`
	Position string `json:"position"` // string like "1", "2.5"
}

type audnexusChaptersResponse struct {
	ASIN     string            `json:"asin"`
	Chapters []audnexusChapter `json:"chapters"`
}

type audnexusChapter struct {
	Title        string  `json:"title"`
	StartOffset  float64 `json:"startOffsetMs"`  // milliseconds
	LengthMs     float64 `json:"lengthMs"`
}
