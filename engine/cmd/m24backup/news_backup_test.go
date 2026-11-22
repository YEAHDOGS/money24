package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/news"
)

const (
	TEST_BULK_EVENTS = 300
	TEST_BULK_CHARS  = 800
	TEST_PART_BYTES  = 20_000
	TEST_LATER_DAY   = "2026-11-22"
)

func newsZone(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation(MARKET_ZONE)
	if err != nil {
		t.Fatal(err)
	}
	return zone
}

func TestExportWritesEachFinishedDayOnce(t *testing.T) {
	root := t.TempDir()
	newsFixture(t, root)
	written, err := exportNewsDays(root, newsZone(t), DAY_OPEN, NEWS_PART_BYTES)
	if err != nil || !reflect.DeepEqual(written, []string{DAY_ONE, DAY_TWO, DAY_THREE}) {
		t.Fatalf("exported %v, %v", written, err)
	}
	if _, err := os.Stat(filepath.Join(root, "news", "days", DAY_OPEN+NEWS_DAY_SUFFIX)); err == nil {
		t.Fatal("the open day was exported")
	}
	want := map[string]map[string]int{
		DAY_ONE:   {"feed_urls": 2, "events": 3, "story_events": 1, "story_metrics": 1, "market_prices": 1, "net_usage": 1},
		DAY_TWO:   {"feed_urls": 2, "events": 3, "stories": 1, "story_events": 1, "markets": 1, "market_prices": 1, "calendar_events": 1, "net_usage": 1},
		DAY_THREE: {"feed_urls": 2, "events": 3, "stories": 1, "story_events": 2, "story_metrics": 1},
	}
	for day, tables := range want {
		records := readNewsFile(t, filepath.Join(root, "news", "days", day+NEWS_DAY_SUFFIX))
		for _, table := range NEWS_TABLES {
			if got := countRows(records, table.name); got != tables[table.name] {
				t.Errorf("%s %s: %d rows, want %d", day, table.name, got, tables[table.name])
			}
		}
		if last := records[len(records)-1]; last.T != RECORD_END || !last.Last {
			t.Errorf("%s does not end with the last-part marker", day)
		}
	}
	first := filepath.Join(root, "news", "days", DAY_ONE+NEWS_DAY_SUFFIX)
	before, _ := os.ReadFile(first)
	writeNewsRow(t, filepath.Join(root, "news", "news.db"), DAY_ONE) // a late change to a finished day
	written, err = exportNewsDays(root, newsZone(t), DAY_OPEN, NEWS_PART_BYTES)
	after, _ := os.ReadFile(first)
	if err != nil || len(written) != 0 || !bytes.Equal(before, after) {
		t.Fatalf("a finished day is exported once, never again: %v, %v", written, err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "news", "days"))
	if len(entries) != 3 {
		t.Fatalf("export left %d entries, want only the three day files", len(entries))
	}
}

func writeNewsRow(t *testing.T, path, day string) {
	t.Helper()
	db := openFixtureDB(t, path)
	defer db.Close()
	execAll(t, db, fmt.Sprintf(`INSERT INTO events (seen_at, feed, source, kind, title, url) VALUES (%d, 'wire', 'Wire', 'news', 'Late', 'https://late.example/%s')`,
		newsAt(t, day, 5), day))
}

func TestStateStaysSmallHoweverMuchHistory(t *testing.T) {
	root := t.TempDir()
	path := newsFixture(t, root)
	db := openFixtureDB(t, path)
	id := 1000
	for day := 1; day <= 30; day++ {
		for n := 0; n < 50; n++ {
			id++
			execAll(t, db, fmt.Sprintf(`INSERT INTO events (id, seen_at, feed, source, kind, title, url) VALUES (%d, %d, 'wire', 'Wire', 'news', 'Old %d', 'https://old.example/%d')`,
				id, newsAt(t, fmt.Sprintf("2026-10-%02d", day), 3), id, id))
		}
	}
	db.Close()
	state := filepath.Join(t.TempDir(), NEWS_STATE_NAME)
	if wrote, err := writeNewsState(root, newsZone(t), DAY_OPEN, state); err != nil || !wrote {
		t.Fatalf("state: %v, %v", wrote, err)
	}
	records := readNewsFile(t, state)
	if records[0].Kind != FILE_STATE || records[0].Cutoff != DAY_OPEN {
		t.Fatalf("the state names its cutoff: %+v", records[0])
	}
	want := map[string]int{"feed_urls": 2, "events": EVENTS_PER_DAY, "stories": 1, "story_events": 1, "story_metrics": 1, "markets": 1,
		"market_prices": 1, "calendar_events": 2, "net_usage": 1, "settings": 1}
	for _, table := range NEWS_TABLES {
		if got := countRows(records, table.name); got != want[table.name] {
			t.Errorf("state %s: %d rows, want %d (history must not reach the state)", table.name, got, want[table.name])
		}
	}
}

