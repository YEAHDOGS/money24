package signals

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
)

const (
	CRYPTO_CALLS_ROUTE       = "GET /api/trust/calls"
	CRYPTO_SOURCE_PARAM      = "id"
	CRYPTO_ENTRY_WINDOW      = 24 * time.Hour
	CRYPTO_HOLD_WINDOW       = 7 * 24 * time.Hour
	CRYPTO_FINE_BAR          = 15 * time.Minute
	CRYPTO_COARSE_BAR        = time.Hour
	CRYPTO_FINE_PAGES        = 2
	CRYPTO_PAGE_BARS         = coinbase.CANDLES_MAX - CRYPTO_COARSE_BAR/CRYPTO_FINE_BAR
	CRYPTO_MIN_SAMPLE        = 30
	CRYPTO_STARTING_EQUITY   = 10_000
	CRYPTO_RISK_DOLLARS      = 100
	CRYPTO_OUTCOME_CACHE     = 4096
	CRYPTO_COINBASE_INTERVAL = 200 * time.Millisecond
	CRYPTO_COINBASE_TIMEOUT  = 10 * time.Second

	CRYPTO_PENDING      = "pending"
	CRYPTO_UNAVAILABLE  = "data_unavailable"
	CRYPTO_NEVER_FILLED = "never_filled"
	CRYPTO_STOPPED      = "stopped"
	CRYPTO_TARGETS_HIT  = "targets_hit"
	CRYPTO_TIMED_OUT    = "time_limit"
	CRYPTO_SMALL_SAMPLE = "Too few resolved calls to judge this source. At least 30 are required."
	CRYPTO_ORIGIN_RULE  = "Only the first recorded version is eligible. A call recorded as it was posted replays from its recorded time. A call imported from history replays from its publication time and is counted apart: channels can edit or delete old posts, so imported records can flatter them. Recaps and updates do not count."
	// A post first recorded this long after it was published was imported from history, not seen live.
	CRYPTO_IMPORT_LAG  = 10 * time.Minute
	CRYPTO_PRICE_RULE  = "Coinbase USD candles proxy USDT/USDC and derivative prices. Full candles only. After roughly six days of 15-minute candles, replay switches to hourly candles. Gaps halt scoring."
	CRYPTO_FILL_RULE   = "Entry fills on a touch within 24 hours. Stops take priority on ambiguous bars. Intrabar entries earn no targets on their fill bar. Targets split the position equally and the first target moves the stop to entry. Remaining size exits at the last full close within seven days of the fill bar's open."
	CRYPTO_FEE_RULE    = "Alpaca tier-one taker fees apply to both entry and each exit. R uses the initial entry-to-stop distance. Funding, borrowing, spread, liquidity, and liquidation are outside the model."
	CRYPTO_STATS_RULE  = "Win rate and average R use resolved fills. Expectancy includes zero-R unfilled calls. Pending and unavailable calls are excluded from those rates. Drawdown books each call's net P&L at its final exit in fixed dollar risk units, without capital constraints on overlapping positions."
	CRYPTO_BROKER_RULE = "Followable means long, spot and a confirmed Coinbase USD product. Check Alpaca pair availability before execution."

	CRYPTO_NOTE_UNLISTED       = "Coinbase USD product not found"
	CRYPTO_NOTE_REQUEST        = "Coinbase candle request failed"
	CRYPTO_NOTE_GAP            = "missing or invalid candle in the replay window"
	CRYPTO_NOTE_NO_TIME        = "first-recorded timestamp is missing"
	CRYPTO_NOTE_SHORT          = "short positions are unavailable in Alpaca spot"
	CRYPTO_NOTE_MARGIN         = "leverage and derivatives are unavailable in Alpaca spot"
	CRYPTO_NOTE_UNKNOWN        = "Coinbase USD listing could not be verified"
	CRYPTO_HTTP_UNAUTHORIZED   = "unauthorized"
	CRYPTO_HTTP_MISSING_SOURCE = "source id is required"
	CRYPTO_HTTP_BUSY           = "a crypto backtest is already running"
	CRYPTO_HTTP_REPLAY         = "could not replay recorded posts"
)

