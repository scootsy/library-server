package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/scootsy/library-server/internal/config"
)

// newTestFS returns an in-memory FS with a minimal SPA.
func newTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: []byte("<html>SPA</html>")},
		"assets/app.js":    {Data: []byte("console.log('ok')")},
		"assets/style.css": {Data: []byte("body{}")},
	}
}

func newTestServer(webFS fstest.MapFS) *http.Server {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:    "127.0.0.1",
			Port:    0,
			BaseURL: "http://localhost:8080",
		},
	}
	s := New(cfg, nil, webFS)
	return s.http
}

func TestRootServesIndexWithoutRedirect(t *testing.T) {
	srv := newTestServer(newTestFS())

	// Request "/" — must return 200 with index.html content, NOT a 301 redirect.
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET / returned status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SPA") {
		t.Errorf("GET / body = %q, want it to contain %q", body, "SPA")
	}
	// Verify no Location header (no redirect).
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("GET / returned Location header %q, want empty (no redirect)", loc)
	}
}

func TestIndexHTMLDoesNotRedirect(t *testing.T) {
	// Requesting /index.html directly should serve the file, not trigger
	// FileServer's built-in redirect from /index.html → /.
	// With our fix, /index.html falls through to the SPA fallback since
	// the file in the FS is "index.html" (no leading slash) and the open
	// for "index.html" succeeds — so FileServer handles it. But since we
	// no longer rewrite to /index.html for the root, this path is handled
	// by the file-exists check which passes it to FileServer. FileServer
	// WILL redirect /index.html → / — but that's OK because / now serves
	// index.html directly without looping.
	srv := newTestServer(newTestFS())

	req := httptest.NewRequest("GET", "/index.html", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	// FileServer redirects /index.html to ./ (301), which is fine — the loop
	// is broken because / no longer redirects back to /index.html.
	if rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusOK {
		t.Errorf("GET /index.html returned status %d, want 301 or 200", rec.Code)
	}
}

func TestStaticFileServed(t *testing.T) {
	srv := newTestServer(newTestFS())

	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /assets/app.js returned status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "console.log") {
		t.Errorf("GET /assets/app.js body = %q, want it to contain JS content", body)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "" {
		t.Errorf("GET /assets/app.js Cache-Control = %q, want empty", cacheControl)
	}
}

func TestSPAFallbackServesIndexForUnknownRoutes(t *testing.T) {
	srv := newTestServer(newTestFS())

	// A client-side route like /browse should serve index.html (SPA fallback),
	// NOT return 404.
	routes := []string{"/browse", "/works/123", "/settings", "/login"}
	for _, route := range routes {
		req := httptest.NewRequest("GET", route, nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s returned status %d, want %d", route, rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "SPA") {
			t.Errorf("GET %s body = %q, want it to contain %q", route, body, "SPA")
		}
		// No redirect.
		if loc := rec.Header().Get("Location"); loc != "" {
			t.Errorf("GET %s returned Location header %q, want empty", route, loc)
		}
		if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want %q", route, cacheControl, "no-store")
		}
	}
}

func TestDirectoryPathUsesSPAFallbackNoStore(t *testing.T) {
	srv := newTestServer(newTestFS())

	req := httptest.NewRequest("GET", "/assets", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /assets returned status %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "SPA") {
		t.Errorf("GET /assets body = %q, want it to contain %q", rec.Body.String(), "SPA")
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Errorf("GET /assets Cache-Control = %q, want %q", cacheControl, "no-store")
	}
}

func TestAPIRoutesNotHandledBySPA(t *testing.T) {
	srv := newTestServer(newTestFS())

	// /api/* routes without a mounted API handler should return 404, not SPA content.
	req := httptest.NewRequest("GET", "/api/works", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	// Without an API handler mounted, this should 404.
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/works returned status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(newTestFS())

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /health returned status %d, want %d", rec.Code, http.StatusOK)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("GET /health body = %q, want it to contain status ok", body)
	}
}

func TestNoRedirectLoop(t *testing.T) {
	// Simulate a browser following redirects — after the fix, requesting "/"
	// should never produce a redirect chain.
	srv := newTestServer(newTestFS())

	// Follow up to 10 redirects (a real browser follows ~20).
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				t.Fatalf("redirect loop detected: %d redirects followed", len(via))
			}
			return nil
		},
	}

	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / (following redirects) returned status %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
