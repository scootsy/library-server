// Package metadata provides the metadata enrichment engine and fuzzy matching
// utilities used to score external source candidates against local hints.
package metadata

import (
	"math"
	"strings"
	"unicode"

	"github.com/scootsy/library-server/internal/metadata/sources"
)

// Score represents the result of comparing a candidate against query hints.
type Score struct {
	// Overall is the final confidence score in [0, 1].
	// ≥0.85 → auto-apply; 0.50–0.84 → provisional; <0.50 → needs review
	Overall float64

	// Component scores (each in [0, 1]) for debugging / UI display.
	TitleScore  float64
	AuthorScore float64
	ISBNScore   float64 // 1.0 if ISBN matched exactly, 0 otherwise
}

// ScoreCandidate computes a confidence score for candidate c against query q.
// The scoring strategy matches the design in PROJECT.md:
//
//   - ISBN exact match → 0.95
//   - ASIN exact match  → 0.90
//   - Title + author fuzzy match → scaled in [0.60, 0.90]
//   - Title-only fuzzy match    → scaled in [0.30, 0.60]
func ScoreCandidate(c sources.Candidate, q sources.Query) Score {
	var s Score

	// ── ISBN match (highest confidence) ──────────────────────────────────────
	if q.ISBN != "" {
		for _, id := range c.Identifiers {
			if normalizeISBN(id) == normalizeISBN(q.ISBN) {
				s.ISBNScore = 1.0
				s.TitleScore = 1.0
				s.AuthorScore = 1.0
				s.Overall = 0.95
				return s
			}
		}
	}

	// ── ASIN match ────────────────────────────────────────────────────────────
	if q.ASIN != "" {
		if asin, ok := c.Identifiers["asin"]; ok && strings.EqualFold(asin, q.ASIN) {
			s.TitleScore = 1.0
			s.AuthorScore = 1.0
			s.Overall = 0.90
			return s
		}
	}

	// ── Fuzzy title + author ──────────────────────────────────────────────────
	s.TitleScore = titleSimilarity(q.Title, c.Title)

	if q.Author != "" {
		s.AuthorScore = bestAuthorScore(q.Author, c.Authors)
	}

	// Weighted combination
	if q.Author != "" {
		// title 60%, author 40%
		base := s.TitleScore*0.60 + s.AuthorScore*0.40
		// Scale to [0.60, 0.90] when both dimensions are present
		s.Overall = 0.60 + base*0.30
	} else {
		// title-only: scale to [0.30, 0.60]
		s.Overall = 0.30 + s.TitleScore*0.30
	}

	// Clamp to [0, 1]
	s.Overall = math.Min(1.0, math.Max(0.0, s.Overall))
	return s
}

// titleSimilarity returns a [0,1] similarity score between two titles after
// normalisation (lowercasing, removing punctuation, collapsing spaces).
func titleSimilarity(a, b string) float64 {
	a = normalizeTitle(a)
	b = normalizeTitle(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1.0
	}
	dist := levenshtein(a, b)
	maxLen := math.Max(float64(len([]rune(a))), float64(len([]rune(b))))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(dist)/maxLen
}

// bestAuthorScore returns the highest similarity between the query author and
// any of the candidate's author contributors.
func bestAuthorScore(queryAuthor string, authors []sources.Contributor) float64 {
	best := 0.0
	for _, contrib := range authors {
		if contrib.Role != "author" && contrib.Role != "" {
			continue
		}
		s := authorSimilarity(queryAuthor, contrib.Name)
		if s > best {
			best = s
		}
	}
	return best
}

// authorSimilarity compares two author name strings, trying both "First Last"
// and "Last, First" orderings to be robust against name-format differences.
func authorSimilarity(a, b string) float64 {
	a = normalizeAuthor(a)
	b = normalizeAuthor(b)
	direct := tokenSetRatio(a, b)

	// Also try inverted "Last, First" form
	aInv := invertName(a)
	inverted := tokenSetRatio(aInv, b)

	if inverted > direct {
		return inverted
	}
	return direct
}

// tokenSetRatio splits both strings into word-tokens, takes the union and
// intersection, and returns a Jaccard-inspired similarity in [0, 1].
func tokenSetRatio(a, b string) float64 {
	aTokens := tokenize(a)
	bTokens := tokenize(b)
	if len(aTokens) == 0 && len(bTokens) == 0 {
		return 1.0
	}
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0.0
	}

	// Find best token-level match using pairwise Levenshtein
	matched := make(map[int]bool)
	score := 0.0
	for _, at := range aTokens {
		bestToken := 0.0
		bestIdx := -1
		for i, bt := range bTokens {
			if matched[i] {
				continue
			}
			sim := tokenSim(at, bt)
			if sim > bestToken {
				bestToken = sim
				bestIdx = i
			}
		}
		if bestIdx >= 0 && bestToken > 0.5 {
			matched[bestIdx] = true
			score += bestToken
		}
	}

	// Normalise by the larger token set to penalise extra tokens
	maxTokens := float64(max(len(aTokens), len(bTokens)))
	return score / maxTokens
}

func tokenSim(a, b string) float64 {
	if a == b {
		return 1.0
	}
	dist := levenshtein(a, b)
	maxLen := float64(max(len([]rune(a)), len([]rune(b))))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(dist)/maxLen
}

// levenshtein computes the edit distance between two strings (rune-aware).
func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la, lb := len(ra), len(rb)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use two-row DP for memory efficiency.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = minOf3(
				prev[j]+1,       // deletion
				curr[j-1]+1,     // insertion
				prev[j-1]+cost,  // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

// ── Normalization helpers ─────────────────────────────────────────────────────

// normalizeTitle lowercases, strips punctuation, and collapses whitespace.
func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	// Remove leading articles for comparison
	for _, art := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(s, art) {
			s = s[len(art):]
			break
		}
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if r == '\'' || r == '\u2019' || r == '\u2018' {
			// Drop apostrophes so "Philosopher's" → "philosophers"
			prevSpace = false
		} else if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// normalizeAuthor lowercases and removes honorifics / punctuation.
func normalizeAuthor(s string) string {
	s = strings.ToLower(s)
	// Strip common honorifics
	for _, prefix := range []string{"dr. ", "dr ", "mr. ", "mr ", "mrs. ", "mrs ", "ms. ", "ms ", "prof. ", "prof "} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
		}
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if r == '\'' || r == '\u2019' {
			b.WriteRune('\'') // normalise apostrophes in names (O'Brien)
			prevSpace = false
		} else if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// invertName converts "First Last" → "last first" and "Last, First" → "first last".
func invertName(s string) string {
	if strings.Contains(s, ",") {
		parts := strings.SplitN(s, ",", 2)
		return strings.TrimSpace(parts[1]) + " " + strings.TrimSpace(parts[0])
	}
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return s
	}
	last := parts[len(parts)-1]
	first := strings.Join(parts[:len(parts)-1], " ")
	return last + " " + first
}

// tokenize splits a string on whitespace, returning lowercase tokens.
func tokenize(s string) []string {
	words := strings.Fields(s)
	tokens := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.ToLower(w)
		if w != "" {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

// normalizeISBN strips hyphens and spaces from an ISBN string.
func normalizeISBN(s string) string {
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.ToLower(s)
}

func minOf3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
