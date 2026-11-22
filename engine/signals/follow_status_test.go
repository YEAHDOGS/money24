package signals

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

const (
	TEST_FOLLOW_SOURCE = "telegram:somechannel"
	TEST_FOLLOW_ERROR  = "page not found"
)

func TestFollowStatusCountsTodayByPostTime(t *testing.T) {
	rec, err := OpenRecorder(filepath.Join(t.TempDir(), TEST_JSON_POSTS))
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	now := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	posts := []RawPost{
		{SeenAt: now, PostedAt: now.Add(-time.Hour), SourceID: TEST_FOLLOW_SOURCE, PostID: "1", Text: "a"},
		{SeenAt: now, PostedAt: now.Add(-48 * time.Hour), SourceID: TEST_FOLLOW_SOURCE, PostID: "2", Text: "b"}, // backfilled today, posted earlier
		{SeenAt: now, SourceID: "reddit:sub", PostID: "3", Text: "c"},
	}
	for _, p := range posts {
		if _, err := rec.recordNew(p); err != nil {
			t.Fatal(err)
		}
	}
	readers := &SignalReaders{}
	readers.failures.note(TEST_FOLLOW_SOURCE, errors.New(TEST_FOLLOW_ERROR), now)
	catalog := &Catalog{Sources: []Source{{Type: TYPE_TELEGRAM, Handle: "somechannel"}}}
	got := readers.FollowStatus(catalog, rec, now)
	telegram := got[0]
	if telegram.Kind != TYPE_TELEGRAM || telegram.Sources != 1 || telegram.Today != 1 || telegram.Total != 2 || !telegram.LastAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("telegram: got %+v, want one of two posts today, the newest an hour ago", telegram)
	}
	if len(telegram.Failing) != 1 || telegram.Failing[0].Error != TEST_FOLLOW_ERROR {
		t.Fatalf("telegram failing: got %+v, want the failed read", telegram.Failing)
	}
	readers.failures.note(TEST_FOLLOW_SOURCE, nil, now)
	if again := readers.FollowStatus(catalog, rec, now)[0]; len(again.Failing) != 0 {
		t.Fatalf("after a good read: got %+v, want no failures", again.Failing)
	}
}
