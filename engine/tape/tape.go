// Package tape records the option chains short-term plays are made of, every minute of the stock
// session, so they can be backtested later. Free intraday option history does not exist; this is
// how Money 24 builds its own. Each minute, for every symbol, it saves the same-day chain and the
// next expiry's (0DTE and weekly), every strike's bid, ask, last, volume, open interest and implied
// volatility, beside the stock's price and the time the quote is from.
//
// Files: DIR/YYYY-MM-DD/SYMBOL.jsonl.gz, New York date, one gzip member appended per minute (a gzip
// reader reads them as one stream). One line per snapshot: {"at", "market_time", "spot", "expiry",
// "calls", "puts"}.
package tape

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/calendar"
	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	EVERY         = time.Minute
	PASS_TIMEOUT  = 50 * time.Second // a pass that runs over is cut, so the next minute starts on time
	REQUEST_GAP   = 200 * time.Millisecond
	EXPIRIES      = 2 // the nearest expiry (the same day when one lists) and the one after it
	DAY_LAYOUT    = "2006-01-02"
	FILE_SUFFIX   = ".jsonl.gz"
	DIR_PERMS     = 0o755
	FILE_PERMS    = 0o644
	SAFE_NAME     = "^"              // index symbols start with it; it is dropped from file names
	OPEN_EARLY    = time.Minute      // the first snapshot is taken right at the open
	CLOSE_LATE    = 15 * time.Minute // Yahoo runs about 15 minutes behind: keep recording that long after the close
	LOG_PREFIX    = "tape: "
	SYMBOLS_LIMIT = 12
)

// DEFAULT_SYMBOLS are the names with daily expiries and the deepest same-day option markets.
var DEFAULT_SYMBOLS = []string{"SPY", "QQQ", "IWM", "^SPX"}

// Snapshot is one chain at one moment.
type Snapshot struct {
	At         time.Time          `json:"at"`
	MarketTime int64              `json:"market_time"`
	Spot       float64            `json:"spot"`
	Expiry     string             `json:"expiry"`
	Calls      []market.OptionRow `json:"calls"`
	Puts       []market.OptionRow `json:"puts"`
}

// Status is what the recorder has done, for the desk.
type Status struct {
	Recording bool      `json:"recording"`
	Symbols   []string  `json:"symbols"`
	Today     int       `json:"today"` // snapshots written today
	LastAt    time.Time `json:"last_at,omitzero"`
	LastError string    `json:"last_error,omitempty"`
	Days      []string  `json:"days"` // days on disk, newest first
	Dir       string    `json:"dir"`
}

// Recorder writes the tape.
type Recorder struct {
	Dir     string
	Symbols []string
	Market  *market.Client

	mu    sync.Mutex
	state Status
}

// session says whether the recorder should be taking snapshots: the stock session, plus the
// minutes after the close that delayed quotes still need.
func session(now time.Time) bool {
	day := calendar.On(now)
	if day.Session == calendar.SESSION_CLOSED {
		return false
	}
	return !now.Before(day.Opens.Add(-OPEN_EARLY)) && now.Before(day.Closes.Add(CLOSE_LATE))
}

// write appends one gzip member holding the lines to the symbol's file for today.
func (r *Recorder) write(day, symbol string, snaps []Snapshot) error {
	dir := filepath.Join(r.Dir, day)
	if err := os.MkdirAll(dir, DIR_PERMS); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, strings.TrimPrefix(symbol, SAFE_NAME)+FILE_SUFFIX), os.O_CREATE|os.O_APPEND|os.O_WRONLY, FILE_PERMS)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := gzip.NewWriter(f)
	enc := json.NewEncoder(zw)
	for _, s := range snaps {
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	return zw.Close()
}

// record takes one symbol's snapshots: the nearest expiries' chains.
func (r *Recorder) record(ctx context.Context, symbol string, now time.Time) ([]Snapshot, error) {
	near, err := r.Market.Chain(ctx, symbol, 0)
	if err != nil {
		return nil, err
	}
	out := []Snapshot{}
	for i, expiry := range near.Expiries {
		if i >= EXPIRIES {
			break
		}
		chain := near
		if i > 0 {
			time.Sleep(REQUEST_GAP)
			if chain, err = r.Market.Chain(ctx, symbol, expiry); err != nil {
				return out, err
			}
		}
		out = append(out, Snapshot{At: now.UTC(), MarketTime: chain.MarketTime, Spot: chain.Spot,
			Expiry: time.Unix(chain.Expiry, 0).UTC().Format(DAY_LAYOUT), Calls: chain.Calls, Puts: chain.Puts})
	}
	return out, nil
}

// pass records every symbol once.
func (r *Recorder) pass(ctx context.Context, now time.Time) {
	day := now.In(calendar.NEW_YORK).Format(DAY_LAYOUT)
	written, failed := 0, []string{}
	for _, symbol := range r.Symbols {
		snaps, err := r.record(ctx, symbol, now)
		if err == nil && len(snaps) > 0 {
			err = r.write(day, symbol, snaps)
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", symbol, err))
			continue
		}
		written += len(snaps)
		time.Sleep(REQUEST_GAP)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state.LastAt.In(calendar.NEW_YORK).Format(DAY_LAYOUT) != day {
		r.state.Today = 0
	}
	r.state.Today += written
	r.state.LastAt = now.UTC()
	r.state.LastError = strings.Join(failed, "; ")
	if len(failed) > 0 {
		log.Print(LOG_PREFIX + r.state.LastError)
	}
}

// Status is the recorder right now, with the days on disk.
func (r *Recorder) Status(now time.Time) Status {
	r.mu.Lock()
	st := r.state
	r.mu.Unlock()
	st.Recording, st.Symbols, st.Dir, st.Days = session(now), r.Symbols, r.Dir, []string{}
	entries, _ := os.ReadDir(r.Dir)
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].IsDir() {
			st.Days = append(st.Days, entries[i].Name())
		}
	}
	return st
}

// Run records every minute of every session until ctx ends.
func (r *Recorder) Run(ctx context.Context) {
	if len(r.Symbols) == 0 {
		r.Symbols = DEFAULT_SYMBOLS
	}
	if len(r.Symbols) > SYMBOLS_LIMIT {
		r.Symbols = r.Symbols[:SYMBOLS_LIMIT]
	}
	tick := time.NewTicker(EVERY)
	defer tick.Stop()
	for {
		if now := time.Now(); session(now) {
			pass, cancel := context.WithTimeout(ctx, PASS_TIMEOUT)
			r.pass(pass, now)
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Read returns every snapshot in one symbol's file for one day, oldest first, for backtests.
func Read(dir, day, symbol string) ([]Snapshot, error) {
	f, err := os.Open(filepath.Join(dir, day, strings.TrimPrefix(symbol, SAFE_NAME)+FILE_SUFFIX))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := []Snapshot{}
	dec := json.NewDecoder(zr)
	for dec.More() {
		var s Snapshot
		if err := dec.Decode(&s); err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}
