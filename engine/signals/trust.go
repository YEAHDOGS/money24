package signals

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// How a source is judged. Every entry it called is priced from market history at the moment it
// was posted, then followed:
//
//   - When the source later posted its own exit for that symbol, the call is judged on that exit.
//   - When it never did, the call is judged on the five trading days after the entry: it counts
//     as a win when the price ran at least MIN_WIN_PCT in the called direction at any point.
//
// The second rule is generous on purpose (it asks "could a follower have taken a profit?"), so
// the page also shows where each call stood at the end of the window and how far it went against.
const (
	DAY                = 24 * time.Hour
	WINDOW_CRYPTO      = 5 * DAY  // coins trade every day
	WINDOW_MARKET      = 7 * DAY  // five trading days span seven calendar days
	MAX_EXIT_WAIT      = 30 * DAY // an "exit" posted later than this is not treated as closing the call
	MIN_WIN_PCT        = 2.0      // a run smaller than this is noise, not a profit a follower could take
	TRUST_Z            = 1.96     // 95% confidence: how sure the trust score is of the win rate
	SCORE_WORKERS      = 4        // price lookups in flight at once
	LONG               = 1.0
	SHORT              = -1.0
	NOTE_NO_HISTORY    = "no price history for this symbol and date"
	NOTE_TOKEN         = "on-chain tokens have no price history source yet"
	TRUST_ROUTE        = "GET /api/trust"
	TRUST_SOURCE_ROUTE = "GET /api/trust/source"
	TRUST_LOAD_ROUTE   = "POST /api/trust/load"
)

var ErrNotLoadable = errors.New("only X sources can load history")

// PriceBar is one candle of market history.
type PriceBar struct {
	At                     time.Time
	Open, High, Low, Close float64
}

// HistoryFunc returns the candles for a signal's symbol between two times, oldest first.
type HistoryFunc func(ctx context.Context, sig Signal, from, to time.Time) ([]PriceBar, error)

// Outcome is one called entry and what the market did after it. Percentages are in the called
// direction: a put that was followed by a falling stock shows a positive number.
type Outcome struct {
	Signal    Signal    `json:"signal"`
	At        time.Time `json:"at"`
	Scored    bool      `json:"scored"`
	Note      string    `json:"note,omitempty"`     // why it could not be scored
	Open      bool      `json:"open,omitempty"`     // the window has not ended: shown, not counted
	Entry     float64   `json:"entry,omitempty"`    // market price when the call was posted
	HasExit   bool      `json:"has_exit,omitempty"` // the source posted its own exit
	ExitAt    time.Time `json:"exit_at,omitzero"`
	Exit      float64   `json:"exit,omitempty"`
	ExitPct   float64   `json:"exit_pct,omitempty"`
	PeakPct   float64   `json:"peak_pct"`   // best moment in the window
	TroughPct float64   `json:"trough_pct"` // worst moment in the window
	EndPct    float64   `json:"end_pct"`    // where it stood when the window closed
	ResultPct float64   `json:"result_pct"` // what the call is judged on: the exit, or the peak
	Win       bool      `json:"win"`
}

// Trust is one source's record. TrustPct is the win rate marked down for a small sample (the
// lower end of its 95% confidence range), so three wins out of three does not outrank sixty out
// of eighty.
type Trust struct {
	SourceID     string  `json:"source_id"`
	Rank         int     `json:"rank"`     // 1 is the most trusted; 0 means nothing scored yet
	Posts        int     `json:"posts"`    // posts on record
	Callouts     int     `json:"callouts"` // posts read as an entry
	Scored       int     `json:"scored"`   // callouts with a finished window and a price
	Open         int     `json:"open"`
	Unpriced     int     `json:"unpriced"`
	Exits        int     `json:"exits"` // scored callouts judged on the source's own exit
	Wins         int     `json:"wins"`
	WinRatePct   float64 `json:"win_rate_pct"`
	TrustPct     float64 `json:"trust_pct"`
	AvgResultPct float64 `json:"avg_result_pct"`
	AvgPeakPct   float64 `json:"avg_peak_pct"`
	AvgTroughPct float64 `json:"avg_trough_pct"`
	AvgEndPct    float64 `json:"avg_end_pct"`
}

// When is the moment a post was made, or when it was first seen if that is all we know.
func (s Signal) When() time.Time {
	if s.PostedAt.IsZero() {
		return s.SeenAt
	}
	return s.PostedAt
}

