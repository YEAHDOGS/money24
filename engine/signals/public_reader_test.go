package signals

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	TEST_TELEGRAM_LATEST      = "telegram_latest.html"
	TEST_TELEGRAM_OLDER       = "telegram_older.html"
	TEST_TELEGRAM_UNAVAILABLE = "telegram_unavailable.html"
	TEST_REDDIT_SUBMISSIONS   = "reddit_submissions.json"
	TEST_REDDIT_COMMENTS      = "reddit_comments.json"
	TEST_REDDIT_OLDER         = "reddit_older.json"
	TEST_TELEGRAM_HANDLE      = "fixturecoin"
	TEST_REDDIT_HANDLE        = "u/fixturecoin"
	TEST_DATE_OLD             = "2026-01-02T16:00:00+02:00"
	TEST_DATE_LATEST          = "2026-01-02T15:01:00Z"
	TEST_REDDIT_DATE          = "1767366000.0"
	TEST_JSON_POSTS           = "posts.jsonl"
)

type readerTransport func(*http.Request) (*http.Response, error)

func (transport readerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := transport(req)
	return res, err
}

func readerFixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func readerResponse(req *http.Request, body string, status int) *http.Response {
	return &http.Response{Request: req, StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestTelegramPublicReader(t *testing.T) {
	source := Source{Type: TYPE_TELEGRAM, Handle: TEST_TELEGRAM_HANDLE}
	latest, older := readerFixture(t, TEST_TELEGRAM_LATEST), readerFixture(t, TEST_TELEGRAM_OLDER)
	var requests []string
	client := &http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.URL.String())
		if req.UserAgent() != PUBLIC_READER_AGENT {
			t.Fatalf("User-Agent %q", req.UserAgent())
		}
		body := latest
		if req.URL.Query().Get("before") != "" {
			body = older
		}
		return readerResponse(req, body, http.StatusOK), nil
	})}
	cutoff := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	posts, err := fetchTelegram(context.Background(), client, source, 10, cutoff, PUBLIC_HISTORY_PAGES)
	if err != nil || len(posts) != 3 {
		t.Fatalf("posts=%+v err=%v", posts, err)
	}
	wantURLs := []string{"https://t.me/s/fixturecoin", "https://t.me/s/fixturecoin?before=7"}
	if !slices.Equal(requests, wantURLs) {
		t.Fatalf("requests %v, want %v", requests, wantURLs)
	}
	if posts[0].PostID != "fixturecoin/8" || posts[0].Text != "" || posts[2].PostID != "fixturecoin/6" {
		t.Fatalf("unexpected ordering or media-only post %+v", posts)
	}
	post := posts[1]
	if post.Text != "Buy $BTC & hold\nEntry < 42\nTarget 50" || post.URL != "https://t.me/fixturecoin/7" ||
		!post.PostedAt.Equal(cutoff.Add(14*time.Hour)) {
		t.Fatalf("unexpected text post %+v", post)
	}
	posts, err = fetchTelegram(context.Background(), client, source, 1, cutoff, PUBLIC_HISTORY_PAGES)
	if err != nil || len(posts) != 1 || posts[0].PostID != "fixturecoin/8" || len(requests) != 3 {
		t.Fatalf("bounded load posts=%+v requests=%v err=%v", posts, requests, err)
	}
}

func TestTelegramPreviewUnavailable(t *testing.T) {
	source := Source{Type: TYPE_TELEGRAM, Handle: TEST_TELEGRAM_HANDLE}
	body := readerFixture(t, TEST_TELEGRAM_UNAVAILABLE)
	client := &http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		return readerResponse(req, body, http.StatusOK), nil
	})}
	_, err := fetchTelegram(context.Background(), client, source, PUBLIC_HISTORY_POSTS, time.Time{}, PUBLIC_HISTORY_PAGES)
	if err == nil || !strings.Contains(err.Error(), "public web preview unavailable") {
		t.Fatalf("err=%v", err)
	}
}

func TestRedditUserReader(t *testing.T) {
	source := Source{Type: TYPE_REDDIT, Handle: TEST_REDDIT_HANDLE}
	submitted := readerFixture(t, TEST_REDDIT_SUBMISSIONS)
	older := readerFixture(t, TEST_REDDIT_OLDER)
	comments := readerFixture(t, TEST_REDDIT_COMMENTS)
	var paths []string
	client := &http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if req.UserAgent() != PUBLIC_READER_AGENT || req.URL.Query().Get(REDDIT_SORT) != REDDIT_SORT_NEW {
			t.Fatalf("unexpected request: %s, agent %s", req.URL, req.UserAgent())
		}
		body := submitted
		switch {
		case req.URL.Path == "/user/fixturecoin/comments.json":
			body = comments
		case req.URL.Query().Get(REDDIT_AFTER) == "t3_a1":
			body = older
		}
		return readerResponse(req, body, http.StatusOK), nil
	})}
	cutoff := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	posts, err := fetchReddit(context.Background(), client, source, 3, cutoff, PUBLIC_HISTORY_PAGES)
	wantPaths := []string{"/user/fixturecoin/submitted.json", "/user/fixturecoin/submitted.json", "/user/fixturecoin/comments.json"}
	if err != nil || len(posts) != 3 || !slices.Equal(paths, wantPaths) {
		t.Fatalf("posts=%+v paths=%v err=%v", posts, paths, err)
	}
	comment, submission := posts[0], posts[1]
	if comment.PostID != "t1_a2" || comment.ThreadID != "t3_a1" || comment.Text != "Trim $BTC & leave a runner" ||
		submission.PostID != "t3_a1" || submission.Text != "BTC > 42\n\nEntry 42\nTarget 50" ||
		!submission.PostedAt.Equal(time.Unix(1767366000, 0)) ||
		submission.URL != "https://www.reddit.com/r/fixturecoin/comments/a1/btc/" || posts[2].PostID != "t3_a0" {
		t.Fatalf("unexpected posts %+v", posts)
	}
}

