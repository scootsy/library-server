package scanner

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scootsy/library-server/internal/database/queries"
	"github.com/scootsy/library-server/internal/security"
)

// supportedFormats maps file extensions to format names.
var supportedFormats = map[string]string{
	".epub": "epub",
	".pdf":  "pdf",
	".m4b":  "m4b",
	".m4a":  "m4a",
	".mp3":  "mp3",
	".cbz":  "cbz",
	".cbr":  "cbr",
	".mobi": "mobi",
	".azw":  "azw",
	".azw3": "azw3",
	".fb2":  "fb2",
	".djvu": "djvu",
	".flac": "flac",
	".ogg":  "ogg",
	".opus": "opus",
}

// audiobookFormats contains formats that carry audiobook content.
var audiobookFormats = map[string]bool{
	"m4b": true, "m4a": true, "mp3": true,
	"flac": true, "ogg": true, "opus": true,
}

// Scanner walks a media root directory, detects works, reads or creates
// sidecars, and indexes everything into the database.
type Scanner struct {
	db        *sql.DB
	mediaRoot *queries.MediaRoot
}

// New creates a Scanner for the given media root.
func New(db *sql.DB, root *queries.MediaRoot) *Scanner {
	return &Scanner{db: db, mediaRoot: root}
}

// Scan walks the media root and indexes all discovered works.
// It is safe to call multiple times; subsequent runs update existing records.
func (s *Scanner) Scan() error {
	slog.Info("starting scan", "root", s.mediaRoot.RootPath, "name", s.mediaRoot.Name)
	start := time.Now()

	count := 0
	err := filepath.WalkDir(s.mediaRoot.RootPath, func(dirPath string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("scan walk error", "path", dirPath, "error", err)
			return nil // continue walking
		}
		if !d.IsDir() {
			return nil
		}

		// Skip hidden directories
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// Validate the directory path falls within the media root.
		safePath, err := security.SafePath(dirPath, s.mediaRoot.RootPath)
		if err != nil {
			slog.Warn("skipping directory outside media root", "path", dirPath)
			return nil
		}

		mediaFiles := findMediaFiles(safePath)
		if len(mediaFiles) == 0 {
			return nil // no media here
		}

		relPath, err := filepath.Rel(s.mediaRoot.RootPath, safePath)
		if err != nil {
			slog.Warn("computing relative path failed", "path", safePath, "error", err)
			return nil
		}

		if err := s.indexDirectory(safePath, relPath, mediaFiles); err != nil {
			slog.Error("indexing directory failed", "path", relPath, "error", err)
			return nil // continue scanning other directories
		}
		count++
		return nil
	})

	elapsed := time.Since(start)
	slog.Info("scan complete", "root", s.mediaRoot.RootPath, "works", count, "duration", elapsed)
	return err
}

// indexDirectory processes one media directory and updates the database.
func (s *Scanner) indexDirectory(absDir, relDir string, mediaFiles []mediaFile) error {
	// Determine whether we have a sidecar, and if so whether it changed.
	if SidecarExists(absDir) {
		sidecarHash, err := hashFile(filepath.Join(absDir, sidecarFilename))
		if err != nil {
			return fmt.Errorf("hashing sidecar: %w", err)
		}

		// Check if we already have an up-to-date record.
		existing, err := queries.GetWorkByPath(s.db, s.mediaRoot.ID, relDir)
		if err != nil {
			return fmt.Errorf("looking up existing work: %w", err)
		}
		if existing != nil && existing.SidecarHash == sidecarHash {
			// Nothing changed; just touch last_scanned_at.
			return queries.TouchLastScanned(s.db, existing.ID)
		}

		// Sidecar is new or changed; re-index from it.
		sc, err := ReadSidecar(absDir, s.mediaRoot.RootPath)
		if err != nil {
			return fmt.Errorf("reading sidecar: %w", err)
		}

		workID := uuid.NewString()
		if existing != nil {
			workID = existing.ID
		}
		return s.indexFromSidecar(workID, relDir, sidecarHash, sc, mediaFiles)
	}

	// No sidecar: extract what we can from the files and create a minimal sidecar.
	extracted, err := s.extractFromFiles(absDir, mediaFiles)
	if err != nil {
		slog.Warn("metadata extraction failed, using folder hints only",
			"dir", relDir, "error", err)
	}

	sc := buildSidecar(relDir, extracted, mediaFiles)

	// Write the sidecar so future scans are faster.
	if err := WriteSidecar(absDir, sc, s.mediaRoot.RootPath); err != nil {
		slog.Warn("failed to write sidecar", "dir", relDir, "error", err)
		// Non-fatal: continue indexing without persisting the sidecar.
	}

	sidecarHash, err := hashFile(filepath.Join(absDir, sidecarFilename))
	if err != nil {
		slog.Warn("failed to hash sidecar after write", "dir", relDir, "error", err)
	}

	existing, err := queries.GetWorkByPath(s.db, s.mediaRoot.ID, relDir)
	if err != nil {
		return fmt.Errorf("looking up existing work: %w", err)
	}
	workID := uuid.NewString()
	if existing != nil {
		workID = existing.ID
	}
	return s.indexFromSidecar(workID, relDir, sidecarHash, sc, mediaFiles)
}