type cryptoCandleHistory func(context.Context, string, time.Duration, time.Time, time.Time) ([]coinbase.Candle, error)

// CryptoCallOutcome distinguishes missed entries from missing data and incomplete windows.
type CryptoCallOutcome struct {
	Call           CryptoCall `json:"call"`
	Status         string     `json:"status"`
	Note           string     `json:"note,omitempty"`
	Listed         *bool      `json:"coinbase_usd_listed"`
	Followable     bool       `json:"followable_on_alpaca"`
	FollowableNote string     `json:"followable_note,omitempty"`
	Entry          float64    `json:"entry,omitempty"`
	EntryAt        time.Time  `json:"entry_bar_at,omitzero"`
	ResolvedAt     time.Time  `json:"resolved_at,omitzero"`
	TargetsHit     int        `json:"targets_hit"`
	R              *float64   `json:"r"`
	Hourly         bool       `json:"used_hourly_candles"`
}

type CryptoCallStats struct {
	Calls          int     `json:"calls"`
	Filled         int     `json:"filled"`
	Resolved       int     `json:"resolved"`
	ResolvedFills  int     `json:"resolved_fills"`
	Wins           int     `json:"wins"`
	Pending        int     `json:"pending"`
	Unavailable    int     `json:"unavailable"`
	WinRatePct     float64 `json:"win_rate_pct"`
	AverageR       float64 `json:"average_r"`
	ExpectancyR    float64 `json:"expectancy_r_per_call"`
	WorstLossRun   int     `json:"worst_loss_run"`
	MaxDrawdown    float64 `json:"max_drawdown_dollars"`
	MaxDrawdownPct float64 `json:"max_drawdown_pct"`
	TooFewToJudge  bool    `json:"too_few_to_judge"`
	SampleNote     string  `json:"sample_note,omitempty"`
}

type CryptoCallRecord struct {
	SourceID       string              `json:"source_id"`
	All            CryptoCallStats     `json:"all"`
	Followable     CryptoCallStats     `json:"followable_on_alpaca"`
	Live           CryptoCallStats     `json:"live_recorded"` // only calls recorded as they were posted: the honest test
	Outcomes       []CryptoCallOutcome `json:"outcomes"`
	FeePctPerSide  float64             `json:"taker_fee_pct_per_side"`
	StartingEquity float64             `json:"starting_equity_dollars"`
	RiskDollars    float64             `json:"risk_dollars_per_call"`
	MinSample      int                 `json:"minimum_resolved_calls"`
	Rules          []string            `json:"rules"`
}

// CryptoBacktester reads immutable post versions and scores them on demand.
type CryptoBacktester struct {
	path    string
	feePct  float64
	history cryptoCandleHistory
	done    map[RawPost]CryptoCallOutcome
	doneMu  sync.RWMutex
	busy    chan struct{}
}

// NewCryptoBacktester uses the broker fee supplied by the runner, avoiding a package cycle.
func NewCryptoBacktester(path string, takerFeePct float64) *CryptoBacktester {
	client := &http.Client{Timeout: CRYPTO_COINBASE_TIMEOUT}
	var next time.Time
	return &CryptoBacktester{
		path: path, feePct: takerFeePct, done: make(map[RawPost]CryptoCallOutcome), busy: make(chan struct{}, 1),
		history: func(ctx context.Context, symbol string, bar time.Duration, from, to time.Time) ([]coinbase.Candle, error) {
			if delay := time.Until(next); delay > 0 {
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-timer.C:
				}
			}
			next = time.Now().Add(CRYPTO_COINBASE_INTERVAL)
			return coinbase.Candles(ctx, client, symbol, bar, from, to)
		},
	}
}

// Routes mounts the read-only replay behind the same access check as historical source loading.
func (b *CryptoBacktester) Routes(allowed func(*http.Request) bool) map[string]http.Handler {
	return map[string]http.Handler{
		CRYPTO_CALLS_ROUTE: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowed(r) {
				http.Error(w, CRYPTO_HTTP_UNAUTHORIZED, http.StatusUnauthorized)
				return
			}
			id := strings.TrimSpace(r.URL.Query().Get(CRYPTO_SOURCE_PARAM))
			if id == "" {
				http.Error(w, CRYPTO_HTTP_MISSING_SOURCE, http.StatusBadRequest)
				return
			}
			select {
			case b.busy <- struct{}{}:
				defer func() { <-b.busy }()
			default:
				http.Error(w, CRYPTO_HTTP_BUSY, http.StatusTooManyRequests)
				return
			}
			record, err := b.source(r.Context(), id, time.Now())
			if err != nil {
				http.Error(w, CRYPTO_HTTP_REPLAY, http.StatusInternalServerError)
				return
			}
			writeJSON(w, record)
		}),
	}
}

