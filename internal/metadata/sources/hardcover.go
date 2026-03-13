package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	hardcoverGraphQLEndpoint = "https://api.hardcover.app/v1/graphql"
	hardcoverMaxRetries      = 3
)

var errHardcoverUnavailable = errors.New("hardcover unavailable")

// Hardcover is a MetadataSource backed by the Hardcover GraphQL API.
type Hardcover struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
	authFailed atomic.Bool
}

// NewHardcover returns a Hardcover source. The API key may be provided either
// as the raw token or a value prefixed with "Bearer ".
func NewHardcover(apiKey string, httpClient *http.Client) *Hardcover {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Hardcover{
		apiKey:     strings.TrimSpace(apiKey),
		endpoint:   hardcoverGraphQLEndpoint,
		httpClient: httpClient,
	}
}

func (h *Hardcover) Name() string { return "hardcover" }

// Search queries Hardcover by ISBN when available, otherwise by title/author.
// It fetches full book details for each search hit so downstream code receives
// rich candidates that can be written directly into sidecars.
func (h *Hardcover) Search(ctx context.Context, q Query) ([]Candidate, error) {
	searchQuery, variables := h.buildSearchRequest(q)
	if searchQuery == "" {
		return nil, nil
	}

	var data hardcoverSearchData
	if err := h.graphQL(ctx, hardcoverSearchQuery, variables, &data); err != nil {
		if errors.Is(err, errHardcoverUnavailable) {
			return nil, nil
		}
		return nil, fmt.Errorf("hardcover search: %w", err)
	}

	candidates := make([]Candidate, 0, len(data.Search.Results))
	for _, result := range data.Search.Results {
		base := h.searchResultToCandidate(result)
		if base.ExternalID == "" {
			if base.Title != "" {
				candidates = append(candidates, base)
			}
			continue
		}

		full, err := h.FetchByID(ctx, "hardcover", base.ExternalID)
		if err != nil {
			slog.Warn("hardcover detail fetch failed",
				"id", base.ExternalID,
				"error", err)
			candidates = append(candidates, base)
			continue
		}
		if full == nil {
			candidates = append(candidates, base)
			continue
		}

		candidates = append(candidates, mergeHardcoverCandidate(*full, base))
	}

	return candidates, nil
}

// FetchByID retrieves a Hardcover book by its internal numeric ID.
func (h *Hardcover) FetchByID(ctx context.Context, idType, idValue string) (*Candidate, error) {
	switch idType {
	case "hardcover", "id":
		// continue
	case "isbn", "isbn_10", "isbn_13":
		candidates, err := h.Search(ctx, Query{ISBN: idValue})
		if err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			return nil, nil
		}
		return &candidates[0], nil
	default:
		return nil, nil
	}

	id, err := strconv.Atoi(strings.TrimSpace(idValue))
	if err != nil {
		return nil, fmt.Errorf("invalid hardcover id %q: %w", idValue, err)
	}

	var data hardcoverDetailsData
	if err := h.graphQL(ctx, hardcoverDetailsQuery, map[string]any{"id": id}, &data); err != nil {
		if errors.Is(err, errHardcoverUnavailable) {
			return nil, nil
		}
		return nil, fmt.Errorf("hardcover fetch_by_id: %w", err)
	}
	if data.Book == nil {
		return nil, nil
	}

	candidate := h.bookToCandidate(*data.Book)
	return &candidate, nil
}

func (h *Hardcover) buildSearchRequest(q Query) (string, map[string]any) {
	if q.ISBN != "" {
		return normalizeISBN(q.ISBN), map[string]any{
			"query":   normalizeISBN(q.ISBN),
			"fields":  "isbns",
			"weights": "5",
		}
	}

	var parts []string
	if title := strings.TrimSpace(q.Title); title != "" {
		parts = append(parts, title)
	}
	if author := strings.TrimSpace(q.Author); author != "" {
		parts = append(parts, author)
	}
	query := strings.TrimSpace(strings.Join(parts, " "))
	if query == "" {
		return "", nil
	}

	return query, map[string]any{
		"query": query,
	}
}

