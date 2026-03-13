// Package scanner handles directory walking, file detection, metadata extraction,
// and sidecar read/write operations.
package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scootsy/library-server/internal/security"
)

const sidecarFilename = "metadata.json"
const schemaVersion = 1

// Sidecar is the complete in-memory representation of a metadata.json file.
// Field names match the JSON keys defined in schema-spec.md.
type Sidecar struct {
	SchemaVersion int    `json:"schema_version"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle,omitempty"`
	SortTitle     string `json:"sort_title,omitempty"`
	OriginalTitle string `json:"original_title,omitempty"`
	Language      string `json:"language,omitempty"`
	Languages     []string `json:"languages,omitempty"`

	Identifiers map[string]*string `json:"identifiers,omitempty"`
	Contributors []SidecarContributor `json:"contributors,omitempty"`
	Series       []SidecarSeries      `json:"series,omitempty"`

	Publisher   string `json:"publisher,omitempty"`
	PublishDate string `json:"publish_date,omitempty"`
	Edition     string `json:"edition,omitempty"`
	PageCount   int    `json:"page_count,omitempty"`
	WordCount   *int   `json:"word_count,omitempty"`

	Description       string `json:"description,omitempty"`
	DescriptionFormat string `json:"description_format,omitempty"`

	Genres   []string `json:"genres,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Subjects []string `json:"subjects,omitempty"`
	Audience string   `json:"audience,omitempty"`

	Ratings map[string]*SidecarRating `json:"ratings,omitempty"`
	Covers  *SidecarCovers            `json:"covers,omitempty"`
	Files   []SidecarFile             `json:"files,omitempty"`

	Audiobook    *SidecarAudiobook    `json:"audiobook,omitempty"`
	MediaOverlay *SidecarMediaOverlay `json:"media_overlay,omitempty"`

	Links    []SidecarLink `json:"links,omitempty"`
	Metadata SidecarMeta  `json:"metadata"`
}

// SidecarContributor is a person (author, narrator, editor, etc.) linked to a work.
type SidecarContributor struct {
	Name        string            `json:"name"`
	SortName    string            `json:"sort_name,omitempty"`
	Roles       []string          `json:"roles"`
	Identifiers map[string]string `json:"identifiers,omitempty"`
}

// SidecarSeries associates a work with a named series/universe.
type SidecarSeries struct {
	Name          string            `json:"name"`
	Position      *float64          `json:"position,omitempty"`
	TotalInSeries *int              `json:"total_in_series,omitempty"`
	Identifiers   map[string]string `json:"identifiers,omitempty"`
}

// SidecarRating holds per-source rating data.
type SidecarRating struct {
	Score     float64   `json:"score"`
	Max       float64   `json:"max"`
	Count     int       `json:"count,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// SidecarCovers holds the multi-source cover collection.
type SidecarCovers struct {
	Selected string                      `json:"selected,omitempty"`
	Sources  map[string]*SidecarCoverSrc `json:"sources,omitempty"`
}

// SidecarCoverSrc describes one cover image file.
type SidecarCoverSrc struct {
	Filename   string    `json:"filename"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	URL        string    `json:"url,omitempty"`
	SourceNote string    `json:"source_note,omitempty"`
	FetchedAt  time.Time `json:"fetched_at,omitzero"`
}

// SidecarFile describes a single media file in the work directory.
type SidecarFile struct {
	Filename        string    `json:"filename"`
	Format          string    `json:"format"`
	SizeBytes       int64     `json:"size_bytes"`
	ChecksumSHA256  string    `json:"checksum_sha256,omitempty"`
	HasMediaOverlay bool      `json:"has_media_overlay,omitempty"`
	DurationSeconds int       `json:"duration_seconds,omitempty"`
	BitrateKbps     int       `json:"bitrate_kbps,omitempty"`
	Codec           string    `json:"codec,omitempty"`
	AddedAt         time.Time `json:"added_at"`
}

// SidecarAudiobook holds audiobook-specific metadata.
type SidecarAudiobook struct {
	DurationSeconds int              `json:"duration_seconds"`
	Chapters        []SidecarChapter `json:"chapters,omitempty"`
	Abridged        bool             `json:"abridged,omitempty"`
}

// SidecarChapter is a single chapter in an audiobook.
type SidecarChapter struct {
	Title        string  `json:"title"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Index        int     `json:"index"`
}

// SidecarMediaOverlay describes an aligned EPUB3 with media overlays.
type SidecarMediaOverlay struct {
	AlignedEPUBFilename string    `json:"aligned_epub_filename"`
	AlignmentTool       string    `json:"alignment_tool,omitempty"`
	AlignmentVersion    string    `json:"alignment_version,omitempty"`
	AlignedAt           time.Time `json:"aligned_at,omitzero"`
}

// SidecarLink is an external reference URL.
type SidecarLink struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// SidecarMeta holds provenance information about the sidecar itself.
type SidecarMeta struct {
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	MatchConfidence float64           `json:"match_confidence,omitempty"`
	MatchMethod     string            `json:"match_method,omitempty"`
	PrimarySource   string            `json:"primary_source,omitempty"`
	NeedsReview     bool              `json:"needs_review,omitempty"`
	LockedFields    []string          `json:"locked_fields,omitempty"`
	RawSources      map[string]any    `json:"raw_sources,omitempty"`
}

// ReadSidecar reads and parses the metadata.json in dir.
// mediaRoot is used to validate that the path stays within the allowed root.
func ReadSidecar(dir string, mediaRoot ...string) (*Sidecar, error) {
	sidecarPath := filepath.Join(dir, sidecarFilename)
	if len(mediaRoot) > 0 {
		safePath, err := security.SafePath(sidecarPath, mediaRoot...)
		if err != nil {
			return nil, fmt.Errorf("validating sidecar read path: %w", err)
		}
		sidecarPath = safePath
	}
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return nil, fmt.Errorf("reading sidecar at %q: %w", sidecarPath, err)
	}

	var s Sidecar
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing sidecar at %q: %w", sidecarPath, err)
	}
	return &s, nil
}