// indexFromSidecar writes a complete work record (and all related records)
// to the database from a parsed sidecar.
func (s *Scanner) indexFromSidecar(workID, relDir, sidecarHash string, sc *Sidecar, mf []mediaFile) error {
	// Build the Work record.
	w := &queries.Work{
		ID:                workID,
		MediaRootID:       s.mediaRoot.ID,
		DirectoryPath:     relDir,
		Title:             sc.Title,
		SortTitle:         sc.SortTitle,
		Subtitle:          sc.Subtitle,
		Language:          sc.Language,
		Publisher:         sc.Publisher,
		PublishDate:       sc.PublishDate,
		Description:       sc.Description,
		DescriptionFormat: sc.DescriptionFormat,
		PageCount:         sc.PageCount,
		NeedsReview:       sc.Metadata.NeedsReview,
		SidecarHash:       sidecarHash,
		MatchConfidence:   sc.Metadata.MatchConfidence,
		MatchMethod:       sc.Metadata.MatchMethod,
		PrimarySource:     sc.Metadata.PrimarySource,
	}
	if sc.DescriptionFormat == "" {
		w.DescriptionFormat = "plain"
	}
	if sc.Audiobook != nil {
		w.DurationSeconds = sc.Audiobook.DurationSeconds
		w.IsAbridged = sc.Audiobook.Abridged
	}
	if sc.MediaOverlay != nil {
		w.HasMediaOverlay = true
	}

	if err := queries.UpsertWork(s.db, w); err != nil {
		return fmt.Errorf("upserting work: %w", err)
	}

	// Contributors
	if err := queries.DeleteWorkContributors(s.db, workID); err != nil {
		return err
	}
	for i, c := range sc.Contributors {
		sortName := c.SortName
		if sortName == "" {
			sortName = deriveSortName(c.Name)
		}
		identifiers := c.Identifiers
		if identifiers == nil {
			identifiers = map[string]string{}
		}
		cID, err := queries.UpsertContributor(s.db, uuid.NewString(), c.Name, sortName, identifiers)
		if err != nil {
			return fmt.Errorf("upserting contributor %q: %w", c.Name, err)
		}
		for _, role := range c.Roles {
			if err := queries.UpsertWorkContributor(s.db, workID, cID, role, i); err != nil {
				return fmt.Errorf("upserting work_contributor: %w", err)
			}
		}
	}

	// Series
	if err := queries.DeleteWorkSeries(s.db, workID); err != nil {
		return err
	}
	for _, sr := range sc.Series {
		identifiers := sr.Identifiers
		if identifiers == nil {
			identifiers = map[string]string{}
		}
		srID, err := queries.UpsertSeries(s.db, uuid.NewString(), sr.Name, identifiers)
		if err != nil {
			return fmt.Errorf("upserting series %q: %w", sr.Name, err)
		}
		if err := queries.UpsertWorkSeries(s.db, workID, srID, sr.Position); err != nil {
			return fmt.Errorf("upserting work_series: %w", err)
		}
	}

	// Tags (genres, tags, subjects)
	if err := queries.DeleteWorkTags(s.db, workID); err != nil {
		return err
	}
	for _, g := range sc.Genres {
		tagID, err := queries.UpsertTag(s.db, uuid.NewString(), g, "genre")
		if err != nil {
			return fmt.Errorf("upserting genre tag %q: %w", g, err)
		}
		if err := queries.UpsertWorkTag(s.db, workID, tagID); err != nil {
			return err
		}
	}
	for _, t := range sc.Tags {
		tagID, err := queries.UpsertTag(s.db, uuid.NewString(), t, "tag")
		if err != nil {
			return fmt.Errorf("upserting tag %q: %w", t, err)
		}
		if err := queries.UpsertWorkTag(s.db, workID, tagID); err != nil {
			return err
		}
	}
	for _, sub := range sc.Subjects {
		tagID, err := queries.UpsertTag(s.db, uuid.NewString(), sub, "subject")
		if err != nil {
			return fmt.Errorf("upserting subject tag %q: %w", sub, err)
		}
		if err := queries.UpsertWorkTag(s.db, workID, tagID); err != nil {
			return err
		}
	}

	// Identifiers
	if err := queries.DeleteWorkIdentifiers(s.db, workID); err != nil {
		return err
	}
	for idType, idVal := range sc.Identifiers {
		if idVal == nil || *idVal == "" {
			continue
		}
		if err := queries.UpsertIdentifier(s.db, workID, idType, *idVal); err != nil {
			return fmt.Errorf("upserting identifier %s: %w", idType, err)
		}
	}

	// Files (from sidecar)
	if err := queries.DeleteWorkFiles(s.db, workID); err != nil {
		return err
	}
	for _, f := range sc.Files {
		wf := &queries.WorkFile{
			ID:              uuid.NewString(),
			WorkID:          workID,
			Filename:        f.Filename,
			Format:          f.Format,
			SizeBytes:       f.SizeBytes,
			ChecksumSHA256:  f.ChecksumSHA256,
			DurationSeconds: f.DurationSeconds,
			BitrateKbps:     f.BitrateKbps,
			Codec:           f.Codec,
			HasMediaOverlay: f.HasMediaOverlay,
		}
		if err := queries.UpsertWorkFile(s.db, wf); err != nil {
			return fmt.Errorf("upserting work file %q: %w", f.Filename, err)
		}
	}
	// Also index any media files found on disk that aren't in the sidecar.
	sidecarFilenames := make(map[string]bool, len(sc.Files))
	for _, f := range sc.Files {
		sidecarFilenames[f.Filename] = true
	}
	for _, mfile := range mf {
		if sidecarFilenames[mfile.name] {
			continue
		}
		wf := &queries.WorkFile{
			ID:        uuid.NewString(),
			WorkID:    workID,
			Filename:  mfile.name,
			Format:    mfile.format,
			SizeBytes: mfile.size,
		}
		if err := queries.UpsertWorkFile(s.db, wf); err != nil {
			return fmt.Errorf("upserting discovered file %q: %w", mfile.name, err)
		}
	}

	// Audiobook chapters
	if sc.Audiobook != nil && len(sc.Audiobook.Chapters) > 0 {
		if err := queries.DeleteWorkChapters(s.db, workID); err != nil {
			return err
		}
		for _, ch := range sc.Audiobook.Chapters {
			c := &queries.AudiobookChapter{
				ID:            uuid.NewString(),
				WorkID:        workID,
				Title:         ch.Title,
				StartSeconds:  ch.StartSeconds,
				EndSeconds:    ch.EndSeconds,
				IndexPosition: ch.Index,
			}
			if err := queries.UpsertAudiobookChapter(s.db, c); err != nil {
				return fmt.Errorf("upserting chapter %d: %w", ch.Index, err)
			}
		}
	}

	// Covers
	if sc.Covers != nil {
		if err := queries.DeleteWorkCovers(s.db, workID); err != nil {
			return err
		}
		for src, cv := range sc.Covers.Sources {
			if cv == nil {
				continue
			}
				cover := &queries.Cover{
				WorkID:     workID,
				Source:     src,
				Filename:   cv.Filename,
				Width:      cv.Width,
				Height:     cv.Height,
				IsSelected: sc.Covers.Selected == src,
			}
			if err := queries.UpsertCover(s.db, cover); err != nil {
				return fmt.Errorf("upserting cover %q: %w", src, err)
			}
		}
	}

	// Update FTS denormalized fields
	if err := queries.UpdateFTSDenormalized(s.db, workID); err != nil {
		slog.Warn("FTS update failed", "work_id", workID, "error", err)
		// Non-fatal
	}

	slog.Debug("indexed work", "dir", relDir, "title", sc.Title)
	return nil
}

