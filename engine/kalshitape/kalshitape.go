// Package kalshitape records Kalshi's board for backtesting: every 30 minutes, a compact snapshot of
// every market closing within the next two days (the part of a market's life where most trading
// happens), every hour the results of every market that settled, and once a day each market's title
// and terms. The whole board is over 80,000 markets and 170 MB a pass, far too much to keep; this is
// the slice prediction strategies are built on, at about 10 MB a day. Paper data only.
//
// Files: DIR/YYYY-MM-DD/{markets,results,catalog}.jsonl.gz, UTC date, one gzip member appended per
// pass (a gzip reader reads them as one stream), one JSON object per line.
package kalshitape

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/predict"
)

const (
	TICK           = time.Minute
	PASS_EVERY     = 30 * time.Minute
	SETTLE_EVERY   = time.Hour
	WINDOW         = 48 * time.Hour // markets closing this soon are snapshot
	SETTLED_LOOK   = 48 * time.Hour // settlements are looked for among markets that closed this recently
	PASS_TIMEOUT   = 10 * time.Minute
	SEEN_DAYS      = 3 // days of result files read back on start, so a restart never writes a result twice
	DAY_LAYOUT     = "2006-01-02"
	FILE_MARKETS   = "markets.jsonl.gz"
	FILE_RESULTS   = "results.jsonl.gz"
	FILE_CATALOG   = "catalog.jsonl.gz"
	DIR_PERMS      = 0o755
	FILE_PERMS     = 0o644
	LINE_MAX       = 1 << 20
	LOG_PREFIX     = "kalshitape: "
	ERR_NO_MARKETS = "kalshi returned no markets"
	DAYS_SHOWN     = 7
)

// Snapshot is one market at one moment, as compact as it can be: prices are dollars a contract.
type Snapshot struct {
	At     int64   `json:"at"` // Unix seconds
	Ticker string  `json:"t"`
	YesBid float64 `json:"yb,omitempty"`
	YesAsk float64 `json:"ya,omitempty"`
	NoBid  float64 `json:"nb,omitempty"`
	NoAsk  float64 `json:"na,omitempty"`
	Volume float64 `json:"v,omitempty"` // contracts traded in the last 24 hours
}

// Result is how a market settled.
type Result struct {
	At      int64   `json:"at"` // when it was recorded
	Ticker  string  `json:"t"`
	Result  string  `json:"r"`
	Value   float64 `json:"v"` // what one YES contract paid
	CloseAt int64   `json:"c"`
}

// Terms is what a market is, written the first time it is seen each day.
type Terms struct {
	Ticker   string  `json:"t"`
	Event    string  `json:"e"`
	Title    string  `json:"title"`
	YesTitle string  `json:"yes,omitempty"`
	Type     string  `json:"type,omitempty"`
	Strike   string  `json:"strike,omitempty"`
	Floor    float64 `json:"floor,omitempty"`
	Cap      float64 `json:"cap,omitempty"`
	CloseAt  int64   `json:"c"`
}

// Status is what the recorder has done, for the desk.
type Status struct {
	Recording     bool      `json:"recording"`
	LastPassAt    time.Time `json:"last_pass_at,omitzero"`
	LastSettleAt  time.Time `json:"last_settle_at,omitzero"`
	MarketsLast   int       `json:"markets_last_pass"`
	SnapshotsDay  int       `json:"snapshots_today"`
	ResultsDay    int       `json:"results_today"`
	CatalogDay    int       `json:"catalog_today"`
	LastError     string    `json:"last_error,omitempty"`
	Dir           string    `json:"dir"`
	BytesOnDisk   int64     `json:"bytes_on_disk"`
	Days          []string  `json:"days"` // newest first
	day           string
	catalogSeenOn map[string]bool
}

// Recorder writes the tape.
type Recorder struct {
	Dir    string
	Kalshi *predict.Kalshi

	mu    sync.Mutex
	state Status
	seen  map[string]bool // tickers whose result is on disk
}

// Run records until ctx ends.
func (r *Recorder) Run(ctx context.Context) {
	r.mu.Lock()
	r.state.Recording, r.state.Dir = true, r.Dir
	r.seen = r.readSeen(time.Now().UTC())
	r.mu.Unlock()
	var nextPass, nextSettle time.Time
	for {
		now := time.Now().UTC()
		if !now.Before(nextPass) {
			r.run(ctx, now, r.pass)
			nextPass = now.Add(PASS_EVERY)
		}
		if !now.Before(nextSettle) {
			r.run(ctx, now, r.settle)
			nextSettle = now.Add(SETTLE_EVERY)
		}
		select {
		case <-ctx.Done():
			r.mu.Lock()
			r.state.Recording = false
			r.mu.Unlock()
			return
		case <-time.After(TICK):
		}
	}
}

// run does one job with a time limit, and records its error.
func (r *Recorder) run(ctx context.Context, now time.Time, job func(context.Context, time.Time) error) {
	pass, cancel := context.WithTimeout(ctx, PASS_TIMEOUT)
	err := job(pass, now)
	cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.LastError = ""
	if err != nil && ctx.Err() == nil {
		r.state.LastError = err.Error()
		log.Printf("%s%v", LOG_PREFIX, err)
	}
}

