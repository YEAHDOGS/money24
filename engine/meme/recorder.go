package meme

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/dexscreener"
)

const (
	SCHEDULER_TICK = time.Second
	MARKET_GAP     = 2 * time.Second
	SAFETY_GAP     = 5 * time.Second
	STREAM_BUFFER  = 512
)

// Status describes the recorder's UTC-day counters and current feed state.
type Status struct {
	Running        bool      `json:"running"`
	Connected      bool      `json:"connected"`
	Day            string    `json:"day"`
	BytesToday     int64     `json:"bytes_today"`
	SeenToday      int       `json:"seen_today"`
	TrackedNow     int       `json:"tracked_now"`
	Graduated      int       `json:"graduated"`
	SnapshotsToday int       `json:"snapshots_today"`
	PricedToday    int       `json:"priced_today"`
	SafetyToday    int       `json:"safety_today"`
	CapacityDrops  int       `json:"capacity_drops"`
	UnlistedDrops  int       `json:"unlisted_drops"`
	LastError      string    `json:"last_error,omitempty"`
	LastErrorAt    time.Time `json:"last_error_at,omitzero"`
	Dir            string    `json:"dir"`
}

// Recorder collects public market observations into Dir until its context ends.
type Recorder struct {
	Dir string
	// OnRecord receives each new observation after it is written, in file order.
	// Set it before Run. The callback runs on the recorder loop and must not mutate the record.
	OnRecord func(Record, Position)

	client    *http.Client
	pumpURL   string
	marketURL string
	rugURL    string
	mu        sync.Mutex
	state     Status
}

type marketResult struct {
	start  time.Time
	end    time.Time
	tokens []*trackedToken
	pairs  map[string]Market
	err    error
}

type safetyResult struct {
	start  time.Time
	end    time.Time
	token  *trackedToken
	report *Safety
	err    error
}

type recording struct {
	recorder *Recorder
	writer   recordWriter
	tracking tracking
	status   Status
}

// Status returns a thread-safe copy of the desk counters.
func (r *Recorder) Status() Status {
	r.mu.Lock()
	status := r.state
	r.mu.Unlock()
	status.Dir = r.Dir
	status.setDay(time.Now())
	status.BytesToday = dayBytes(r.Dir, status.Day)
	return status
}

func (s *Status) setDay(now time.Time) {
	day := now.UTC().Format(DAY_LAYOUT)
	if s.Day == day {
		return
	}
	s.Day = day
	s.SeenToday, s.Graduated, s.SnapshotsToday, s.PricedToday, s.SafetyToday = 0, 0, 0, 0, 0
	s.CapacityDrops, s.UnlistedDrops = 0, 0
}

func (run *recording) publish() {
	run.status.setDay(time.Now())
	run.status.TrackedNow = len(run.tracking.tokens)
	run.recorder.mu.Lock()
	run.recorder.state = run.status
	run.recorder.mu.Unlock()
}

func (run *recording) report(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	run.status.LastError, run.status.LastErrorAt = err.Error(), time.Now().UTC()
}

func (run *recording) apply(row Record) {
	switch row.Kind {
	case KIND_LAUNCH, KIND_GRADUATION:
		run.tracking.prune(row.At)
		run.tracking.applyEvent(row)
	case KIND_SNAPSHOT:
		run.tracking.applySample(row, row.RequestedAt)
	case KIND_SAFETY:
		if token := run.tracking.tokens[row.Mint]; token != nil {
			token.safetyAttempts++
			token.safetyDone = row.Safety != nil
			token.nextSafety = row.At.Add(SAFETY_RETRY)
		}
	}
	if row.At.Format(DAY_LAYOUT) != run.status.Day {
		return
	}
	switch row.Kind {
	case KIND_LAUNCH:
		run.status.SeenToday++
	case KIND_GRADUATION:
		run.status.Graduated++
	case KIND_SNAPSHOT:
		run.status.SnapshotsToday++
		if row.Market != nil && row.Market.PriceUSD != nil {
			run.status.PricedToday++
		}
	case KIND_SAFETY:
		if row.Safety != nil {
			run.status.SafetyToday++
		}
	}
	if row.Tracking == TRACK_CAPACITY || row.Evicted != "" {
		run.status.CapacityDrops++
	}
	if row.Tracking == TRACK_UNLISTED {
		run.status.UnlistedDrops++
	}
}

func (run *recording) save(row Record) {
	now := time.Now().UTC()
	run.status.setDay(now)
	day := row.At.UTC().Format(DAY_LAYOUT)
	run.report(run.writer.sealExpired(now, day))
	pos, err := run.writer.write(row, now, day)
	if err != nil {
		run.report(fmt.Errorf("meme dataset: %w", err))
		return
	}
	run.apply(row)
	if observe := run.recorder.OnRecord; observe != nil {
		observe(row, pos)
	}
}

func (run *recording) stream(update streamUpdate) {
	if update.connected != run.status.Connected || (!update.connected && update.err != nil) {
		kind := KIND_CONNECTED
		if !update.connected {
			kind = KIND_DISCONNECTED
		}
		row := Record{At: time.Now().UTC(), Kind: kind, Source: PUMP_SOURCE}
		if update.err != nil {
			row.Error = update.err.Error()
		}
		run.save(row)
		run.status.Connected = update.connected
	}
	run.report(update.err)
	row := update.row
	if row.Kind == "" {
		return
	}
	if _, ok := run.tracking.recent[eventKey{mint: row.Mint, kind: row.Kind}]; ok {
		return
	}
	run.tracking.prune(row.At)
	run.tracking.planEvent(&row)
	run.save(row)
}