// extractFromFiles attempts to read metadata from media files in a directory.
func (s *Scanner) extractFromFiles(absDir string, mf []mediaFile) (*extractedMeta, error) {
	meta := &extractedMeta{}

	// Try EPUB first (richest metadata source)
	for _, f := range mf {
		if f.format == "epub" {
			epubPath, err := security.SafePath(filepath.Join(absDir, f.name), s.mediaRoot.RootPath)
			if err != nil {
				slog.Warn("skipping epub with unsafe path", "file", f.name, "error", err)
				continue
			}
			m, err := ExtractEPUBMeta(epubPath)
			if err != nil {
				slog.Debug("EPUB extraction failed", "file", f.name, "error", err)
				continue
			}
			meta.fromEPUB(m)
			break
		}
	}

	// Try M4B/audiobook for audio metadata and chapters
	for _, f := range mf {
		if audiobookFormats[f.format] {
			audioPath, err := security.SafePath(filepath.Join(absDir, f.name), s.mediaRoot.RootPath)
			if err != nil {
				slog.Warn("skipping audio file with unsafe path", "file", f.name, "error", err)
				continue
			}
			m, err := ExtractAudioMeta(audioPath)
			if err != nil {
				slog.Debug("audio extraction failed", "file", f.name, "error", err)
				continue
			}
			meta.fromAudio(m)
			break
		}
	}

	// Fall back to folder hints if we couldn't get a title
	if meta.title == "" {
		relDir, err := filepath.Rel(s.mediaRoot.RootPath, absDir)
		if err != nil {
			slog.Warn("computing relative path for hints failed", "path", absDir, "error", err)
			return meta, nil
		}
		hints := ParseFolderHints(relDir)
		meta.fromHints(hints)
	}

	return meta, nil
}

