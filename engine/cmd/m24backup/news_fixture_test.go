package main

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/news"
)

// The fixture is a small news database built from the real migrations: four New York days, the
// last of them still open, with rows in every table the backup carries.
const (
	DAY_ONE        = "2026-11-18"
	DAY_TWO        = "2026-11-19"
	DAY_THREE      = "2026-11-20"
	DAY_OPEN       = "2026-11-21" // the cutoff: today in New York, so not finished
	EVENTS_PER_DAY = 3
)

var fixtureDays = []string{DAY_ONE, DAY_TWO, DAY_THREE, DAY_OPEN}

// newsAt is a moment on a New York day, in Unix milliseconds.
func newsAt(t *testing.T, day string, hour int) int64 {
	t.Helper()
	zone, err := time.LoadLocation(MARKET_ZONE)
	if err != nil {
		t.Fatal(err)
	}
	start, err := time.ParseInLocation(DAY_FORMAT, day, zone)
	if err != nil {
		t.Fatal(err)
	}
	return start.Add(time.Duration(hour) * time.Hour).UnixMilli()
}

func execAll(t *testing.T, db *sql.DB, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%v\n%s", err, statement)
		}
	}
}

// newsFixture creates root/news/news.db through the migrations and fills it.
func newsFixture(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, NEWS_DIR)
	if err := os.MkdirAll(dir, DIR_MODE); err != nil {
		t.Fatal(err)
	}
	store, err := news.OpenStore(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "news.db")
	db := openFixtureDB(t, path)
	defer db.Close()
	execAll(t, db,
		`INSERT INTO feed_urls (id, feed, url, first_seen) VALUES (1, 'wire', 'https://feeds.example/wire.xml', 1000),
			(2, 'central', 'https://feeds.example/bank.xml?a=1&b=2', 2000)`,
		`INSERT INTO settings (key, value) VALUES ('poll_every_ms', '60000')`)
	id := 0
	for d, day := range fixtureDays {
		for n := 0; n < EVENTS_PER_DAY; n++ {
			id++
			seen := newsAt(t, day, 1+n*7)
			var published, image, author, excerpt any = seen - 90_000, nil, "A. Reporter", fmt.Sprintf("Excerpt with ünïcode and \"quotes\" for article %d", id)
			switch n {
			case 0:
				image = "https://img.example/1.png"
			case 1:
				author = nil
			case 2:
				published, excerpt = nil, nil // the source gave no date and no excerpt
			}
			if _, err := db.Exec(`INSERT INTO events (id, seen_at, published_at, feed, source, kind, title, url, summary, lang,
				country, tags, image, author, excerpt, outlet_url, feed_url_id)
				VALUES (?, ?, ?, 'wire', ?, 'news', ?, ?, ?, 'en', 'US', '["SPY","CPI"]', ?, ?, ?, 'https://news.example', ?)`,
				id, seen, published, fmt.Sprintf("Wire %d", d), fmt.Sprintf("Headline %d about inflation", id),
				fmt.Sprintf("https://news.example/%d", id), fmt.Sprintf("Summary %d", id), image, author, excerpt, 1+n%2); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Event 1-3 are day one. Stories: one spans days one and two, one ends on day three, one is open.
	execAll(t, db,
		fmt.Sprintf(`INSERT INTO stories (id, first_seen, last_seen, title, category, angle, articles, outlets, attention, validity, detail)
			VALUES (1, %d, %d, 'Inflation prints hot', 'macro', 'bearish', 2, 2, 71.5, 80, '{"id":1}'),
			(2, %d, %d, 'Bank decision', 'central_bank', 'mixed', 2, 1, 40, 55.25, '{"id":2}'),
			(3, %d, %d, 'Open story', 'macro', 'unclear', 1, 1, 10, 20, '{"id":3}')`,
			newsAt(t, DAY_ONE, 1), newsAt(t, DAY_TWO, 8), newsAt(t, DAY_THREE, 1), newsAt(t, DAY_THREE, 15),
			newsAt(t, DAY_OPEN, 1), newsAt(t, DAY_OPEN, 8)),
		`INSERT INTO story_events (event_id, story_id) VALUES (1, 1), (4, 1), (7, 2), (8, 2), (10, 3)`,
		fmt.Sprintf(`INSERT INTO story_metrics (story_id, at, articles, outlets, last_hour, attention, validity, symbol, rel_volume, move_pct, reading)
			VALUES (1, %d, 1, 1, 1, 30, 50, 'SPY', 1.5, -0.25, 'quiet'), (2, %d, 2, 1, 2, 40, 55, NULL, NULL, NULL, NULL),
			(3, %d, 1, 1, 1, 10, 20, NULL, NULL, NULL, NULL)`, newsAt(t, DAY_ONE, 2), newsAt(t, DAY_THREE, 2), newsAt(t, DAY_OPEN, 2)),
		fmt.Sprintf(`INSERT INTO markets (id, venue, question, slug, end_at, volume, prices, tags, updated_at) VALUES
			('poly:1', 'poly', 'Will CPI beat?', 'cpi-beat', NULL, 1500.5, '{"outcomes":["Yes","No"],"prices":[0.4,0.6]}', '["CPI"]', %d),
			('poly:2', 'poly', 'Will the Fed cut?', 'fed-cut', %d, 12, NULL, '[]', %d)`,
			newsAt(t, DAY_TWO, 3), newsAt(t, DAY_OPEN, 30), newsAt(t, DAY_OPEN, 3)),
		fmt.Sprintf(`INSERT INTO market_prices (market_id, at, prices, volume) VALUES
			('poly:1', %d, '{"prices":[0.5,0.5]}', 1400), ('poly:1', %d, '{"prices":[0.4,0.6]}', 1500.5), ('poly:2', %d, '{"prices":[0.1,0.9]}', 12)`,
			newsAt(t, DAY_ONE, 3), newsAt(t, DAY_TWO, 3), newsAt(t, DAY_OPEN, 3)),
		fmt.Sprintf(`INSERT INTO calendar_events (uid, source, category, starts_at, all_day, title, region, symbol, impact, forecast, previous,
			actual, detail, url, updated_at) VALUES
			('c:1', 'econ', 'macro', %d, 0, 'CPI', 'USD', '', 'high', '3.1', '3.0', '3.2', '', '', 5),
			('c:2', 'econ', 'macro', %d, 0, 'Retail sales', 'USD', '', 'medium', '', '', '', '', '', 6),
			('c:3', 'econ', 'holiday', %d, 1, 'Future holiday', '', '', '', '', '', '', '', '', 7)`,
			newsAt(t, DAY_TWO, 8), newsAt(t, DAY_OPEN, 9), newsAt(t, "2026-12-25", 0)),
		`INSERT INTO net_usage (day, channel, requests, bytes_in, bytes_out) VALUES
			('2026-11-18', 'wire', 10, 1000, 20), ('2026-11-19', 'wire', 12, 1200, 30), ('2026-11-21', 'wire', 1, 100, 2)`)
	return path
}

func openFixtureDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open(NEWS_DRIVER, "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// dumpNews reads every table the backup carries, plus what the database derives from them,
// as text lines in storage order. Two databases with the same rows give the same dump.
func dumpNews(t *testing.T, path string) map[string][]string {
	t.Helper()
	db := openFixtureDB(t, path)
	defer db.Close()
	dump := map[string][]string{}
	for _, table := range NEWS_TABLES {
		rows, err := db.Query(`SELECT * FROM ` + table.name + ` ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		columns, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range values {
				targets[i] = &values[i]
			}
			if err := rows.Scan(targets...); err != nil {
				t.Fatal(err)
			}
			var line strings.Builder
			for i, value := range values {
				fmt.Fprintf(&line, "%s=%T:%v|", columns[i], value, value)
			}
			dump[table.name] = append(dump[table.name], line.String())
		}
		rows.Close()
	}
	// The word-search index is rebuilt from the rows, so a search must find the same articles.
	rows, err := db.Query(`SELECT rowid FROM events_fts WHERE events_fts MATCH 'inflation' ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		dump["events_fts"] = append(dump["events_fts"], fmt.Sprint(id))
	}
	return dump
}

// readNewsFile decodes every record of a news file.
func readNewsFile(t *testing.T, path string) []newsRecord {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(compressed)
	decoder.UseNumber()
	var records []newsRecord
	for {
		var record newsRecord
		if err := decoder.Decode(&record); err == io.EOF {
			return records
		} else if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
}

func countRows(records []newsRecord, table string) int {
	count := 0
	for _, record := range records {
		if record.T == table {
			count++
		}
	}
	return count
}
