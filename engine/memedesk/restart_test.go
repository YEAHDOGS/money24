package memedesk

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	TEST_UNOBSERVED    = "unobserved"
	TEST_ORPHAN        = "journal-only"
	TEST_RESTART_SPLIT = 4
)

func TestMiddayRestartMatchesUninterruptedDesk(t *testing.T) {
	entry := testStart.Add(SHADOW_DELAY)
	expired := entry.Add(ENTRY_WINDOW)
	rows := []meme.Record{
		safetyRow(TEST_MINT, testStart, false),
		migration(TEST_MINT, testStart),
		{At: testStart, Kind: meme.KIND_LAUNCH, Mint: TEST_UNOBSERVED},
		snapshot(TEST_MINT, entry, 1),
		{At: entry.Add(time.Second), Kind: meme.KIND_LAUNCH, Mint: TEST_PREVIOUS},
		snapshot(TEST_MINT, expired, 0.75),
		snapshot(TEST_MINT, expired.Add(SIMULATED_LATENCY), 0.70),
	}
	run := func(restart bool) Status {
		dir := t.TempDir()
		recorderDir := filepath.Join(dir, meme.DATA_SUBDIR)
		d, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		positions := writeRecords(t, recorderDir, testStart, rows[:TEST_RESTART_SPLIT]...)
		for i, pos := range positions {
			if rows[i].Mint != TEST_UNOBSERVED {
				d.Observe(rows[i], pos)
			}
		}
		rb := DefaultRulebook()
		rb.Strategies[0].On = false
		if err := d.applyRulebook(rb, true); err != nil {
			t.Fatal(err)
		}
		d.tick(entry)
		if restart {
			midday := d.model.status(true)
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			if err := meme.Compact(recorderDir); err != nil {
				t.Fatal(err)
			}
			d, err = Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := d.model.status(true); !reflect.DeepEqual(midday, got) {
				t.Fatalf("midday restore changed the desk: got %+v, want %+v", got, midday)
			}
		}
		defer d.Close()
		positions = writeRecords(t, recorderDir, testStart, rows[TEST_RESTART_SPLIT:]...)
		d.Observe(rows[TEST_RESTART_SPLIT], positions[0])
		d.tick(expired)
		d.Observe(rows[TEST_RESTART_SPLIT+1], positions[1])
		d.Observe(rows[TEST_RESTART_SPLIT+2], positions[2])
		d.tick(expired.Add(FOLLOW_WINDOW))
		return d.model.status(true)
	}

	want, got := run(false), run(true)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("resumed desk diverged: got %+v, want %+v", got, want)
	}
	for _, s := range got.Strategies {
		if s.Record.Candidates != 2 || s.Record.Pending != 0 || s.Record.Following != 0 {
			t.Fatalf("wrong observation coverage after restart: %+v", s.Record)
		}
		if s.Config.ID == KIND_ORGANIC && (s.Record.PaperEntries != 1 || s.Record.PaperClosed != 1) {
			t.Fatalf("restart fixture should produce a paper entry and exit: %+v", s.Record)
		}
	}
}

func TestLegacyJournalMigrationKeepsDecisionsAndUnmatchedRows(t *testing.T) {
	dir := t.TempDir()
	recorderDir := filepath.Join(dir, meme.DATA_SUBDIR)
	at := testStart.Add(SHADOW_DELAY)
	rows := []meme.Record{
		safetyRow(TEST_MINT, testStart, false),
		migration(TEST_MINT, testStart),
		snapshot(TEST_MINT, at, 1),
	}
	writeRecords(t, recorderDir, testStart, rows...)
	if err := meme.Compact(recorderDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, JOURNAL_FILE)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	rb := DefaultRulebook()
	orphan := meme.Record{At: at, Kind: meme.KIND_LAUNCH, Mint: TEST_ORPHAN}
	legacy := []journalLine{
		{Rulebook: &rb}, {Record: &rows[0]}, {Record: &rows[1]}, {Clock: at},
		{Record: &orphan}, {Record: &rows[2]}, {Clock: at.Add(ENTRY_WINDOW)},
	}
	for _, line := range legacy {
		line.Version = JOURNAL_LEGACY_VERSION
		if err := json.NewEncoder(f).Encode(line); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.WriteString(TEST_TORN_LINE); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	var expected Status
	for pass := 0; pass < 2; pass++ {
		model := newEngine()
		file, err := openJournal(dir, model)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if pass == 0 {
			expected = model.status(true)
		} else if got := model.status(true); !reflect.DeepEqual(got, expected) {
			t.Fatalf("migrated replay changed the desk: got %+v, want %+v", got, expected)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(data, []byte(`"record":`)) != 1 || !bytes.Contains(data, []byte(TEST_ORPHAN)) ||
		!bytes.Contains(data, []byte(JOURNAL_NEXT)) || bytes.Contains(data, []byte(TEST_TORN_LINE+"\n")) {
		t.Fatalf("unexpected migrated journal: %s", data)
	}
}
