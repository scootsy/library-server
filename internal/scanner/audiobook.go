package scanner

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// AudioMeta holds metadata extracted from an audiobook file (M4B, MP3, etc.).
type AudioMeta struct {
	Title           string
	SortTitle       string
	Artists         []string
	Album           string
	AlbumArtist     string
	Year            string
	Description     string
	Publisher       string
	Narrator        string
	DurationSeconds int
	Chapters        []AudioChapter
	CoverData       []byte
	CoverExt        string // ".jpg" or ".png"
}

// AudioChapter is a timed chapter within an audiobook.
type AudioChapter struct {
	Title        string
	StartSeconds float64
	EndSeconds   float64
	Index        int
}

// ExtractAudioMeta reads tag metadata from an M4B, MP3, FLAC, or OGG file.
// filePath must be an absolute path that has already been validated.
func ExtractAudioMeta(filePath string) (*AudioMeta, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".m4b", ".m4a", ".mp4":
		return extractMP4Meta(filePath)
	case ".mp3":
		return extractID3Meta(filePath)
	default:
		return &AudioMeta{}, nil
	}
}

// extractMP4Meta reads metadata from an M4B/M4A/MP4 file using the iTunes
// metadata box (ilst). Chapter data is extracted from the chpl (Nero) atom
// when present.
func extractMP4Meta(filePath string) (*AudioMeta, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening m4b: %w", err)
	}
	defer f.Close()

	// Parse MP4 boxes to find the moov/udta/meta/ilst atoms.
	parser := &mp4Parser{r: f}
	if err := parser.parse(); err != nil {
		slog.Warn("mp4 parse failed, returning empty metadata", "file", filePath, "error", err)
		return &AudioMeta{}, nil
	}

	m := &AudioMeta{
		Title:           parser.itunesTitle,
		Album:           parser.itunesAlbum,
		AlbumArtist:     parser.itunesAlbumArtist,
		Year:            parser.itunesYear,
		Description:     parser.itunesDescription,
		Publisher:       parser.itunesPublisher,
		DurationSeconds: parser.durationSeconds,
		Chapters:        parser.chapters,
		CoverData:       parser.coverData,
		CoverExt:        parser.coverExt,
	}
	if len(parser.itunesArtists) > 0 {
		m.Artists = parser.itunesArtists
	}
	if m.Title != "" {
		m.SortTitle = SortTitle(m.Title)
	}
	return m, nil
}

// extractID3Meta reads metadata from an MP3 file using ID3 tags.
func extractID3Meta(filePath string) (*AudioMeta, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening mp3: %w", err)
	}
	defer f.Close()

	parser := &id3Parser{r: f}
	if err := parser.parse(); err != nil {
		slog.Warn("id3 parse failed, returning empty metadata", "file", filePath, "error", err)
		return &AudioMeta{}, nil
	}

	m := &AudioMeta{
		Title:       parser.title,
		Album:       parser.album,
		AlbumArtist: parser.albumArtist,
		Year:        parser.year,
		Description: parser.description,
		CoverData:   parser.coverData,
		CoverExt:    parser.coverExt,
	}
	if parser.artist != "" {
		m.Artists = []string{parser.artist}
	}
	if m.Title != "" {
		m.SortTitle = SortTitle(m.Title)
	}
	return m, nil
}
