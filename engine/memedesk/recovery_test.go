package memedesk

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const TEST_SURVIVING_ROWS = 2

func writeLegacyJournal(t *testing.T, path string, lines ...journalLine) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, line := range lines {
		line.Version = JOURNAL_LEGACY_VERSION
		if err := json.NewEncoder(&buf).Encode(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), JOURNAL_MODE); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMigrationResumesAfterCrashAndClearsBackup(t *testing.T) {
	dir := t.TempDir()
	rows := []meme.Record{safetyRow(TEST_MINT, testStart, false), migration(TEST_MINT, testStart)}
	writeRecords(t, filepath.Join(dir, meme.DATA_SUBDIR), testStart, rows...)
	rb := DefaultRulebook()
	path := filepath.Join(dir, JOURNAL_FILE)
	legacy := writeLegacyJournal(t, path, journalLine{Rulebook: &rb}, journalLine{Record: &rows[0]}, journalLine{Record: &rows[1]})
	// A crash after the backup and a half-written replacement.
	if err := os.WriteFile(path+BACKUP_SUFFIX, legacy, JOURNAL_MODE); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+MIGRATION_SUFFIX, []byte(TEST_TORN_LINE), JOURNAL_MODE); err != nil {
		t.Fatal(err)
	}
	want := newEngine()
	legacyFile := openOrFail(t, path)
	if _, err := restoreJournal(legacyFile, want, "", nil); err != nil {
		t.Fatal(err)
	}
	legacyFile.Close() // Windows cannot replace a file another handle holds open
	got := newEngine()
	file, err := openJournal(dir, got)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migration changed the desk: got %+v, want %+v", got.status(true), want.status(true))
	}
	for _, leftover := range []string{path + BACKUP_SUFFIX, path + MIGRATION_SUFFIX} {
		if _, err := os.Stat(leftover); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone after a verified migration: %v", leftover, err)
		}
	}
}

func TestFailedMigrationLeavesLegacyJournal(t *testing.T) {
	dir := t.TempDir()
	row := safetyRow(TEST_MINT, testStart, false)
	rb := DefaultRulebook()
	path := filepath.Join(dir, JOURNAL_FILE)
	legacy := writeLegacyJournal(t, path, journalLine{Rulebook: &rb}, journalLine{Record: &row})
	// The replacement cannot be written, after the backup was taken.
	if err := os.MkdirAll(filepath.Join(path+MIGRATION_SUFFIX, TEST_PREVIOUS), meme.DIRECTORY_MODE); err != nil {
		t.Fatal(err)
	}
	if d, err := Open(dir); err == nil {
		d.Close()
		t.Fatal("migration should fail when its replacement cannot be written")
	}
	for _, kept := range []string{path, path + BACKUP_SUFFIX} {
		if data, err := os.ReadFile(kept); err != nil || !bytes.Equal(data, legacy) {
			t.Fatalf("%s should hold the legacy journal unchanged: %v", kept, err)
		}
	}
}

// A power failure can keep the desk's references while the recorder's last
// unsynced lines are lost. The desk drops those entries and resumes as if it
// had seen only the rows that survived.
func TestPowerLossDropsReferencesPastTheRecorder(t *testing.T) {
	rows := []meme.Record{
		safetyRow(TEST_MINT, testStart, false),
		migration(TEST_MINT, testStart),
		snapshot(TEST_MINT, testStart.Add(SHADOW_DELAY), 1),
	}
	observe := func(dir string, rows []meme.Record, tick bool) *Desk {
		d, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		positions := writeRecords(t, filepath.Join(dir, meme.DATA_SUBDIR), testStart, rows...)
		for i, row := range rows {
			d.Observe(row, positions[i])
		}
		if tick {
			d.tick(testStart.Add(SHADOW_DELAY))
		}
		return d
	}
	want := observe(t.TempDir(), rows[:TEST_SURVIVING_ROWS], false)
	want.Close()

	dir := t.TempDir()
	// The clock tick after the lost row is part of the lost tail too.
	cut := observe(dir, rows, true)
	cut.Close()
	shards, err := meme.RecordFiles(filepath.Join(dir, meme.DATA_SUBDIR), testStart.Format(meme.DAY_LAYOUT))
	if err != nil || len(shards) != 1 {
		t.Fatalf("expected one shard: %v %v", shards, err)
	}
	shard := filepath.Join(dir, meme.DATA_SUBDIR, shards[0])
	data, err := os.ReadFile(shard)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.SplitAfter(data, []byte{JOURNAL_END})
	lost := bytes.Join(lines[:TEST_SURVIVING_ROWS], nil)
	if err := os.WriteFile(shard, append(lost, TEST_TORN_LINE...), JOURNAL_MODE); err != nil {
		t.Fatal(err)
	}
	resumed, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if !reflect.DeepEqual(resumed.model, want.model) {
		t.Fatalf("resumed desk: got %+v, want %+v", resumed.model.status(true), want.model.status(true))
	}
	// The dropped entries are gone, so the next restart reads cleanly too.
	again := newEngine()
	if _, err := restoreJournal(openOrFail(t, filepath.Join(dir, JOURNAL_FILE)), again, filepath.Join(dir, meme.DATA_SUBDIR), nil); err != nil {
		t.Fatal(err)
	}
}

func TestMissingRecorderDataBeforeLaterRowsStopsDesk(t *testing.T) {
	dir := t.TempDir()
	recorderDir := filepath.Join(dir, meme.DATA_SUBDIR)
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	early := safetyRow(TEST_MINT, testStart, false)
	late := migration(TEST_MINT, testStart)
	first := writeRecords(t, recorderDir, testStart, early)
	second := writeRecords(t, recorderDir, testStart, late)
	d.Observe(early, first[0])
	d.Observe(late, second[0])
	d.Close()
	if err := os.Remove(filepath.Join(recorderDir, first[0].File)); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("a hole before rows that still exist is damage, not a lost tail")
	}
}

func openOrFail(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}
