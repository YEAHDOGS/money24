package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite driver, registers "sqlite"
)

const (
	NEWS_DRIVER      = "sqlite"
	NEWS_READ_DSN    = "?mode=ro&_pragma=busy_timeout(10000)"
	FIRST_EVENT_SQL  = `SELECT MIN(seen_at) FROM events`
	EXPORT_TIMEOUT   = 2 * time.Hour
	MILLIS_UNBOUNDED = math.MaxInt64
)

// newsExport reads the live news database. Every read goes through one deferred transaction, which in
// WAL mode is a stable snapshot that never blocks newsd's writes; the database file is never copied.
type newsExport struct {
	db   *sql.DB
	zone *time.Location
}

// openNewsExport opens news.db read-only. The second result is false when there is no news database.
func openNewsExport(root string, zone *time.Location) (*newsExport, bool, error) {
	path := filepath.Join(root, filepath.FromSlash(NEWS_DB))
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	db, err := sql.Open(NEWS_DRIVER, "file:"+filepath.ToSlash(path)+NEWS_READ_DSN)
	if err != nil {
		return nil, false, err
	}
	db.SetMaxOpenConns(1)
	return &newsExport{db: db, zone: zone}, true, nil
}

// exportNewsDays writes a day file for every finished New York day (before cutoff) that has none
// yet, oldest first, and returns the days it wrote. A finished day is exported once: a file
// that exists is never written again, so what was uploaded stays what the backup holds.
func exportNewsDays(root string, zone *time.Location, cutoff string, partBytes int64) ([]string, error) {
	export, found, err := openNewsExport(root, zone)
	if err != nil || !found {
		return nil, err
	}
	defer export.db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), EXPORT_TIMEOUT)
	defer cancel()
	var first sql.NullInt64
	if err := export.db.QueryRowContext(ctx, FIRST_EVENT_SQL).Scan(&first); err != nil {
		return nil, fmt.Errorf("read the first news event: %w", err)
	}
	if !first.Valid {
		return nil, nil
	}
	dir := filepath.Join(root, filepath.FromSlash(NEWS_DAYS_DIR))
	if err := os.MkdirAll(dir, DIR_MODE); err != nil {
		return nil, err
	}
	var written []string
	day := time.UnixMilli(first.Int64).In(zone).Format(DAY_FORMAT)
	for firstDay := true; day < cutoff; firstDay = false {
		start, end, err := dayBounds(zone, day)
		if err != nil {
			return written, err
		}
		next := time.UnixMilli(end).In(zone).Format(DAY_FORMAT)
		if _, err := os.Stat(filepath.Join(dir, newsPartName(day, 1))); errors.Is(err, os.ErrNotExist) {
			span := dayRange{start: start, end: end, from: day, to: next}
			if firstDay {
				// Rows dated before the first event (a calendar entry, an old usage count) go with the first day.
				span.start, span.from = math.MinInt64, ""
			}
			exported, err := export.exportDay(ctx, dir, day, span, partBytes)
			if err != nil {
				return written, fmt.Errorf("export %s: %w", day, err)
			}
			if exported {
				written = append(written, day)
			}
		} else if err != nil {
			return written, err
		}
		day = next
	}
	return written, nil
}

// exportDay writes one day into a private folder beside the day files, then moves the files in
// with the first part last: the first part existing means every part is in place.
func (n *newsExport) exportDay(ctx context.Context, dir, day string, span dayRange, partBytes int64) (bool, error) {
	scratch, err := os.MkdirTemp(dir, NEWS_TEMP_DIR)
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(scratch)
	out := &fileWriter{dir: scratch, head: newsRecord{T: RECORD_FILE, Kind: FILE_DAY, Day: day}, limit: partBytes,
		name: func(part int) string { return newsPartName(day, part) }}
	defer out.abort()
	tx, err := n.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err := exportRange(ctx, tx, span, false, out); err != nil {
		return false, err
	}
	if err := out.closePart(true); err != nil || out.total == 0 {
		return false, err
	}
	for i := len(out.names) - 1; i >= 0; i-- { // the first part is names[0], moved last
		if err := os.Rename(filepath.Join(scratch, out.names[i]), filepath.Join(dir, out.names[i])); err != nil {
			return false, err
		}
	}
	return true, nil
}

// writeNewsState writes the open day onward (everything from the start of cutoff's day, including
// later-dated calendar entries) plus the small tables that are not split by day. It stays small
// however much history the database holds, because every finished day lives in its day file.
func writeNewsState(root string, zone *time.Location, cutoff, dst string) (bool, error) {
	export, found, err := openNewsExport(root, zone)
	if err != nil || !found {
		return false, err
	}
	defer export.db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), EXPORT_TIMEOUT)
	defer cancel()
	start, _, err := dayBounds(zone, cutoff)
	if err != nil {
		return false, err
	}
	out := &fileWriter{dir: filepath.Dir(dst), head: newsRecord{T: RECORD_FILE, Kind: FILE_STATE, Cutoff: cutoff},
		name: func(int) string { return filepath.Base(dst) }}
	defer out.abort()
	tx, err := export.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	span := dayRange{start: start, end: MILLIS_UNBOUNDED, from: cutoff, to: DAY_END_LABEL}
	if err := exportRange(ctx, tx, span, true, out); err != nil {
		return false, err
	}
	if out.total == 0 {
		// An empty state still records the cutoff, so a restore knows which days to expect.
		if err := out.open(); err != nil {
			return false, err
		}
	}
	return true, out.closePart(true)
}
