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
)

// Record types written to a news file. Row records use the table's own name; control records
// start with RECORD_MARK so they can never collide with a table.
const (
	RECORD_MARK   = "#"
	RECORD_FILE   = "#file"
	RECORD_END    = "#end"
	FILE_DAY      = "day"
	FILE_STATE    = "state"
	COLUMN_HIDDEN = 0 // PRAGMA table_xinfo: 0 is an ordinary column, 2 and 3 are generated ones
)

// newsTable says how one table of news.db is split by day. Where clauses take ?1 and ?2, the
// start and end (exclusive) in Unix milliseconds, and ?3 and ?4, the same two days as labels.
type newsTable struct {
	name    string
	day     string // the rows of one finished day
	open    string // the rows of the state; empty means the day condition over the open day onward
	replace bool   // on restore a later copy replaces an earlier one (rows that change), else the first is kept
}

const (
	WHERE_SEEN   = `seen_at >= ?1 AND seen_at < ?2`
	WHERE_AT     = `at >= ?1 AND at < ?2`
	WHERE_NEVER  = `0`
	WHERE_ALWAYS = `1`
)

// NEWS_TABLES lists every table that holds news, in the order a file writes them. A row belongs
// to the day of its time column, so a finished day never changes again; rows still changing (a
// story, a market, a calendar entry) are exported with the day they last changed, and a later
// day's copy replaces an earlier one on restore. schema_migrations and the search index are not
// exported: a rebuilt database creates them from the migrations.
var NEWS_TABLES = []newsTable{
	{name: "feed_urls", day: `id IN (SELECT feed_url_id FROM events WHERE ` + WHERE_SEEN + `)`, open: WHERE_ALWAYS},
	{name: "events", day: WHERE_SEEN},
	{name: "stories", day: `last_seen >= ?1 AND last_seen < ?2`, replace: true},
	{name: "story_events", day: `event_id IN (SELECT id FROM events WHERE ` + WHERE_SEEN + `)`, replace: true},
	{name: "story_metrics", day: WHERE_AT},
	{name: "markets", day: `updated_at >= ?1 AND updated_at < ?2`, replace: true},
	{name: "market_prices", day: WHERE_AT},
	{name: "calendar_events", day: `starts_at >= ?1 AND starts_at < ?2`, replace: true},
	{name: "net_usage", day: `day >= ?3 AND day < ?4`, replace: true},
	{name: "settings", day: WHERE_NEVER, open: WHERE_ALWAYS, replace: true},
}

// newsRecord is one line of a news file.
type newsRecord struct {
	T      string         `json:"t"`
	Row    map[string]any `json:"r,omitempty"`
	Kind   string         `json:"kind,omitempty"`
	Day    string         `json:"day,omitempty"`
	Part   int            `json:"part,omitempty"`
	Cutoff string         `json:"cutoff,omitempty"`
	Rows   int64          `json:"rows,omitempty"`
	Last   bool           `json:"last,omitempty"`
}

// dayRange is the slice of the news database one file holds.
type dayRange struct {
	start, end int64  // Unix milliseconds, end exclusive
	from, to   string // the same days as YYYY-MM-DD labels, to exclusive
}

type countingWriter struct {
	dst     io.Writer
	written int64
}

func (c *countingWriter) Write(data []byte) (int, error) {
	n, err := c.dst.Write(data)
	c.written += int64(n)
	return n, err
}

// fileWriter streams records into gzip files, starting a new part when one grows past limit
// (limit 0 means a single file). Every part ends with an #end record, so a cut-off file is noticed.
type fileWriter struct {
	dir     string
	head    newsRecord            // the #file record; Part is filled in per file
	name    func(part int) string // file name for a part number
	limit   int64                 // compressed bytes that close a part
	file    *os.File
	counter *countingWriter
	gz      *gzip.Writer
	enc     *json.Encoder
	part    int
	rows    int64 // rows in the open part
	total   int64 // rows in every part
	names   []string
}

func (w *fileWriter) open() error {
	w.part++
	file, err := os.OpenFile(filepath.Join(w.dir, w.name(w.part)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, FILE_MODE)
	if err != nil {
		return err
	}
	w.file, w.rows = file, 0
	w.counter = &countingWriter{dst: file}
	w.gz = gzip.NewWriter(w.counter)
	w.enc = json.NewEncoder(w.gz)
	w.names = append(w.names, w.name(w.part))
	head := w.head
	head.Part = w.part
	return w.enc.Encode(head)
}

// closePart ends the open part; the last one is marked so a restore can tell a day is complete.
// A writer that wrote nothing leaves no file.
func (w *fileWriter) closePart(last bool) error {
	if w.file == nil {
		return nil
	}
	if err := w.enc.Encode(newsRecord{T: RECORD_END, Part: w.part, Rows: w.rows, Last: last}); err != nil {
		return err
	}
	if err := w.gz.Close(); err != nil {
		return err
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *fileWriter) write(table string, row map[string]any) error {
	if w.file != nil && w.limit > 0 && w.rows > 0 && w.counter.written >= w.limit {
		if err := w.closePart(false); err != nil {
			return err
		}
	}
	if w.file == nil {
		if err := w.open(); err != nil {
			return err
		}
	}
	w.rows++
	w.total++
	return w.enc.Encode(newsRecord{T: table, Row: row})
}

// abort closes any open part; the caller removes the directory.
func (w *fileWriter) abort() {
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
}

// tableColumns lists a table's stored columns; generated ones are rebuilt by the database.
func tableColumns(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, table string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT name, hidden FROM pragma_table_xinfo(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var name string
		var hidden int
		if err := rows.Scan(&name, &hidden); err != nil {
			return nil, err
		}
		if hidden == COLUMN_HIDDEN {
			columns = append(columns, name)
		}
	}
	return columns, rows.Err()
}

func quoteName(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// exportTable writes the rows of one table that satisfy where, in storage order.
func exportTable(ctx context.Context, q *sql.Tx, table, where string, args []any, out *fileWriter) error {
	columns, err := tableColumns(ctx, q, table)
	if err != nil {
		return err
	}
	if len(columns) == 0 {
		return nil // the table does not exist in this database yet
	}
	quoted := make([]string, len(columns))
	for i, name := range columns {
		quoted[i] = quoteName(name)
	}
	rows, err := q.QueryContext(ctx, `SELECT `+strings.Join(quoted, ", ")+` FROM `+quoteName(table)+
		` WHERE `+where+` ORDER BY rowid`, args...)
	if err != nil {
		return fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			return err
		}
		row := make(map[string]any, len(columns))
		for i, name := range columns {
			if _, blob := values[i].([]byte); blob {
				return fmt.Errorf("%s.%s holds a blob, which the news backup does not carry", table, name)
			}
			row[name] = values[i]
		}
		if err := out.write(table, row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exportRange writes every table's rows for one range through a single read transaction, so a day
// or the state is one consistent view of the database even while newsd keeps writing.
func exportRange(ctx context.Context, tx *sql.Tx, span dayRange, open bool, out *fileWriter) error {
	args := []any{span.start, span.end, span.from, span.to}
	for _, table := range NEWS_TABLES {
		where := table.day
		if open && table.open != "" {
			where = table.open
		}
		if err := exportTable(ctx, tx, table.name, where, args, out); err != nil {
			return err
		}
	}
	return nil
}
