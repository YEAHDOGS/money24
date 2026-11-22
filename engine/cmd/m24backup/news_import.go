package main

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/YEAHDOGS/money24/engine/news"
)

const (
	NEWS_WRITE_DSN   = "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(OFF)&_pragma=foreign_keys(0)"
	MAX_MISSING_SHOW = 10
	CHECKPOINT_SQL   = `PRAGMA wal_checkpoint(TRUNCATE)`
)

// rebuildReport says what a rebuild did and what it could not find.
type rebuildReport struct {
	files   int
	rows    int64
	missing []string // days with no file between the first day file and the state's cutoff
	partial []string // days whose part files are not all there
}

// rebuildNews creates news/news.db inside dir from the day files and the state file found there,
// then removes the state file when removeState is set (its rows now live in the database). It does nothing when dir has
// neither, which is how a backup made before day files existed restores: its state archive carries
// the whole database, and a database already there is filled in rather than replaced.
func rebuildNews(dir string, removeState bool) (rebuildReport, error) {
	var report rebuildReport
	files, err := listNewsFiles(dir)
	if err != nil {
		return report, err
	}
	statePath := filepath.Join(dir, filepath.FromSlash(NEWS_STATE_FILE))
	_, stateErr := os.Stat(statePath)
	if len(files) == 0 && errors.Is(stateErr, os.ErrNotExist) {
		return report, nil
	}
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return report, stateErr
	}
	ctx := context.Background()
	store, err := news.OpenStore(ctx, filepath.Join(dir, NEWS_DIR))
	if err != nil {
		return report, fmt.Errorf("create the news database: %w", err)
	}
	if err := store.Close(); err != nil {
		return report, err
	}
	dbPath := filepath.Join(dir, filepath.FromSlash(NEWS_DB))
	db, err := sql.Open(NEWS_DRIVER, "file:"+filepath.ToSlash(dbPath)+NEWS_WRITE_DSN)
	if err != nil {
		return report, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	importer := &newsImporter{db: db, columns: map[string]map[string]bool{}, parts: map[string]*dayParts{}}
	for _, file := range files {
		if err := importer.load(ctx, filepath.Join(dir, filepath.FromSlash(file.name))); err != nil {
			return report, fmt.Errorf("%s: %w", file.name, err)
		}
		report.files++
	}
	if !errors.Is(stateErr, os.ErrNotExist) {
		if err := importer.load(ctx, statePath); err != nil {
			return report, fmt.Errorf("%s: %w", NEWS_STATE_FILE, err)
		}
		report.files++
	}
	report.rows = importer.rows
	report.missing, report.partial = importer.gaps(files)
	if _, err := db.ExecContext(ctx, CHECKPOINT_SQL); err != nil {
		return report, err
	}
	if !removeState {
		return report, nil
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	return report, nil
}

// dayParts tracks which parts of one day a rebuild has seen.
type dayParts struct {
	seen     map[int]bool
	finished bool // the part marked last was read
}

type newsImporter struct {
	db         *sql.DB
	statements map[string]*sql.Stmt       // the open file's inserts, one per table and column set
	columns    map[string]map[string]bool // each table's stored columns
	parts      map[string]*dayParts       // day files read, by day
	cutoff     string                     // the state's cutoff day, once read
	rows       int64
}

func (i *newsImporter) closeStatements() {
	for _, stmt := range i.statements {
		stmt.Close()
	}
	i.statements = nil
}

// load imports one news file in a single transaction, so a damaged file changes nothing.
func (i *newsImporter) load(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	i.statements = map[string]*sql.Stmt{}
	defer i.closeStatements()
	decoder := json.NewDecoder(compressed)
	decoder.UseNumber()
	var head newsRecord
	ended := false
	for {
		var record newsRecord
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if ended {
			return fmt.Errorf("records after the end of the file")
		}
		switch {
		case record.T == RECORD_FILE:
			head = record
		case record.T == RECORD_END:
			ended = true
			i.note(head, record)
		case strings.HasPrefix(record.T, RECORD_MARK):
			return fmt.Errorf("unknown record %q", record.T)
		default:
			if err := i.insert(ctx, tx, record); err != nil {
				return err
			}
		}
	}
	if head.T != RECORD_FILE || !ended {
		return fmt.Errorf("the file is cut short")
	}
	if _, err := io.Copy(io.Discard, compressed); err != nil { // verify gzip's checksum
		return err
	}
	return tx.Commit()
}

func (i *newsImporter) note(head, end newsRecord) {
	if head.Kind == FILE_STATE {
		i.cutoff = head.Cutoff
		return
	}
	entry := i.parts[head.Day]
	if entry == nil {
		entry = &dayParts{seen: map[int]bool{}}
		i.parts[head.Day] = entry
	}
	entry.seen[end.Part] = true
	entry.finished = entry.finished || end.Last
}

// insert adds one row, keeping the first copy of rows that never change and letting a later copy
// replace rows that do. Columns the database no longer has are dropped; new ones take their default.
func (i *newsImporter) insert(ctx context.Context, tx *sql.Tx, record newsRecord) error {
	var table *newsTable
	for n := range NEWS_TABLES {
		if NEWS_TABLES[n].name == record.T {
			table = &NEWS_TABLES[n]
		}
	}
	if table == nil {
		return fmt.Errorf("unknown table %q", record.T)
	}
	known, ok := i.columns[table.name]
	if !ok {
		columns, err := tableColumns(ctx, tx, table.name)
		if err != nil {
			return err
		}
		known = make(map[string]bool, len(columns))
		for _, name := range columns {
			known[name] = true
		}
		i.columns[table.name] = known
	}
	names := make([]string, 0, len(record.Row))
	for name := range record.Row {
		if known[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	values := make([]any, len(names))
	for n, name := range names {
		value, err := sqlValue(record.Row[name])
		if err != nil {
			return fmt.Errorf("%s.%s: %w", table.name, name, err)
		}
		values[n] = value
	}
	key := table.name + "\x00" + strings.Join(names, "\x00")
	stmt, ok := i.statements[key]
	if !ok {
		verb := "INSERT OR IGNORE"
		if table.replace {
			verb = "INSERT OR REPLACE"
		}
		quoted := make([]string, len(names))
		marks := make([]string, len(names))
		for n, name := range names {
			quoted[n], marks[n] = quoteName(name), "?"
		}
		var err error
		stmt, err = tx.PrepareContext(ctx, verb+" INTO "+quoteName(table.name)+" ("+strings.Join(quoted, ", ")+
			") VALUES ("+strings.Join(marks, ", ")+")")
		if err != nil {
			return err
		}
		i.statements[key] = stmt
	}
	if _, err := stmt.ExecContext(ctx, values...); err != nil {
		return fmt.Errorf("%s: %w", table.name, err)
	}
	i.rows++
	return nil
}

// sqlValue turns a decoded JSON value into what SQLite stores: whole numbers stay integers.
func sqlValue(value any) (any, error) {
	switch v := value.(type) {
	case nil, string:
		return v, nil
	case json.Number:
		if whole, err := v.Int64(); err == nil {
			return whole, nil
		}
		return v.Float64()
	}
	return nil, fmt.Errorf("unsupported value %T", value)
}

// gaps lists the days a rebuild expected but did not find: days between the first day file and the
// state's cutoff with no file (an empty day, or one the backup has not uploaded yet), and days
// whose parts are incomplete.
func (i *newsImporter) gaps(files []newsFile) (missing, partial []string) {
	if len(files) == 0 || i.cutoff == "" {
		return nil, nil
	}
	have := map[string]bool{}
	for _, file := range files {
		have[file.day] = true
	}
	for _, day := range dayRangeLabels(files[0].day, i.cutoff) {
		if !have[day] {
			missing = append(missing, day)
		}
	}
	days := make([]string, 0, len(i.parts))
	for day := range i.parts {
		days = append(days, day)
	}
	sort.Strings(days)
	for _, day := range days {
		entry := i.parts[day]
		complete := entry.finished
		for part := 1; complete && part <= len(entry.seen); part++ {
			complete = entry.seen[part]
		}
		if !complete {
			partial = append(partial, day)
		}
	}
	return missing, partial
}
