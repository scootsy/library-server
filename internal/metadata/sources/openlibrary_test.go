package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenLibraryDocToCandidate(t *testing.T) {
	ol := NewOpenLibrary(nil)

	doc := olSearchDoc{
		Key:                 "/works/OL123W",
		Title:               "Dune",
		Subtitle:            "A Novel",
		AuthorName:          []string{"Frank Herbert"},
		FirstPublishYear:    1965,
		ISBN:                []string{"9780441172719", "0441172717"},
		Publisher:           []string{"Ace Books"},
		Language:            []string{"eng"},
		Subject:             []string{"Science Fiction", "Desert"},
		CoverI:              12345,
		NumberOfPagesMedian: 412,
		Series:              []string{"Dune Chronicles"},
	}

	c := ol.docToCandidate(doc)

	if c.Title != "Dune" {
		t.Errorf("Title = %q, want %q", c.Title, "Dune")
	}
	if c.Subtitle != "A Novel" {
		t.Errorf("Subtitle = %q, want %q", c.Subtitle, "A Novel")
	}
	if c.PublishDate != "1965" {
		t.Errorf("PublishDate = %q, want %q", c.PublishDate, "1965")
	}
	if c.PageCount != 412 {
		t.Errorf("PageCount = %d, want %d", c.PageCount, 412)
	}
	if c.Identifiers["isbn_13"] != "9780441172719" {
		t.Errorf("ISBN-13 = %q, want %q", c.Identifiers["isbn_13"], "9780441172719")
	}
	if len(c.Authors) != 1 || c.Authors[0].Name != "Frank Herbert" {
		t.Errorf("Authors = %+v, want [Frank Herbert]", c.Authors)
	}
	if c.CoverURL == "" {
		t.Error("expected non-empty CoverURL")
	}
	if len(c.Series) != 1 || c.Series[0].Name != "Dune Chronicles" {
		t.Errorf("Series = %+v, want [Dune Chronicles]", c.Series)
	}
}

func TestOpenLibraryGet_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ol := NewOpenLibrary(srv.Client())
	body, err := ol.get(context.Background(), srv.URL+"/works/fake.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Errorf("expected nil body for 404, got %d bytes", len(body))
	}
}

func TestOpenLibraryGet_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ol := NewOpenLibrary(srv.Client())
	_, err := ol.get(context.Background(), srv.URL+"/search.json")
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestOpenLibraryWorkToCandidate(t *testing.T) {
	ol := NewOpenLibrary(nil)

	work := olWork{
		Key:         "/works/OL456W",
		Title:       "Neuromancer",
		Description: "A cyberpunk novel",
		Covers:      []int{67890},
		Subjects:    []string{"Cyberpunk", "Science Fiction"},
	}

	c := ol.workToCandidate(work, nil)

	if c.Title != "Neuromancer" {
		t.Errorf("Title = %q, want %q", c.Title, "Neuromancer")
	}
	if c.Description != "A cyberpunk novel" {
		t.Errorf("Description = %q, want %q", c.Description, "A cyberpunk novel")
	}
	if c.CoverURL == "" {
		t.Error("expected non-empty CoverURL from cover ID")
	}
	if len(c.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 items", c.Tags)
	}
}
