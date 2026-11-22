package predict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DEFAULT_EVERY_MINUTES  = 5
	DEFAULT_LEFT_MINUTES   = 5    // not in the last few minutes, when there is no time to fill
	DEFAULT_MIN_PRICE      = 0.85 // the favorite's ask, dollars a contract
	DEFAULT_MAX_PRICE      = 0.95
	DEFAULT_MAX_SPREAD     = 0.03
	DEFAULT_CAPITAL_USD    = 1000.0
	DEFAULT_DAILY_LOSS_USD = 150.0

	TICK          = time.Minute // how often the desk checks which strategies are due
	SETTLE_EVERY  = 5 * time.Minute
	PASS_TIMEOUT  = 4 * time.Minute
	CLOSED_SHOWN  = 20
	RULEBOOK_MAX  = 1 << 20
	ENV_PREDICT   = "M24D_PREDICT" // "off" turns the desk off
	SWITCH_OFF    = "off"
	LOG_PREFIX    = "predict: "
	VENUE         = "Kalshi"
	REASONS_JOIN  = " · "
	NOTE_FILL     = "%d levels of Kalshi's book, best ask %.2f"
	ROUTE_STATUS  = "GET /api/predict"
	ROUTE_RULES   = "PUT /api/predict/strategies"
	ERR_NO_WRITE  = "open the desk on this machine, or send the webhook secret"
	STATE_LOOKING = "looking"
	STATE_OFF     = "off"
	STATE_WATCH   = "watching" // above the desk's mode: records, buys nothing
	STATE_FULL    = "full"
	STATE_PAUSED  = "paused"
	STATE_ERROR   = "error"
	WHY_OFF       = "Turned off in the rulebook."
	WHY_DESK_OFF  = "Turned off with M24D_PREDICT=off."
	WHY_WATCH     = "Risk %s is above the desk's mode (%s): it follows markets and keeps its record, but buys nothing."
	WHY_FULL      = "Today's stake of $%.0f is used. New positions resume tomorrow, New York time."
	WHY_PAUSED    = "The desk lost $%.0f on settled positions today. New positions resume tomorrow, New York time."
)

// Pass is what a strategy's last look found.
type Pass struct {
	Markets    int `json:"markets"`    // markets in its scope
	Candidates int `json:"candidates"` // of those, ones that met its rules
	Watched    int `json:"watched"`    // newly followed
	Bought     int `json:"bought"`
}

// StrategyStatus is one strategy as the page shows it.
type StrategyStatus struct {
	Config    Config    `json:"config"`
	State     string    `json:"state"`
	Why       string    `json:"why,omitempty"`
	LastAt    time.Time `json:"last_at,omitzero"`
	NextAt    time.Time `json:"next_at,omitzero"`
	LastError string    `json:"last_error,omitempty"`
	Pass      Pass      `json:"pass"`
	Record    Record    `json:"record"`
	Account   Account   `json:"account"`
}

// Status is the desk as the page shows it.
type Status struct {
	Venue         string           `json:"venue"`
	KeysSet       bool             `json:"keys_set"` // Kalshi keys in .env, for demo orders later; the paper book needs none
	On            bool             `json:"on"`
	Mode          string           `json:"mode"`
	CapitalUSD    float64          `json:"capital_usd"`
	DailyLossUSD  float64          `json:"daily_loss_usd"`
	LostTodayUSD  float64          `json:"lost_today_usd"`
	RulebookFile  string           `json:"rulebook_file"`
	RulebookError string           `json:"rulebook_error,omitempty"`
	SettledAt     time.Time        `json:"settled_at,omitzero"`
	Account       Account          `json:"account"`
	Strategies    []StrategyStatus `json:"strategies"`
	Kinds         []Kind           `json:"kinds"`
	Open          []Position       `json:"open"`
	Closed        []Position       `json:"closed"`
}

type runState struct {
	lastAt, nextAt time.Time
	lastError      string
	pass           Pass
	state, why     string
}

// Desk runs every strategy in the rulebook against Kalshi, around the clock, on one paper book.
type Desk struct {
	Book         *Book
	Kalshi       *Kalshi
	Env          *Env
	RulebookPath string
	MayWrite     func(*http.Request) bool

	mu          sync.Mutex
	rulebook    Rulebook
	strategies  []running
	loadedMod   time.Time
	rulebookErr string
	runs        map[string]*runState
	settledAt   time.Time
	off         bool
}

