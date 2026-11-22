package news

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/YEAHDOGS/money24/engine/migrations"
)

const (
	ARCHIVE_MIGRATION = "20261122_001_news_archive.sql"
	TEST_DAY          = "2026-11-20"
	NEXT_DAY          = "2026-11-21"
)

func newYork(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation(MARKET_ZONE)
	if err != nil {
		t.Fatal(err)
	}
	return zone
}

func at(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return &parsed
}

// openOldStore builds news.db as it was before the archive migration, with two articles in it.
func openOldStore(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open(DB_DRIVER, "file:"+filepath.ToSlash(filepath.Join(dir, DB_FILE))+DB_PRAGMAS)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := &Store{db: db}
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	files, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name() >= ARCHIVE_MIGRATION || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		if err := old.applyMigration(ctx, f.Name()); err != nil {
			t.Fatalf("%s: %v", f.Name(), err)
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO events (seen_at, published_at, feed, source, kind, title, url, summary, tags)
		VALUES (?, ?, 'old-feed', 'Old Outlet', 'news', 'Old headline about tariffs', 'https://old.example/1', 'Summary mentions copper', '["HG"]'),
		       (?, NULL, 'old-feed', 'Old Outlet', 'news', 'Undated headline', 'https://old.example/2', NULL, '[]')`,
		at(t, "2026-11-20T15:00:00Z").UnixMilli(), at(t, "2026-11-20T14:00:00Z").UnixMilli(), at(t, "2026-11-20T16:00:00Z").UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
}

func TestArchiveMigrationKeepsOldRows(t *testing.T) {
	dir := t.TempDir()
	openOldStore(t, dir)
	ctx := context.Background()
	store, err := OpenStore(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start, _ := dayStart(TEST_DAY, newYork(t))
	page, _, err := store.archivePage(ctx, ArchiveQuery{From: start, To: start.AddDate(0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 {
		t.Fatalf("got %d old rows, want 2", len(page))
	}
	undated, dated := page[0], page[1]
	if dated.Excerpt != "Summary mentions copper" || dated.Author != "" || dated.FeedURL != "" {
		t.Errorf("old row: excerpt %q author %q feed %q", dated.Excerpt, dated.Author, dated.FeedURL)
	}
	if !dated.At.Equal(*at(t, "2026-11-20T14:00:00Z")) || !undated.At.Equal(undated.SeenAt) {
		t.Errorf("archive clock: dated %v, undated %v (seen %v)", dated.At, undated.At, undated.SeenAt)
	}
	hits, _, err := store.archivePage(ctx, ArchiveQuery{From: start, To: start.AddDate(0, 0, 1), Search: searchExpr("copper")})
	if err != nil || len(hits) != 1 {
		t.Fatalf("old summaries are searchable: %d hits, %v", len(hits), err)
	}
}

func archiveFixture(t *testing.T) (*Store, *time.Location) {
	t.Helper()
	store, err := OpenStore(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	long := strings.Repeat("Yields climbed again as traders priced in fewer cuts. ", 60)
	events := []Event{
		{PublishedAt: at(t, "2026-11-20T14:30:00Z"), Feed: "a", FeedURL: "https://a.example/rss", Source: "Outlet A", Kind: "news",
			Title: "Federal Reserve holds rates", URL: "https://a.example/1", Author: "Jane Writer", Excerpt: excerpt(long), Tags: []string{"FED"}},
		{PublishedAt: at(t, "2026-11-20T16:00:00Z"), Feed: "a", FeedURL: "https://a.example/rss", Source: "Outlet A", Kind: "news",
			Title: "Oil slips on supply", URL: "https://a.example/2", Tags: []string{"CL"}},
		{PublishedAt: at(t, "2026-11-21T04:30:00Z"), Feed: "b", FeedURL: "https://b.example/atom", Source: "Outlet B", Kind: "filing",
			Title: "Late filing before New York midnight", URL: "https://b.example/1", Tags: []string{}},
		{PublishedAt: at(t, "2026-11-21T05:30:00Z"), Feed: "b", FeedURL: "https://b.example/atom", Source: "Outlet B", Kind: "news",
			Title: "Early news after New York midnight", URL: "https://b.example/2", Tags: []string{}},
		{PublishedAt: at(t, "2099-01-01T00:00:00Z"), Feed: "b", FeedURL: "https://b.example/atom", Source: "Outlet B", Kind: "news",
			Title: "A date in the future", URL: "https://b.example/3", Tags: []string{}},
	}
	if _, err := store.insertEvents(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	return store, newYork(t)
}

func TestArchiveDaysAndPages(t *testing.T) {
	store, zone := archiveFixture(t)
	ctx := context.Background()
	start, _ := dayStart(TEST_DAY, zone)
	next, _ := dayStart(NEXT_DAY, zone)
	counts, err := store.archiveDays(ctx, []archiveSpan{{TEST_DAY, start, next}, {NEXT_DAY, next, next.AddDate(0, 0, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	if counts[TEST_DAY] != 3 || counts[NEXT_DAY] != 1 {
		t.Errorf("days = %v, want 3 on %s (New York time) and 1 on %s", counts, TEST_DAY, NEXT_DAY)
	}
	today := time.Now().In(zone).Format(DAY_LAYOUT)
	todayStart, _ := dayStart(today, zone)
	if n, _ := store.archiveDays(ctx, []archiveSpan{{today, todayStart, todayStart.AddDate(0, 0, 1)}}); n[today] != 1 {
		t.Errorf("a future publish date files under the day we saw it: %v", n)
	}

	q := ArchiveQuery{From: start, To: next, Limit: 2}
	first, cursor, err := store.archivePage(ctx, q)
	if err != nil || len(first) != 2 || cursor == "" {
		t.Fatalf("first page: %d rows, cursor %q, %v", len(first), cursor, err)
	}
	if first[0].Title != "Late filing before New York midnight" || first[0].FeedURL != "https://b.example/atom" {
		t.Errorf("newest first: %q from %q", first[0].Title, first[0].FeedURL)
	}
	beforeAt, beforeID, _ := strings.Cut(cursor, CURSOR_SEP)
	q.BeforeAt, q.BeforeID = mustInt(t, beforeAt), mustInt(t, beforeID)
	rest, cursor, err := store.archivePage(ctx, q)
	if err != nil || len(rest) != 1 || cursor != "" || rest[0].Title != "Federal Reserve holds rates" {
		t.Fatalf("second page: %v, cursor %q, %v", rest, cursor, err)
	}
	fed := rest[0]
	if fed.Author != "Jane Writer" || utf8.RuneCountInString(fed.Excerpt) > EXCERPT_MAX_CHARS || fed.OutletURL != "" {
		t.Errorf("stored fields: author %q, excerpt %d characters", fed.Author, utf8.RuneCountInString(fed.Excerpt))
	}

	day := ArchiveQuery{From: start, To: next}
	for name, c := range map[string]struct {
		q    ArchiveQuery
		want int
	}{
		"prefix search":      {ArchiveQuery{Search: searchExpr("fed")}, 1},
		"excerpt search":     {ArchiveQuery{Search: searchExpr("traders priced")}, 1},
		"every word":         {ArchiveQuery{Search: searchExpr("oil federal")}, 0},
		"query syntax":       {ArchiveQuery{Search: searchExpr(`oil" (* -^:`)}, 1},
		"operators as words": {ArchiveQuery{Search: searchExpr(`oil OR NEAR AND`)}, 0},
		"outlet":             {ArchiveQuery{Source: "Outlet B"}, 1},
		"kind":               {ArchiveQuery{Kind: "news"}, 2},
		"tag":                {ArchiveQuery{Tag: "CL"}, 1},
		"tag and outlet":     {ArchiveQuery{Tag: "CL", Source: "Outlet B"}, 0},
		"nothing to find":    {ArchiveQuery{Search: searchExpr("zzzz")}, 0},
	} {
		c.q.From, c.q.To = day.From, day.To
		n, err := store.archiveTotal(ctx, c.q)
		if err != nil || n != c.want {
			t.Errorf("%s: %d, want %d (%v)", name, n, c.want, err)
		}
	}
	facets, err := store.archiveFacets(ctx, start, next)
	if err != nil || len(facets.Sources) != 2 || facets.Sources[0] != (Facet{"Outlet A", 2}) || len(facets.Tags) != 2 {
		t.Errorf("facets = %+v, %v", facets, err)
	}
}