func (run *recording) market(result marketResult) {
	if result.err != nil {
		run.report(fmt.Errorf("DEX Screener: %w", result.err))
		return
	}
	for _, token := range result.tokens {
		if run.tracking.tokens[token.mint] != token {
			continue
		}
		row := Record{At: result.end, RequestedAt: result.start, Kind: KIND_SNAPSHOT, Source: DEX_SOURCE, Mint: token.mint}
		if pair, found := result.pairs[token.mint]; found {
			row.Market = &pair
		} else if token.graduated.IsZero() && result.end.Sub(token.start) >= UNLISTED_WINDOW &&
			result.end.Sub(token.lastActivity) >= UNLISTED_WINDOW {
			row.Tracking = TRACK_UNLISTED
		}
		run.save(row)
	}
}

func (run *recording) safety(result safetyResult) {
	if errors.Is(result.err, context.Canceled) || run.tracking.tokens[result.token.mint] != result.token {
		return
	}
	row := Record{At: result.end, RequestedAt: result.start, Kind: KIND_SAFETY, Source: RUG_SOURCE,
		Mint: result.token.mint, Safety: result.report}
	if result.err != nil {
		run.report(fmt.Errorf("RugCheck: %w", result.err))
		row.Error = result.err.Error()
	}
	run.save(row)
}

// Run records feed events and schedules market and safety lookups.
func (r *Recorder) Run(ctx context.Context) {
	r.mu.Lock()
	if r.state.Running {
		r.mu.Unlock()
		return
	}
	r.state.Running = true
	r.mu.Unlock()
	run := recording{
		recorder: r, writer: recordWriter{dir: r.Dir},
		tracking: tracking{tokens: make(map[string]*trackedToken), recent: make(map[eventKey]struct{})},
		status:   Status{Running: true},
	}
	run.status.setDay(time.Now())
	run.loop(ctx)
	if run.status.Connected {
		run.stream(streamUpdate{})
	}
	run.report(run.writer.seal())
	run.status.Running, run.status.Connected = false, false
	run.publish()
}

func (run *recording) loop(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	r := run.recorder
	client := r.client
	if client == nil {
		client = http.DefaultClient
	}
	pumpURL, marketURL, rugURL := r.pumpURL, r.marketURL, r.rugURL
	if pumpURL == "" {
		pumpURL = PUMP_URL
	}
	if marketURL == "" {
		marketURL = dexscreener.TOKEN_URL
	}
	if rugURL == "" {
		rugURL = RUG_URL
	}
	updates := make(chan streamUpdate, STREAM_BUFFER)
	markets := make(chan marketResult, 1)
	safeties := make(chan safetyResult, 1)
	var workers sync.WaitGroup
	workers.Go(func() { pumpLoop(ctx, client, pumpURL, updates) })
	defer func() {
		cancel()
		workers.Wait()
		close(updates)
		for update := range updates {
			run.stream(update)
		}
		select {
		case result := <-markets:
			run.market(result)
		default:
		}
		select {
		case result := <-safeties:
			run.safety(result)
		default:
		}
	}()
	run.report(Compact(run.writer.dir))
	run.report(run.restore(ctx, time.Now().UTC()))
	run.publish()
	ticker := time.NewTicker(SCHEDULER_TICK)
	defer ticker.Stop()
	var nextMarket, nextSafety time.Time
	marketBusy, safetyBusy := false, false
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case update := <-updates:
			run.stream(update)
		case result := <-markets:
			marketBusy = false
			run.market(result)
			if result.err != nil {
				nextMarket = result.end.Add(retryDelay(result.err, result.end))
			}
		case result := <-safeties:
			safetyBusy = false
			run.safety(result)
			var response *httpFailure
			if result.err != nil && (!errors.As(result.err, &response) ||
				response.code == http.StatusTooManyRequests || response.code >= http.StatusInternalServerError) {
				nextSafety = result.end.Add(retryDelay(result.err, result.end))
			}
		case <-ticker.C:
			now := time.Now().UTC()
			run.report(run.writer.sealExpired(now, now.Format(DAY_LAYOUT)))
			run.tracking.prune(now)
			if !marketBusy && !now.Before(nextMarket) {
				tokens := run.tracking.marketDue(now)
				if len(tokens) > 0 {
					marketBusy, nextMarket = true, now.Add(MARKET_GAP)
					workers.Go(func() {
						mints := make([]string, 0, len(tokens))
						for _, token := range tokens {
							mints = append(mints, token.mint)
						}
						pairs, err := fetchMarket(ctx, client, marketURL, mints)
						markets <- marketResult{start: now, end: time.Now().UTC(), tokens: tokens, pairs: pairs, err: err}
					})
				}
			}
			if !safetyBusy && !now.Before(nextSafety) {
				if token := run.tracking.safetyDue(now); token != nil {
					safetyBusy, nextSafety = true, now.Add(SAFETY_GAP)
					workers.Go(func() {
						safety, err := fetchSafety(ctx, client, rugURL, token.mint)
						safeties <- safetyResult{start: now, end: time.Now().UTC(), token: token, report: safety, err: err}
					})
				}
			}
		}
		run.publish()
	}
}
