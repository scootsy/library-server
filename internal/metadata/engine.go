// Package metadata provides the metadata enrichment engine: queue processing,
// external source querying, fuzzy matching, confidence scoring, merge strategy,
// sidecar writing, and cover downloading.
package metadata

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/database/queries"
	"github.com/scootsy/library-server/internal/metadata/sources"
)

// ScoredCandidate pairs a source candidate with its confidence score.
type ScoredCandidate struct {
	Candidate sources.Candidate `json:"candidate"`
	Score     Score             `json:"score"`
}

// Engine is the metadata enrichment orchestrator. It dequeues tasks, queries
// enabled metadata sources, scores candidates, and either auto-applies high-
// confidence matches or marks the task for manual review.
type Engine struct {
	db      *sql.DB
	cfg     *config.MetadataConfig
	sources []sources.MetadataSource
	writer  *SidecarWriter

	// stopCh signals the background worker to shut down.
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewEngine creates a metadata engine with the given sources and config.
func NewEngine(db *sql.DB, cfg *config.MetadataConfig, srcs []sources.MetadataSource) *Engine {
	return &Engine{
		db:      db,
		cfg:     cfg,
		sources: srcs,
		writer:  NewSidecarWriter(nil), // default HTTP client
		stopCh:  make(chan struct{}),
	}
}

// Start begins the background task queue worker. It polls the queue every
// pollInterval and processes one task at a time. Call Stop to shut down.
func (e *Engine) Start(pollInterval time.Duration) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.runWorker(pollInterval)
	}()
	slog.Info("metadata engine started", "sources", len(e.sources), "poll_interval", pollInterval)
}

// Stop gracefully shuts down the background worker and waits for it to finish.
func (e *Engine) Stop() {
	close(e.stopCh)
	e.wg.Wait()
	slog.Info("metadata engine stopped")
}

// runWorker is the main loop that polls for and processes tasks.
func (e *Engine) runWorker(pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			if err := e.processNextTask(); err != nil {
				slog.Error("metadata task processing failed", "error", err)
			}
		}
	}
}

// processNextTask dequeues and processes a single task. Returns nil if the
// queue is empty.
func (e *Engine) processNextTask() error {
	task, err := queries.DequeueMetadataTask(e.db)
	if err != nil {
		return fmt.Errorf("dequeuing task: %w", err)
	}
	if task == nil {
		return nil // queue empty
	}

	slog.Info("processing metadata task", "task_id", task.ID, "work_id", task.WorkID, "type", task.TaskType)

	scored, err := e.fetchAndScore(task.WorkID)
	if err != nil {
		errMsg := err.Error()
		if completeErr := queries.CompleteMetadataTask(e.db, task.ID, "failed", "", errMsg); completeErr != nil {
			slog.Error("failed to mark task as failed", "task_id", task.ID, "error", completeErr)
		}
		return fmt.Errorf("fetch and score for work %q: %w", task.WorkID, err)
	}

	if len(scored) == 0 {
		if completeErr := queries.CompleteMetadataTask(e.db, task.ID, "completed", "[]", ""); completeErr != nil {
			return fmt.Errorf("completing empty task: %w", completeErr)
		}
		slog.Info("no candidates found", "work_id", task.WorkID)
		return nil
	}

	candidatesJSON, err := json.Marshal(scored)
	if err != nil {
		return fmt.Errorf("marshalling candidates: %w", err)
	}

	best := scored[0]
	if best.Score.Overall >= e.cfg.ConfidenceAutoApply {
		// Auto-apply: write merged metadata to sidecar
		if err := e.applyCandidate(task.WorkID, best); err != nil {
			slog.Error("auto-apply failed, marking for review",
				"work_id", task.WorkID, "error", err)
			return queries.CompleteMetadataTask(e.db, task.ID, "review", string(candidatesJSON), "")
		}
		if err := queries.CompleteMetadataTask(e.db, task.ID, "completed", string(candidatesJSON), ""); err != nil {
			return fmt.Errorf("completing auto-applied task: %w", err)
		}
		if err := queries.SetTaskSelected(e.db, task.ID, 0); err != nil {
			slog.Warn("failed to set task selected index", "task_id", task.ID, "error", err)
		}
		slog.Info("auto-applied metadata",
			"work_id", task.WorkID,
			"source", best.Candidate.Source,
			"confidence", fmt.Sprintf("%.2f", best.Score.Overall))
		return nil
	}

	// Below auto-apply threshold: mark for review
	status := "review"
	if best.Score.Overall < e.cfg.ConfidenceMinMatch {
		// Below minimum match threshold: no useful candidates
		status = "completed"
		slog.Info("no candidates above minimum threshold",
			"work_id", task.WorkID,
			"best_score", fmt.Sprintf("%.2f", best.Score.Overall))
	} else {
		slog.Info("candidates require review",
			"work_id", task.WorkID,
			"best_score", fmt.Sprintf("%.2f", best.Score.Overall),
			"candidate_count", len(scored))
	}
	return queries.CompleteMetadataTask(e.db, task.ID, status, string(candidatesJSON), "")
}

