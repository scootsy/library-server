package metadata

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scootsy/library-server/internal/metadata/sources"
	"github.com/scootsy/library-server/internal/scanner"
	"github.com/scootsy/library-server/internal/security"
)

// SidecarWriter handles merging metadata candidates into sidecar files,
// respecting locked fields, and downloading cover images.
type SidecarWriter struct {
	httpClient *http.Client
}

// NewSidecarWriter creates a SidecarWriter. If httpClient is nil, a default
// client with a 30-second timeout is used.
func NewSidecarWriter(httpClient *http.Client) *SidecarWriter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &SidecarWriter{httpClient: httpClient}
}

// MergeAndWrite reads the existing sidecar (if any), merges the scored
// candidate into it respecting locked fields, downloads covers, and writes
// the result back to disk.
func (w *SidecarWriter) MergeAndWrite(absDir, mediaRoot string, sc ScoredCandidate) error {
	// Read existing sidecar or start fresh
	existing, err := scanner.ReadSidecar(absDir, mediaRoot)
	if err != nil {
		slog.Debug("no existing sidecar, creating new", "dir", absDir, "error", err)
		existing = &scanner.Sidecar{
			SchemaVersion: 1,
		}
	}

	locked := buildLockedSet(existing.Metadata.LockedFields)

	// Merge candidate fields into the sidecar
	mergeCandidateIntoSidecar(existing, sc.Candidate, locked)

	// Update metadata provenance
	existing.Metadata.MatchConfidence = sc.Score.Overall
	existing.Metadata.MatchMethod = matchMethod(sc)
	existing.Metadata.PrimarySource = sc.Candidate.Source
	existing.Metadata.NeedsReview = false

	// Download cover image if candidate provides one
	if sc.Candidate.CoverURL != "" {
		if err := w.downloadCover(absDir, mediaRoot, existing, sc.Candidate); err != nil {
			slog.Warn("cover download failed",
				"dir", absDir,
				"source", sc.Candidate.Source,
				"error", err)
			// Non-fatal: proceed without cover
		}
	}

	// Write the merged sidecar
	if err := scanner.WriteSidecar(absDir, existing, mediaRoot); err != nil {
		return fmt.Errorf("writing merged sidecar: %w", err)
	}

	return nil
}

// mergeCandidateIntoSidecar applies candidate fields to the sidecar, skipping
// any fields in the locked set. This is the field-level merge strategy.
func mergeCandidateIntoSidecar(sc *scanner.Sidecar, c sources.Candidate, locked map[string]bool) {
	if !locked["title"] && c.Title != "" {
		sc.Title = c.Title
	}
	if !locked["subtitle"] && c.Subtitle != "" {
		sc.Subtitle = c.Subtitle
	}
	if !locked["sort_title"] && c.SortTitle != "" {
		sc.SortTitle = c.SortTitle
	} else if !locked["sort_title"] && c.Title != "" && sc.SortTitle == "" {
		sc.SortTitle = scanner.SortTitle(c.Title)
	}
	if !locked["language"] && c.Language != "" {
		sc.Language = c.Language
	}
	if !locked["publisher"] && c.Publisher != "" {
		sc.Publisher = c.Publisher
	}
	if !locked["publish_date"] && c.PublishDate != "" {
		sc.PublishDate = c.PublishDate
	}
	if !locked["description"] && c.Description != "" {
		sc.Description = c.Description
		sc.DescriptionFormat = "plain"
	}
	if !locked["page_count"] && c.PageCount > 0 {
		sc.PageCount = c.PageCount
	}

	// Contributors: merge if not locked
	if !locked["contributors"] && (len(c.Authors) > 0 || len(c.Narrators) > 0) {
		var contributors []scanner.SidecarContributor
		for _, a := range c.Authors {
			sortName := a.SortName
			if sortName == "" {
				sortName = deriveSortName(a.Name)
			}
			contributors = append(contributors, scanner.SidecarContributor{
				Name:     a.Name,
				SortName: sortName,
				Roles:    []string{roleOrDefault(a.Role, "author")},
			})
		}
		for _, n := range c.Narrators {
			sortName := n.SortName
			if sortName == "" {
				sortName = deriveSortName(n.Name)
			}
			contributors = append(contributors, scanner.SidecarContributor{
				Name:     n.Name,
				SortName: sortName,
				Roles:    []string{roleOrDefault(n.Role, "narrator")},
			})
		}
		sc.Contributors = contributors
	}

	// Series: merge if not locked
	if !locked["series"] && len(c.Series) > 0 {
		var series []scanner.SidecarSeries
		for _, s := range c.Series {
			ss := scanner.SidecarSeries{Name: s.Name}
			if s.Position > 0 {
				pos := s.Position
				ss.Position = &pos
			}
			series = append(series, ss)
		}
		sc.Series = series
	}

	// Tags: merge (append unique, don't replace)
	if !locked["tags"] && len(c.Tags) > 0 {
		existing := make(map[string]bool, len(sc.Tags))
		for _, t := range sc.Tags {
			existing[strings.ToLower(t)] = true
		}
		for _, t := range c.Tags {
			if !existing[strings.ToLower(t)] {
				sc.Tags = append(sc.Tags, t)
				existing[strings.ToLower(t)] = true
			}
		}
	}

	// Identifiers: merge new identifiers from candidate
	if !locked["identifiers"] && len(c.Identifiers) > 0 {
		if sc.Identifiers == nil {
			sc.Identifiers = make(map[string]*string)
		}
		for k, v := range c.Identifiers {
			if v == "" {
				continue
			}
			// Only add identifiers we don't already have
			if existing, ok := sc.Identifiers[k]; !ok || existing == nil || *existing == "" {
				val := v
				sc.Identifiers[k] = &val
			}
		}
	}

	// Ratings are stored per-source and never merged across sources.
	if !locked["ratings"] && c.Rating != nil && c.Source != "" {
		if sc.Ratings == nil {
			sc.Ratings = make(map[string]*scanner.SidecarRating)
		}
		fetchedAt := c.Rating.FetchedAt
		if fetchedAt.IsZero() {
			fetchedAt = c.FetchedAt
		}
		if fetchedAt.IsZero() {
			fetchedAt = time.Now().UTC()
		}
		maxScore := c.Rating.Max
		if maxScore <= 0 {
			maxScore = 5
		}
		sc.Ratings[c.Source] = &scanner.SidecarRating{
			Score:     c.Rating.Score,
			Max:       maxScore,
			Count:     c.Rating.Count,
			FetchedAt: fetchedAt,
		}
	}

	// Audiobook duration from candidate
	if !locked["audiobook"] && c.DurationSecs > 0 {
		if sc.Audiobook == nil {
			sc.Audiobook = &scanner.SidecarAudiobook{}
		}
		if sc.Audiobook.DurationSeconds == 0 {
			sc.Audiobook.DurationSeconds = c.DurationSecs
		}
	}
}

