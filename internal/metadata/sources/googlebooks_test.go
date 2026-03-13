package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleBooksSearch(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"totalItems": 1,
			"items": [{
				"id": "vol-123",
				"volumeInfo": {
					"title": "The Way of Kings",
					"subtitle": "Book 1 of the Stormlight Archive",
					"authors": ["Brandon Sanderson"],
					"publisher": "Tor Books",
					"publishedDate": "2010-08-31",
					"description": "Epic fantasy novel",
					"pageCount": 1007,
					"language": "en",
					"industryIdentifiers": [
						{"type": "ISBN_13", "identifier": "9780765326355"},
						{"type": "ISBN_10", "identifier": "0765326353"}
					],
					"imageLinks": {
						"thumbnail": "http://books.google.com/thumb.jpg"
					}
				}
			}]
		}`))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Override the base URL by using the test server's client
	gb := NewGoogleBooks("", srv.Client())

	// We can't easily override the base URL constant, so test the parsing via
	// a direct FetchByID with a mocked server isn't straightforward.
	// Instead, test the volumeToCandidate conversion directly.
	vol := gbVolume{
		ID: "vol-123",
		VolumeInfo: gbVolumeInfo{
			Title:         "The Way of Kings",
			Subtitle:      "Book 1 of the Stormlight Archive",
			Authors:       []string{"Brandon Sanderson"},
			Publisher:     "Tor Books",
			PublishedDate: "2010-08-31",
			Description:   "Epic fantasy novel",
			PageCount:     1007,
			Language:      "en",
			IndustryIdentifiers: []gbIdentifier{
				{Type: "ISBN_13", Identifier: "9780765326355"},
				{Type: "ISBN_10", Identifier: "0765326353"},
			},
			ImageLinks: gbImageLinks{
				Thumbnail: "http://books.google.com/thumb.jpg",
			},
		},
	}

	c := gb.volumeToCandidate(vol)

	if c.Title != "The Way of Kings" {
		t.Errorf("Title = %q, want %q", c.Title, "The Way of Kings")
	}
	if c.Publisher != "Tor Books" {
		t.Errorf("Publisher = %q, want %q", c.Publisher, "Tor Books")
	}
	if c.PageCount != 1007 {
		t.Errorf("PageCount = %d, want %d", c.PageCount, 1007)
	}
	if c.Identifiers["isbn_13"] != "9780765326355" {
		t.Errorf("ISBN-13 = %q, want %q", c.Identifiers["isbn_13"], "9780765326355")
	}
	if c.Identifiers["isbn_10"] != "0765326353" {
		t.Errorf("ISBN-10 = %q, want %q", c.Identifiers["isbn_10"], "0765326353")
	}
	if len(c.Authors) != 1 || c.Authors[0].Name != "Brandon Sanderson" {
		t.Errorf("Authors = %+v, want [Brandon Sanderson]", c.Authors)
	}
	// Cover URL should be upgraded to HTTPS
	if c.CoverURL != "https://books.google.com/thumb.jpg" {
		t.Errorf("CoverURL = %q, want HTTPS upgrade", c.CoverURL)
	}
	// Series parsed from subtitle
	if len(c.Series) != 1 || c.Series[0].Name != "Stormlight Archive" {
		t.Errorf("Series = %+v, want [Stormlight Archive]", c.Series)
	}
	if c.Series[0].Position != 1 {
		t.Errorf("Series position = %f, want 1", c.Series[0].Position)
	}
}

func TestGoogleBooksBuildQuery(t *testing.T) {
	gb := NewGoogleBooks("", nil)

	tests := []struct {
		name  string
		query Query
		want  string
	}{
		{"isbn", Query{ISBN: "9780765326355"}, "isbn:9780765326355"},
		{"title only", Query{Title: "The Way of Kings"}, "intitle:The Way of Kings"},
		{"title and author", Query{Title: "Mistborn", Author: "Sanderson"}, "intitle:Mistborn+inauthor:Sanderson"},
		{"empty", Query{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gb.buildQuery(tt.query)
			if got != tt.want {
				t.Errorf("buildQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoogleBooksGet_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	gb := NewGoogleBooks("", srv.Client())
	body, err := gb.get(context.Background(), srv.URL+"/volumes/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Errorf("expected nil body for 404, got %d bytes", len(body))
	}
}

func TestGoogleBooksGet_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	gb := NewGoogleBooks("", srv.Client())
	_, err := gb.get(context.Background(), srv.URL+"/volumes")
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestParseSeriesFromSubtitle(t *testing.T) {
	tests := []struct {
		subtitle string
		wantName string
		wantPos  float64
	}{
		{"Book 1 of the Stormlight Archive", "Stormlight Archive", 1},
		{"Book 3 of Mistborn", "Mistborn", 3},
		{"Stormlight Archive, Book 2", "Stormlight Archive", 2},
		{"No series info here", "", 0},
		{"", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.subtitle, func(t *testing.T) {
			name, pos := parseSeriesFromSubtitle(tt.subtitle)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if pos != tt.wantPos {
				t.Errorf("pos = %f, want %f", pos, tt.wantPos)
			}
		})
	}
}
