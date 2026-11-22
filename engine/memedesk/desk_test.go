package memedesk

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	TEST_PREVIOUS  = "prior-day"
	TEST_LATER     = "later-day"
	TEST_FILE      = "records-fixture-*.jsonl"
	TEST_TORN_LINE = `{"version":`
)

func TestReplayReadsPastSafetyAndFutureExitsForOneDay(t *testing.T) {
	dir := t.TempDir()
	recorderDir := filepath.Join(dir, "meme")
	at := testStart.AddDate(0, 0, 1).Add(-time.Minute)
	before := testStart.Add(-time.Minute)
	after := at.Add(DEFAULT_HOLD_SECS*time.Second + 2*SIMULATED_LATENCY)
	writeRecords(t, recorderDir, before,
		meme.Record{At: before, Kind: meme.KIND_LAUNCH, Mint: TEST_MINT},
		safetyRow(TEST_MINT, before, false),
		meme.Record{At: before, Kind: meme.KIND_LAUNCH, Mint: TEST_PREVIOUS},
	)
	if err := meme.Compact(recorderDir); err != nil {
		t.Fatal(err)
	}
	writeRecords(t, recorderDir, at,
		migration(TEST_MINT, at),
		snapshot(TEST_MINT, at.Add(SIMULATED_LATENCY), 1),
	)
	writeRecords(t, recorderDir, after,
		migration(TEST_LATER, after),
		snapshot(TEST_MINT, after, 1.05),
	)
	got, err := Replay(t.Context(), dir, recorderDir, testStart.Format(meme.DAY_LAYOUT))
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 3 || got.DayRecords != 2 || got.MalformedRows != 0 {
		t.Fatalf("wrong replay coverage: %+v", got)
	}
	for _, s := range got.Strategies {
		if s.Record.Candidates != 1 || s.Record.Graduations != 1 {
			t.Fatalf("incorrect cohort for %s: %+v", s.Config.ID, s.Record)
		}
		if s.Config.ID == KIND_ORGANIC && (s.Record.PaperClosed != 1 || s.Recent[0].Sales[0].Reason != EXIT_TIME) {
			t.Fatalf("missing post-midnight exit: %+v", s)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, RULEBOOK_FILE)); !os.IsNotExist(err) {
		t.Fatalf("replay mutated rulebook: %v", err)
	}
}

func writeRecords(t *testing.T, dir string, day time.Time, rows ...meme.Record) []meme.Position {
	t.Helper()
	dayDir := filepath.Join(dir, day.Format(meme.DAY_LAYOUT))
	if err := os.MkdirAll(dayDir, meme.DIRECTORY_MODE); err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(dayDir, TEST_FILE)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	name := day.Format(meme.DAY_LAYOUT) + "/" + filepath.Base(f.Name())
	var positions []meme.Position
	var offset int64
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		n, err := f.Write(append(data, JOURNAL_END))
		if err != nil {
			t.Fatal(err)
		}
		end := offset + int64(n)
		positions = append(positions, meme.Position{File: name, Offset: offset, End: end})
		offset = end
	}
	return positions
}

func TestDeskRestoresJournalAndGuardsRulebookWrites(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			d.Close()
		}
	})

	rb := DefaultRulebook()
	rb.Strategies[0].On = false
	body, err := json.Marshal(rb)
	if err != nil {
		t.Fatal(err)
	}
	request := func() *http.Request {
		return httptest.NewRequest(http.MethodPut, "/api/memedesk/strategies", bytes.NewReader(body))
	}
	out := httptest.NewRecorder()
	d.Routes()[ROUTE_RULEBOOK].ServeHTTP(out, request())
	if out.Code != http.StatusForbidden {
		t.Fatalf("unguarded write returned %d", out.Code)
	}

	d.MayWrite = func(r *http.Request) bool { return r.Method == http.MethodPut }
	out = httptest.NewRecorder()
	d.Routes()[ROUTE_RULEBOOK].ServeHTTP(out, request())
	if out.Code != http.StatusOK {
		t.Fatalf("authorized write returned %d: %s", out.Code, out.Body)
	}

	rows := []meme.Record{
		safetyRow(TEST_MINT, testStart, false),
		migration(TEST_MINT, testStart),
		snapshot(TEST_MINT, testStart.Add(SIMULATED_LATENCY), 1),
	}
	positions := writeRecords(t, filepath.Join(dir, meme.DATA_SUBDIR), testStart, rows...)
	for i, row := range rows {
		d.Observe(row, positions[i])
	}
	before := d.Status()
	out = httptest.NewRecorder()
	d.Routes()[ROUTE_STATUS].ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/memedesk", nil))
	if out.Code != http.StatusOK || !json.Valid(out.Body.Bytes()) {
		t.Fatalf("invalid status response: %d %s", out.Code, out.Body)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true

	file, err := os.OpenFile(filepath.Join(dir, JOURNAL_FILE), os.O_APPEND|os.O_WRONLY, JOURNAL_MODE)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(TEST_TORN_LINE); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	closed = false
	after := d.Status()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("status changed after restore: before %+v after %+v", before, after)
	}
	rb.CapitalUSD += DEFAULT_STAKE
	if err := saveRulebook(filepath.Join(dir, RULEBOOK_FILE), rb); err != nil {
		t.Fatal(err)
	}
	d.tick(time.Now().Add(RULEBOOK_RELOAD))
	near(t, d.Status().CapitalUSD, rb.CapitalUSD)
}

func TestFadeWaitsForPostSpikeQuoteAndNeverBuys(t *testing.T) {
	e := testEngine(t, DefaultRulebook())
	e.observe(meme.Record{At: testStart, Kind: meme.KIND_LAUNCH, Mint: TEST_MINT}, true)
	entry := testStart.Add(SIMULATED_LATENCY)
	e.observe(snapshot(TEST_MINT, entry, 1), true)
	spike := entry.Add(time.Minute)
	e.observe(snapshot(TEST_MINT, spike, 1.4), true)
	c := e.states[KIND_FADE].recent[0]
	if !c.DecidedAt.IsZero() || c.SpikeUSD != 1.4 {
		t.Fatalf("incorrect spike handling: %+v", c)
	}
	e.observe(snapshot(TEST_MINT, spike.Add(SIMULATED_LATENCY), 1.1), true)
	if c.Decision.Action != DECISION_FADE || c.Benchmark == nil || c.Paper != nil {
		t.Fatalf("fade should benchmark the post-latency quote without buying: %+v", c)
	}
	near(t, c.Benchmark.Buy.ObservedUSD, 1.1)
	near(t, e.cashDelta, 0)
}