// Tracker scores sources from the recorded posts. Finished outcomes are kept in memory: market
// history does not change, so each call is priced once.
type Tracker struct {
	rec     *Recorder
	catalog *Catalog
	history HistoryFunc
	mu      sync.Mutex
	done    map[string]Outcome
}

func NewTracker(rec *Recorder, c *Catalog, history HistoryFunc) *Tracker {
	return &Tracker{rec: rec, catalog: c, history: history, done: map[string]Outcome{}}
}

// follow prices one entry and what came after it. exit is the source's own exit post, if any.
func (t *Tracker) follow(ctx context.Context, sig Signal, exit *Signal, now time.Time) Outcome {
	at := sig.When()
	out := Outcome{Signal: sig, At: at}
	if sig.Address != "" {
		out.Note = NOTE_TOKEN
		return out
	}
	window := WINDOW_MARKET
	if sig.Market == MARKET_CRYPTO {
		window = WINDOW_CRYPTO
	}
	end := at.Add(window)
	to := end
	if exit != nil && exit.When().After(to) {
		to = exit.When()
	}
	bars, err := t.history(ctx, sig, at, to.Add(DAY))
	first := sort.Search(len(bars), func(i int) bool { return !bars[i].At.Before(at) })
	if err != nil || first == len(bars) || bars[first].Open <= 0 || bars[first].At.After(end) {
		out.Note = NOTE_NO_HISTORY
		return out
	}
	side := LONG
	if sig.Right == RIGHT_PUT {
		side = SHORT
	}
	entry := bars[first].Open
	move := func(price float64) float64 { return side * (price - entry) / entry * PERCENT }
	out.Scored, out.Entry, out.Open = true, entry, now.Before(end)
	out.PeakPct, out.TroughPct = math.Inf(-1), math.Inf(1)
	for _, b := range bars[first:] {
		if b.At.After(end) {
			break
		}
		best, worst := move(b.High), move(b.Low)
		if side == SHORT {
			best, worst = worst, best
		}
		out.PeakPct, out.TroughPct, out.EndPct = max(out.PeakPct, best), min(out.TroughPct, worst), move(b.Close)
	}
	out.ResultPct, out.Win = out.PeakPct, out.PeakPct >= MIN_WIN_PCT
	if exit == nil {
		return out
	}
	sold := sort.Search(len(bars), func(i int) bool { return !bars[i].At.Before(exit.When()) })
	if sold == len(bars) {
		return out // the exit is newer than the history we have: judged on the window for now
	}
	out.HasExit, out.ExitAt, out.Exit, out.ExitPct = true, exit.When(), bars[sold].Open, move(bars[sold].Open)
	out.ResultPct, out.Win, out.Open = out.ExitPct, out.ExitPct > 0, false
	return out
}

// Source returns one source's record and every entry it called, newest first.
func (t *Tracker) Source(ctx context.Context, id string) (Trust, []Outcome) {
	return t.tally(ctx, id, t.rec.Signals()[id], time.Now())
}

