package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHardcoverSearchFetchesDetails(t *testing.T) {
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("authorization"); got != "test-token" {
			t.Errorf("authorization header = %q, want %q", got, "test-token")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		query, _ := body["query"].(string)

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(query, "SearchBooks"):
			_, _ = w.Write([]byte(`{
				"data": {
					"search": {
						"results": [{
							"id": 321,
							"title": "James",
							"author_names": ["Percival Everett"],
							"cover_url": "https://images.example/search.jpg",
							"release_year": 2024,
							"isbns": ["9780385550369"],
							"series_names": ["Standalone"]
						}]
					}
				}
			}`))
		case strings.Contains(query, "GetBookDetails"):
			_, _ = w.Write([]byte(`{
				"data": {
					"books_by_pk": {
						"id": 321,
						"title": "James",
						"subtitle": "A Novel",
						"description": "A retelling of Huckleberry Finn.",
						"release_date": "2024-03-19",
						"pages": 320,
						"audio_seconds": 42000,
						"isbns": ["9780385550369"],
						"slug": "james",
						"image": "https://images.example/book.jpg",
						"author_names": ["Percival Everett"],
						"tags": ["Historical Fiction", "Literary Fiction"],
						"rating": 4.6,
						"ratings_count": 1280,
						"users_read_count": 9000,
						"contributions": [{
							"contribution": "Author",
							"author": {"name": "Percival Everett"}
						}],
						"featured_book_series": {
							"position": "1",
							"series": {"id": 99, "name": "Booker Shortlist"}
						},
						"book_series": [],
						"default_physical_edition": {
							"isbn_10": "0385550367",
							"isbn_13": "9780385550369",
							"pages": 320,
							"publisher": {"name": "Doubleday"},
							"language": {"language": "English"},
							"image": {"url": "https://images.example/physical.jpg"},
							"release_date": "2024-03-19"
						},
						"default_audio_edition": {
							"isbn_10": "0593942535",
							"isbn_13": "9780593942536",
							"pages": 320,
							"publisher": {"name": "Random House Audio"},
							"language": {"language": "English"},
							"release_date": "2024-03-19",
							"audio_seconds": 42000,
							"image": {"url": "https://images.example/audio.jpg"},
							"cached_contributors": [{
								"contribution": "Narrator",
								"author": {"name": "Dominic Hoffman"}
							}]
						}
					}
				}
			}`))
		default:
			t.Errorf("unexpected query: %s", query)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}))
	defer srv.Close()

	h := NewHardcover("test-token", srv.Client())
	h.endpoint = srv.URL

	candidates, err := h.Search(context.Background(), Query{
		Title:  "James",
		Author: "Percival Everett",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("requestCount = %d, want 2", requestCount)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}

	c := candidates[0]
	if c.ExternalID != "321" {
		t.Errorf("ExternalID = %q, want %q", c.ExternalID, "321")
	}
	if c.Title != "James" {
		t.Errorf("Title = %q, want %q", c.Title, "James")
	}
	if c.Subtitle != "A Novel" {
		t.Errorf("Subtitle = %q, want %q", c.Subtitle, "A Novel")
	}
	if c.Publisher != "Doubleday" {
		t.Errorf("Publisher = %q, want %q", c.Publisher, "Doubleday")
	}
	if c.Language != "en" {
		t.Errorf("Language = %q, want %q", c.Language, "en")
	}
	if c.PublishDate != "2024-03-19" {
		t.Errorf("PublishDate = %q, want %q", c.PublishDate, "2024-03-19")
	}
	if c.PageCount != 320 {
		t.Errorf("PageCount = %d, want %d", c.PageCount, 320)
	}
	if c.DurationSecs != 42000 {
		t.Errorf("DurationSecs = %d, want %d", c.DurationSecs, 42000)
	}
	if c.CoverURL != "https://images.example/physical.jpg" {
		t.Errorf("CoverURL = %q, want %q", c.CoverURL, "https://images.example/physical.jpg")
	}
	if len(c.Authors) != 1 || c.Authors[0].Name != "Percival Everett" {
		t.Errorf("Authors = %+v, want Percival Everett", c.Authors)
	}
	if len(c.Narrators) != 1 || c.Narrators[0].Name != "Dominic Hoffman" {
		t.Errorf("Narrators = %+v, want Dominic Hoffman", c.Narrators)
	}
	if len(c.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 tags", c.Tags)
	}
	if len(c.Series) != 1 || c.Series[0].Name != "Booker Shortlist" || c.Series[0].Position != 1 {
		t.Errorf("Series = %+v, want Booker Shortlist #1", c.Series)
	}
	if c.Identifiers["hardcover"] != "321" {
		t.Errorf("hardcover identifier = %q, want %q", c.Identifiers["hardcover"], "321")
	}
	if c.Identifiers["isbn_13"] != "9780385550369" {
		t.Errorf("isbn_13 = %q, want %q", c.Identifiers["isbn_13"], "9780385550369")
	}
	if c.Rating == nil {
		t.Fatal("expected rating to be set")
	}
	if c.Rating.Score != 4.6 || c.Rating.Count != 1280 || c.Rating.Max != 5 {
		t.Errorf("Rating = %+v, want score 4.6 count 1280 max 5", c.Rating)
	}
}

func TestHardcoverSearchByISBNSetsSearchWeights(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		variables, ok := body["variables"].(map[string]any)
		if !ok {
			t.Errorf("variables = %#v, want map", body["variables"])
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if variables["query"] != "9780385550369" {
			t.Errorf("variables.query = %#v, want %q", variables["query"], "9780385550369")
		}
		if variables["fields"] != "isbns" {
			t.Errorf("variables.fields = %#v, want %q", variables["fields"], "isbns")
		}
		if variables["weights"] != "5" {
			t.Errorf("variables.weights = %#v, want %q", variables["weights"], "5")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"search":{"results":[]}}}`))
	}))
	defer srv.Close()

	h := NewHardcover("test-token", srv.Client())
	h.endpoint = srv.URL

	candidates, err := h.Search(context.Background(), Query{ISBN: "978-0385550369"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(candidates))
	}
}

func TestHardcoverAuthFailureDisablesSource(t *testing.T) {
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	h := NewHardcover("bad-token", srv.Client())
	h.endpoint = srv.URL

	candidates, err := h.Search(context.Background(), Query{Title: "James"})
	if err != nil {
		t.Fatalf("Search returned error, want graceful skip: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(candidates))
	}

	candidates, err = h.Search(context.Background(), Query{Title: "James"})
	if err != nil {
		t.Fatalf("second Search returned error, want graceful skip: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("second len(candidates) = %d, want 0", len(candidates))
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1 after auth disable", requestCount)
	}
}

func TestParseCachedContributors(t *testing.T) {
	raw := json.RawMessage(`[
		{"contribution":"Author","author":{"name":"Percival Everett"}},
		{"contribution":"Narrator","author":{"name":"Dominic Hoffman"}}
	]`)

	authors, narrators := parseCachedContributors(raw)
	if len(authors) != 1 || authors[0].Name != "Percival Everett" {
		t.Errorf("authors = %+v, want Percival Everett", authors)
	}
	if len(narrators) != 1 || narrators[0].Name != "Dominic Hoffman" {
		t.Errorf("narrators = %+v, want Dominic Hoffman", narrators)
	}
}
