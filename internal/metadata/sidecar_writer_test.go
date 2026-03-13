package metadata

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scootsy/library-server/internal/metadata/sources"
	"github.com/scootsy/library-server/internal/scanner"
)

func TestMergeCandidateIntoSidecar_BasicFields(t *testing.T) {
	sc := &scanner.Sidecar{
		SchemaVersion: 1,
		Title:         "Old Title",
	}

	c := sources.Candidate{
		Source:      "google_books",
		Title:       "New Title",
		Subtitle:    "A Subtitle",
		Publisher:   "Tor Books",
		PublishDate: "2010-08-31",
		Language:    "en",
		Description: "A great book.",
		PageCount:   1007,
		Authors: []sources.Contributor{
			{Name: "Brandon Sanderson", Role: "author"},
		},
		Series: []sources.Series{
			{Name: "The Stormlight Archive", Position: 1},
		},
		Tags:        []string{"Fantasy", "Epic"},
		Identifiers: map[string]string{"isbn_13": "9780765326355"},
	}

	locked := map[string]bool{}
	mergeCandidateIntoSidecar(sc, c, locked)

	if sc.Title != "New Title" {
		t.Errorf("Title = %q, want %q", sc.Title, "New Title")
	}
	if sc.Subtitle != "A Subtitle" {
		t.Errorf("Subtitle = %q, want %q", sc.Subtitle, "A Subtitle")
	}
	if sc.Publisher != "Tor Books" {
		t.Errorf("Publisher = %q, want %q", sc.Publisher, "Tor Books")
	}
	if sc.Language != "en" {
		t.Errorf("Language = %q, want %q", sc.Language, "en")
	}
	if sc.Description != "A great book." {
		t.Errorf("Description = %q, want %q", sc.Description, "A great book.")
	}
	if sc.PageCount != 1007 {
		t.Errorf("PageCount = %d, want %d", sc.PageCount, 1007)
	}
	if len(sc.Contributors) != 1 {
		t.Fatalf("Contributors = %d, want 1", len(sc.Contributors))
	}
	if sc.Contributors[0].Name != "Brandon Sanderson" {
		t.Errorf("Contributor[0].Name = %q, want %q", sc.Contributors[0].Name, "Brandon Sanderson")
	}
	if len(sc.Series) != 1 {
		t.Fatalf("Series = %d, want 1", len(sc.Series))
	}
	if sc.Series[0].Name != "The Stormlight Archive" {
		t.Errorf("Series[0].Name = %q, want %q", sc.Series[0].Name, "The Stormlight Archive")
	}
	if sc.Series[0].Position == nil || *sc.Series[0].Position != 1 {
		t.Error("Series[0].Position should be 1")
	}
	if len(sc.Tags) != 2 {
		t.Errorf("Tags = %d, want 2", len(sc.Tags))
	}
	if sc.Identifiers == nil {
		t.Fatal("expected identifiers to be set")
	}
	if v := sc.Identifiers["isbn_13"]; v == nil || *v != "9780765326355" {
		t.Error("expected isbn_13 identifier")
	}
}

func TestMergeCandidateIntoSidecar_LockedFields(t *testing.T) {
	sc := &scanner.Sidecar{
		SchemaVersion: 1,
		Title:         "User's Custom Title",
		Publisher:     "Custom Publisher",
		Tags:          []string{"UserTag"},
	}

	c := sources.Candidate{
		Source:    "google_books",
		Title:     "API Title",
		Publisher: "API Publisher",
		Tags:      []string{"APITag"},
	}

	locked := map[string]bool{
		"title":     true,
		"publisher": true,
	}
	mergeCandidateIntoSidecar(sc, c, locked)

	// Locked fields should not be changed
	if sc.Title != "User's Custom Title" {
		t.Errorf("Title changed despite being locked: %q", sc.Title)
	}
	if sc.Publisher != "Custom Publisher" {
		t.Errorf("Publisher changed despite being locked: %q", sc.Publisher)
	}
	// Tags are not locked, so APITag should be appended
	if len(sc.Tags) != 2 {
		t.Errorf("Tags = %d, want 2 (UserTag + APITag)", len(sc.Tags))
	}
}

func TestMergeCandidateIntoSidecar_TagsAppendUnique(t *testing.T) {
	sc := &scanner.Sidecar{
		SchemaVersion: 1,
		Tags:          []string{"Fantasy", "Existing"},
	}

	c := sources.Candidate{
		Tags: []string{"fantasy", "New Tag", "Fantasy"}, // duplicate in different case
	}

	locked := map[string]bool{}
	mergeCandidateIntoSidecar(sc, c, locked)

	// Should have: Fantasy, Existing, New Tag (no duplicate fantasy)
	if len(sc.Tags) != 3 {
		t.Errorf("Tags = %v, want 3 unique tags", sc.Tags)
	}
}