// downloadCover fetches a cover image from the candidate's CoverURL and saves
// it to the work directory. The filename follows the pattern:
// cover_<source>.<ext>
func (w *SidecarWriter) downloadCover(absDir, mediaRoot string, sc *scanner.Sidecar, c sources.Candidate) error {
	if c.CoverURL == "" {
		return nil
	}

	// Determine file extension from URL
	ext := coverExtFromURL(c.CoverURL)

	// Sanitize source name for filename
	safeName := sanitizeForFilename(c.Source)
	filename := fmt.Sprintf("cover_%s%s", safeName, ext)

	// Validate destination path (file may not exist yet, so use SafePathParent)
	destPath, err := security.SafePathParent(filepath.Join(absDir, filename), mediaRoot)
	if err != nil {
		return fmt.Errorf("validating cover path: %w", err)
	}

	// Download the image
	resp, err := w.httpClient.Get(c.CoverURL)
	if err != nil {
		return fmt.Errorf("fetching cover from %s: %w", c.Source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover download returned status %d", resp.StatusCode)
	}

	// Limit response body to 10MB to prevent abuse
	limited := io.LimitReader(resp.Body, 10*1024*1024)

	// Write to temp file then rename atomically
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating cover temp file: %w", err)
	}
	if _, err := io.Copy(f, limited); err != nil {
		f.Close()
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			slog.Warn("failed to clean up cover temp file", "path", tmpPath, "error", removeErr)
		}
		return fmt.Errorf("writing cover data: %w", err)
	}
	if err := f.Close(); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			slog.Warn("failed to clean up cover temp file", "path", tmpPath, "error", removeErr)
		}
		return fmt.Errorf("closing cover temp file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			slog.Warn("failed to clean up cover temp file", "path", tmpPath, "error", removeErr)
		}
		return fmt.Errorf("renaming cover temp file: %w", err)
	}

	// Update sidecar cover collection
	if sc.Covers == nil {
		sc.Covers = &scanner.SidecarCovers{
			Sources: make(map[string]*scanner.SidecarCoverSrc),
		}
	}
	sc.Covers.Sources[c.Source] = &scanner.SidecarCoverSrc{
		Filename:   filename,
		URL:        c.CoverURL,
		SourceNote: fmt.Sprintf("Downloaded from %s", c.Source),
		FetchedAt:  time.Now().UTC(),
	}

	// If no cover is selected yet, select this one
	if sc.Covers.Selected == "" {
		sc.Covers.Selected = c.Source
	}

	slog.Debug("downloaded cover", "source", c.Source, "filename", filename)
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// buildLockedSet converts a slice of locked field names into a set for O(1) lookup.
func buildLockedSet(fields []string) map[string]bool {
	m := make(map[string]bool, len(fields))
	for _, f := range fields {
		m[f] = true
	}
	return m
}

// matchMethod returns a human-readable description of how the match was made.
func matchMethod(sc ScoredCandidate) string {
	if sc.Score.ISBNScore == 1.0 {
		return "isbn_exact"
	}
	if sc.Score.AuthorScore > 0 {
		return "title_author_fuzzy"
	}
	return "title_fuzzy"
}

// deriveSortName converts "First Last" → "Last, First" for sort ordering.
func deriveSortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return name
	}
	last := parts[len(parts)-1]
	first := strings.Join(parts[:len(parts)-1], " ")
	return last + ", " + first
}

// roleOrDefault returns role if non-empty, otherwise the default.
func roleOrDefault(role, def string) string {
	if role != "" {
		return role
	}
	return def
}

// coverExtFromURL extracts the file extension from a cover URL.
// Defaults to ".jpg" if not determinable.
func coverExtFromURL(rawURL string) string {
	// Strip query parameters
	path := rawURL
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext
	default:
		return ".jpg"
	}
}

// sanitizeForFilename removes any characters that aren't alphanumeric or underscores.
func sanitizeForFilename(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if result == "" {
		return "unknown"
	}
	return result
}