func (b *CryptoBacktester) source(ctx context.Context, id string, now time.Time) (CryptoCallRecord, error) {
	record := CryptoCallRecord{SourceID: id, Outcomes: []CryptoCallOutcome{}, FeePctPerSide: b.feePct,
		StartingEquity: CRYPTO_STARTING_EQUITY, RiskDollars: CRYPTO_RISK_DOLLARS, MinSample: CRYPTO_MIN_SAMPLE,
		Rules: []string{CRYPTO_ORIGIN_RULE, CRYPTO_PRICE_RULE, CRYPTO_FILL_RULE, CRYPTO_FEE_RULE, CRYPTO_STATS_RULE, CRYPTO_BROKER_RULE}}
	seen := make(map[string]struct{})
	type recordedCall struct {
		post RawPost
		call CryptoCall
	}
	var calls []recordedCall
	err := Replay(b.path, func(post RawPost, _ Signal) {
		if !strings.EqualFold(post.SourceID, id) || ctx.Err() != nil {
			return
		}
		key := postIndex(post)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{} // Mark rejected first versions before looking at their text.
		if call, ok := ParseCryptoCall(post); ok {
			if !call.PostedAt.IsZero() && call.SeenAt.Sub(call.PostedAt) > CRYPTO_IMPORT_LAG {
				call.Imported, call.SeenAt = true, call.PostedAt
			}
			calls = append(calls, recordedCall{post, call})
		}
	})
	if err != nil {
		return record, err
	}
	for _, c := range calls {
		if ctx.Err() != nil {
			return record, ctx.Err()
		}
		outcome, exists := b.done[c.post]
		if !exists {
			outcome = b.backtest(ctx, c.call, now)
			if outcome.R != nil {
				b.doneMu.Lock()
				if len(b.done) >= CRYPTO_OUTCOME_CACHE {
					clear(b.done)
				}
				b.done[c.post] = outcome
				b.doneMu.Unlock()
			}
		}
		record.Outcomes = append(record.Outcomes, outcome)
	}
	record.All = cryptoCallStats(record.Outcomes, false)
	record.Followable = cryptoCallStats(record.Outcomes, true)
	live := []CryptoCallOutcome{}
	for _, o := range record.Outcomes {
		if !o.Call.Imported {
			live = append(live, o)
		}
	}
	record.Live = cryptoCallStats(live, false)
	slices.SortStableFunc(record.Outcomes, func(a, b CryptoCallOutcome) int {
		return b.Call.SeenAt.Compare(a.Call.SeenAt)
	})
	return record, nil
}

type cryptoPosition struct {
	stop      float64
	openRisk  float64
	remaining int
	pnl       float64
}

func (out *CryptoCallOutcome) setFollowable() {
	switch {
	case out.Call.Direction == CRYPTO_SHORT:
		out.FollowableNote = CRYPTO_NOTE_SHORT
	case out.Call.Derivative || out.Call.Leverage > 1:
		out.FollowableNote = CRYPTO_NOTE_MARGIN
	case out.Listed == nil:
		out.FollowableNote = CRYPTO_NOTE_UNKNOWN
	case !*out.Listed:
		out.FollowableNote = CRYPTO_NOTE_UNLISTED
	default:
		out.Followable = true
	}
}