// WriteSidecar serializes s to metadata.json in dir, creating the file if
// necessary. It writes to a temp file first, then renames atomically.
// mediaRoot is used to validate that the path stays within the allowed root.
func WriteSidecar(dir string, s *Sidecar, mediaRoot ...string) error {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = schemaVersion
	}
	now := time.Now().UTC()
	if s.Metadata.CreatedAt.IsZero() {
		s.Metadata.CreatedAt = now
	}
	s.Metadata.UpdatedAt = now

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling sidecar: %w", err)
	}

	sidecarPath := filepath.Join(dir, sidecarFilename)
	if len(mediaRoot) > 0 {
		safePath, err := security.SafePath(sidecarPath, mediaRoot...)
		if err != nil {
			return fmt.Errorf("validating sidecar write path: %w", err)
		}
		sidecarPath = safePath
	}
	tmpPath := sidecarPath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing sidecar temp file: %w", err)
	}
	if err := os.Rename(tmpPath, sidecarPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			slog.Warn("failed to clean up sidecar temp file", "path", tmpPath, "error", removeErr)
		}
		return fmt.Errorf("renaming sidecar temp file: %w", err)
	}

	slog.Debug("wrote sidecar", "path", sidecarPath)
	return nil
}

// HashSidecar computes the SHA-256 of the metadata.json file in dir and
// returns the hex-encoded digest. Used to detect external edits.
// dir must be an absolute path that has already been validated by SafePath.
func HashSidecar(mediaRoot, dir string) (string, error) {
	sidecarPath, err := security.SafePath(filepath.Join(dir, sidecarFilename), mediaRoot)
	if err != nil {
		return "", fmt.Errorf("validating sidecar path: %w", err)
	}

	f, err := os.Open(sidecarPath)
	if err != nil {
		return "", fmt.Errorf("opening sidecar for hashing: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing sidecar: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SidecarExists reports whether a metadata.json file exists in dir.
func SidecarExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, sidecarFilename))
	return err == nil
}

// SortTitle derives a sort-friendly title by moving leading articles to the end.
func SortTitle(title string) string {
	articles := []string{"the ", "a ", "an "}
	lower := strings.ToLower(title)
	for _, art := range articles {
		if strings.HasPrefix(lower, art) {
			return title[len(art):] + ", " + title[:len(art)-1]
		}
	}
	return title
}