// fetchAndScore queries all enabled sources and returns scored candidates
// sorted by confidence (highest first). Candidates below ConfidenceMinMatch
// are filtered out.
func (e *Engine) fetchAndScore(workID string) ([]ScoredCandidate, error) {
	// Build search query from the work's existing metadata
	query, err := e.buildQuery(workID)
	if err != nil {
		return nil, fmt.Errorf("building query: %w", err)
	}

	if query.Title == "" {
		return nil, fmt.Errorf("work %q has no title for metadata search", workID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var allCandidates []sources.Candidate
	for _, src := range e.sources {
		candidates, err := e.querySource(ctx, src, *query, workID)
		if err != nil {
			slog.Warn("source query failed",
				"source", src.Name(),
				"work_id", workID,
				"error", err)
			continue
		}
		allCandidates = append(allCandidates, candidates...)
	}

	// Score and filter
	var scored []ScoredCandidate
	for _, c := range allCandidates {
		s := ScoreCandidate(c, *query)
		if s.Overall >= e.cfg.ConfidenceMinMatch {
			scored = append(scored, ScoredCandidate{
				Candidate: c,
				Score:     s,
			})
		}
	}

	// Sort by confidence descending
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score.Overall == scored[j].Score.Overall {
			pi := e.sourcePriority(scored[i].Candidate.Source)
			pj := e.sourcePriority(scored[j].Candidate.Source)
			if pi == pj {
				return scored[i].Candidate.Source < scored[j].Candidate.Source
			}
			return pi < pj
		}
		return scored[i].Score.Overall > scored[j].Score.Overall
	})

	return scored, nil
}

// querySource queries a single source and caches the raw response.
func (e *Engine) querySource(ctx context.Context, src sources.MetadataSource, query sources.Query, workID string) ([]sources.Candidate, error) {
	// Check source cache first
	cached, err := queries.GetSourceCache(e.db, workID, src.Name())
	if err != nil {
		slog.Warn("source cache lookup failed", "source", src.Name(), "error", err)
		// Non-fatal: proceed without cache
	}
	if cached != nil {
		retentionCutoff := time.Now().AddDate(0, 0, -e.cfg.SourceCacheRetentionDays)
		if cached.FetchedAt.After(retentionCutoff) {
			// Cache hit: decode stored candidates
			var candidates []sources.Candidate
			if err := json.Unmarshal([]byte(cached.Response), &candidates); err != nil {
				slog.Warn("cached response decode failed, re-fetching",
					"source", src.Name(), "work_id", workID, "error", err)
			} else {
				slog.Debug("using cached source response",
					"source", src.Name(), "work_id", workID)
				return candidates, nil
			}
		}
	}

	// Fetch from source
	candidates, err := src.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", src.Name(), err)
	}

	// Cache the response
	queryJSON, err := json.Marshal(query)
	if err != nil {
		slog.Warn("failed to marshal query for cache", "error", err)
		return candidates, nil
	}
	responseJSON, err := json.Marshal(candidates)
	if err != nil {
		slog.Warn("failed to marshal candidates for cache", "error", err)
		return candidates, nil
	}
	cacheEntry := &queries.SourceCacheEntry{
		WorkID:    workID,
		Source:    src.Name(),
		QueryUsed: string(queryJSON),
		Response:  string(responseJSON),
	}
	if err := queries.UpsertSourceCache(e.db, cacheEntry); err != nil {
		slog.Warn("failed to write source cache", "source", src.Name(), "error", err)
	}

	return candidates, nil
}