func (b *CryptoBacktester) backtest(ctx context.Context, call CryptoCall, now time.Time) (out CryptoCallOutcome) {
	out = CryptoCallOutcome{Call: call, Status: CRYPTO_PENDING}
	defer out.setFollowable()
	if call.SeenAt.IsZero() {
		return unavailableCryptoCall(out, CRYPTO_NOTE_NO_TIME)
	}
	from := call.SeenAt.Truncate(CRYPTO_FINE_BAR)
	if from.Before(call.SeenAt) {
		from = from.Add(CRYPTO_FINE_BAR)
	}
	entryDeadline := call.SeenAt.Add(CRYPTO_ENTRY_WINDOW)
	deadline := entryDeadline
	fineEnd := from.Add(CRYPTO_FINE_PAGES * CRYPTO_PAGE_BARS * CRYPTO_FINE_BAR).Truncate(CRYPTO_COARSE_BAR)
	position := cryptoPosition{stop: call.Stop, remaining: len(call.Targets)}
	var last coinbase.Candle
	var lastEnd time.Time
	fee := b.feePct / PERCENT
	for {
		step := CRYPTO_FINE_BAR
		if !from.Before(fineEnd) {
			step = CRYPTO_COARSE_BAR
		}
		until := deadline.Truncate(step)
		if !from.Before(until) {
			if deadline.After(now) {
				return out
			}
			if out.Entry == 0 {
				zero := 0.0
				out.Status, out.R, out.ResolvedAt = CRYPTO_NEVER_FILLED, &zero, entryDeadline
				return out
			}
			position.exit(out, last.C, position.remaining, fee)
			return position.finish(out, CRYPTO_TIMED_OUT, lastEnd)
		}
		to := minCryptoTime(until, now.Truncate(step), from.Add(CRYPTO_PAGE_BARS*step))
		if step == CRYPTO_FINE_BAR {
			to = minCryptoTime(to, fineEnd)
		}
		if !from.Before(to) {
			return out
		}
		bars, err := b.history(ctx, call.Base, step, from, to)
		listed := !errors.Is(err, coinbase.ErrNotFound)
		if !listed {
			out.Listed = &listed
			return unavailableCryptoCall(out, CRYPTO_NOTE_UNLISTED)
		}
		if err != nil {
			return unavailableCryptoCall(out, CRYPTO_NOTE_REQUEST)
		}
		out.Listed = &listed
		out.Hourly = out.Hourly || step == CRYPTO_COARSE_BAR
		for _, bar := range bars {
			at := time.Unix(bar.T, 0).UTC()
			if at.Before(from) || !at.Before(to) {
				continue // Coinbase can return bars outside the requested interval.
			}
			if !at.Equal(from) || !validCryptoBar(bar) {
				return unavailableCryptoCall(out, CRYPTO_NOTE_GAP)
			}
			from, last, lastEnd = at.Add(step), bar, at.Add(step)
			fillAtOpen := true
			if out.Entry == 0 {
				out.Entry, fillAtOpen = cryptoEntryFill(call, bar)
				if out.Entry == 0 {
					continue
				}
				out.EntryAt = at
				position.openRisk = max(out.Entry-call.Stop, call.Stop-out.Entry)
				// The entry time within a bar is unknown. Start the holding clock at its open.
				deadline = at.Add(CRYPTO_HOLD_WINDOW)
			}
			if status := position.bar(&out, bar, fillAtOpen, fee); status != "" {
				return position.finish(out, status, lastEnd)
			}
		}
		if from.Before(to) {
			return unavailableCryptoCall(out, CRYPTO_NOTE_GAP)
		}
	}
}

func minCryptoTime(first time.Time, others ...time.Time) time.Time {
	for _, t := range others {
		if t.Before(first) {
			first = t
		}
	}
	return first
}

func unavailableCryptoCall(out CryptoCallOutcome, note string) CryptoCallOutcome {
	out.Status, out.Note = CRYPTO_UNAVAILABLE, note
	return out
}

func validCryptoBar(b coinbase.Candle) bool {
	return b.L >= CRYPTO_MIN_PRICE && b.H <= CRYPTO_MAX_PRICE && b.L <= min(b.O, b.C) && b.H >= max(b.O, b.C)
}

func cryptoEntryFill(call CryptoCall, bar coinbase.Candle) (float64, bool) {
	if bar.H < call.EntryLow || bar.L > call.EntryHigh {
		return 0, false
	}
	return min(call.EntryHigh, max(call.EntryLow, bar.O)), bar.O >= call.EntryLow && bar.O <= call.EntryHigh
}