func (h *Hardcover) graphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	if strings.TrimSpace(h.apiKey) == "" || h.authFailed.Load() {
		return errHardcoverUnavailable
	}

	payload := hardcoverRequest{
		Query:     query,
		Variables: variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling request body: %w", err)
	}

	raw, err := h.postWithRetry(ctx, body)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}

	var envelope hardcoverResponseEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decoding response envelope: %w", err)
	}
	if len(envelope.Errors) > 0 {
		if hasHardcoverAuthError(envelope.Errors) {
			h.authFailed.Store(true)
			slog.Warn("hardcover authentication failed; disabling source until restart")
			return errHardcoverUnavailable
		}
		return fmt.Errorf("graphql error: %s", envelope.Errors[0].Message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" || out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decoding response data: %w", err)
	}
	return nil
}

func (h *Hardcover) postWithRetry(ctx context.Context, body []byte) ([]byte, error) {
	for attempt := 0; attempt <= hardcoverMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("building request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("authorization", h.authorizationHeader())

		resp, err := h.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing request: %w", err)
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading response body: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("closing response body: %w", closeErr)
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return respBody, nil
		case http.StatusUnauthorized, http.StatusForbidden:
			h.authFailed.Store(true)
			slog.Warn("hardcover authentication failed",
				"status", resp.StatusCode,
				"endpoint", sanitizeURL(h.endpoint))
			return nil, errHardcoverUnavailable
		case http.StatusTooManyRequests:
			if attempt == hardcoverMaxRetries {
				return nil, fmt.Errorf("rate limited after %d retries", hardcoverMaxRetries)
			}
			backoff := hardcoverRetryDelay(attempt, resp.Header.Get("Retry-After"))
			slog.Warn("hardcover rate limited, backing off",
				"attempt", attempt+1,
				"retry_in", backoff.String())
			if err := sleepWithContext(ctx, backoff); err != nil {
				return nil, err
			}
			continue
		default:
			return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
		}
	}

	return nil, fmt.Errorf("hardcover request exhausted retries")
}

func (h *Hardcover) authorizationHeader() string {
	return strings.TrimSpace(h.apiKey)
}

func (h *Hardcover) searchResultToCandidate(result hardcoverSearchBook) Candidate {
	candidate := Candidate{
		Source:      h.Name(),
		ExternalID:  rawMessageToString(result.ID),
		Title:       strings.TrimSpace(result.Title),
		Identifiers: make(map[string]string),
		FetchedAt:   time.Now().UTC(),
		RawData:     result,
		CoverURL:    imageURLFromRaw(result.CoverURL),
	}

	if candidate.ExternalID != "" {
		candidate.Identifiers["hardcover"] = candidate.ExternalID
	}
	if result.ReleaseYear != 0 {
		candidate.PublishDate = strconv.Itoa(result.ReleaseYear)
	}
	for _, name := range result.AuthorNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		candidate.Authors = append(candidate.Authors, Contributor{
			Name: name,
			Role: "author",
		})
	}
	for _, isbn := range result.ISBNs {
		addISBNIdentifier(candidate.Identifiers, isbn)
	}
	for _, name := range result.SeriesNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		candidate.Series = append(candidate.Series, Series{Name: name})
	}

	return candidate
}