// buildQuery constructs a search query from the work's stored metadata.
func (e *Engine) buildQuery(workID string) (*sources.Query, error) {
	work, err := queries.GetWorkByID(e.db, workID)
	if err != nil {
		return nil, fmt.Errorf("fetching work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %q not found", workID)
	}

	q := &sources.Query{
		Title: work.Title,
	}

	// Get primary author
	authors, err := queries.GetWorkAuthorNames(e.db, workID)
	if err != nil {
		slog.Warn("failed to fetch authors for query", "work_id", workID, "error", err)
	} else if len(authors) > 0 {
		q.Author = authors[0]
	}

	// Get identifiers (ISBN, ASIN)
	ids, err := queries.GetWorkIdentifiers(e.db, workID)
	if err != nil {
		slog.Warn("failed to fetch identifiers for query", "work_id", workID, "error", err)
	} else {
		if isbn, ok := ids["isbn_13"]; ok {
			q.ISBN = isbn
		} else if isbn, ok := ids["isbn_10"]; ok {
			q.ISBN = isbn
		}
		if asin, ok := ids["asin"]; ok {
			q.ASIN = asin
		}
	}

	return q, nil
}

// ApplyCandidate applies a specific scored candidate to a work's sidecar.
// This is the public entry point for manual selection from the review queue.
func (e *Engine) ApplyCandidate(workID string, candidate ScoredCandidate) error {
	return e.applyCandidate(workID, candidate)
}

// applyCandidate merges a scored candidate into the work's sidecar and updates
// the database.
func (e *Engine) applyCandidate(workID string, sc ScoredCandidate) error {
	rootPath, dirPath, err := queries.GetWorkDirectoryPath(e.db, workID)
	if err != nil {
		return fmt.Errorf("getting work directory: %w", err)
	}

	absDir := filepath.Join(rootPath, dirPath)

	if err := e.writer.MergeAndWrite(absDir, rootPath, sc); err != nil {
		return fmt.Errorf("merge and write sidecar: %w", err)
	}

	slog.Info("applied candidate to sidecar",
		"work_id", workID,
		"source", sc.Candidate.Source,
		"confidence", fmt.Sprintf("%.2f", sc.Score.Overall))
	return nil
}

// EnqueueWork creates a metadata task for the given work if auto-enrich is
// enabled and no pending/running task already exists.
func (e *Engine) EnqueueWork(workID, taskType string, priority int) error {
	if !e.cfg.AutoEnrich && taskType == "auto_match" {
		return nil
	}

	task := &queries.MetadataTask{
		ID:       generateTaskID(),
		WorkID:   workID,
		TaskType: taskType,
		Priority: priority,
	}
	return queries.EnqueueMetadataTask(e.db, task)
}

// PurgeExpiredCache removes source cache entries older than the configured
// retention period. Returns the number of entries purged.
func (e *Engine) PurgeExpiredCache() (int64, error) {
	return queries.PurgeExpiredSourceCache(e.db, e.cfg.SourceCacheRetentionDays)
}

// generateTaskID creates a unique task identifier with a timestamp prefix
// for rough ordering and a cryptographic random suffix for uniqueness.
func generateTaskID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback: timestamp-only (still unique per ON CONFLICT in DB)
		return fmt.Sprintf("mt_%d", time.Now().UnixMilli())
	}
	return fmt.Sprintf("mt_%d_%s", time.Now().UnixMilli(), hex.EncodeToString(b))
}

func (e *Engine) sourcePriority(source string) int {
	switch source {
	case "google_books":
		if e.cfg.GoogleBooks.Priority > 0 {
			return e.cfg.GoogleBooks.Priority
		}
		return 10
	case "hardcover":
		if e.cfg.Hardcover.Priority > 0 {
			return e.cfg.Hardcover.Priority
		}
		return 20
	case "open_library":
		if e.cfg.OpenLibrary.Priority > 0 {
			return e.cfg.OpenLibrary.Priority
		}
		return 30
	case "audnexus":
		if e.cfg.Audnexus.Priority > 0 {
			return e.cfg.Audnexus.Priority
		}
		return 40
	default:
		return 1000
	}
}