func TestMergeCandidateIntoSidecar_IdentifiersMergeOnly(t *testing.T) {
	existing := "9780765326355"
	sc := &scanner.Sidecar{
		SchemaVersion: 1,
		Identifiers: map[string]*string{
			"isbn_13": &existing,
		},
	}

	c := sources.Candidate{
		Identifiers: map[string]string{
			"isbn_13": "9999999999999", // should NOT overwrite existing
			"asin":    "B001QKBHG4",   // should be added (new)
		},
	}

	locked := map[string]bool{}
	mergeCandidateIntoSidecar(sc, c, locked)

	if v := sc.Identifiers["isbn_13"]; v == nil || *v != "9780765326355" {
		t.Error("isbn_13 should not be overwritten when already present")
	}
	if v := sc.Identifiers["asin"]; v == nil || *v != "B001QKBHG4" {
		t.Error("asin should be added as new identifier")
	}
}

func TestMergeCandidateIntoSidecar_RatingsBySource(t *testing.T) {
	sc := &scanner.Sidecar{SchemaVersion: 1}

	c := sources.Candidate{
		Source: "hardcover",
		Rating: &sources.Rating{
			Score:     4.7,
			Max:       5,
			Count:     1520,
			FetchedAt: time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC),
		},
	}

	mergeCandidateIntoSidecar(sc, c, map[string]bool{})

	if sc.Ratings == nil {
		t.Fatal("expected ratings to be initialized")
	}
	rating := sc.Ratings["hardcover"]
	if rating == nil {
		t.Fatal("expected hardcover rating entry")
	}
	if rating.Score != 4.7 || rating.Max != 5 || rating.Count != 1520 {
		t.Errorf("rating = %+v, want score 4.7 max 5 count 1520", rating)
	}
}

func TestMergeCandidateIntoSidecar_SortTitleDerivation(t *testing.T) {
	sc := &scanner.Sidecar{SchemaVersion: 1}

	c := sources.Candidate{
		Title: "The Way of Kings",
	}
	locked := map[string]bool{}
	mergeCandidateIntoSidecar(sc, c, locked)

	if sc.SortTitle == "" {
		t.Error("expected sort_title to be derived when not provided")
	}
	// SortTitle should move "The" to end: "Way of Kings, The"
	if sc.SortTitle != "Way of Kings, The" {
		t.Errorf("SortTitle = %q, want %q", sc.SortTitle, "Way of Kings, The")
	}
}

func TestMergeCandidateIntoSidecar_Narrators(t *testing.T) {
	sc := &scanner.Sidecar{SchemaVersion: 1}

	c := sources.Candidate{
		Authors: []sources.Contributor{
			{Name: "Frank Herbert", Role: "author"},
		},
		Narrators: []sources.Contributor{
			{Name: "Simon Vance", Role: "narrator"},
		},
	}
	locked := map[string]bool{}
	mergeCandidateIntoSidecar(sc, c, locked)

	if len(sc.Contributors) != 2 {
		t.Fatalf("Contributors = %d, want 2", len(sc.Contributors))
	}
	if sc.Contributors[0].Roles[0] != "author" {
		t.Errorf("first contributor should be author, got %v", sc.Contributors[0].Roles)
	}
	if sc.Contributors[1].Roles[0] != "narrator" {
		t.Errorf("second contributor should be narrator, got %v", sc.Contributors[1].Roles)
	}
}

func TestDownloadCover(t *testing.T) {
	// Create a test HTTP server that serves a fake image
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fake-jpeg-data"))
	}))
	defer srv.Close()

	// Create a temp directory to act as both media root and work dir
	tmpDir, err := os.MkdirTemp("", "codex-cover-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workDir := filepath.Join(tmpDir, "Author", "Book")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	writer := NewSidecarWriter(srv.Client())

	sc := &scanner.Sidecar{SchemaVersion: 1}
	c := sources.Candidate{
		Source:   "test_source",
		CoverURL: srv.URL + "/cover.jpg",
	}

	err = writer.downloadCover(workDir, tmpDir, sc, c)
	if err != nil {
		t.Fatalf("downloadCover: %v", err)
	}

	// Verify the cover file was created
	coverPath := filepath.Join(workDir, "cover_test_source.jpg")
	if _, err := os.Stat(coverPath); os.IsNotExist(err) {
		t.Error("cover file was not created")
	}

	// Verify sidecar cover metadata was updated
	if sc.Covers == nil {
		t.Fatal("expected covers to be set in sidecar")
	}
	if sc.Covers.Selected != "test_source" {
		t.Errorf("Selected = %q, want %q", sc.Covers.Selected, "test_source")
	}
	coverSrc := sc.Covers.Sources["test_source"]
	if coverSrc == nil {
		t.Fatal("expected cover source entry")
	}
	if coverSrc.Filename != "cover_test_source.jpg" {
		t.Errorf("Filename = %q, want %q", coverSrc.Filename, "cover_test_source.jpg")
	}
}

