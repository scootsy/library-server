package scanner

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FolderHints contains metadata hints extracted from a directory path.
// These are low-confidence guesses used only when no sidecar or embedded
// metadata is available.
type FolderHints struct {
	Author         string
	SeriesName     string
	SeriesPosition *float64
	Title          string
}

// Common folder name patterns for position + title extraction:
//   "01 - The Way of Kings"
//   "01. The Way of Kings"
//   "Book 1 - The Way of Kings"
//   "The Way of Kings (Stormlight Archive #1)"
var (
	rePositionDash  = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*[-–—]\s*(.+)$`)
	rePositionDot   = regexp.MustCompile(`^(\d+(?:\.\d+)?)\.\s*(.+)$`)
	reBookNum       = regexp.MustCompile(`(?i)^book\s+(\d+(?:\.\d+)?)\s*[-–—]?\s*(.+)$`)
	reSeriesInTitle = regexp.MustCompile(`^(.+?)\s*\(([^)]+?)\s*#(\d+(?:\.\d+)?)\)$`)
	reYearSuffix    = regexp.MustCompile(`\s*\(\d{4}\)$`)
)

// ParseFolderHints extracts author, series, position, and title hints from
// a relative directory path. The path is expected to follow common patterns
// used by book management tools like Calibre, Readarr, or Beets:
//
//	Author Name/Title
//	Author Name/Series Name/01 - Title
//	Author, Last/Series Name/01. Title
func ParseFolderHints(relPath string) FolderHints {
	parts := splitPath(relPath)
	if len(parts) == 0 {
		return FolderHints{}
	}

	hints := FolderHints{}
	last := parts[len(parts)-1]

	switch len(parts) {
	case 1:
		// Single component: treat as title only
		hints.Title, hints.SeriesName, hints.SeriesPosition = parseTitle(last)

	case 2:
		// Two components: Author/Title
		hints.Author = normalizeAuthor(parts[0])
		hints.Title, hints.SeriesName, hints.SeriesPosition = parseTitle(last)

	default:
		// Three or more: Author/Series/Position - Title  (or Author/Series/Title)
		hints.Author = normalizeAuthor(parts[0])
		seriesCandidate := parts[len(parts)-2]

		title, _, _ := parseTitle(last)
		pos, titleFromPos := extractPosition(last)

		if pos != nil {
			hints.SeriesName = cleanName(seriesCandidate)
			hints.SeriesPosition = pos
			hints.Title = titleFromPos
		} else {
			hints.Title = title
			// The second-to-last component may or may not be a series name
			// (it could be a sub-directory like "Fantasy"). Without more context
			// we treat it as the series.
			hints.SeriesName = cleanName(seriesCandidate)
		}
	}

	return hints
}

// parseTitle attempts to extract a series reference from a title like
// "The Way of Kings (Stormlight Archive #1)".
func parseTitle(s string) (title, series string, pos *float64) {
	s = cleanName(s)

	// Check for position prefix first
	if p, t := extractPosition(s); p != nil {
		return t, "", p
	}

	// Check for inline series reference: "Title (Series #N)"
	if m := reSeriesInTitle.FindStringSubmatch(s); m != nil {
		title = strings.TrimSpace(m[1])
		series = strings.TrimSpace(m[2])
		if f, err := strconv.ParseFloat(m[3], 64); err == nil {
			pos = &f
		}
		return
	}

	return s, "", nil
}

// extractPosition looks for a numeric position prefix in s and returns
// (position, remaining title). Returns nil if no prefix is found.
func extractPosition(s string) (*float64, string) {
	for _, re := range []*regexp.Regexp{rePositionDash, rePositionDot, reBookNum} {
		if m := re.FindStringSubmatch(s); m != nil {
			if f, err := strconv.ParseFloat(m[1], 64); err == nil {
				remaining := strings.TrimSpace(m[2])
				return &f, remaining
			}
		}
	}
	return nil, s
}

// normalizeAuthor converts "Last, First" to "First Last" and trims whitespace.
func normalizeAuthor(s string) string {
	s = cleanName(s)
	if idx := strings.Index(s, ", "); idx >= 0 {
		last := s[:idx]
		first := s[idx+2:]
		return first + " " + last
	}
	return s
}

// cleanName strips trailing year annotations like "(2010)" and trims space.
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	s = reYearSuffix.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// splitPath splits a relative path into its components, skipping empty parts.
func splitPath(p string) []string {
	// Normalize separators
	p = filepath.ToSlash(p)
	raw := strings.Split(p, "/")
	var parts []string
	for _, r := range raw {
		if r != "" && r != "." {
			parts = append(parts, r)
		}
	}
	return parts
}
