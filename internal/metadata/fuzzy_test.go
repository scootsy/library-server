package metadata

import (
	"testing"

	"github.com/scootsy/library-server/internal/metadata/sources"
)

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"sunday", "saturday", 3},
		{"the way of kings", "the way of kings", 0},
	}
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d; want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"The Way of Kings", "way of kings"},
		{"A Game of Thrones", "game of thrones"},
		{"An American Tragedy", "american tragedy"},
		{"Foundation", "foundation"},
		{"Harry Potter and the Philosopher's Stone", "harry potter and the philosophers stone"},
	}
	for _, tt := range tests {
		got := normalizeTitle(tt.input)
		if got != tt.want {
			t.Errorf("normalizeTitle(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

func TestTitleSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		// Exact match
		{"The Way of Kings", "The Way of Kings", 1.0, 1.0},
		// Minor typo
		{"The Way of Kings", "The Way of King", 0.80, 1.0},
		// Article removal
		{"The Way of Kings", "Way of Kings", 0.80, 1.0},
		// Completely different
		{"Hamlet", "The Way of Kings", 0.0, 0.40},
		// Empty strings
		{"", "anything", 0.0, 0.0},
	}
	for _, tt := range tests {
		got := titleSimilarity(tt.a, tt.b)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("titleSimilarity(%q, %q) = %.3f; want [%.3f, %.3f]",
				tt.a, tt.b, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestInvertName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"brandon sanderson", "sanderson brandon"},
		{"tolkien j r r", "r tolkien j r"},
		{"j k rowling", "rowling j k"},
	}
	for _, tt := range tests {
		got := invertName(tt.input)
		if got != tt.want {
			t.Errorf("invertName(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

func TestScoreCandidate_ISBNMatch(t *testing.T) {
	q := sources.Query{
		Title:  "Some Book",
		Author: "Some Author",
		ISBN:   "9780765326355",
	}
	c := sources.Candidate{
		Title: "Some Book",
		Authors: []sources.Contributor{{Name: "Some Author", Role: "author"}},
		Identifiers: map[string]string{
			"isbn_13": "9780765326355",
		},
	}
	s := ScoreCandidate(c, q)
	if s.Overall < 0.94 {
		t.Errorf("ISBN match should give confidence ≥0.94, got %.3f", s.Overall)
	}
}

func TestScoreCandidate_TitleAuthorMatch(t *testing.T) {
	q := sources.Query{
		Title:  "The Way of Kings",
		Author: "Brandon Sanderson",
	}
	c := sources.Candidate{
		Title: "The Way of Kings",
		Authors: []sources.Contributor{
			{Name: "Brandon Sanderson", Role: "author"},
		},
		Identifiers: map[string]string{},
	}
	s := ScoreCandidate(c, q)
	if s.Overall < 0.85 {
		t.Errorf("exact title+author match should give confidence ≥0.85, got %.3f", s.Overall)
	}
}

func TestScoreCandidate_TitleOnlyMatch(t *testing.T) {
	q := sources.Query{
		Title: "The Way of Kings",
	}
	c := sources.Candidate{
		Title:       "The Way of Kings",
		Identifiers: map[string]string{},
	}
	s := ScoreCandidate(c, q)
	// Title-only should be in [0.30, 0.60]
	if s.Overall < 0.30 || s.Overall > 0.60 {
		t.Errorf("title-only match confidence %.3f should be in [0.30, 0.60]", s.Overall)
	}
}

func TestScoreCandidate_PoorMatch(t *testing.T) {
	q := sources.Query{
		Title:  "The Way of Kings",
		Author: "Brandon Sanderson",
	}
	c := sources.Candidate{
		Title: "Harry Potter and the Philosopher's Stone",
		Authors: []sources.Contributor{
			{Name: "J. K. Rowling", Role: "author"},
		},
		Identifiers: map[string]string{},
	}
	s := ScoreCandidate(c, q)
	// Poor match: both title and author are different. Score must stay below
	// the auto-apply threshold (0.85), putting it in the review queue.
	if s.Overall >= 0.85 {
		t.Errorf("poor match should not reach auto-apply threshold (0.85), got %.3f", s.Overall)
	}
}

func TestNormalizeISBN(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"978-0-7653-2635-5", "9780765326355"},
		{"9780765326355", "9780765326355"},
		{"0-7653-2635-5", "0765326355"},
	}
	for _, tt := range tests {
		got := normalizeISBN(tt.input)
		if got != tt.want {
			t.Errorf("normalizeISBN(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}