func TestDownloadCover_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir, err := os.MkdirTemp("", "codex-cover-err-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	writer := NewSidecarWriter(srv.Client())
	sc := &scanner.Sidecar{SchemaVersion: 1}
	c := sources.Candidate{
		Source:   "test_source",
		CoverURL: srv.URL + "/notfound.jpg",
	}

	err = writer.downloadCover(tmpDir, tmpDir, sc, c)
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestMergeAndWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "codex-merge-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workDir := filepath.Join(tmpDir, "Author", "Book")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	writer := NewSidecarWriter(nil)

	sc := ScoredCandidate{
		Candidate: sources.Candidate{
			Source:      "google_books",
			Title:       "Dune",
			Publisher:   "Ace Books",
			PublishDate: "1965",
			Authors: []sources.Contributor{
				{Name: "Frank Herbert", Role: "author"},
			},
			FetchedAt: time.Now(),
		},
		Score: Score{
			Overall:     0.92,
			TitleScore:  1.0,
			AuthorScore: 1.0,
		},
	}

	err = writer.MergeAndWrite(workDir, tmpDir, sc)
	if err != nil {
		t.Fatalf("MergeAndWrite: %v", err)
	}

	// Read back the sidecar and verify
	result, err := scanner.ReadSidecar(workDir, tmpDir)
	if err != nil {
		t.Fatalf("ReadSidecar: %v", err)
	}
	if result.Title != "Dune" {
		t.Errorf("Title = %q, want %q", result.Title, "Dune")
	}
	if result.Metadata.MatchConfidence != 0.92 {
		t.Errorf("MatchConfidence = %f, want 0.92", result.Metadata.MatchConfidence)
	}
	if result.Metadata.PrimarySource != "google_books" {
		t.Errorf("PrimarySource = %q, want %q", result.Metadata.PrimarySource, "google_books")
	}
	if result.Metadata.NeedsReview {
		t.Error("NeedsReview should be false after merge")
	}
}

func TestMergeAndWrite_PreservesLockedFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "codex-locked-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workDir := filepath.Join(tmpDir, "Author", "Book")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write an initial sidecar with locked fields
	initial := &scanner.Sidecar{
		SchemaVersion: 1,
		Title:         "My Custom Title",
		Publisher:     "Original Publisher",
		Metadata: scanner.SidecarMeta{
			LockedFields: []string{"title", "publisher"},
		},
	}
	if err := scanner.WriteSidecar(workDir, initial, tmpDir); err != nil {
		t.Fatal(err)
	}

	writer := NewSidecarWriter(nil)
	sc := ScoredCandidate{
		Candidate: sources.Candidate{
			Source:    "google_books",
			Title:     "API Title",
			Publisher: "API Publisher",
		},
		Score: Score{Overall: 0.90},
	}

	if err := writer.MergeAndWrite(workDir, tmpDir, sc); err != nil {
		t.Fatal(err)
	}

	result, err := scanner.ReadSidecar(workDir, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Locked fields should be preserved
	if result.Title != "My Custom Title" {
		t.Errorf("Title = %q, want %q (locked)", result.Title, "My Custom Title")
	}
	if result.Publisher != "Original Publisher" {
		t.Errorf("Publisher = %q, want %q (locked)", result.Publisher, "Original Publisher")
	}
}

// ── Helper function tests ────────────────────────────────────────────────────

func TestCoverExtFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/cover.jpg", ".jpg"},
		{"https://example.com/cover.png?size=large", ".png"},
		{"https://example.com/cover.webp", ".webp"},
		{"https://example.com/cover", ".jpg"},
		{"https://example.com/api/image/123", ".jpg"},
	}
	for _, tt := range tests {
		got := coverExtFromURL(tt.url)
		if got != tt.want {
			t.Errorf("coverExtFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestSanitizeForFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"google_books", "google_books"},
		{"open-library", "openlibrary"},
		{"My Source!", "mysource"},
		{"../evil", "evil"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		got := sanitizeForFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeForFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMatchMethod(t *testing.T) {
	tests := []struct {
		name string
		sc   ScoredCandidate
		want string
	}{
		{
			"ISBN match",
			ScoredCandidate{Score: Score{ISBNScore: 1.0}},
			"isbn_exact",
		},
		{
			"title+author fuzzy",
			ScoredCandidate{Score: Score{TitleScore: 0.9, AuthorScore: 0.8}},
			"title_author_fuzzy",
		},
		{
			"title only",
			ScoredCandidate{Score: Score{TitleScore: 0.7}},
			"title_fuzzy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchMethod(tt.sc)
			if got != tt.want {
				t.Errorf("matchMethod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeriveSortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Brandon Sanderson", "Sanderson, Brandon"},
		{"J. R. R. Tolkien", "Tolkien, J. R. R."},
		{"Cher", "Cher"},
	}
	for _, tt := range tests {
		got := deriveSortName(tt.input)
		if got != tt.want {
			t.Errorf("deriveSortName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