func (h *Hardcover) bookToCandidate(book hardcoverBook) Candidate {
	now := time.Now().UTC()
	candidate := Candidate{
		Source:      h.Name(),
		ExternalID:  strconv.Itoa(book.ID),
		Title:       strings.TrimSpace(book.Title),
		Subtitle:    strings.TrimSpace(book.Subtitle),
		Description: strings.TrimSpace(book.Description),
		Identifiers: map[string]string{"hardcover": strconv.Itoa(book.ID)},
		FetchedAt:   now,
		RawData:     book,
	}

	physical := book.DefaultPhysicalEdition
	audio := book.DefaultAudioEdition

	candidate.PublishDate = firstNonEmpty(
		strings.TrimSpace(book.ReleaseDate),
		editionReleaseDate(physical),
		editionReleaseDate(audio),
	)
	candidate.Publisher = firstNonEmpty(
		editionPublisher(physical),
		editionPublisher(audio),
	)
	candidate.Language = firstNonEmpty(
		normalizeHardcoverLanguage(editionLanguage(physical)),
		normalizeHardcoverLanguage(editionLanguage(audio)),
	)
	candidate.PageCount = firstPositive(
		book.Pages,
		editionPages(physical),
		editionPages(audio),
	)
	candidate.DurationSecs = firstPositive(
		book.AudioSeconds,
		editionAudioSeconds(audio),
	)
	candidate.CoverURL = firstNonEmpty(
		editionImageURL(physical),
		editionImageURL(audio),
		imageURLFromRaw(book.Image),
	)

	for _, isbn := range book.ISBNs {
		addISBNIdentifier(candidate.Identifiers, isbn)
	}
	addISBNIdentifier(candidate.Identifiers, editionISBN13(physical))
	addISBNIdentifier(candidate.Identifiers, editionISBN10(physical))
	addISBNIdentifier(candidate.Identifiers, editionISBN13(audio))
	addISBNIdentifier(candidate.Identifiers, editionISBN10(audio))

	addHardcoverContributions(&candidate, book.Contributions)
	if len(candidate.Authors) == 0 {
		for _, name := range book.AuthorNames {
			appendUniqueContributor(&candidate.Authors, Contributor{
				Name: strings.TrimSpace(name),
				Role: "author",
			})
		}
	}

	audioAuthors, audioNarrators := parseCachedContributors(audioCachedContributors(audio))
	for _, contributor := range audioAuthors {
		appendUniqueContributor(&candidate.Authors, contributor)
	}
	for _, contributor := range audioNarrators {
		appendUniqueContributor(&candidate.Narrators, contributor)
	}

	for _, tag := range book.Tags {
		appendUniqueString(&candidate.Tags, tag)
	}

	addHardcoverSeries(&candidate.Series, book.FeaturedBookSeries)
	for _, series := range book.BookSeries {
		addHardcoverSeries(&candidate.Series, series)
	}
	for _, name := range book.SeriesNames {
		if strings.TrimSpace(name) == "" {
			continue
		}
		appendUniqueSeries(&candidate.Series, Series{Name: strings.TrimSpace(name)})
	}

	if book.Rating > 0 {
		candidate.Rating = &Rating{
			Score:     book.Rating,
			Max:       5,
			Count:     book.RatingsCount,
			FetchedAt: now,
		}
	}

	return candidate
}

func mergeHardcoverCandidate(detail Candidate, search Candidate) Candidate {
	if detail.Title == "" {
		detail.Title = search.Title
	}
	if detail.PublishDate == "" {
		detail.PublishDate = search.PublishDate
	}
	if detail.CoverURL == "" {
		detail.CoverURL = search.CoverURL
	}
	if len(detail.Authors) == 0 {
		detail.Authors = append(detail.Authors, search.Authors...)
	}
	if len(detail.Series) == 0 {
		detail.Series = append(detail.Series, search.Series...)
	}
	if detail.Identifiers == nil {
		detail.Identifiers = make(map[string]string)
	}
	for key, value := range search.Identifiers {
		if value == "" {
			continue
		}
		if _, ok := detail.Identifiers[key]; !ok {
			detail.Identifiers[key] = value
		}
	}
	return detail
}

func addHardcoverContributions(candidate *Candidate, contributions []hardcoverContribution) {
	for _, contribution := range contributions {
		name := ""
		if contribution.Author != nil {
			name = strings.TrimSpace(contribution.Author.Name)
		}
		if name == "" {
			continue
		}

		switch normalizeContributionRole(contribution.Contribution) {
		case "narrator":
			appendUniqueContributor(&candidate.Narrators, Contributor{
				Name: name,
				Role: "narrator",
			})
		default:
			appendUniqueContributor(&candidate.Authors, Contributor{
				Name: name,
				Role: "author",
			})
		}
	}
}

func addHardcoverSeries(seriesList *[]Series, item *hardcoverBookSeries) {
	if item == nil || item.Series == nil {
		return
	}
	series := Series{
		Name:     strings.TrimSpace(item.Series.Name),
		Position: parseSeriesPosition(item.Position),
	}
	if series.Name == "" {
		return
	}
	appendUniqueSeries(seriesList, series)
}

func appendUniqueContributor(list *[]Contributor, contributor Contributor) {
	if strings.TrimSpace(contributor.Name) == "" {
		return
	}
	for _, existing := range *list {
		if strings.EqualFold(existing.Name, contributor.Name) && existing.Role == contributor.Role {
			return
		}
	}
	*list = append(*list, contributor)
}

