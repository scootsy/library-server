package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scootsy/library-server/internal/database/queries"
	"github.com/scootsy/library-server/internal/metadata"
	"github.com/scootsy/library-server/internal/metadata/sources"
	"github.com/scootsy/library-server/internal/scanner"
	"github.com/scootsy/library-server/internal/security"
)

// MetadataHandler handles REST endpoints for metadata operations.
type MetadataHandler struct {
	db     *sql.DB
	engine *metadata.Engine
}

// Refresh triggers a metadata refresh for a specific work.
func (h *MetadataHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("workID")

	work, err := queries.GetWorkByID(h.db, workID)
	if err != nil {
		slog.Error("failed to get work", "id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get work")
		return
	}
	if work == nil {
		writeError(w, http.StatusNotFound, "work not found")
		return
	}

	if err := h.engine.EnqueueWork(workID, "refresh", 1); err != nil {
		slog.Error("failed to enqueue refresh", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to enqueue refresh")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "queued",
		"work_id": workID,
	})
}

// GetTasks returns all metadata tasks for a given work.
func (h *MetadataHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("workID")

	tasks, err := queries.GetTasksForWork(h.db, workID)
	if err != nil {
		slog.Error("failed to get tasks", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tasks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tasks})
}

type applyCandidateRequest struct {
	CandidateIndex int `json:"candidate_index"`
}

