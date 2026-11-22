package kalshitape

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/predict"
)

const (
	openPage    = `{"cursor":"","markets":[{"ticker":"A-1","event_ticker":"A","title":"Will A?","market_type":"binary","status":"active","close_time":"2026-11-23T00:00:00Z","yes_bid_dollars":"0.40","yes_ask_dollars":"0.42","no_bid_dollars":"0.58","no_ask_dollars":"0.60","volume_24h_fp":"120"}]}`
	settledPage = `{"cursor":"","markets":[{"ticker":"B-1","event_ticker":"B","title":"Will B?","market_type":"binary","status":"finalized","result":"yes","settlement_value_dollars":"1.0000","close_time":"2026-11-21T20:00:00Z"},{"ticker":"C-1","event_ticker":"C","title":"Will C?","market_type":"binary","status":"closed","close_time":"2026-11-21T21:00:00Z"}]}`
)

func fakeKalshi(t *testing.T) *predict.Kalshi {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") == predict.STATUS_SETTLED_QUERY {
			w.Write([]byte(settledPage))
			return
		}
		w.Write([]byte(openPage))
	}))
	t.Cleanup(srv.Close)
	return &predict.Kalshi{HTTP: srv.Client(), Base: srv.URL}
}

func lines(t *testing.T, path string) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	if err := eachLine(path, func(b []byte) {
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			out = append(out, m)
		}
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTapeRecordsSnapshotsTermsAndEachResultOnce(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 22, 6, 0, 0, 0, time.UTC)
	day := filepath.Join(dir, now.Format(DAY_LAYOUT))
	r := &Recorder{Dir: dir, Kalshi: fakeKalshi(t), seen: map[string]bool{}}
	ctx := context.Background()
	for range 2 {
		if err := r.pass(ctx, now); err != nil {
			t.Fatal(err)
		}
		if err := r.settle(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	if got := lines(t, filepath.Join(day, FILE_MARKETS)); len(got) != 2 || got[0]["t"] != "A-1" || got[0]["ya"] != 0.42 {
		t.Fatalf("snapshots %v, want A-1 twice with its ask", got)
	}
	if got := lines(t, filepath.Join(day, FILE_CATALOG)); len(got) != 1 || got[0]["title"] != "Will A?" {
		t.Fatalf("catalog %v, want A-1's terms once", got)
	}
	results := lines(t, filepath.Join(day, FILE_RESULTS))
	if len(results) != 1 || results[0]["t"] != "B-1" || results[0]["r"] != "yes" || results[0]["v"] != 1.0 {
		t.Fatalf("results %v, want B-1 once (C-1 is closed, not finished)", results)
	}
	again := &Recorder{Dir: dir, Kalshi: fakeKalshi(t)}
	again.seen = again.readSeen(now)
	if err := again.settle(ctx, now); err != nil {
		t.Fatal(err)
	}
	if got := lines(t, filepath.Join(day, FILE_RESULTS)); len(got) != 1 {
		t.Fatalf("a restart wrote B-1's result again: %v", got)
	}
	if s := again.Status(); len(s.Days) != 1 || s.BytesOnDisk == 0 {
		t.Fatalf("status %+v", s)
	}
}
