package signals

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublicPollRecordsSnapshotAndQueuesOnlyNewPost(t *testing.T) {
	rec, err := OpenRecorder(filepath.Join(t.TempDir(), TEST_JSON_POSTS))
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	now := time.Now().UTC().Truncate(time.Second)
	body := strings.NewReplacer(
		TEST_DATE_OLD, now.Add(-30*time.Second).Format(time.RFC3339),
		TEST_DATE_LATEST, now.Add(-10*time.Second).Format(time.RFC3339),
		`<a class="tgme_widget_message_photo_wrap"></a>`,
		`<div class="tgme_widget_message_text">BTO $SPY 600C 12/31 @ 1.20</div>`,
	).Replace(readerFixture(t, TEST_TELEGRAM_LATEST))
	client := &http.Client{Transport: readerTransport(func(req *http.Request) (*http.Response, error) {
		return readerResponse(req, body, http.StatusOK), nil
	})}
	source := Source{Type: TYPE_TELEGRAM, Handle: TEST_TELEGRAM_HANDLE}
	catalog := &Catalog{Sources: []Source{source}, Defaults: Settings{MaxSignalAgeSec: 120}}
	queue := make(chan Signal, 2)
	started := now.Add(-20 * time.Second)
	for range 2 {
		if err := pollPublicSource(context.Background(), client, catalog, rec, queue, source, started); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(queue); n != 1 {
		t.Fatalf("queued %d posts, want 1", n)
	}
	sig := <-queue
	if sig.SourceID != source.ID() || sig.PostID != "fixturecoin/8" || sig.Kind != KIND_ENTRY || sig.Symbol != "SPY" ||
		!sig.PostedAt.Equal(now.Add(-10*time.Second)) || sig.SeenAt.Before(now) || sig.Manual {
		t.Fatalf("unexpected signal %+v", sig)
	}
	if n := len(rec.Signals()[source.ID()]); n != 2 {
		t.Fatalf("recorded %d posts, want 2", n)
	}
}

func TestRunnerDropsExpiredPublicSignalBeforeProcessing(t *testing.T) {
	journal, err := OpenJournal(filepath.Join(t.TempDir(), "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	now := time.Now()
	catalog := &Catalog{Defaults: Settings{MaxSignalAgeSec: 60}}
	runner := NewRunner(catalog, journal, nil)
	for _, kind := range []Kind{KIND_ENTRY, KIND_EXIT} {
		sig := Signal{SourceID: "reddit:r/fixturecoin", Kind: kind, PostedAt: now.Add(-61 * time.Second), SeenAt: now}
		entry, fresh, err := runner.Handle(sig, now)
		if err != nil || fresh || entry.Rejected != ErrStale.Error() {
			t.Fatalf("%s: fresh=%v rejected=%q err=%v", kind, fresh, entry.Rejected, err)
		}
	}
	if n := len(journal.All()); n != 0 {
		t.Fatalf("journal has %d expired signals", n)
	}
}
