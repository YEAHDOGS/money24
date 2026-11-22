package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const (
	TEST_BASE         = "/desk"
	TEST_ARCHIVE_PATH = "/api/archive"
	TEST_ARCHIVE_DAYS = "/api/archive/days"
	TEST_ARCHIVE_ASK  = "day=2026-11-20&q=fed+rates&source=Outlet+A&limit=50&before=1795000000000_42"
	TEST_PAGES_DIR    = "m24d-pages-"
)

// The day view reads the archive through the runner: /desk/news/api/archive reaches newsd as
// /api/archive, with its query intact.
func TestNewsForwardingReachesArchive(t *testing.T) {
	var gotPath, gotQuery string
	news := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"articles":[]}`)
	}))
	defer news.Close()

	// The runner holds the pages folder open for its whole life, so Windows cannot remove it while
	// the test runs; it goes best-effort afterwards, not through t.TempDir.
	dir, err := os.MkdirTemp("", TEST_PAGES_DIR)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err = os.WriteFile(filepath.Join(dir, WEB_START_PAGE), []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	routes, err := webRoutes(dir, news.URL)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for pattern, h := range routes {
		mux.Handle(pattern, h)
	}
	handler := withBase(TEST_BASE, mux)

	for _, path := range []string{TEST_ARCHIVE_PATH, TEST_ARCHIVE_DAYS} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, TEST_BASE+NEWS_PREFIX+path+"?"+TEST_ARCHIVE_ASK, nil))
		if w.Code != http.StatusOK || gotPath != path || gotQuery != TEST_ARCHIVE_ASK {
			t.Errorf("%s: status %d, newsd saw %q ? %q", path, w.Code, gotPath, gotQuery)
		}
	}
}