// candidate is a market a strategy's rules take, and on which side.
type candidate struct {
	market  Market
	side    string
	ask     float64
	spread  float64
	prob    float64 // the chance the side wins; 0 when the strategy gives none
	edge    float64
	reasons []string
}

// Reload reads the rulebook when its file changed. A broken file keeps the last good rulebook running.
func (d *Desk) Reload() error {
	d.mu.Lock()
	loaded, ready := d.loadedMod, d.strategies != nil
	d.mu.Unlock()
	info, err := os.Stat(d.RulebookPath)
	if err == nil && !info.ModTime().After(loaded) && ready {
		return nil
	}
	rb, err := LoadRulebook(d.RulebookPath)
	if err == nil {
		err = d.apply(rb)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rulebookErr = ""
	if err != nil {
		d.rulebookErr = err.Error()
	}
	if info, statErr := os.Stat(d.RulebookPath); statErr == nil {
		d.loadedMod = info.ModTime()
	}
	return err
}

// apply builds a rulebook's strategies and swaps them in.
func (d *Desk) apply(rb Rulebook) error {
	built, err := rb.Build()
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rulebook, d.strategies = rb, built
	d.Book.SetCapital(rb.CapitalUSD)
	if d.runs == nil {
		d.runs = map[string]*runState{}
	}
	for _, r := range built {
		if d.runs[r.cfg.ID] == nil {
			d.runs[r.cfg.ID] = &runState{}
		}
	}
	return nil
}

// Run checks every minute which strategies are due, runs them, and settles what finished.
func (d *Desk) Run(ctx context.Context) {
	if strings.EqualFold(os.Getenv(ENV_PREDICT), SWITCH_OFF) {
		d.mu.Lock()
		d.off = true
		d.mu.Unlock()
		return
	}
	for {
		if err := d.Reload(); err != nil {
			log.Printf("%srulebook: %v", LOG_PREFIX, err)
		}
		pass, cancel := context.WithTimeout(ctx, PASS_TIMEOUT)
		d.tick(pass, time.Now())
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(TICK):
		}
	}
}

// tick runs the strategies that are due, then settles.
func (d *Desk) tick(ctx context.Context, now time.Time) {
	d.mu.Lock()
	due := []running{}
	for _, r := range d.strategies {
		st := d.runs[r.cfg.ID]
		if !r.cfg.On {
			st.state, st.why = STATE_OFF, WHY_OFF
			continue
		}
		if !now.Before(st.nextAt) {
			due = append(due, r)
		}
	}
	d.mu.Unlock()

	cache := &marketCache{kalshi: d.Kalshi, series: map[string][]Market{}}
	for _, r := range due {
		markets, err := cache.scope(ctx, r.strategy.Scope(), now)
		pass, state, why, runErr := d.runOne(ctx, r, markets, now)
		err = errors.Join(err, runErr)
		d.mu.Lock()
		st := d.runs[r.cfg.ID]
		st.lastAt, st.nextAt, st.pass, st.state, st.why, st.lastError = now.UTC(), now.Add(time.Duration(r.cfg.EveryMinutes)*time.Minute).UTC(), pass, state, why, ""
		if err != nil {
			st.lastError = err.Error()
			log.Printf("%s%s: %v", LOG_PREFIX, r.cfg.ID, err)
		}
		d.mu.Unlock()
		if pass.Bought > 0 {
			log.Printf("%s%s: %d markets, %d candidates, %d new, %d bought", LOG_PREFIX, r.cfg.ID, pass.Markets, pass.Candidates, pass.Watched, pass.Bought)
		}
	}
	d.mu.Lock()
	settleDue := now.Sub(d.settledAt) >= SETTLE_EVERY
	d.mu.Unlock()
	if settleDue {
		if n, err := d.settle(ctx, now); err != nil {
			log.Printf("%ssettle: %v", LOG_PREFIX, err)
		} else if n > 0 {
			log.Printf("%s%d settled", LOG_PREFIX, n)
		}
		d.mu.Lock()
		d.settledAt = now
		d.mu.Unlock()
	}
}

// marketCache fetches each scope's markets once a tick, however many strategies share it.
type marketCache struct {
	kalshi    *Kalshi
	closing   []Market
	closingTo time.Time
	series    map[string][]Market
}

