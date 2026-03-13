package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const openLibraryBaseURL = "https://openlibrary.org"

// OpenLibrary is a MetadataSource backed by the Open Library REST API.
// No authentication is required.
type OpenLibrary struct {
	httpClient *http.Client
}

// NewOpenLibrary returns an OpenLibrary source.
func NewOpenLibrary() *OpenLibrary {
	return &OpenLibrary{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (o *OpenLibrary) Name() string { return "open_library" }

// Search queries Open Library using ISBN (highest confidence) or title+author.
func (o *OpenLibrary) Search(ctx context.Context, q Query) ([]Candidate, error) {
	params := url.Values{}
	params.Set("limit", "5")
	params.Set("fields", "key,title,subtitle,author_name,first_publish_year,isbn,publisher,language,subject,cover_i,number_of_pages_median,series")

	if q.ISBN != "" {
		params.Set("isbn", q.ISBN)
	} else {
		if q.Title != "" {
			params.Set("title", q.Title)
		}
		if q.Author != "" {
			params.Set("author", q.Author)
		}
	}

	reqURL := openLibraryBaseURL + "/search.json?" + params.Encode()
	raw, err := o.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("open_library search: %w", err)
	}
	if raw == nil {
		return nil, nil
	}

	var result olSearchResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("open_library search decode: %w", err)
	}

	candidates := make([]Candidate, 0, len(result.Docs))
	for _, doc := range result.Docs {
		c := o.docToCandidate(doc)
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// FetchByID retrieves a work or edition by Open Library key or ISBN.
// idType: "olid" → /works/OL…W, "isbn_13"/"isbn_10"/"isbn" → ISBN API.
func (o *OpenLibrary) FetchByID(ctx context.Context, idType, idValue string) (*Candidate, error) {
	switch idType {
	case "olid":
		return o.fetchByOLID(ctx, idValue)
	case "isbn_13", "isbn_10", "isbn":
		return o.fetchByISBN(ctx, idValue)
	default:
		return nil, nil
	}
}

func (o *OpenLibrary) fetchByOLID(ctx context.Context, olid string) (*Candidate, error) {
	// Normalize: strip leading /works/ if present, then add it back.
	olid = strings.TrimPrefix(olid, "/works/")
	reqURL := openLibraryBaseURL + "/works/" + url.PathEscape(olid) + ".json"

	raw, err := o.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("open_library fetch_by_olid: %w", err)
	}
	if raw == nil {
		return nil, nil
	}

	var work olWork
	if err := json.Unmarshal(raw, &work); err != nil {
		return nil, fmt.Errorf("open_library fetch_by_olid decode: %w", err)
	}

	c := o.workToCandidate(work, raw)
	return &c, nil
}

func (o *OpenLibrary) fetchByISBN(ctx context.Context, isbn string) (*Candidate, error) {
	bibKey := "ISBN:" + isbn
	params := url.Values{}
	params.Set("bibkeys", bibKey)
	params.Set("format", "json")
	params.Set("jscmd", "data")

	reqURL := openLibraryBaseURL + "/api/books?" + params.Encode()
	raw, err := o.get(ctx, reqURL)
	if err != nil {
		return nil, fmt.Errorf("open_library fetch_by_isbn: %w", err)
	}
	if raw == nil {
		return nil, nil
	}

	var result map[string]olBooksAPIEntry
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("open_library fetch_by_isbn decode: %w", err)
	}

	entry, ok := result[bibKey]
	if !ok {
		return nil, nil
	}

	c := o.booksAPIToCandidate(entry, raw)
	return &c, nil
}

// get performs an HTTP GET and returns the body, or nil for 404.
func (o *OpenLibrary) get(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := o.httpClient.Do(req)
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
		slog.Warn("open_library unexpected status", "status", resp.StatusCode, "url", reqURL)
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	return body, nil
}

// docToCandidate maps a search result document to a Candidate.
func (o *OpenLibrary) docToCandidate(doc olSearchDoc) Candidate {
	c := Candidate{
		Source:      o.Name(),
		ExternalID:  doc.Key,
		Title:       doc.Title,
		Subtitle:    doc.Subtitle,
		Publisher:   firstString(doc.Publisher),
		Description: "",
		Identifiers: make(map[string]string),
		FetchedAt:   time.Now().UTC(),
		RawData:     doc,
	}

	if doc.FirstPublishYear != 0 {
		c.PublishDate = strconv.Itoa(doc.FirstPublishYear)
	}
	if len(doc.Language) > 0 {
		c.Language = doc.Language[0]
	}
	c.PageCount = doc.NumberOfPagesMedian
	c.Identifiers["olid"] = doc.Key

	for _, isbn := range doc.ISBN {
		switch len(isbn) {
		case 13:
			if _, set := c.Identifiers["isbn_13"]; !set {
				c.Identifiers["isbn_13"] = isbn
			}
		case 10:
			if _, set := c.Identifiers["isbn_10"]; !set {
				c.Identifiers["isbn_10"] = isbn
			}
		}
	}

	for _, name := range doc.AuthorName {
		c.Authors = append(c.Authors, Contributor{Name: name, Role: "author"})
	}

	if doc.CoverI != 0 {
		c.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
	}

	for _, s := range doc.Series {
		c.Series = append(c.Series, Series{Name: s})
	}

	return c
}

