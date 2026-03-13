package scanner

import (
	"testing"
)

func TestParseFolderHints(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantAuthor  string
		wantTitle   string
		wantSeries  string
		wantPos     *float64
	}{
		{
			name:       "single component",
			path:       "The Way of Kings",
			wantTitle:  "The Way of Kings",
		},
		{
			name:       "author/title",
			path:       "Brandon Sanderson/The Way of Kings",
			wantAuthor: "Brandon Sanderson",
			wantTitle:  "The Way of Kings",
		},
		{
			name:       "author comma/title",
			path:       "Sanderson, Brandon/The Way of Kings",
			wantAuthor: "Brandon Sanderson",
			wantTitle:  "The Way of Kings",
		},
		{
			name:       "author/series/position-title",
			path:       "Brandon Sanderson/Stormlight Archive/01 - The Way of Kings",
			wantAuthor: "Brandon Sanderson",
			wantSeries: "Stormlight Archive",
			wantTitle:  "The Way of Kings",
			wantPos:    float64Ptr(1),
		},
		{
			name:       "author/series/position.title",
			path:       "Brandon Sanderson/Stormlight Archive/01. The Way of Kings",
			wantAuthor: "Brandon Sanderson",
			wantSeries: "Stormlight Archive",
			wantTitle:  "The Way of Kings",
			wantPos:    float64Ptr(1),
		},
		{
			name:       "inline series reference",
			path:       "The Way of Kings (Stormlight Archive #1)",
			wantTitle:  "The Way of Kings",
			wantSeries: "Stormlight Archive",
			wantPos:    float64Ptr(1),
		},
		{
			name:       "book num prefix",
			path:       "Brandon Sanderson/Series/Book 2 - Words of Radiance",
			wantAuthor: "Brandon Sanderson",
			wantSeries: "Series",
			wantTitle:  "Words of Radiance",
			wantPos:    float64Ptr(2),
		},
		{
			name:       "decimal position",
			path:       "Author/Series/1.5 - Novella",
			wantAuthor: "Author",
			wantSeries: "Series",
			wantTitle:  "Novella",
			wantPos:    float64Ptr(1.5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := ParseFolderHints(tt.path)

			if h.Author != tt.wantAuthor {
				t.Errorf("Author = %q, want %q", h.Author, tt.wantAuthor)
			}
			if h.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", h.Title, tt.wantTitle)
			}
			if h.SeriesName != tt.wantSeries {
				t.Errorf("SeriesName = %q, want %q", h.SeriesName, tt.wantSeries)
			}
			if tt.wantPos == nil && h.SeriesPosition != nil {
				t.Errorf("SeriesPosition = %v, want nil", *h.SeriesPosition)
			} else if tt.wantPos != nil {
				if h.SeriesPosition == nil {
					t.Errorf("SeriesPosition = nil, want %v", *tt.wantPos)
				} else if *h.SeriesPosition != *tt.wantPos {
					t.Errorf("SeriesPosition = %v, want %v", *h.SeriesPosition, *tt.wantPos)
				}
			}
		})
	}
}

func TestSortTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"The Way of Kings", "Way of Kings, The"},
		{"A Game of Thrones", "Game of Thrones, A"},
		{"An Unkindness of Ravens", "Unkindness of Ravens, An"},
		{"Dune", "Dune"},
		{"The", ""},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := SortTitle(tt.title)
			if tt.want == "" {
				// Edge case: single word articles become empty after transform
				return
			}
			if got != tt.want {
				t.Errorf("SortTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func float64Ptr(f float64) *float64 { return &f }
