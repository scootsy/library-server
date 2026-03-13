package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAudnexusBookToCandidate(t *testing.T) {
	a := NewAudnexus(nil)

	book := audnexusBook{
		ASIN:             "B003B02OO4",
		Title:            "The Way of Kings",
		Authors:          []audnexusPerson{{ASIN: "A1", Name: "Brandon Sanderson"}},
		Narrators:        []audnexusPerson{{ASIN: "N1", Name: "Michael Kramer"}},
		PublisherName:    "Macmillan Audio",
		ReleaseDate:      "2010-08-31",
		Language:         "english",
		Summary:          "An epic fantasy audiobook",
		RuntimeLengthMin: 2700,
		Image:            "https://example.com/cover._SL500_.jpg",
		Genres:           []audnexusGenre{{ASIN: "G1", Name: "Fantasy", Type: "genre"}},
		SeriesPrimary:    []audnexusSeries{{ASIN: "S1", Name: "Stormlight Archive", Position: "1"}},
		SeriesSecondary:  []audnexusSeries{{ASIN: "S2", Name: "Cosmere"}},
	}

	c := a.bookToCandidate(book)

	if c.Title != "The Way of Kings" {
		t.Errorf("Title = %q, want %q", c.Title, "The Way of Kings")
	}
	if c.ExternalID != "B003B02OO4" {
		t.Errorf("ExternalID = %q, want %q", c.ExternalID, "B003B02OO4")
	}
	if c.DurationSecs != 2700*60 {
		t.Errorf("DurationSecs = %d, want %d", c.DurationSecs, 2700*60)
	}
	if len(c.Authors) != 1 || c.Authors[0].Name != "Brandon Sanderson" {
		t.Errorf("Authors = %+v, want [Brandon Sanderson]", c.Authors)
	}
	if len(c.Narrators) != 1 || c.Narrators[0].Name != "Michael Kramer" {
		t.Errorf("Narrators = %+v, want [Michael Kramer]", c.Narrators)
	}
	if c.Identifiers["asin"] != "B003B02OO4" {
		t.Errorf("ASIN identifier = %q, want %q", c.Identifiers["asin"], "B003B02OO4")
	}
	if len(c.Series) != 2 {
		t.Errorf("Series count = %d, want 2", len(c.Series))
	}
	if c.Series[0].Name != "Stormlight Archive" || c.Series[0].Position != 1.0 {
		t.Errorf("Series[0] = %+v, want Stormlight Archive pos 1", c.Series[0])
	}
	if len(c.Tags) != 1 || c.Tags[0] != "Fantasy" {
		t.Errorf("Tags = %v, want [Fantasy]", c.Tags)
	}
	// Cover URL should have SL800 upgrade
	if c.CoverURL != "https://example.com/cover._SL800_.jpg" {
		t.Errorf("CoverURL = %q, want SL800 upgrade", c.CoverURL)
	}
}

func TestAudnexusGet_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	a := NewAudnexus(srv.Client())
	body, err := a.get(context.Background(), srv.URL+"/books/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Errorf("expected nil body for 404, got %d bytes", len(body))
	}
}

func TestAudnexusGet_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := NewAudnexus(srv.Client())
	_, err := a.get(context.Background(), srv.URL+"/books")
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestAudnexusSeriesPositionParsing(t *testing.T) {
	a := NewAudnexus(nil)

	book := audnexusBook{
		ASIN:  "B123",
		Title: "Test",
		SeriesPrimary: []audnexusSeries{
			{Name: "Series A", Position: "2.5"},
			{Name: "Series B", Position: "invalid"},
			{Name: "Series C", Position: ""},
		},
	}

	c := a.bookToCandidate(book)

	if len(c.Series) != 3 {
		t.Fatalf("Series count = %d, want 3", len(c.Series))
	}
	if c.Series[0].Position != 2.5 {
		t.Errorf("Series A position = %f, want 2.5", c.Series[0].Position)
	}
	if c.Series[1].Position != 0 {
		t.Errorf("Series B position = %f, want 0 (invalid input)", c.Series[1].Position)
	}
	if c.Series[2].Position != 0 {
		t.Errorf("Series C position = %f, want 0 (empty input)", c.Series[2].Position)
	}
}
