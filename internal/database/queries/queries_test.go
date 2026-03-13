package queries_test

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/scootsy/library-server/internal/database/migrations"
	"github.com/scootsy/library-server/internal/database/queries"
)

// openTestDB creates a temporary in-memory SQLite database with the full schema applied.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// Use a temp file so EvalSymlinks in SafePath works; in-memory DBs don't
	// support concurrent connections needed by some tests.
	f, err := os.CreateTemp("", "codex-test-*.db")
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

// ── Media roots ──────────────────────────────────────────────────────────────

func TestUpsertAndGetMediaRoot(t *testing.T) {
	db := openTestDB(t)

	if err := queries.UpsertMediaRoot(db, "root-1", "Books", "/media/books"); err != nil {
		t.Fatalf("UpsertMediaRoot: %v", err)
	}

	got, err := queries.GetMediaRootByPath(db, "/media/books")
	if err != nil {
		t.Fatalf("GetMediaRootByPath: %v", err)
	}
	if got == nil {
		t.Fatal("expected media root, got nil")
	}
	if got.Name != "Books" {
		t.Errorf("Name = %q, want %q", got.Name, "Books")
	}
	if got.RootPath != "/media/books" {
		t.Errorf("RootPath = %q, want %q", got.RootPath, "/media/books")
	}
}

func TestGetMediaRootByPath_NotFound(t *testing.T) {
	db := openTestDB(t)
	got, err := queries.GetMediaRootByPath(db, "/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestUpsertMediaRoot_UpdatesName(t *testing.T) {
	db := openTestDB(t)

	if err := queries.UpsertMediaRoot(db, "root-1", "Old Name", "/media/books"); err != nil {
		t.Fatal(err)
	}
	if err := queries.UpsertMediaRoot(db, "root-1", "New Name", "/media/books"); err != nil {
		t.Fatal(err)
	}

	got, err := queries.GetMediaRootByPath(db, "/media/books")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "New Name" {
		t.Errorf("Name = %q, want %q", got.Name, "New Name")
	}
}

// ── Works ────────────────────────────────────────────────────────────────────

func insertTestRoot(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := "test-root-id"
	if err := queries.UpsertMediaRoot(db, id, "Test", "/media"); err != nil {
		t.Fatalf("inserting test root: %v", err)
	}
	return id
}

func TestUpsertAndGetWork(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)

	w := &queries.Work{
		ID:            "work-1",
		MediaRootID:   rootID,
		DirectoryPath: "Author/Book",
		Title:         "Test Book",
		SortTitle:     "Test Book",
		NeedsReview:   true,
	}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatalf("UpsertWork: %v", err)
	}

	got, err := queries.GetWorkByPath(db, rootID, "Author/Book")
	if err != nil {
		t.Fatalf("GetWorkByPath: %v", err)
	}
	if got == nil {
		t.Fatal("expected work, got nil")
	}
	if got.Title != "Test Book" {
		t.Errorf("Title = %q, want %q", got.Title, "Test Book")
	}
	if !got.NeedsReview {
		t.Error("expected NeedsReview = true")
	}
}

func TestGetWorkByPath_NotFound(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)

	got, err := queries.GetWorkByPath(db, rootID, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestUpsertWork_UpdatesFields(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)

	w := &queries.Work{
		ID: "work-1", MediaRootID: rootID, DirectoryPath: "dir",
		Title: "Old", SortTitle: "Old",
	}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}

	w.Title = "New"
	w.SortTitle = "New"
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}

	got, _ := queries.GetWorkByPath(db, rootID, "dir")
	if got.Title != "New" {
		t.Errorf("Title = %q, want %q", got.Title, "New")
	}
}

// ── Contributors ─────────────────────────────────────────────────────────────

func TestUpsertContributor(t *testing.T) {
	db := openTestDB(t)

	id, err := queries.UpsertContributor(db, "c-1", "Brandon Sanderson", "Sanderson, Brandon", nil)
	if err != nil {
		t.Fatalf("UpsertContributor: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}

	// Upsert again with same name — should return same ID
	id2, err := queries.UpsertContributor(db, "c-2", "Brandon Sanderson", "Sanderson, Brandon", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != id2 {
		t.Errorf("second upsert returned different ID: %q vs %q", id, id2)
	}
}

// ── Tags ─────────────────────────────────────────────────────────────────────

func TestUpsertTag(t *testing.T) {
	db := openTestDB(t)

	id1, err := queries.UpsertTag(db, "tag-1", "Fantasy", "genre")
	if err != nil {
		t.Fatalf("UpsertTag: %v", err)
	}
	id2, err := queries.UpsertTag(db, "tag-2", "Fantasy", "genre")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Errorf("same tag returned different IDs: %q vs %q", id1, id2)
	}
}

// ── Series ────────────────────────────────────────────────────────────────────

func TestUpsertSeries(t *testing.T) {
	db := openTestDB(t)

	id1, err := queries.UpsertSeries(db, "s-1", "Stormlight Archive", nil)
	if err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}
	id2, err := queries.UpsertSeries(db, "s-2", "Stormlight Archive", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Errorf("same series returned different IDs: %q vs %q", id1, id2)
	}
}

// ── Identifiers ───────────────────────────────────────────────────────────────

func TestUpsertAndDeleteIdentifiers(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)
	w := &queries.Work{ID: "w-1", MediaRootID: rootID, DirectoryPath: "d", Title: "T", SortTitle: "T"}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}

	if err := queries.UpsertIdentifier(db, "w-1", "isbn_13", "9780765326355"); err != nil {
		t.Fatalf("UpsertIdentifier: %v", err)
	}
	if err := queries.DeleteWorkIdentifiers(db, "w-1"); err != nil {
		t.Fatalf("DeleteWorkIdentifiers: %v", err)
	}
}