// rollDay resets the day's counters at UTC midnight. The caller holds r.mu.
func (r *Recorder) rollDay(day string) {
	if r.state.day == day {
		return
	}
	r.state.day, r.state.SnapshotsDay, r.state.ResultsDay, r.state.CatalogDay = day, 0, 0, 0
	r.state.catalogSeenOn = map[string]bool{}
}

// pass snapshots every market closing within WINDOW, and writes the terms of any not seen today.
func (r *Recorder) pass(ctx context.Context, now time.Time) error {
	markets, err := r.Kalshi.Closing(ctx, now, now.Add(WINDOW))
	if len(markets) == 0 {
		if err == nil {
			err = errors.New(ERR_NO_MARKETS)
		}
		return err
	}
	day := now.Format(DAY_LAYOUT)
	r.mu.Lock()
	r.rollDay(day)
	snaps, terms := make([]any, 0, len(markets)), []any{}
	for _, m := range markets {
		snaps = append(snaps, Snapshot{At: now.Unix(), Ticker: m.Ticker, YesBid: m.YesBid, YesAsk: m.YesAsk, NoBid: m.NoBid, NoAsk: m.NoAsk, Volume: m.Volume24h})
		if r.state.catalogSeenOn[m.Ticker] {
			continue
		}
		r.state.catalogSeenOn[m.Ticker] = true
		terms = append(terms, Terms{Ticker: m.Ticker, Event: m.Event, Title: m.Title, YesTitle: m.YesTitle, Type: m.Type,
			Strike: m.Strike, Floor: m.Floor, Cap: m.Cap, CloseAt: m.CloseAt.Unix()})
	}
	r.mu.Unlock()
	if werr := r.write(day, FILE_MARKETS, snaps); werr != nil {
		return werr
	}
	if werr := r.write(day, FILE_CATALOG, terms); werr != nil {
		return werr
	}
	r.mu.Lock()
	r.state.LastPassAt, r.state.MarketsLast = now, len(markets)
	r.state.SnapshotsDay += len(snaps)
	r.state.CatalogDay += len(terms)
	r.mu.Unlock()
	return err
}

// settle writes the result of every market that settled and is not on record yet.
func (r *Recorder) settle(ctx context.Context, now time.Time) error {
	markets, err := r.Kalshi.Settled(ctx, now.Add(-SETTLED_LOOK))
	day := now.Format(DAY_LAYOUT)
	r.mu.Lock()
	r.rollDay(day)
	results := []any{}
	fresh := []string{}
	for _, m := range markets {
		if r.seen[m.Ticker] || !m.Finished() {
			continue
		}
		results = append(results, Result{At: now.Unix(), Ticker: m.Ticker, Result: m.Result, Value: m.Payout(predict.SIDE_YES), CloseAt: m.CloseAt.Unix()})
		fresh = append(fresh, m.Ticker)
	}
	r.mu.Unlock()
	if werr := r.write(day, FILE_RESULTS, results); werr != nil {
		return werr
	}
	r.mu.Lock()
	for _, t := range fresh {
		r.seen[t] = true
	}
	r.state.LastSettleAt = now
	r.state.ResultsDay += len(results)
	r.mu.Unlock()
	return err
}

// write appends one gzip member holding rows, one JSON object a line.
func (r *Recorder) write(day, name string, rows []any) error {
	if len(rows) == 0 {
		return nil
	}
	dir := filepath.Join(r.Dir, day)
	if err := os.MkdirAll(dir, DIR_PERMS); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, FILE_PERMS)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	enc := json.NewEncoder(zw)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			zw.Close()
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// readSeen loads the tickers already settled on disk in the last few days.
func (r *Recorder) readSeen(now time.Time) map[string]bool {
	seen := map[string]bool{}
	for i := range SEEN_DAYS {
		path := filepath.Join(r.Dir, now.AddDate(0, 0, -i).Format(DAY_LAYOUT), FILE_RESULTS)
		if err := eachLine(path, func(line []byte) {
			var res Result
			if json.Unmarshal(line, &res) == nil && res.Ticker != "" {
				seen[res.Ticker] = true
			}
		}); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("%sreading %s: %v", LOG_PREFIX, path, err)
		}
	}
	return seen
}

// eachLine calls fn for every line of a gzip file of appended members.
func eachLine(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 0, LINE_MAX), LINE_MAX)
	for sc.Scan() {
		fn(sc.Bytes())
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	return nil
}

// Status is the recorder now: counts, the days on disk and their size.
func (r *Recorder) Status() Status {
	r.mu.Lock()
	s := r.state
	r.mu.Unlock()
	s.catalogSeenOn = nil
	s.Days, s.BytesOnDisk = []string{}, 0
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return s
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s.Days = append(s.Days, e.Name())
		files, _ := os.ReadDir(filepath.Join(r.Dir, e.Name()))
		for _, f := range files {
			if info, err := f.Info(); err == nil {
				s.BytesOnDisk += info.Size()
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(s.Days)))
	s.Days = s.Days[:min(len(s.Days), DAYS_SHOWN)]
	return s
}