func TestExportReadsAConsistentSnapshot(t *testing.T) {
	root := t.TempDir()
	path := newsFixture(t, root)
	writer := openFixtureDB(t, path)
	defer writer.Close()
	tx, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO events (seen_at, feed, source, kind, title, url) VALUES (?, 'wire', 'Wire', 'news', 'Mid-write', 'https://mid.example/1')`,
		newsAt(t, DAY_OPEN, 6)); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), NEWS_STATE_NAME)
	if _, err := writeNewsState(root, newsZone(t), DAY_OPEN, state); err != nil { // the write is open and uncommitted
		t.Fatalf("a snapshot must not wait for or see a write in progress: %v", err)
	}
	if got := countRows(readNewsFile(t, state), "events"); got != EVENTS_PER_DAY {
		t.Fatalf("the snapshot saw %d events, an uncommitted one among them", got)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), NEWS_STATE_NAME)
	if _, err := writeNewsState(root, newsZone(t), DAY_OPEN, other); err != nil {
		t.Fatal(err)
	}
	if got := countRows(readNewsFile(t, other), "events"); got != EVENTS_PER_DAY+1 {
		t.Fatalf("after the commit the snapshot has %d events", got)
	}
}

// untarNames lists the files in a backup archive.
func untarNames(t *testing.T, archive []byte) []string {
	t.Helper()
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(compressed)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			sort.Strings(names)
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
}

func TestRestoreRebuildsTheNewsDatabaseRowForRow(t *testing.T) {
	root := t.TempDir()
	path := newsFixture(t, root)
	writeFixture(t, root, "trading/paper-journal.jsonl", "journal")
	writeFixture(t, root, "trading/meme/"+DAY_ONE+"/records.jsonl", "meme")
	store := memoryStore{make(map[string][]byte)}
	report, err := backup(store, root, testNight(t, DAY_OPEN), testLimits())
	if err != nil || len(report.waiting) != 0 {
		t.Fatalf("backup: %v, waiting %v", err, report.waiting)
	}
	var keys []string
	for key := range store.files {
		keys = append(keys, strings.TrimPrefix(key, OBJECT_PREFIX))
	}
	sort.Strings(keys)
	wantKeys := []string{"days/" + DAY_ONE + ARCHIVE_SUFFIX, "news/" + DAY_ONE + ARCHIVE_SUFFIX, "news/" + DAY_TWO + ARCHIVE_SUFFIX,
		"news/" + DAY_THREE + ARCHIVE_SUFFIX, "state/" + DAY_OPEN + ARCHIVE_SUFFIX}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("uploaded %v, want %v", keys, wantKeys)
	}
	names := untarNames(t, store.files[OBJECT_PREFIX+"state/"+DAY_OPEN+ARCHIVE_SUFFIX])
	if !reflect.DeepEqual(names, []string{NEWS_STATE_FILE, "trading/paper-journal.jsonl"}) {
		t.Fatalf("the state archive holds %v: it never carries the live database", names)
	}

	target := filepath.Join(t.TempDir(), "fresh")
	if err := restore(store, target); err != nil {
		t.Fatal(err)
	}
	if got, want := dumpNews(t, filepath.Join(target, "news", "news.db")), dumpNews(t, path); !reflect.DeepEqual(got, want) {
		for table := range want {
			if !reflect.DeepEqual(got[table], want[table]) {
				t.Errorf("%s differs after the round trip:\n got %q\nwant %q", table, got[table], want[table])
			}
		}
		t.FailNow()
	}
	if len(dumpNews(t, path)["events"]) != 4*EVENTS_PER_DAY || len(dumpNews(t, path)["events_fts"]) != 4*EVENTS_PER_DAY {
		t.Fatal("the fixture should hold twelve events, all found by word search")
	}
	if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(NEWS_STATE_FILE))); err == nil {
		t.Fatal("the transient state file was left in the restored folder")
	}
	if _, err := os.Stat(filepath.Join(target, "news", "days", DAY_TWO+NEWS_DAY_SUFFIX)); err != nil {
		t.Fatal("restore keeps the day files, so the restored machine can back up and be pulled from")
	}
	again, err := news.OpenStore(t.Context(), filepath.Join(target, "news"))
	if err != nil {
		t.Fatalf("the restored database must open as a working news store: %v", err)
	}
	again.Close()
	if data, err := os.ReadFile(filepath.Join(target, "trading", "paper-journal.jsonl")); err != nil || string(data) != "journal" {
		t.Fatalf("trading state was not restored: %q, %v", data, err)
	}
}

// bulkEvents adds events whose text hardly compresses, so a day spans several part files.
func bulkEvents(t *testing.T, path, day string) {
	t.Helper()
	db := openFixtureDB(t, path)
	defer db.Close()
	seed := sha256.Sum256([]byte(day))
	for n := 0; n < TEST_BULK_EVENTS; n++ {
		var text strings.Builder
		for text.Len() < TEST_BULK_CHARS {
			seed = sha256.Sum256(seed[:])
			text.WriteString(hex.EncodeToString(seed[:]))
		}
		if _, err := db.Exec(`INSERT INTO events (id, seen_at, feed, source, kind, title, url, excerpt) VALUES (?, ?, 'wire', 'Wire', 'news', ?, ?, ?)`,
			2000+n, newsAt(t, day, 4)+int64(n), fmt.Sprintf("Bulk %d", n), fmt.Sprintf("https://bulk.example/%d", n), text.String()[:TEST_BULK_CHARS]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBigDaySplitsIntoPartsAndRestoresWhole(t *testing.T) {
	root := t.TempDir()
	path := newsFixture(t, root)
	bulkEvents(t, path, DAY_TWO)
	if _, err := exportNewsDays(root, newsZone(t), DAY_OPEN, TEST_PART_BYTES); err != nil {
		t.Fatal(err)
	}
	files, err := listNewsFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	var parts []newsFile
	for _, file := range files {
		if file.day == DAY_TWO {
			parts = append(parts, file)
		}
	}
	if len(parts) < 3 || parts[0].name != NEWS_DAYS_DIR+DAY_TWO+NEWS_DAY_SUFFIX || parts[1].name != NEWS_DAYS_DIR+DAY_TWO+".002"+NEWS_DAY_SUFFIX {
		t.Fatalf("the big day should continue in numbered parts: %+v", parts)
	}
	rows := 0
	for i, part := range parts {
		records := readNewsFile(t, filepath.Join(root, filepath.FromSlash(part.name)))
		rows += countRows(records, "events")
		if last := records[len(records)-1]; last.Last != (i == len(parts)-1) {
			t.Errorf("part %d: last marker %v", part.part, last.Last)
		}
	}
	if rows != EVENTS_PER_DAY+TEST_BULK_EVENTS {
		t.Fatalf("the parts hold %d events", rows)
	}

	store := memoryStore{make(map[string][]byte)}
	many := testLimits()
	many.uploads = 2 * MAX_UPLOADS // every part is its own upload
	if report, err := backup(store, root, testNight(t, DAY_OPEN), many); err != nil || len(report.waiting) != 0 {
		t.Fatalf("%v %v", err, report.waiting)
	}
	for _, part := range parts {
		if _, ok := store.files[OBJECT_PREFIX+"news/"+part.stem()+ARCHIVE_SUFFIX]; !ok {
			t.Fatalf("part %s was not uploaded as its own object", part.stem())
		}
	}
	target := filepath.Join(t.TempDir(), "fresh")
	if err := restore(store, target); err != nil {
		t.Fatal(err)
	}
	if got, want := dumpNews(t, filepath.Join(target, "news", "news.db")), dumpNews(t, path); !reflect.DeepEqual(got, want) {
		t.Fatal("a day in several parts does not restore row for row")
	}

	// With a middle part missing the rebuild still works and says which day is incomplete.
	partial := t.TempDir()
	for _, part := range parts {
		if part.part == 2 {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(part.name)))
		writeFixtureBytes(t, partial, part.name, data)
	}
	rebuilt, err := rebuildNews(partial, false)
	if err != nil || !reflect.DeepEqual(rebuilt.partial, []string{DAY_TWO}) {
		t.Fatalf("rebuild with a missing part: %+v, %v", rebuilt, err)
	}
}

func writeFixtureBytes(t *testing.T, root, name string, data []byte) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), DIR_MODE); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, FILE_MODE); err != nil {
		t.Fatal(err)
	}
}

// uploadedKeys lists what a store holds, shortened and sorted.
func uploadedKeys(report nightReport) []string {
	keys := make([]string, len(report.uploaded))
	for i, key := range report.uploaded {
		keys[i] = strings.TrimPrefix(key, OBJECT_PREFIX)
	}
	return keys
}

func TestTightNightsSendWhatFitsInPriorityOrder(t *testing.T) {
	root := t.TempDir()
	newsFixture(t, root)
	writeFixture(t, root, "trading/paper-journal.jsonl", "journal")
	writeFixture(t, root, "trading/meme/"+DAY_ONE+"/records.jsonl", "meme")
	sizes := map[string]int64{}
	measure := memoryStore{make(map[string][]byte)}
	if _, err := backup(measure, root, testNight(t, DAY_OPEN), testLimits()); err != nil {
		t.Fatal(err)
	}
	for key, data := range measure.files {
		sizes[strings.TrimPrefix(key, OBJECT_PREFIX)] = int64(len(data))
	}
	state, trading := sizes["state/"+DAY_OPEN+ARCHIVE_SUFFIX], sizes["days/"+DAY_ONE+ARCHIVE_SUFFIX]
	newsOne := sizes["news/"+DAY_ONE+ARCHIVE_SUFFIX]
	if state <= trading {
		t.Fatalf("the fixture needs a state (%d) bigger than a trading day (%d)", state, trading)
	}
	night := func(bytes int64) limits {
		budget := testLimits()
		budget.bytes, budget.night = bytes, max(bytes, budget.night)
		return budget
	}

	// Room for the state, the trading day and one news day: the oldest news day goes, two wait.
	store := memoryStore{make(map[string][]byte)}
	report, err := backup(store, root, testNight(t, DAY_OPEN), night(state+trading+newsOne+1))
	wantFirst := []string{"state/" + DAY_OPEN + ARCHIVE_SUFFIX, "days/" + DAY_ONE + ARCHIVE_SUFFIX, "news/" + DAY_ONE + ARCHIVE_SUFFIX}
	if err != nil || !reflect.DeepEqual(uploadedKeys(report), wantFirst) {
		t.Fatalf("tight night sent %v, %v", uploadedKeys(report), err)
	}
	if len(report.waiting) != 1 || !strings.Contains(report.waiting[0], "2 news day file(s)") || !strings.Contains(report.waiting[0], DAY_TWO+" to "+DAY_THREE) {
		t.Fatalf("the report must say what waits: %v", report.waiting)
	}
	// The next night carries on with the oldest unbacked day and sends the state again.
	report, err = backup(store, root, testNight(t, TEST_LATER_DAY, DAY_OPEN), testLimits())
	if err != nil || len(report.waiting) != 0 || len(store.files) != 6 {
		t.Fatalf("the next night: %v, waiting %v, %d objects", err, report.waiting, len(store.files))
	}

	// Nothing fits: the run still succeeds and says everything waits.
	empty := memoryStore{make(map[string][]byte)}
	report, err = backup(empty, root, testNight(t, DAY_OPEN), night(1))
	if err != nil || len(empty.files) != 0 || len(report.waiting) != 3 {
		t.Fatalf("a night with no room must wait, not fail: %v, %d objects, %v", err, len(empty.files), report.waiting)
	}

	// The state is too big for the bytes left, but a smaller trading day fits, so it goes.
	small := memoryStore{make(map[string][]byte)}
	report, err = backup(small, root, testNight(t, DAY_OPEN), night(trading))
	if err != nil || !reflect.DeepEqual(uploadedKeys(report), []string{"days/" + DAY_ONE + ARCHIVE_SUFFIX}) || len(report.waiting) != 2 {
		t.Fatalf("what fits goes: %v, %v, waiting %v", uploadedKeys(report), err, report.waiting)
	}

	// Few uploads left.
	few := memoryStore{make(map[string][]byte)}
	budget := testLimits()
	budget.uploads = 2
	report, err = backup(few, root, testNight(t, DAY_OPEN), budget)
	if err != nil || len(few.files) != 2 || len(report.waiting) != 1 {
		t.Fatalf("an upload limit waits too: %v, %d objects, %v", err, len(few.files), report.waiting)
	}

	// Archives bigger than a whole night's allowance are named, with the setting to raise.
	huge := memoryStore{make(map[string][]byte)}
	budget = testLimits()
	budget.night = 1
	report, err = backup(huge, root, testNight(t, DAY_OPEN), budget)
	if err != nil || len(huge.files) != 0 || len(report.waiting) < 3 || !strings.Contains(report.waiting[0], NIGHT_ENV) {
		t.Fatalf("too-big archives wait with advice: %v, %v", err, report.waiting)
	}

	// A bucket that is full keeps what it has and says so.
	full := memoryStore{make(map[string][]byte)}
	budget = testLimits()
	budget.capBytes = state
	report, err = backup(full, root, testNight(t, DAY_OPEN), budget)
	if err != nil || !reflect.DeepEqual(uploadedKeys(report), []string{"state/" + DAY_OPEN + ARCHIVE_SUFFIX}) ||
		len(report.waiting) == 0 || !strings.Contains(strings.Join(report.waiting, " "), "retention cap") {
		t.Fatalf("a full bucket: %v, %v, %v", uploadedKeys(report), err, report.waiting)
	}
}
