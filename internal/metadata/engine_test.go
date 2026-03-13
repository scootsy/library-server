package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/scootsy/library-server/internal/config"
	"github.com/scootsy/library-server/internal/database/migrations"
	"github.com/scootsy/library-server/internal/database/queries"
	"github.com/scootsy/library-server/internal/metadata/sources"
)

// openTestDB creates a temporary SQLite database with the full schema applied.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp("", "codex-engine-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite3", f.Name()+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := migrations.Run(db); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}
	return db
}

// insertTestWork creates a media root and work for testing.
func insertTestWork(t *testing.T, db *sql.DB, title, author string) string {
	t.Helper()
	rootID := "test-root"
	if err := queries.UpsertMediaRoot(db, rootID, "Test", "/media"); err != nil {
		t.Fatal(err)
	}
	workID := "test-work-1"
	w := &queries.Work{
		ID:            workID,
		MediaRootID:   rootID,
		DirectoryPath: "Author/Book",
		Title:         title,
		SortTitle:     title,
		NeedsReview:   true,
	}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}
	// Add author as contributor
	if author != "" {
		cID, err := queries.UpsertContributor(db, "c-1", author, author, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := queries.UpsertWorkContributor(db, workID, cID, "author", 0); err != nil {
			t.Fatal(err)
		}
	}
	return workID
}

// mockSource is a test MetadataSource that returns preconfigured candidates.
type mockSource struct {
	name       string
	candidates []sources.Candidate
	err        error
}

func (m *mockSource) Name() string { return m.name }
func (m *mockSource) Search(_ context.Context, _ sources.Query) ([]sources.Candidate, error) {
	return m.candidates, m.err
}
func (m *mockSource) FetchByID(_ context.Context, _, _ string) (*sources.Candidate, error) {
	return nil, nil
}

func TestEngine_FetchAndScore_HighConfidence(t *testing.T) {
	db := openTestDB(t)
	workID := insertTestWork(t, db, "The Way of Kings", "Brandon Sanderson")

	src := &mockSource{
		name: "test_source",
		candidates: []sources.Candidate{
			{
				Source:     "test_source",
				ExternalID: "ext-1",
				Title:      "The Way of Kings",
				Authors: []sources.Contributor{
					{Name: "Brandon Sanderson", Role: "author"},
				},
				Identifiers: map[string]string{"isbn_13": "9780765326355"},
				CoverURL:    "",
				FetchedAt:   time.Now(),
			},
		},
	}

	cfg := &config.MetadataConfig{
		AutoEnrich:               true,
		ConfidenceAutoApply:      0.85,
		ConfidenceMinMatch:       0.50,
		SourceCacheRetentionDays: 90,
	}

	engine := NewEngine(db, cfg, []sources.MetadataSource{src})
	scored, err := engine.fetchAndScore(workID)
	if err != nil {
		t.Fatalf("fetchAndScore: %v", err)
	}
	if len(scored) == 0 {
		t.Fatal("expected at least one scored candidate")
	}
	if scored[0].Score.Overall < 0.85 {
		t.Errorf("expected high confidence, got %.2f", scored[0].Score.Overall)
	}
	if scored[0].Candidate.Title != "The Way of Kings" {
		t.Errorf("Title = %q, want %q", scored[0].Candidate.Title, "The Way of Kings")
	}
}

func TestEngine_FetchAndScore_LowConfidence(t *testing.T) {
	db := openTestDB(t)
	workID := insertTestWork(t, db, "The Way of Kings", "Brandon Sanderson")

	src := &mockSource{
		name: "test_source",
		candidates: []sources.Candidate{
			{
				Source:     "test_source",
				ExternalID: "ext-2",
				Title:      "Completely Different Book",
				Authors: []sources.Contributor{
					{Name: "Some Other Author", Role: "author"},
				},
				FetchedAt: time.Now(),
			},
		},
	}

	cfg := &config.MetadataConfig{
		AutoEnrich:               true,
		ConfidenceAutoApply:      0.85,
		ConfidenceMinMatch:       0.50,
		SourceCacheRetentionDays: 90,
	}

	engine := NewEngine(db, cfg, []sources.MetadataSource{src})
	scored, err := engine.fetchAndScore(workID)
	if err != nil {
		t.Fatalf("fetchAndScore: %v", err)
	}
	// Low confidence match should be filtered out
	if len(scored) > 0 && scored[0].Score.Overall >= 0.85 {
		t.Errorf("expected low confidence, got %.2f", scored[0].Score.Overall)
	}
}

func TestEngine_FetchAndScore_NoTitle(t *testing.T) {
	db := openTestDB(t)
	rootID := "test-root"
	if err := queries.UpsertMediaRoot(db, rootID, "Test", "/media"); err != nil {
		t.Fatal(err)
	}
	w := &queries.Work{
		ID:            "empty-work",
		MediaRootID:   rootID,
		DirectoryPath: "empty",
		Title:         "",
		SortTitle:     "",
	}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}

	cfg := &config.MetadataConfig{
		ConfidenceAutoApply: 0.85,
		ConfidenceMinMatch:  0.50,
	}
	engine := NewEngine(db, cfg, nil)
	_, err := engine.fetchAndScore("empty-work")
	if err == nil {
		t.Error("expected error for work with no title")
	}
}