func (c *marketCache) scope(ctx context.Context, s Scope, now time.Time) ([]Market, error) {
	if len(s.Series) == 0 {
		to := now.Add(time.Duration(s.WithinHours * float64(time.Hour)))
		if c.closing == nil || to.After(c.closingTo) {
			markets, err := c.kalshi.Closing(ctx, now, to)
			if err != nil && len(markets) == 0 {
				return nil, fmt.Errorf("markets: %w", err)
			}
			c.closing, c.closingTo = markets, to
		}
		out := []Market{}
		for _, m := range c.closing {
			if !m.CloseAt.After(to) {
				out = append(out, m)
			}
		}
		return out, nil
	}
	out, errs := []Market{}, []error{}
	series := append([]string{}, s.Series...)
	sort.Strings(series)
	for _, name := range series {
		markets, ok := c.series[name]
		if !ok {
			var err error
			markets, err = c.kalshi.InSeries(ctx, name)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
			c.series[name] = markets
			if err := pause(ctx); err != nil {
				return out, err
			}
		}
		out = append(out, markets...)
	}
	return out, errors.Join(errs...)
}

// runOne is one strategy's pass: its opinion on every market, its rules on every opinion, a watch line
// for every new candidate, and buys within its limits and the desk's.
func (d *Desk) runOne(ctx context.Context, r running, markets []Market, now time.Time) (Pass, string, string, error) {
	pass := Pass{Markets: len(markets)}
	prepErr := r.strategy.Prepare(ctx, d.Env, markets, now)
	picks := []candidate{}
	for _, m := range markets {
		if c, ok := consider(r, m, now); ok {
			picks = append(picks, c)
		}
	}
	// The best edge first; without chances, the soonest to close.
	sort.SliceStable(picks, func(i, j int) bool {
		if picks[i].edge != picks[j].edge {
			return picks[i].edge > picks[j].edge
		}
		return picks[i].market.CloseAt.Before(picks[j].market.CloseAt)
	})
	pass.Candidates = len(picks)

	d.mu.Lock()
	for _, m := range markets {
		if p := d.Book.Holding(m.Ticker); p != nil {
			d.Book.Mark(m.Ticker, m.Bid(p.Buy.Side))
		}
	}
	for _, c := range picks {
		if d.Book.Watched(r.cfg.ID, c.market.Ticker) {
			continue
		}
		if _, err := d.Book.Append(c.line(r.cfg.ID, KIND_WATCH, now)); err != nil {
			d.mu.Unlock()
			return pass, STATE_ERROR, "", err
		}
		pass.Watched++
	}
	state, why := d.limits(r.cfg, now)
	d.mu.Unlock()
	if state != STATE_LOOKING {
		return pass, state, why, prepErr
	}
	bought, err := d.buy(ctx, r.cfg, picks, now)
	pass.Bought = bought
	d.mu.Lock()
	state, why = d.limits(r.cfg, now)
	d.mu.Unlock()
	return pass, state, why, errors.Join(prepErr, err)
}

// consider applies a strategy's opinion and its entry rules to one market.
func consider(r running, m Market, now time.Time) (candidate, bool) {
	e := r.cfg.Entry
	if m.Type != MARKET_BINARY || m.CloseAt.Sub(now) < time.Duration(e.LeftMinutes)*time.Minute || m.Volume24h < e.MinVolume24h {
		return candidate{}, false
	}
	op, ok := r.strategy.Evaluate(m, now)
	if !ok {
		return candidate{}, false
	}
	ask, bid := m.Ask(op.Side), m.Bid(op.Side)
	if ask < e.MinPrice || ask > e.MaxPrice || bid <= 0 || ask-bid > e.MaxSpread+CENT/2 {
		return candidate{}, false
	}
	c := candidate{market: m, side: op.Side, ask: ask, spread: round(ask-bid, PRICE_DIGITS), reasons: op.Reasons}
	if op.Prob > 0 {
		c.prob = op.Prob
		if op.Side == SIDE_NO {
			c.prob = DOLLAR - op.Prob
		}
		c.edge = round(c.prob-ask-feeEach(ask), PRICE_DIGITS)
		if c.prob < e.MinProb || c.edge < e.MinEdge || (e.MaxEdge > 0 && c.edge > e.MaxEdge) {
			return candidate{}, false
		}
	}
	return c, true
}