// extractedMeta is an intermediate struct that accumulates metadata from
// multiple sources before building the final Sidecar.
type extractedMeta struct {
	title       string
	sortTitle   string
	subtitle    string
	language    string
	publisher   string
	publishDate string
	description string
	isbn13      string
	isbn10      string
	subjects    []string
	authors     []SidecarContributor
	narrators   []SidecarContributor
	seriesName  string
	seriesPos   *float64
	duration    int
	chapters    []SidecarChapter
	coverData   []byte
	coverExt    string
}

func (m *extractedMeta) fromEPUB(e *EPUBMeta) {
	if e.Title != "" {
		m.title = e.Title
		m.sortTitle = e.SortTitle
	}
	if e.Subtitle != "" {
		m.subtitle = e.Subtitle
	}
	if e.Language != "" {
		m.language = e.Language
	}
	if e.Publisher != "" {
		m.publisher = e.Publisher
	}
	if e.PublishDate != "" {
		m.publishDate = e.PublishDate
	}
	if e.Description != "" {
		m.description = e.Description
	}
	if e.ISBN13 != "" {
		m.isbn13 = e.ISBN13
	}
	if e.ISBN10 != "" {
		m.isbn10 = e.ISBN10
	}
	m.subjects = append(m.subjects, e.Subjects...)

	for _, a := range e.Authors {
		m.authors = append(m.authors, SidecarContributor{
			Name:     a.Name,
			SortName: a.SortName,
			Roles:    []string{"author"},
		})
	}
	for _, n := range e.Narrators {
		m.narrators = append(m.narrators, SidecarContributor{
			Name:     n.Name,
			SortName: n.SortName,
			Roles:    []string{"narrator"},
		})
	}
}

func (m *extractedMeta) fromAudio(a *AudioMeta) {
	if m.title == "" && a.Title != "" {
		m.title = a.Title
		m.sortTitle = a.SortTitle
	}
	if len(m.authors) == 0 && len(a.Artists) > 0 {
		for _, artist := range a.Artists {
			m.authors = append(m.authors, SidecarContributor{
				Name:     artist,
				SortName: deriveSortName(artist),
				Roles:    []string{"author"},
			})
		}
	}
	if len(m.narrators) == 0 && a.Narrator != "" {
		m.narrators = append(m.narrators, SidecarContributor{
			Name:     a.Narrator,
			SortName: deriveSortName(a.Narrator),
			Roles:    []string{"narrator"},
		})
	}
	if m.publisher == "" && a.Publisher != "" {
		m.publisher = a.Publisher
	}
	if m.description == "" && a.Description != "" {
		m.description = a.Description
	}
	if a.DurationSeconds > 0 {
		m.duration = a.DurationSeconds
	}
	if len(a.Chapters) > 0 {
		for _, ch := range a.Chapters {
			m.chapters = append(m.chapters, SidecarChapter{
				Title:        ch.Title,
				StartSeconds: ch.StartSeconds,
				EndSeconds:   ch.EndSeconds,
				Index:        ch.Index,
			})
		}
	}
	if len(a.CoverData) > 0 && len(m.coverData) == 0 {
		m.coverData = a.CoverData
		m.coverExt = a.CoverExt
	}
}