func TestArchiveFillsBlanksWhenSeenAgain(t *testing.T) {
	store, zone := archiveFixture(t)
	ctx := context.Background()
	again := Event{PublishedAt: at(t, "2026-11-20T16:00:00Z"), Feed: "a", FeedURL: "https://a.example/rss", Source: "Outlet A",
		Kind: "news", Title: "Oil slips on supply", URL: "https://a.example/2", Author: "Sam Reporter",
		Excerpt: "Brent fell below eighty dollars.", Tags: []string{}}
	added, err := store.insertEvents(ctx, []Event{again})
	if err != nil || len(added) != 0 {
		t.Fatalf("a known article is not stored twice: %d added, %v", len(added), err)
	}
	start, _ := dayStart(TEST_DAY, zone)
	hits, _, err := store.archivePage(ctx, ArchiveQuery{From: start, To: start.AddDate(0, 0, 1), Search: searchExpr("brent")})
	if err != nil || len(hits) != 1 || hits[0].Author != "Sam Reporter" {
		t.Fatalf("filled row: %+v, %v", hits, err)
	}
}

func mustInt(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestArchiveHTTP(t *testing.T) {
	store, zone := archiveFixture(t)
	svc := &Service{store: store, hub: NewHub(), monitor: NewMonitor(nil), stories: &storyBook{zone: zone}}
	handler := svc.Handler(HTTPOptions{})
	get := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w
	}

	w := get(ARCHIVE_PATH + "?day=" + TEST_DAY + "&limit=2&q=")
	var page ArchivePage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", w.Code, w.Body)
	}
	if len(page.Articles) != 2 || page.Next == "" || page.Total == nil || *page.Total != 3 || page.Facets == nil {
		t.Errorf("page = %+v", page)
	}
	if w.Header().Get("Cache-Control") != CACHE_ARCHIVE {
		t.Errorf("cache = %q", w.Header().Get("Cache-Control"))
	}
	w = get(ARCHIVE_PATH + "?day=" + TEST_DAY + "&before=" + page.Next)
	var rest ArchivePage
	json.Unmarshal(w.Body.Bytes(), &rest)
	if len(rest.Articles) != 1 || rest.Total != nil {
		t.Errorf("next page = %+v", rest)
	}

	w = get(ARCHIVE_DAYS_PATH + "?from=" + TEST_DAY + "&to=" + NEXT_DAY)
	var days ArchiveDays
	if err := json.Unmarshal(w.Body.Bytes(), &days); err != nil || days.Days[TEST_DAY] != 3 || days.First != TEST_DAY ||
		days.Since != time.Now().In(zone).Format(DAY_LAYOUT) {
		t.Errorf("days: %d %s", w.Code, w.Body)
	}

	for _, bad := range []string{
		ARCHIVE_PATH, ARCHIVE_PATH + "?day=20-11-2026", ARCHIVE_PATH + "?day=" + TEST_DAY + "&before=x",
		ARCHIVE_PATH + "?day=" + TEST_DAY + "&source=" + strings.Repeat("a", FILTER_MAX_CHARS+1),
		ARCHIVE_DAYS_PATH + "?from=" + NEXT_DAY + "&to=" + TEST_DAY, ARCHIVE_DAYS_PATH + "?from=2020-01-01&to=" + TEST_DAY,
	} {
		if w := get(bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", bad, w.Code)
		}
	}
}