func appendUniqueSeries(list *[]Series, series Series) {
	if strings.TrimSpace(series.Name) == "" {
		return
	}
	for i, existing := range *list {
		if !strings.EqualFold(existing.Name, series.Name) {
			continue
		}
		if existing.Position == 0 && series.Position > 0 {
			(*list)[i].Position = series.Position
		}
		return
	}
	*list = append(*list, series)
}

func appendUniqueString(list *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range *list {
		if strings.EqualFold(existing, value) {
			return
		}
	}
	*list = append(*list, value)
}

func parseCachedContributors(raw json.RawMessage) ([]Contributor, []Contributor) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, nil
	}

	var authors []Contributor
	var narrators []Contributor
	for _, row := range rows {
		name := nestedString(row, "author", "name")
		if name == "" {
			name = stringValue(row["name"])
		}
		role := normalizeContributionRole(firstNonEmpty(
			stringValue(row["contribution"]),
			stringValue(row["role"]),
		))
		if name == "" {
			continue
		}
		switch role {
		case "narrator":
			appendUniqueContributor(&narrators, Contributor{Name: name, Role: "narrator"})
		default:
			appendUniqueContributor(&authors, Contributor{Name: name, Role: "author"})
		}
	}

	return authors, narrators
}

func addISBNIdentifier(identifiers map[string]string, raw string) {
	isbn := normalizeISBN(strings.TrimSpace(raw))
	switch len(isbn) {
	case 10:
		if identifiers["isbn_10"] == "" {
			identifiers["isbn_10"] = isbn
		}
	case 13:
		if identifiers["isbn_13"] == "" {
			identifiers["isbn_13"] = isbn
		}
	}
}

func parseSeriesPosition(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var asFloat float64
	if err := json.Unmarshal(raw, &asFloat); err == nil {
		return asFloat
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		f, err := strconv.ParseFloat(strings.TrimSpace(asString), 64)
		if err == nil {
			return f
		}
	}
	return 0
}

func rawMessageToString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return strconv.Itoa(asInt)
	}
	var asFloat float64
	if err := json.Unmarshal(raw, &asFloat); err == nil {
		return strconv.FormatInt(int64(asFloat), 10)
	}
	return ""
}

func imageURLFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var wrapped struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return strings.TrimSpace(wrapped.URL)
	}
	return ""
}

func normalizeContributionRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "", "author", "authors":
		return "author"
	case "narrator", "narration":
		return "narrator"
	default:
		return role
	}
}

func normalizeHardcoverLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return ""
	}
	switch strings.ToLower(language) {
	case "english":
		return "en"
	default:
		return strings.ToLower(language)
	}
}