func (p *cryptoPosition) bar(out *CryptoCallOutcome, bar coinbase.Candle, openPosition bool, fee float64) string {
	long := out.Call.Direction == CRYPTO_LONG
	stopHit := long && bar.L <= p.stop || !long && bar.H >= p.stop
	if stopHit {
		price := p.stop
		if openPosition && (long && bar.O < price || !long && bar.O > price) {
			price = bar.O // A gap can realize more than the initial risk.
		}
		p.exit(*out, price, p.remaining, fee)
		return CRYPTO_STOPPED
	}
	if !openPosition {
		return ""
	}
	for p.remaining > 0 {
		target := out.Call.Targets[out.TargetsHit]
		if long && bar.H < target || !long && bar.L > target {
			break
		}
		p.exit(*out, target, 1, fee)
		out.TargetsHit++
		if p.remaining == 0 {
			return CRYPTO_TARGETS_HIT
		}
		p.stop = out.Entry
		next := out.Call.Targets[out.TargetsHit]
		// At the open, gaps through targets have known ordering. Use the posted target price.
		nextAtOpen := long && bar.O >= next || !long && bar.O <= next
		if !nextAtOpen && (long && bar.L <= p.stop || !long && bar.H >= p.stop) {
			p.exit(*out, p.stop, p.remaining, fee)
			return CRYPTO_STOPPED
		}
	}
	return ""
}

func (p *cryptoPosition) exit(out CryptoCallOutcome, price float64, shares int, fee float64) {
	move := price - out.Entry
	if out.Call.Direction == CRYPTO_SHORT {
		move = -move
	}
	p.pnl += (move - fee*(out.Entry+price)) * float64(shares) / float64(len(out.Call.Targets))
	p.remaining -= shares
}

func (p *cryptoPosition) finish(out CryptoCallOutcome, status string, at time.Time) CryptoCallOutcome {
	r := p.pnl / p.openRisk
	out.Status, out.ResolvedAt, out.R = status, at, &r
	return out
}

func cryptoCallStats(outcomes []CryptoCallOutcome, followable bool) CryptoCallStats {
	var stats CryptoCallStats
	var closed []CryptoCallOutcome
	var sum float64
	for _, o := range outcomes {
		if followable && !o.Followable {
			continue
		}
		stats.Calls++
		if o.Entry > 0 {
			stats.Filled++
		}
		switch {
		case o.Status == CRYPTO_PENDING:
			stats.Pending++
		case o.R == nil:
			stats.Unavailable++
		default:
			stats.Resolved++
			if o.Entry > 0 {
				stats.ResolvedFills++
				closed = append(closed, o)
				sum += *o.R
			}
		}
	}
	slices.SortStableFunc(closed, func(a, b CryptoCallOutcome) int {
		if c := a.ResolvedAt.Compare(b.ResolvedAt); c != 0 {
			return c
		}
		return a.Call.SeenAt.Compare(b.Call.SeenAt)
	})
	equity, peak := float64(CRYPTO_STARTING_EQUITY), float64(CRYPTO_STARTING_EQUITY)
	losses := 0
	for _, o := range closed {
		if *o.R > 0 {
			stats.Wins++
		}
		if *o.R < 0 {
			losses++
			stats.WorstLossRun = max(stats.WorstLossRun, losses)
		} else {
			losses = 0
		}
		equity += CRYPTO_RISK_DOLLARS * *o.R
		peak = max(peak, equity)
		stats.MaxDrawdown = max(stats.MaxDrawdown, peak-equity)
		stats.MaxDrawdownPct = max(stats.MaxDrawdownPct, (peak-equity)/peak*PERCENT)
	}
	if stats.ResolvedFills > 0 {
		stats.WinRatePct = PERCENT * float64(stats.Wins) / float64(stats.ResolvedFills)
		stats.AverageR = sum / float64(stats.ResolvedFills)
	}
	if stats.Resolved > 0 {
		stats.ExpectancyR = sum / float64(stats.Resolved)
	}
	stats.TooFewToJudge = stats.Resolved < CRYPTO_MIN_SAMPLE
	if stats.TooFewToJudge {
		stats.SampleNote = CRYPTO_SMALL_SAMPLE
	}
	return stats
}