func TestGetWorkIdentifiers(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)
	w := &queries.Work{ID: "w-1", MediaRootID: rootID, DirectoryPath: "d", Title: "T", SortTitle: "T"}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}

	if err := queries.UpsertIdentifier(db, "w-1", "isbn_13", "9780765326355"); err != nil {
		t.Fatal(err)
	}
	if err := queries.UpsertIdentifier(db, "w-1", "asin", "B001QKBHG4"); err != nil {
		t.Fatal(err)
	}

	ids, err := queries.GetWorkIdentifiers(db, "w-1")
	if err != nil {
		t.Fatalf("GetWorkIdentifiers: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 identifiers, got %d", len(ids))
	}
	if ids["isbn_13"] != "9780765326355" {
		t.Errorf("isbn_13 = %q, want %q", ids["isbn_13"], "9780765326355")
	}
	if ids["asin"] != "B001QKBHG4" {
		t.Errorf("asin = %q, want %q", ids["asin"], "B001QKBHG4")
	}
}

func TestGetWorkIdentifiers_Empty(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)
	w := &queries.Work{ID: "w-1", MediaRootID: rootID, DirectoryPath: "d", Title: "T", SortTitle: "T"}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}

	ids, err := queries.GetWorkIdentifiers(db, "w-1")
	if err != nil {
		t.Fatalf("GetWorkIdentifiers: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 identifiers, got %d", len(ids))
	}
}

func TestGetWorkDirectoryPath(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)
	w := &queries.Work{ID: "w-1", MediaRootID: rootID, DirectoryPath: "Author/Book", Title: "T", SortTitle: "T"}
	if err := queries.UpsertWork(db, w); err != nil {
		t.Fatal(err)
	}

	rootPath, dirPath, err := queries.GetWorkDirectoryPath(db, "w-1")
	if err != nil {
		t.Fatalf("GetWorkDirectoryPath: %v", err)
	}
	if rootPath != "/media" {
		t.Errorf("rootPath = %q, want %q", rootPath, "/media")
	}
	if dirPath != "Author/Book" {
		t.Errorf("dirPath = %q, want %q", dirPath, "Author/Book")
	}
}

func TestGetWorkDirectoryPath_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, _, err := queries.GetWorkDirectoryPath(db, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent work")
	}
}

func TestUpsertAndDeleteWorkRatings(t *testing.T) {
	db := openTestDB(t)
	rootID := insertTestRoot(t, db)
	work := &queries.Work{
		ID:            "w-ratings",
		MediaRootID:   rootID,
		DirectoryPath: "ratings",
		Title:         "Ratings",
		SortTitle:     "Ratings",
	}
	if err := queries.UpsertWork(db, work); err != nil {
		t.Fatal(err)
	}

	err := queries.UpsertWorkRating(db, &queries.Rating{
		WorkID:    "w-ratings",
		Source:    "hardcover",
		Score:     4.7,
		MaxScore:  5,
		Count:     1500,
		FetchedAt: time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("UpsertWorkRating: %v", err)
	}

	ratings, err := queries.GetWorkRatings(db, "w-ratings")
	if err != nil {
		t.Fatalf("GetWorkRatings: %v", err)
	}
	if len(ratings) != 1 {
		t.Fatalf("len(ratings) = %d, want 1", len(ratings))
	}
	if ratings[0].Source != "hardcover" || ratings[0].Score != 4.7 || ratings[0].Count != 1500 {
		t.Errorf("ratings[0] = %+v, want hardcover 4.7 count 1500", ratings[0])
	}

	if err := queries.DeleteWorkRatings(db, "w-ratings"); err != nil {
		t.Fatalf("DeleteWorkRatings: %v", err)
	}
	ratings, err = queries.GetWorkRatings(db, "w-ratings")
	if err != nil {
		t.Fatalf("GetWorkRatings after delete: %v", err)
	}
	if len(ratings) != 0 {
		t.Fatalf("len(ratings) after delete = %d, want 0", len(ratings))
	}
}