// workToCandidate maps a /works/{olid}.json response to a Candidate.
func (o *OpenLibrary) workToCandidate(w olWork, raw []byte) Candidate {
	c := Candidate{
		Source:      o.Name(),
		ExternalID:  w.Key,
		Title:       w.Title,
		Identifiers: make(map[string]string),
		FetchedAt:   time.Now().UTC(),
		RawData:     w,
	}
	c.Identifiers["olid"] = w.Key

	// Description may be a string or an object with "value"
	switch v := w.Description.(type) {
	case string:
		c.Description = v
	case map[string]any:
		if val, ok := v["value"].(string); ok {
			c.Description = val
		}
	}

	for _, covers := range w.Covers {
		if covers > 0 {
			c.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", covers)
			break
		}
	}

	for _, subj := range w.Subjects {
		c.Tags = append(c.Tags, subj)
	}

	return c
}

// booksAPIToCandidate maps an /api/books?jscmd=data entry to a Candidate.
func (o *OpenLibrary) booksAPIToCandidate(e olBooksAPIEntry, raw []byte) Candidate {
	c := Candidate{
		Source:      o.Name(),
		ExternalID:  e.Key,
		Title:       e.Title,
		Publisher:   firstString(e.Publishers),
		PublishDate: e.PublishDate,
		Identifiers: make(map[string]string),
		FetchedAt:   time.Now().UTC(),
		RawData:     e,
	}
	if e.Key != "" {
		c.Identifiers["olid"] = e.Key
	}
	for _, id := range e.Identifiers.ISBN13 {
		c.Identifiers["isbn_13"] = id
		break
	}
	for _, id := range e.Identifiers.ISBN10 {
		c.Identifiers["isbn_10"] = id
		break
	}

	for _, author := range e.Authors {
		c.Authors = append(c.Authors, Contributor{Name: author.Name, Role: "author"})
	}

	if e.Cover.Large != "" {
		c.CoverURL = e.Cover.Large
	} else if e.Cover.Medium != "" {
		c.CoverURL = e.Cover.Medium
	}

	for _, subj := range e.Subjects {
		c.Tags = append(c.Tags, subj.Name)
	}

	c.PageCount = e.NumberOfPages
	return c
}

func firstString(ss []string) string {
	if len(ss) > 0 {
		return ss[0]
	}
	return ""
}

// ── Open Library API response types ──────────────────────────────────────────

type olSearchResponse struct {
	NumFound int           `json:"numFound"`
	Docs     []olSearchDoc `json:"docs"`
}

type olSearchDoc struct {
	Key                 string   `json:"key"`
	Title               string   `json:"title"`
	Subtitle            string   `json:"subtitle"`
	AuthorName          []string `json:"author_name"`
	FirstPublishYear    int      `json:"first_publish_year"`
	ISBN                []string `json:"isbn"`
	Publisher           []string `json:"publisher"`
	Language            []string `json:"language"`
	Subject             []string `json:"subject"`
	CoverI              int      `json:"cover_i"`
	NumberOfPagesMedian int      `json:"number_of_pages_median"`
	Series              []string `json:"series"`
}

type olWork struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description any    `json:"description"` // string or {"type":"…","value":"…"}
	Covers      []int  `json:"covers"`
	Subjects    []string `json:"subjects"`
}

type olBooksAPIEntry struct {
	Key           string         `json:"key"`
	Title         string         `json:"title"`
	Authors       []olAuthorRef  `json:"authors"`
	Publishers    []string       `json:"publishers"`
	PublishDate   string         `json:"publish_date"`
	NumberOfPages int            `json:"number_of_pages"`
	Cover         olCoverLinks   `json:"cover"`
	Subjects      []olSubject    `json:"subjects"`
	Identifiers   olIdentifiers  `json:"identifiers"`
}

type olAuthorRef struct {
	Name string `json:"name"`
}

type olCoverLinks struct {
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

type olSubject struct {
	Name string `json:"name"`
}

type olIdentifiers struct {
	ISBN13 []string `json:"isbn_13"`
	ISBN10 []string `json:"isbn_10"`
}
