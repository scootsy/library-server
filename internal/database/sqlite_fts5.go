//go:build fts5

// This file is intentionally empty. Its only purpose is to ensure that when
// this package is built with the "fts5" build tag, the tag propagates to the
// mattn/go-sqlite3 dependency, enabling the FTS5 extension in the embedded
// SQLite amalgamation.
//
// Build with: go build -tags fts5 ./...
// Test with:  go test -tags fts5 ./...
package database
