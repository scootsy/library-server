package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/scootsy/library-server/internal/config"
)

func TestSPAFallbackServesIndexForLoginPath(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080, BaseURL: "http://localhost:8080"}}

	webFS := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<html>spa</html>")},
		"assets/app.js":     &fstest.MapFile{Data: []byte("console.log('app')")},
		"assets/styles.css": &fstest.MapFile{Data: []byte("body{}")},
	}

	srv := New(cfg, http.NewServeMux(), fs.FS(webFS))

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /login fallback, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "spa") {
		t.Fatalf("expected index.html body for /login fallback, got %q", w.Body.String())
	}
}