// ApplyCandidate applies a selected metadata candidate from a task.
func (h *MetadataHandler) ApplyCandidate(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")

	var req applyCandidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	task, err := queries.GetMetadataTaskByID(h.db, taskID)
	if err != nil {
		slog.Error("failed to get task", "task_id", taskID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Parse candidates from JSON.
	var candidates []metadata.ScoredCandidate
	if task.Candidates != "" {
		if err := json.Unmarshal([]byte(task.Candidates), &candidates); err != nil {
			slog.Error("failed to parse candidates", "task_id", taskID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to parse candidates")
			return
		}
	}

	if req.CandidateIndex < 0 || req.CandidateIndex >= len(candidates) {
		writeError(w, http.StatusBadRequest, "candidate_index out of range")
		return
	}

	selected := candidates[req.CandidateIndex]
	if err := h.engine.ApplyCandidate(task.WorkID, selected); err != nil {
		slog.Error("failed to apply candidate", "task_id", taskID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to apply candidate")
		return
	}

	if err := queries.SetTaskSelected(h.db, taskID, req.CandidateIndex); err != nil {
		slog.Error("failed to record selection", "task_id", taskID, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "applied",
		"task_id": taskID,
	})
}

// ReviewQueue returns works that need metadata review.
func (h *MetadataHandler) ReviewQueue(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	works, total, err := queries.GetReviewQueue(h.db, limit, offset)
	if err != nil {
		slog.Error("failed to get review queue", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get review queue")
		return
	}

	writeJSON(w, http.StatusOK, paginatedResponse{
		Data:   works,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

type metadataFieldSet struct {
	Title           string                 `json:"title"`
	Subtitle        string                 `json:"subtitle"`
	Description     string                 `json:"description"`
	Publisher       string                 `json:"publisher"`
	PublishDate     string                 `json:"publish_date"`
	Language        string                 `json:"language"`
	PageCount       int                    `json:"page_count"`
	Authors         []string               `json:"authors"`
	Narrators       []string               `json:"narrators"`
	Series          string                 `json:"series"`
	SeriesPosition  float64                `json:"series_position"`
	Tags            []string               `json:"tags"`
	ISBN            string                 `json:"isbn"`
	CoverURL        string                 `json:"cover_url"`
	Rating          *sources.Rating        `json:"rating"`
	DurationSeconds int                    `json:"duration_seconds"`
	Identifiers     map[string]string      `json:"-"`
	Raw             map[string]interface{} `json:"-"`
}

type fetchFromSourcesResponse struct {
	Current metadataFieldSet             `json:"current"`
	Sources map[string]*metadataFieldSet `json:"sources"`
}

// FetchFromSources queries all enabled metadata sources in real time for a work.
func (h *MetadataHandler) FetchFromSources(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("id")

	work, err := queries.GetWorkByID(h.db, workID)
	if err != nil {
		slog.Error("failed to get work", "id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get work")
		return
	}
	if work == nil {
		writeError(w, http.StatusNotFound, "work not found")
		return
	}

	current, err := h.buildCurrentMetadata(workID, work)
	if err != nil {
		slog.Error("failed to build current metadata", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to build current metadata")
		return
	}

	sourceCandidates, err := h.engine.FetchAllSources(context.Background(), workID)
	if err != nil {
		slog.Error("failed to fetch from metadata sources", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch metadata")
		return
	}

	sourcePayload := make(map[string]*metadataFieldSet, len(sourceCandidates))
	for sourceName, candidate := range sourceCandidates {
		sourcePayload[sourceName] = candidateToFieldSet(candidate)
	}

	writeJSON(w, http.StatusOK, fetchFromSourcesResponse{Current: *current, Sources: sourcePayload})
}

type patchMetadataRequest map[string]json.RawMessage

// PatchMetadata applies partial metadata updates for a work.
func (h *MetadataHandler) PatchMetadata(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("id")

	work, err := queries.GetWorkByID(h.db, workID)
	if err != nil {
		slog.Error("failed to get work", "id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get work")
		return
	}
	if work == nil {
		writeError(w, http.StatusNotFound, "work not found")
		return
	}

	var payload patchMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := h.applyMetadataPatch(workID, payload); err != nil {
		slog.Error("failed to patch metadata", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to patch metadata")
		return
	}

	updated, err := queries.GetWorkByID(h.db, workID)
	if err != nil {
		slog.Error("failed to re-fetch work", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "metadata updated but failed to fetch work")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *MetadataHandler) buildCurrentMetadata(workID string, work *queries.Work) (*metadataFieldSet, error) {
	authors, err := queries.GetWorkAuthorNames(h.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching authors: %w", err)
	}

	contribs, err := queries.GetWorkContributors(h.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching contributors: %w", err)
	}
	narrators := make([]string, 0)
	for _, c := range contribs {
		if strings.EqualFold(c.Role, "narrator") {
			narrators = append(narrators, c.Name)
		}
	}

	seriesRows, err := queries.GetWorkSeries(h.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching series: %w", err)
	}
	series := ""
	seriesPosition := 0.0
	if len(seriesRows) > 0 {
		series = seriesRows[0].Name
		if seriesRows[0].Position != nil {
			seriesPosition = *seriesRows[0].Position
		}
	}

	tagRows, err := queries.GetWorkTags(h.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}
	tags := make([]string, 0, len(tagRows))
	for _, t := range tagRows {
		tags = append(tags, t.Name)
	}

	ids, err := queries.GetWorkIdentifiers(h.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching identifiers: %w", err)
	}
	isbn := ""
	if v := ids["isbn_13"]; v != "" {
		isbn = v
	} else if v := ids["isbn"]; v != "" {
		isbn = v
	} else if v := ids["isbn_10"]; v != "" {
		isbn = v
	}

	covers, err := queries.GetWorkCovers(h.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching covers: %w", err)
	}
	coverURL := ""
	for _, c := range covers {
		if c.IsSelected {
			coverURL = fmt.Sprintf("/api/works/%s/cover", workID)
			break
		}
	}
	if coverURL == "" && len(covers) > 0 {
		coverURL = fmt.Sprintf("/api/works/%s/cover", workID)
	}

	ratings, err := queries.GetWorkRatings(h.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching ratings: %w", err)
	}
	var rating *sources.Rating
	if len(ratings) > 0 {
		best := ratings[0]
		for _, rt := range ratings[1:] {
			if rt.Count > best.Count {
				best = rt
			}
		}
		rating = &sources.Rating{Score: best.Score, Max: best.MaxScore, Count: best.Count}
	}

	return &metadataFieldSet{
		Title:           work.Title,
		Subtitle:        work.Subtitle,
		Description:     work.Description,
		Publisher:       work.Publisher,
		PublishDate:     work.PublishDate,
		Language:        work.Language,
		PageCount:       work.PageCount,
		Authors:         authors,
		Narrators:       narrators,
		Series:          series,
		SeriesPosition:  seriesPosition,
		Tags:            tags,
		ISBN:            isbn,
		CoverURL:        coverURL,
		Rating:          rating,
		DurationSeconds: work.DurationSeconds,
	}, nil
}

func candidateToFieldSet(c *sources.Candidate) *metadataFieldSet {
	if c == nil {
		return &metadataFieldSet{}
	}
	authors := make([]string, 0, len(c.Authors))
	for _, a := range c.Authors {
		authors = append(authors, a.Name)
	}
	narrators := make([]string, 0, len(c.Narrators))
	for _, n := range c.Narrators {
		narrators = append(narrators, n.Name)
	}
	series := ""
	seriesPos := 0.0
	if len(c.Series) > 0 {
		series = c.Series[0].Name
		seriesPos = c.Series[0].Position
	}
	isbn := c.Identifiers["isbn_13"]
	if isbn == "" {
		isbn = c.Identifiers["isbn_10"]
	}
	if isbn == "" {
		isbn = c.Identifiers["isbn"]
	}
	return &metadataFieldSet{
		Title:           c.Title,
		Subtitle:        c.Subtitle,
		Description:     c.Description,
		Publisher:       c.Publisher,
		PublishDate:     c.PublishDate,
		Language:        c.Language,
		PageCount:       c.PageCount,
		Authors:         authors,
		Narrators:       narrators,
		Series:          series,
		SeriesPosition:  seriesPos,
		Tags:            c.Tags,
		ISBN:            isbn,
		CoverURL:        c.CoverURL,
		Rating:          c.Rating,
		DurationSeconds: c.DurationSecs,
	}
}

func (h *MetadataHandler) applyMetadataPatch(workID string, payload patchMetadataRequest) error {
	workFields := map[string]any{}

	if raw, ok := payload["title"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["title"] = strings.TrimSpace(v)
	}
	if raw, ok := payload["subtitle"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["subtitle"] = strings.TrimSpace(v)
	}
	if raw, ok := payload["description"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["description"] = strings.TrimSpace(v)
		workFields["description_format"] = "plain"
	}
	if raw, ok := payload["publisher"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["publisher"] = strings.TrimSpace(v)
	}
	if raw, ok := payload["publish_date"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["publish_date"] = strings.TrimSpace(v)
	}
	if raw, ok := payload["language"]; ok {
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["language"] = strings.TrimSpace(v)
	}
	if raw, ok := payload["page_count"]; ok {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["page_count"] = v
	}
	if raw, ok := payload["duration_seconds"]; ok {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		workFields["duration_seconds"] = v
	}

	if err := queries.UpdateWorkMetadata(h.db, workID, workFields); err != nil {
		return fmt.Errorf("updating work metadata fields: %w", err)
	}
	if raw, ok := payload["authors"]; ok {
		var names []string
		if err := json.Unmarshal(raw, &names); err != nil {
			return err
		}
		if err := queries.DeleteWorkContributorsByRole(h.db, workID, "author"); err != nil {
			return fmt.Errorf("deleting authors: %w", err)
		}
		for i, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id, err := queries.UpsertContributor(h.db, fmt.Sprintf("contrib_%d_%d", time.Now().UnixNano(), i), name, deriveSortName(name), map[string]string{})
			if err != nil {
				return err
			}
			if err := queries.UpsertWorkContributor(h.db, workID, id, "author", i); err != nil {
				return err
			}
		}
	}
	if raw, ok := payload["narrators"]; ok {
		var names []string
		if err := json.Unmarshal(raw, &names); err != nil {
			return err
		}
		if err := queries.DeleteWorkContributorsByRole(h.db, workID, "narrator"); err != nil {
			return fmt.Errorf("deleting narrators: %w", err)
		}
		for i, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id, err := queries.UpsertContributor(h.db, fmt.Sprintf("contrib_%d_%d", time.Now().UnixNano(), i), name, deriveSortName(name), map[string]string{})
			if err != nil {
				return err
			}
			if err := queries.UpsertWorkContributor(h.db, workID, id, "narrator", i); err != nil {
				return err
			}
		}
	}

	if raw, ok := payload["tags"]; ok {
		var tags []string
		if err := json.Unmarshal(raw, &tags); err != nil {
			return err
		}
		if err := queries.DeleteWorkTags(h.db, workID); err != nil {
			return err
		}
		for i, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			tagID, err := queries.UpsertTag(h.db, fmt.Sprintf("tag_%d_%d", time.Now().UnixNano(), i), tag, "genre")
			if err != nil {
				return err
			}
			if err := queries.UpsertWorkTag(h.db, workID, tagID); err != nil {
				return err
			}
		}
	}

	seriesSet := false
	var seriesName string
	var seriesPos float64
	if raw, ok := payload["series"]; ok {
		seriesSet = true
		if err := json.Unmarshal(raw, &seriesName); err != nil {
			return err
		}
		seriesName = strings.TrimSpace(seriesName)
	}
	if raw, ok := payload["series_position"]; ok {
		seriesSet = true
		if err := json.Unmarshal(raw, &seriesPos); err != nil {
			return err
		}
	}
	if seriesSet {
		if err := queries.DeleteWorkSeries(h.db, workID); err != nil {
			return err
		}
		if seriesName != "" {
			seriesID, err := queries.UpsertSeries(h.db, fmt.Sprintf("series_%d", time.Now().UnixNano()), seriesName, map[string]string{})
			if err != nil {
				return err
			}
			var p *float64
			if seriesPos > 0 {
				p = &seriesPos
			}
			if err := queries.UpsertWorkSeries(h.db, workID, seriesID, p); err != nil {
				return err
			}
		}
	}

	if raw, ok := payload["isbn_13"]; ok {
		var isbn string
		if err := json.Unmarshal(raw, &isbn); err != nil {
			return err
		}
		isbn = strings.TrimSpace(isbn)
		if isbn != "" {
			if err := queries.UpsertIdentifier(h.db, workID, "isbn_13", isbn); err != nil {
				return err
			}
		}
	}
	if raw, ok := payload["isbn"]; ok {
		var isbn string
		if err := json.Unmarshal(raw, &isbn); err != nil {
			return err
		}
		isbn = strings.TrimSpace(isbn)
		if isbn != "" {
			if err := queries.UpsertIdentifier(h.db, workID, "isbn_13", isbn); err != nil {
				return err
			}
		}
	}

	ratingPatch := struct {
		Score  *float64
		Max    *float64
		Count  *int
		Source string
	}{}
	if raw, ok := payload["rating_score"]; ok {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		ratingPatch.Score = &v
	}
	if raw, ok := payload["rating_max"]; ok {
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		ratingPatch.Max = &v
	}
	if raw, ok := payload["rating_count"]; ok {
		var v int
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		ratingPatch.Count = &v
	}
	if raw, ok := payload["rating_source"]; ok {
		if err := json.Unmarshal(raw, &ratingPatch.Source); err != nil {
			return err
		}
	}
	if ratingPatch.Score != nil {
		source := strings.TrimSpace(ratingPatch.Source)
		if source == "" {
			source = "manual"
		}
		max := 5.0
		if ratingPatch.Max != nil && *ratingPatch.Max > 0 {
			max = *ratingPatch.Max
		}
		count := 0
		if ratingPatch.Count != nil {
			count = *ratingPatch.Count
		}
		if err := queries.UpsertWorkRating(h.db, &queries.Rating{WorkID: workID, Source: source, Score: *ratingPatch.Score, MaxScore: max, Count: count, FetchedAt: time.Now().UTC()}); err != nil {
			return err
		}
	}

	if raw, ok := payload["cover_url"]; ok {
		var coverURL string
		if err := json.Unmarshal(raw, &coverURL); err != nil {
			return err
		}
		coverURL = strings.TrimSpace(coverURL)
		if coverURL != "" {
			if err := h.downloadAndStoreCover(workID, coverURL); err != nil {
				return fmt.Errorf("updating cover: %w", err)
			}
		}
	}

	if err := queries.UpdateFTSDenormalized(h.db, workID); err != nil {
		slog.Warn("failed to refresh FTS metadata", "work_id", workID, "error", err)
	}

	return h.writeSidecarFromDB(workID)
}

func (h *MetadataHandler) downloadAndStoreCover(workID, coverURL string) error {
	rootPath, dirPath, err := queries.GetWorkDirectoryPath(h.db, workID)
	if err != nil {
		return err
	}
	absDir := filepath.Join(rootPath, dirPath)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(coverURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover download returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return err
	}

	ext := ".jpg"
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "png") {
		ext = ".png"
	}
	if strings.Contains(ct, "webp") {
		ext = ".webp"
	}
	filename := "cover_manual" + ext
	dest, err := security.SafePathParent(filepath.Join(absDir, filename), rootPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return err
	}

	if err := queries.UpsertCover(h.db, &queries.Cover{WorkID: workID, Source: "manual", Filename: filename, IsSelected: true}); err != nil {
		return err
	}
	if err := queries.SelectCover(h.db, workID, "manual"); err != nil {
		return err
	}
	return nil
}

func (h *MetadataHandler) writeSidecarFromDB(workID string) error {
	work, err := queries.GetWorkByID(h.db, workID)
	if err != nil {
		return err
	}
	if work == nil {
		return fmt.Errorf("work not found")
	}
	rootPath, dirPath, err := queries.GetWorkDirectoryPath(h.db, workID)
	if err != nil {
		return err
	}
	absDir := filepath.Join(rootPath, dirPath)

	sc, _ := scanner.ReadSidecar(absDir, rootPath)
	if sc == nil {
		sc = &scanner.Sidecar{SchemaVersion: 1}
	}

	sc.Title = work.Title
	sc.Subtitle = work.Subtitle
	sc.SortTitle = work.SortTitle
	sc.Language = work.Language
	sc.Publisher = work.Publisher
	sc.PublishDate = work.PublishDate
	sc.Description = work.Description
	sc.DescriptionFormat = work.DescriptionFormat
	sc.PageCount = work.PageCount
	if work.DurationSeconds > 0 {
		if sc.Audiobook == nil {
			sc.Audiobook = &scanner.SidecarAudiobook{}
		}
		sc.Audiobook.DurationSeconds = work.DurationSeconds
	}

	authors, _ := queries.GetWorkAuthorNames(h.db, workID)
	contribs, _ := queries.GetWorkContributors(h.db, workID)
	contributors := make([]scanner.SidecarContributor, 0, len(contribs))
	for _, name := range authors {
		contributors = append(contributors, scanner.SidecarContributor{Name: name, SortName: deriveSortName(name), Roles: []string{"author"}})
	}
	for _, c := range contribs {
		if strings.EqualFold(c.Role, "narrator") {
			contributors = append(contributors, scanner.SidecarContributor{Name: c.Name, SortName: c.SortName, Roles: []string{"narrator"}})
		}
	}
	sc.Contributors = contributors

	seriesRows, _ := queries.GetWorkSeries(h.db, workID)
	if len(seriesRows) > 0 {
		sc.Series = []scanner.SidecarSeries{{Name: seriesRows[0].Name, Position: seriesRows[0].Position}}
	} else {
		sc.Series = nil
	}

	tagRows, _ := queries.GetWorkTags(h.db, workID)
	tags := make([]string, 0, len(tagRows))
	for _, t := range tagRows {
		tags = append(tags, t.Name)
	}
	sc.Tags = tags

	ids, _ := queries.GetWorkIdentifiers(h.db, workID)
	sc.Identifiers = map[string]*string{}
	for k, v := range ids {
		vv := v
		sc.Identifiers[k] = &vv
	}

	ratings, _ := queries.GetWorkRatings(h.db, workID)
	sc.Ratings = map[string]*scanner.SidecarRating{}
	for _, rt := range ratings {
		sc.Ratings[rt.Source] = &scanner.SidecarRating{Score: rt.Score, Max: rt.MaxScore, Count: rt.Count, FetchedAt: rt.FetchedAt}
	}

	covers, _ := queries.GetWorkCovers(h.db, workID)
	if len(covers) > 0 {
		sourcesMap := make(map[string]*scanner.SidecarCoverSrc)
		selected := ""
		for _, c := range covers {
			sourcesMap[c.Source] = &scanner.SidecarCoverSrc{Filename: c.Filename, Width: c.Width, Height: c.Height, FetchedAt: time.Now().UTC()}
			if c.IsSelected {
				selected = c.Source
			}
		}
		sc.Covers = &scanner.SidecarCovers{Selected: selected, Sources: sourcesMap}
	}

	return scanner.WriteSidecar(absDir, sc, rootPath)
}

func deriveSortName(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) <= 1 {
		return strings.TrimSpace(name)
	}
	last := parts[len(parts)-1]
	first := strings.Join(parts[:len(parts)-1], " ")
	return strings.TrimSpace(last + ", " + first)
}