func TestEngine_BuildQuery(t *testing.T) {
	db := openTestDB(t)
	workID := insertTestWork(t, db, "Mistborn", "Brandon Sanderson")

	// Add identifiers
	if err := queries.UpsertIdentifier(db, workID, "isbn_13", "9780765311788"); err != nil {
		t.Fatal(err)
	}
	if err := queries.UpsertIdentifier(db, workID, "asin", "B001QKBHG4"); err != nil {
		t.Fatal(err)
	}

	cfg := &config.MetadataConfig{}
	engine := NewEngine(db, cfg, nil)
	q, err := engine.buildQuery(workID)
	if err != nil {
		t.Fatalf("buildQuery: %v", err)
	}
	if q.Title != "Mistborn" {
		t.Errorf("Title = %q, want %q", q.Title, "Mistborn")
	}
	if q.Author != "Brandon Sanderson" {
		t.Errorf("Author = %q, want %q", q.Author, "Brandon Sanderson")
	}
	if q.ISBN != "9780765311788" {
		t.Errorf("ISBN = %q, want %q", q.ISBN, "9780765311788")
	}
	if q.ASIN != "B001QKBHG4" {
		t.Errorf("ASIN = %q, want %q", q.ASIN, "B001QKBHG4")
	}
}

func TestEngine_SourceCache(t *testing.T) {
	db := openTestDB(t)
	workID := insertTestWork(t, db, "Dune", "Frank Herbert")

	callCount := 0
	src := &mockSource{
		name: "cache_test",
		candidates: []sources.Candidate{
			{
				Source:     "cache_test",
				ExternalID: "ext-cache",
				Title:      "Dune",
				Authors:    []sources.Contributor{{Name: "Frank Herbert", Role: "author"}},
				FetchedAt:  time.Now(),
			},
		},
	}

	cfg := &config.MetadataConfig{
		ConfidenceAutoApply:      0.85,
		ConfidenceMinMatch:       0.50,
		SourceCacheRetentionDays: 90,
	}

	engine := NewEngine(db, cfg, []sources.MetadataSource{src})

	// First call should hit the source
	_, err := engine.fetchAndScore(workID)
	if err != nil {
		t.Fatalf("first fetchAndScore: %v", err)
	}

	// Verify cache was written
	cached, err := queries.GetSourceCache(db, workID, "cache_test")
	if err != nil {
		t.Fatalf("GetSourceCache: %v", err)
	}
	if cached == nil {
		t.Fatal("expected cache entry after first fetch")
	}

	// Verify cached response is valid JSON
	var cachedCandidates []sources.Candidate
	if err := json.Unmarshal([]byte(cached.Response), &cachedCandidates); err != nil {
		t.Fatalf("cached response is not valid JSON: %v", err)
	}
	if len(cachedCandidates) != 1 {
		t.Errorf("cached %d candidates, want 1", len(cachedCandidates))
	}

	_ = callCount // mockSource doesn't track calls, but cache is verified
}

func TestEngine_EnqueueWork(t *testing.T) {
	db := openTestDB(t)
	_ = insertTestWork(t, db, "Test", "Author")

	tests := []struct {
		name       string
		autoEnrich bool
		taskType   string
		wantQueued bool
	}{
		{"auto_match with auto_enrich on", true, "auto_match", true},
		{"auto_match with auto_enrich off", false, "auto_match", false},
		{"manual_search always enqueues", false, "manual_search", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDB := openTestDB(t)
			_ = insertTestWork(t, testDB, "Test", "Author")

			cfg := &config.MetadataConfig{AutoEnrich: tt.autoEnrich}
			engine := NewEngine(testDB, cfg, nil)

			err := engine.EnqueueWork("test-work-1", tt.taskType, 0)
			if err != nil {
				t.Fatalf("EnqueueWork: %v", err)
			}

			count, err := queries.GetPendingTaskCount(testDB)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantQueued && count == 0 {
				t.Error("expected task to be queued")
			}
			if !tt.wantQueued && count > 0 {
				t.Error("expected no task to be queued")
			}
		})
	}
}

func TestEngine_ProcessNextTask_EmptyQueue(t *testing.T) {
	db := openTestDB(t)
	cfg := &config.MetadataConfig{
		ConfidenceAutoApply: 0.85,
		ConfidenceMinMatch:  0.50,
	}
	engine := NewEngine(db, cfg, nil)
	err := engine.processNextTask()
	if err != nil {
		t.Fatalf("processNextTask on empty queue: %v", err)
	}
}

func TestEngine_PurgeExpiredCache(t *testing.T) {
	db := openTestDB(t)
	_ = insertTestWork(t, db, "Test", "Author")

	// Insert a cache entry
	entry := &queries.SourceCacheEntry{
		WorkID:    "test-work-1",
		Source:    "test_source",
		QueryUsed: "{}",
		Response:  "[]",
	}
	if err := queries.UpsertSourceCache(db, entry); err != nil {
		t.Fatal(err)
	}

	cfg := &config.MetadataConfig{SourceCacheRetentionDays: 90}
	engine := NewEngine(db, cfg, nil)

	// Purge should not remove anything (entry is fresh)
	n, err := engine.PurgeExpiredCache()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("purged %d entries, want 0", n)
	}
}