func (t *Tracker) tally(ctx context.Context, id string, posts []Signal, now time.Time) (Trust, []Outcome) {
	trust := Trust{SourceID: id, Posts: len(posts)}
	outcomes := []Outcome{}
	type job struct {
		slot int
		sig  Signal
		exit *Signal
		key  string
	}
	jobs := []job{}
	for i, sig := range posts {
		if sig.Kind != KIND_ENTRY || sig.Symbol == "" {
			continue
		}
		if t.catalog.IsCrypto(sig) {
			sig.Market = MARKET_CRYPTO
		}
		// The source's own exit: its first exit post for this symbol after the entry.
		var exit *Signal
		for j := i + 1; j < len(posts) && exit == nil; j++ {
			later := posts[j]
			if later.Kind == KIND_EXIT && later.Symbol == sig.Symbol && later.Address == sig.Address && later.When().Sub(sig.When()) <= MAX_EXIT_WAIT {
				exit = &later
			}
		}
		key := postIndex(RawPost{SourceID: id, PostID: sig.PostID, Text: sig.Raw})
		if exit != nil {
			key += "|" + exit.PostID
		}
		t.mu.Lock()
		kept, ok := t.done[key]
		t.mu.Unlock()
		outcomes = append(outcomes, kept)
		if ok {
			continue
		}
		jobs = append(jobs, job{len(outcomes) - 1, sig, exit, key})
	}
	// Priced only once the list is complete, so each worker writes to its own fixed slot.
	var wg sync.WaitGroup
	slots := make(chan struct{}, SCORE_WORKERS)
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			o := t.follow(ctx, j.sig, j.exit, now)
			<-slots
			outcomes[j.slot] = o
			if !o.Open && ctx.Err() == nil {
				t.mu.Lock()
				t.done[j.key] = o
				t.mu.Unlock()
			}
		}()
	}
	wg.Wait()

	for _, o := range outcomes {
		trust.Callouts++
		switch {
		case !o.Scored:
			trust.Unpriced++
			continue
		case o.Open:
			trust.Open++
			continue
		}
		trust.Scored++
		trust.AvgResultPct += o.ResultPct
		trust.AvgPeakPct += o.PeakPct
		trust.AvgTroughPct += o.TroughPct
		trust.AvgEndPct += o.EndPct
		if o.HasExit {
			trust.Exits++
		}
		if o.Win {
			trust.Wins++
		}
	}
	if n := float64(trust.Scored); n > 0 {
		trust.AvgResultPct, trust.AvgPeakPct, trust.AvgTroughPct, trust.AvgEndPct = trust.AvgResultPct/n, trust.AvgPeakPct/n, trust.AvgTroughPct/n, trust.AvgEndPct/n
		p := float64(trust.Wins) / n
		trust.WinRatePct = p * PERCENT
		// Wilson score interval, lower bound.
		z2 := TRUST_Z * TRUST_Z
		trust.TrustPct = (p + z2/(2*n) - TRUST_Z*math.Sqrt(p*(1-p)/n+z2/(4*n*n))) / (1 + z2/n) * PERCENT
	}
	sort.SliceStable(outcomes, func(a, b int) bool { return outcomes[a].At.After(outcomes[b].At) })
	return trust, outcomes
}

// Ranking scores every source that has posts on record, most trusted first.
func (t *Tracker) Ranking(ctx context.Context) []Trust {
	now := time.Now()
	out := []Trust{}
	for id, posts := range t.rec.Signals() {
		trust, _ := t.tally(ctx, id, posts, now)
		out = append(out, trust)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].TrustPct != out[b].TrustPct {
			return out[a].TrustPct > out[b].TrustPct
		}
		if out[a].Scored != out[b].Scored {
			return out[a].Scored > out[b].Scored
		}
		return out[a].SourceID < out[b].SourceID
	})
	for i := range out {
		if out[i].Scored > 0 {
			out[i].Rank = i + 1
		}
	}
	return out
}

// Load pulls an X source's public sample of posts into the record and returns how many were new.
// Old posts are only recorded: they never reach the runner, so they cannot open a paper trade.
func (t *Tracker) Load(ctx context.Context, client *http.Client, id string) (int, error) {
	handle, isX := strings.CutPrefix(id, X_SOURCE_PREFIX)
	if !isX {
		return 0, ErrNotLoadable
	}
	tweets, err := FetchTimeline(ctx, client, handle)
	if err != nil {
		return 0, err
	}
	added, now := 0, time.Now()
	for _, tw := range tweets {
		if t.rec.Has(id, tw.ID) {
			continue
		}
		sig := Signal{SourceID: id, PostID: tw.ID, Raw: tw.Text, SeenAt: now, PostedAt: tw.At, URL: tw.URL}
		if err := t.rec.Record(sig); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// Routes are the tracker's HTTP routes, for APIOptions.Mount. allowed guards the one route that
// reaches out to the network on request.
//
//	GET  /api/trust               every source's record, ranked
//	GET  /api/trust/source?id=    one source's record and each entry it called
//	POST /api/trust/load?id=      pull an X source's public sample of posts into the record
func (t *Tracker) Routes(client *http.Client, allowed func(*http.Request) bool) map[string]http.Handler {
	return map[string]http.Handler{
		TRUST_ROUTE: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, t.Ranking(r.Context())) }),
		TRUST_SOURCE_ROUTE: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trust, outcomes := t.Source(r.Context(), r.URL.Query().Get("id"))
			for _, ranked := range t.Ranking(r.Context()) {
				if ranked.SourceID == trust.SourceID {
					trust.Rank = ranked.Rank
				}
			}
			writeJSON(w, struct {
				Trust    Trust     `json:"trust"`
				Outcomes []Outcome `json:"outcomes"`
			}{trust, outcomes})
		}),
		TRUST_LOAD_ROUTE: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowed(r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			added, err := t.Load(r.Context(), client, r.URL.Query().Get("id"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			json.NewEncoder(w).Encode(map[string]int{"added": added})
		}),
	}
}