func (m *extractedMeta) fromHints(h FolderHints) {
	if m.title == "" {
		m.title = h.Title
		m.sortTitle = SortTitle(h.Title)
	}
	if len(m.authors) == 0 && h.Author != "" {
		m.authors = []SidecarContributor{{
			Name:     h.Author,
			SortName: deriveSortName(h.Author),
			Roles:    []string{"author"},
		}}
	}
	if m.seriesName == "" && h.SeriesName != "" {
		m.seriesName = h.SeriesName
		m.seriesPos = h.SeriesPosition
	}
}

// buildSidecar creates a minimal Sidecar from extracted metadata and media files.
func buildSidecar(relDir string, meta *extractedMeta, mf []mediaFile) *Sidecar {
	if meta == nil {
		meta = &extractedMeta{}
	}

	title := meta.title
	if title == "" {
		// Last resort: use the directory name
		title = filepath.Base(relDir)
	}
	sortTitle := meta.sortTitle
	if sortTitle == "" {
		sortTitle = SortTitle(title)
	}

	sc := &Sidecar{
		SchemaVersion:     schemaVersion,
		Title:             title,
		SortTitle:         sortTitle,
		Subtitle:          meta.subtitle,
		Language:          meta.language,
		Publisher:         meta.publisher,
		PublishDate:       meta.publishDate,
		Description:       meta.description,
		DescriptionFormat: "plain",
		Subjects:          meta.subjects,
		Metadata: SidecarMeta{
			NeedsReview: true, // always needs review until enriched
		},
	}

	// Identifiers
	if meta.isbn13 != "" || meta.isbn10 != "" {
		sc.Identifiers = map[string]*string{}
		if meta.isbn13 != "" {
			v := meta.isbn13
			sc.Identifiers["isbn_13"] = &v
		}
		if meta.isbn10 != "" {
			v := meta.isbn10
			sc.Identifiers["isbn_10"] = &v
		}
	}

	// Contributors
	sc.Contributors = append(sc.Contributors, meta.authors...)
	sc.Contributors = append(sc.Contributors, meta.narrators...)

	// Series
	if meta.seriesName != "" {
		sc.Series = []SidecarSeries{{
			Name:     meta.seriesName,
			Position: meta.seriesPos,
		}}
	}

	// Files
	now := time.Now().UTC()
	for _, f := range mf {
		sf := SidecarFile{
			Filename:  f.name,
			Format:    f.format,
			SizeBytes: f.size,
			AddedAt:   now,
		}
		sc.Files = append(sc.Files, sf)
	}

	// Audiobook
	if meta.duration > 0 || len(meta.chapters) > 0 {
		sc.Audiobook = &SidecarAudiobook{
			DurationSeconds: meta.duration,
			Chapters:        meta.chapters,
		}
	}

	return sc
}

// ── File detection helpers ───────────────────────────────────────────────────

type mediaFile struct {
	name   string
	format string
	size   int64
}

// findMediaFiles returns all supported media files in dir (not recursive).
func findMediaFiles(dir string) []mediaFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("failed to read directory for media files", "dir", dir, "error", err)
		return nil
	}

	var files []mediaFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Reject filenames containing path separators to prevent traversal.
		if strings.ContainsAny(name, "/\\") || strings.ContainsRune(name, 0) {
			slog.Warn("skipping file with suspicious name", "dir", dir, "name", name)
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		format, ok := supportedFormats[ext]
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			slog.Debug("failed to stat media file", "dir", dir, "name", name, "error", err)
			continue
		}
		files = append(files, mediaFile{
			name:   name,
			format: format,
			size:   info.Size(),
		})
	}
	return files
}

// hashFile computes the SHA-256 hex digest of a file.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