func TestRedditReaderBackoff(t *testing.T) {
	t.Cleanup(func() {
		redditRateLimit.Lock()
		redditRateLimit.until = time.Time{}
		redditRateLimit.Unlock()
	})
	requests := 0
	client := &http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Path != "/r/fixturecoin/new.json" {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		res := readerResponse(req, "", http.StatusTooManyRequests)
		res.Header.Set(PUBLIC_RETRY_AFTER_HEADER, "120")
		return res, nil
	})}
	for _, handle := range []string{"r/fixturecoin", "r/anothercoin"} {
		_, err := fetchReddit(context.Background(), client, Source{Type: TYPE_REDDIT, Handle: handle}, 1, time.Time{}, 1)
		if err == nil || !strings.Contains(err.Error(), "HTTP 429; retry after") {
			t.Fatalf("err=%v", err)
		}
	}
	if requests != 1 {
		t.Fatalf("made %d requests during backoff", requests)
	}
	redditRateLimit.Lock()
	until := redditRateLimit.until
	redditRateLimit.Unlock()
	if time.Until(until) <= time.Minute {
		t.Fatalf("did not honor Retry-After: %s", until)
	}
}

func TestPublicHistoryRoutesDeduplicate(t *testing.T) {
	rec, err := OpenRecorder(filepath.Join(t.TempDir(), TEST_JSON_POSTS))
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	telegram := readerFixture(t, TEST_TELEGRAM_OLDER)
	telegram = strings.NewReplacer(
		"2026-01-01T13:00:00Z", time.Now().Add(-PUBLIC_HISTORY_DAYS-time.Hour).UTC().Format(time.RFC3339),
		"2026-01-02T13:00:00Z", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"2026-01-02T14:00:00Z", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
	).Replace(telegram)
	reddit := strings.Replace(readerFixture(t, TEST_REDDIT_SUBMISSIONS),
		TEST_REDDIT_DATE, fmt.Sprint(time.Now().Add(-time.Minute).Unix()), 1)
	reddit = strings.Replace(reddit, `"after": "t3_a1"`, `"after": null`, 1)
	client := &http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/s/fixturecoin":
			return readerResponse(req, telegram, http.StatusOK), nil
		case "/r/fixturecoin/new.json":
			return readerResponse(req, reddit, http.StatusOK), nil
		default:
			t.Fatalf("unexpected request: %s", req.URL)
			return nil, nil
		}
	})}
	tracker := NewTracker(rec, &Catalog{}, nil)
	load := tracker.Routes(client, func(*http.Request) bool { return true })[TRUST_LOAD_ROUTE]
	for id, want := range map[string]int{"telegram:fixturecoin": 2, "reddit:r/fixturecoin": 1} {
		for _, count := range []int{want, 0} {
			res := httptest.NewRecorder()
			load.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/trust/load?id="+id, nil))
			if res.Code != http.StatusOK || strings.TrimSpace(res.Body.String()) != fmt.Sprintf(`{"added":%d}`, count) {
				t.Fatalf("%s: %d %s", id, res.Code, res.Body.String())
			}
		}
		if got := len(rec.Signals()[id]); got != want {
			t.Fatalf("%s: recorded %d posts, want %d", id, got, want)
		}
	}
}

func TestRecorderClaimsPostOnce(t *testing.T) {
	rec, err := OpenRecorder(filepath.Join(t.TempDir(), TEST_JSON_POSTS))
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	post := RawPost{SourceID: "telegram:fixturecoin", PostID: "fixturecoin/7", Text: "Example"}
	var tasks sync.WaitGroup
	for range 2 {
		tasks.Go(func() {
			if _, err := rec.recordNew(post); err != nil {
				t.Error(err)
			}
		})
	}
	tasks.Wait()
	var lines int
	if err := Replay(rec.file.Name(), func(RawPost, Signal) { lines++ }); err != nil || lines != 1 {
		t.Fatalf("replayed %d lines, err=%v", lines, err)
	}
}