func (c candidate) line(strategy, kind string, now time.Time) Line {
	return Line{At: now.UTC(), Kind: kind, Strategy: strategy, Ticker: c.market.Ticker, Event: c.market.Event, Title: title(c.market),
		Side: c.side, Price: c.ask, Prob: round(c.prob, PRICE_DIGITS), CloseAt: c.market.CloseAt, Volume24h: c.market.Volume24h,
		Spread: c.spread, Note: strings.Join(c.reasons, REASONS_JOIN)}
}

// title is the market's question with the outcome it names, when Kalshi gives one.
func title(m Market) string {
	if m.YesTitle == "" || strings.Contains(m.Title, m.YesTitle) {
		return m.Title
	}
	return m.Title + " — " + m.YesTitle
}

// limits says whether a strategy may open positions now. The caller holds d.mu.
func (d *Desk) limits(c Config, now time.Time) (string, string) {
	day := Day(now)
	if RISK_ORDER[c.Risk] > RISK_ORDER[d.rulebook.Mode] {
		return STATE_WATCH, fmt.Sprintf(WHY_WATCH, c.Risk, d.rulebook.Mode)
	}
	if lost := d.Book.LostOn(day); lost >= d.rulebook.DailyLossUSD {
		return STATE_PAUSED, fmt.Sprintf(WHY_PAUSED, lost)
	}
	if d.Book.StakedOn(c.ID, day)+c.Limits.StakeUSD > c.Limits.DailyStakeUSD+CENT/2 {
		return STATE_FULL, fmt.Sprintf(WHY_FULL, c.Limits.DailyStakeUSD)
	}
	return STATE_LOOKING, ""
}

// fillFrom buys whole contracts from the cheapest asks up to limit, spending at most stakeUSD with the
// fee. It returns the contracts, their cost, the fee and how many price levels it took.
func fillFrom(asks []Level, limit, stakeUSD float64) (qty, cost, fee float64, levels int) {
	for _, l := range asks {
		if l.Price > limit+CENT/2 {
			break
		}
		perContract := l.Price + feeEach(l.Price)
		n := math.Min(math.Floor(l.Qty), math.Floor((stakeUSD-cost-fee)/perContract))
		for n > 0 && cost+fee+n*l.Price+Fee(n, l.Price) > stakeUSD+CENT/2 {
			n--
		}
		if n <= 0 {
			break
		}
		qty, cost, fee, levels = qty+n, cost+n*l.Price, fee+Fee(n, l.Price), levels+1
	}
	return qty, round(cost, USD_DIGITS), round(fee, USD_DIGITS), levels
}

// priceLimit is the most a strategy pays a contract: its top price, and for a strategy with a chance,
// no more than keeps its edge.
func priceLimit(c Config, pick candidate) float64 {
	limit := c.Entry.MaxPrice
	if pick.prob > 0 {
		for limit > pick.ask && pick.prob-limit-feeEach(limit) < c.Entry.MinEdge {
			limit -= CENT
		}
	}
	return limit
}

// buy fills a strategy's best candidates against the real order book, within every limit.
func (d *Desk) buy(ctx context.Context, c Config, picks []candidate, now time.Time) (int, error) {
	bought := 0
	for _, p := range picks {
		d.mu.Lock()
		state, _ := d.limits(c, now)
		account := d.Book.Account("")
		skip := d.Book.Holding(p.market.Ticker) != nil || d.Book.OpenEvents()[p.market.Event] > 0
		stop := state != STATE_LOOKING || bought >= c.Limits.NewPerPass || d.Book.OpenBy(c.ID) >= c.Limits.MaxOpen || account.CashUSD < c.Limits.StakeUSD
		d.mu.Unlock()
		if stop {
			return bought, nil
		}
		if skip {
			continue
		}
		asks, err := d.Kalshi.Asks(ctx, p.market.Ticker, p.side)
		if err != nil {
			return bought, err
		}
		qty, cost, fee, levels := fillFrom(asks, priceLimit(c, p), c.Limits.StakeUSD)
		if qty <= 0 || cost/qty < c.Entry.MinPrice-CENT/2 {
			continue
		}
		line := p.line(c.ID, KIND_BUY, time.Now())
		line.Price, line.Qty, line.CostUSD, line.FeeUSD = round(cost/qty, PRICE_DIGITS), qty, cost, fee
		line.Note = fmt.Sprintf(NOTE_FILL, levels, asks[0].Price) + REASONS_JOIN + line.Note
		d.mu.Lock()
		_, err = d.Book.Append(line)
		d.mu.Unlock()
		if err != nil {
			return bought, err
		}
		bought++
		log.Printf("%s%s bought %.0f %s %s at %.4f, $%.2f + $%.2f fee", LOG_PREFIX, c.ID, qty, strings.ToUpper(p.side), p.market.Ticker, line.Price, cost, fee)
		if err := pause(ctx); err != nil {
			return bought, err
		}
	}
	return bought, nil
}