func hardcoverRetryDelay(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}

	delay := time.Second << attempt
	if delay > 8*time.Second {
		return 8 * time.Second
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func hasHardcoverAuthError(errs []hardcoverGraphQLError) bool {
	for _, err := range errs {
		msg := strings.ToLower(err.Message)
		if strings.Contains(msg, "authorization") ||
			strings.Contains(msg, "unauthorized") ||
			strings.Contains(msg, "forbidden") ||
			strings.Contains(msg, "invalid token") {
			return true
		}
	}
	return false
}

func stringValue(value any) string {
	str, _ := value.(string)
	return strings.TrimSpace(str)
}

func nestedString(row map[string]any, keys ...string) string {
	current := any(row)
	for _, key := range keys {
		asMap, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = asMap[key]
	}
	return stringValue(current)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func editionReleaseDate(edition *hardcoverEdition) string {
	if edition == nil {
		return ""
	}
	return strings.TrimSpace(edition.ReleaseDate)
}

func editionPublisher(edition *hardcoverEdition) string {
	if edition == nil || edition.Publisher == nil {
		return ""
	}
	return strings.TrimSpace(edition.Publisher.Name)
}

func editionLanguage(edition *hardcoverEdition) string {
	if edition == nil || edition.Language == nil {
		return ""
	}
	return strings.TrimSpace(edition.Language.Language)
}

func editionPages(edition *hardcoverEdition) int {
	if edition == nil {
		return 0
	}
	return edition.Pages
}

func editionAudioSeconds(edition *hardcoverEdition) int {
	if edition == nil {
		return 0
	}
	return edition.AudioSeconds
}

func editionISBN10(edition *hardcoverEdition) string {
	if edition == nil {
		return ""
	}
	return edition.ISBN10
}

func editionISBN13(edition *hardcoverEdition) string {
	if edition == nil {
		return ""
	}
	return edition.ISBN13
}

func editionImageURL(edition *hardcoverEdition) string {
	if edition == nil {
		return ""
	}
	return imageURLFromRaw(edition.Image)
}

func audioCachedContributors(edition *hardcoverEdition) json.RawMessage {
	if edition == nil {
		return nil
	}
	return edition.CachedContributors
}

const hardcoverSearchQuery = `
query SearchBooks($query: String!, $fields: String, $weights: String) {
  search(query: $query, query_type: "Book", per_page: 10, fields: $fields, weights: $weights) {
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
}`

const hardcoverDetailsQuery = `
query GetBookDetails($id: Int!) {
  books_by_pk(id: $id) {
    id
    title
    subtitle
    description
    release_date
    pages
    audio_seconds
    isbns
    slug
    image
    author_names
    series_names
    tags
    rating
    ratings_count
    users_read_count
    contributions {
      contribution
      author {
        name
      }
    }
    featured_book_series {
      position
      series {
        id
        name
      }
    }
    book_series {
      position
      series {
        id
        name
      }
    }
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
      language {
        language
      }
      release_date
      audio_seconds
      image {
        url
      }
      cached_contributors
    }
  }
}`

type hardcoverRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type hardcoverResponseEnvelope struct {
	Data   json.RawMessage      `json:"data"`
	Errors []hardcoverGraphQLError `json:"errors"`
}

type hardcoverGraphQLError struct {
	Message string `json:"message"`
}

type hardcoverSearchData struct {
	Search struct {
		Results []hardcoverSearchBook `json:"results"`
	} `json:"search"`
}

type hardcoverSearchBook struct {
	ID          json.RawMessage `json:"id"`
	Title       string          `json:"title"`
	AuthorNames []string        `json:"author_names"`
	CoverURL    json.RawMessage `json:"cover_url"`
	ReleaseYear int             `json:"release_year"`
	ISBNs       []string        `json:"isbns"`
	SeriesNames []string        `json:"series_names"`
}

type hardcoverDetailsData struct {
	Book *hardcoverBook `json:"books_by_pk"`
}

type hardcoverBook struct {
	ID                   int                    `json:"id"`
	Title                string                 `json:"title"`
	Subtitle             string                 `json:"subtitle"`
	Description          string                 `json:"description"`
	ReleaseDate          string                 `json:"release_date"`
	Pages                int                    `json:"pages"`
	AudioSeconds         int                    `json:"audio_seconds"`
	ISBNs                []string               `json:"isbns"`
	Slug                 string                 `json:"slug"`
	Image                json.RawMessage        `json:"image"`
	AuthorNames          []string               `json:"author_names"`
	SeriesNames          []string               `json:"series_names"`
	Tags                 []string               `json:"tags"`
	Rating               float64                `json:"rating"`
	RatingsCount         int                    `json:"ratings_count"`
	UsersReadCount       int                    `json:"users_read_count"`
	Contributions        []hardcoverContribution `json:"contributions"`
	FeaturedBookSeries   *hardcoverBookSeries   `json:"featured_book_series"`
	BookSeries           []hardcoverBookSeries  `json:"book_series"`
	DefaultPhysicalEdition *hardcoverEdition    `json:"default_physical_edition"`
	DefaultAudioEdition  *hardcoverEdition      `json:"default_audio_edition"`
}

type hardcoverContribution struct {
	Contribution string           `json:"contribution"`
	Author       *hardcoverAuthor `json:"author"`
}

type hardcoverAuthor struct {
	Name string `json:"name"`
}

type hardcoverBookSeries struct {
	Position json.RawMessage  `json:"position"`
	Series   *hardcoverSeries `json:"series"`
}

type hardcoverSeries struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type hardcoverEdition struct {
	ISBN10            string          `json:"isbn_10"`
	ISBN13            string          `json:"isbn_13"`
	Pages             int             `json:"pages"`
	ReleaseDate       string          `json:"release_date"`
	AudioSeconds      int             `json:"audio_seconds"`
	CachedContributors json.RawMessage `json:"cached_contributors"`
	Publisher         *hardcoverPublisher `json:"publisher"`
	Language          *hardcoverLanguage  `json:"language"`
	Image             json.RawMessage     `json:"image"`
}

type hardcoverPublisher struct {
	Name string `json:"name"`
}

type hardcoverLanguage struct {
	Language string `json:"language"`
}