// settle looks up every market waiting for a result and books the ones that finished.
func (d *Desk) settle(ctx context.Context, now time.Time) (int, error) {
	d.mu.Lock()
	tickers := d.Book.Unsettled()
	d.mu.Unlock()
	if len(tickers) == 0 {
		return 0, nil
	}
	found, err := d.Kalshi.Lookup(ctx, tickers)
	d.mu.Lock()
	defer d.mu.Unlock()
	settled := 0
	for _, t := range tickers {
		m, ok := found[t]
		if !ok {
			continue
		}
		p := d.Book.Holding(t)
		if !m.Finished() {
			if p != nil {
				d.Book.Mark(t, m.Bid(p.Buy.Side))
			}
			continue
		}
		line := Line{At: now.UTC(), Kind: KIND_SETTLE, Ticker: t, Result: m.Result, Value: m.Payout(SIDE_YES)}
		if p != nil && !p.Settled {
			line.PayoutUSD = round(m.Payout(p.Buy.Side)*p.Buy.Qty, USD_DIGITS)
		}
		if _, err := d.Book.Append(line); err != nil {
			return settled, err
		}
		settled++
	}
	if err != nil {
		return settled, fmt.Errorf("results: %w", err)
	}
	return settled, nil
}

// Status is the desk now, for the page.
func (d *Desk) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	rb := d.rulebook
	s := Status{Venue: VENUE, KeysSet: KeysSet(), On: !d.off, Mode: rb.Mode, CapitalUSD: rb.CapitalUSD, DailyLossUSD: rb.DailyLossUSD,
		LostTodayUSD: round(d.Book.LostOn(Day(time.Now())), USD_DIGITS), RulebookFile: d.RulebookPath, RulebookError: d.rulebookErr,
		SettledAt: d.settledAt, Account: d.Book.Account(""), Kinds: Kinds(), Strategies: []StrategyStatus{}}
	for _, r := range d.strategies {
		st := d.runs[r.cfg.ID]
		ss := StrategyStatus{Config: r.cfg, State: st.state, Why: st.why, LastAt: st.lastAt, NextAt: st.nextAt, LastError: st.lastError,
			Pass: st.pass, Record: d.Book.Record(r.cfg.ID, r.cfg.Bands), Account: d.Book.Account(r.cfg.ID)}
		if d.off {
			ss.State, ss.Why = STATE_OFF, WHY_DESK_OFF
		}
		if ss.State == "" {
			ss.State = STATE_LOOKING
		}
		s.Strategies = append(s.Strategies, ss)
	}
	s.Open, s.Closed = d.Book.Positions(CLOSED_SHOWN)
	return s
}

// Routes are the desk's HTTP routes: its status, and replacing its rulebook.
func (d *Desk) Routes() map[string]http.Handler {
	return map[string]http.Handler{
		ROUTE_STATUS: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, d.Status()) }),
		ROUTE_RULES:  http.HandlerFunc(d.putRulebook),
	}
}

// putRulebook replaces the rulebook with the one sent, once it builds.
func (d *Desk) putRulebook(w http.ResponseWriter, r *http.Request) {
	if d.MayWrite != nil && !d.MayWrite(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": ERR_NO_WRITE})
		return
	}
	var rb Rulebook
	if err := json.NewDecoder(io.LimitReader(r.Body, RULEBOOK_MAX)).Decode(&rb); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if _, err := rb.Build(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := SaveRulebook(d.RulebookPath, rb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := d.Reload(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, d.Status())
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
