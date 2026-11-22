package predict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// The first rule on the desk: favorites near the close. Over thousands of markets, contracts priced
	// 85–95¢ in the last hours before they settle have tended to win a little more often than their
	// price says (the favorite–long-shot bias). Whether that survives Kalshi's fee is what this desk is
	// here to find out, one settled market at a time.
	DEFAULT_EVERY_MINUTES  = 5
	DEFAULT_WITHIN_HOURS   = 6.0  // markets whose trading ends within this many hours
	DEFAULT_LEFT_MINUTES   = 5    // and not in the last few minutes, when there is no time to fill
	DEFAULT_MIN_PRICE      = 0.85 // the favorite's ask, dollars a contract
	DEFAULT_MAX_PRICE      = 0.95
	DEFAULT_MAX_SPREAD     = 0.03  // the favorite's ask minus its bid
	DEFAULT_MIN_VOLUME     = 200.0 // contracts traded in the last 24 hours
	DEFAULT_CAPITAL_USD    = 1000.0
	DEFAULT_STAKE_USD      = 20.0 // the most put into one market, fee included
	DEFAULT_MAX_OPEN       = 25
	DEFAULT_NEW_PER_PASS   = 5
	DEFAULT_DAILY_STAKE    = 200.0 // dollars into new positions per New York day
	DEFAULT_DAILY_LOSS_USD = 100.0 // settled losses in a day that stop new positions until the next
	PASS_TIMEOUT           = 4 * time.Minute
	CLOSED_SHOWN           = 20
	ENV_PREDICT            = "M24D_PREDICT" // "off" turns the desk off
	SWITCH_OFF             = "off"
	LOG_PREFIX             = "predict: "
	VENUE                  = "Kalshi"

	STATE_LOOKING = "looking"
	STATE_OFF     = "off"
	STATE_FULL    = "full"
	STATE_PAUSED  = "paused"
	STATE_ERROR   = "error"

	WHY_OFF    = "Turned off with M24D_PREDICT=off."
	WHY_FULL   = "Today's stake of $%.0f is used. New positions resume tomorrow, New York time."
	WHY_PAUSED = "Lost $%.0f on settled positions today. New positions resume tomorrow, New York time."
	NOTE_FILL  = "%d levels of Kalshi's book, best ask %.2f"
)

// Rules are the desk's limits and its one rule, shown on the desk as they are.
type Rules struct {
	EveryMinutes  int     `json:"every_minutes"`
	WithinHours   float64 `json:"within_hours"`
	LeftMinutes   int     `json:"left_minutes"`
	MinPrice      float64 `json:"min_price"`
	MaxPrice      float64 `json:"max_price"`
	MaxSpread     float64 `json:"max_spread"`
	MinVolume24h  float64 `json:"min_volume_24h"`
	CapitalUSD    float64 `json:"capital_usd"`
	StakeUSD      float64 `json:"stake_usd"`
	MaxOpen       int     `json:"max_open"`
	NewPerPass    int     `json:"new_per_pass"`
	DailyStakeUSD float64 `json:"daily_stake_usd"`
	DailyLossUSD  float64 `json:"daily_loss_usd"`
}

// DefaultRules are the rules the desk starts with.
func DefaultRules() Rules {
	return Rules{EveryMinutes: DEFAULT_EVERY_MINUTES, WithinHours: DEFAULT_WITHIN_HOURS, LeftMinutes: DEFAULT_LEFT_MINUTES,
		MinPrice: DEFAULT_MIN_PRICE, MaxPrice: DEFAULT_MAX_PRICE, MaxSpread: DEFAULT_MAX_SPREAD, MinVolume24h: DEFAULT_MIN_VOLUME,
		CapitalUSD: DEFAULT_CAPITAL_USD, StakeUSD: DEFAULT_STAKE_USD, MaxOpen: DEFAULT_MAX_OPEN, NewPerPass: DEFAULT_NEW_PER_PASS,
		DailyStakeUSD: DEFAULT_DAILY_STAKE, DailyLossUSD: DEFAULT_DAILY_LOSS_USD}
}

// Pass is what the last look found.
type Pass struct {
	Markets    int `json:"markets"`    // open markets closing inside the window
	Candidates int `json:"candidates"` // of those, favorites the rule takes
	Watched    int `json:"watched"`    // newly followed
	Bought     int `json:"bought"`
	Settled    int `json:"settled"`
}

// Status is the desk as the page shows it.
type Status struct {
	Venue     string     `json:"venue"`
	KeysSet   bool       `json:"keys_set"` // Kalshi keys in .env, for demo orders later; the paper book needs none
	State     string     `json:"state"`
	Why       string     `json:"why,omitempty"`
	LastAt    time.Time  `json:"last_at,omitzero"`
	NextAt    time.Time  `json:"next_at,omitzero"`
	LastError string     `json:"last_error,omitempty"`
	Pass      Pass       `json:"pass"`
	Rules     Rules      `json:"rules"`
	Account   Account    `json:"account"`
	Bands     []Band     `json:"bands"`
	Open      []Position `json:"open"`
	Closed    []Position `json:"closed"`
}

// Desk runs the prediction book around the clock.
type Desk struct {
	Book   *Book
	Kalshi *Kalshi
	Rules  Rules

	mu    sync.Mutex
	state Status
}

// pick is a favorite the rule takes, and which side of it.
type pick struct {
	market Market
	side   string
	ask    float64
	spread float64
}

// favorite returns the side of a market the rule would buy, if any.
func favorite(m Market, r Rules, now time.Time) (pick, bool) {
	left := m.CloseAt.Sub(now)
	if m.Type != MARKET_BINARY || left < time.Duration(r.LeftMinutes)*time.Minute || m.Volume24h < r.MinVolume24h {
		return pick{}, false
	}
	for _, side := range []string{SIDE_YES, SIDE_NO} {
		ask, bid := m.Ask(side), m.Bid(side)
		if ask < r.MinPrice || ask > r.MaxPrice || bid <= 0 || ask-bid > r.MaxSpread+CENT/2 {
			continue
		}
		return pick{market: m, side: side, ask: ask, spread: round(ask-bid, PRICE_DIGITS)}, true
	}
	return pick{}, false
}

// fillFrom buys whole contracts from the cheapest asks up to maxPrice, spending at most stakeUSD with
// the fee. It returns the contracts, their cost, the fee and how many price levels it took.
func fillFrom(asks []Level, maxPrice, stakeUSD float64) (qty, cost, fee float64, levels int) {
	for _, l := range asks {
		if l.Price > maxPrice+CENT/2 {
			break
		}
		perContract := l.Price * (DOLLAR + FEE_RATE*(DOLLAR-l.Price))
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

// Run looks every few minutes until ctx ends.
func (d *Desk) Run(ctx context.Context) {
	if strings.EqualFold(os.Getenv(ENV_PREDICT), SWITCH_OFF) {
		d.set(func(s *Status) { s.State, s.Why = STATE_OFF, WHY_OFF })
		return
	}
	every := time.Duration(max(1, d.Rules.EveryMinutes)) * time.Minute
	for {
		pass, cancel := context.WithTimeout(ctx, PASS_TIMEOUT)
		err := d.pass(pass, time.Now())
		cancel()
		d.set(func(s *Status) {
			s.LastAt, s.NextAt, s.LastError = time.Now().UTC(), time.Now().Add(every).UTC(), ""
			if err != nil {
				s.LastError = err.Error()
				log.Printf("%s%v", LOG_PREFIX, err)
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

func (d *Desk) set(change func(*Status)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	change(&d.state)
}

// pass is one look: follow every favorite, buy the best few within the limits, settle what finished.
func (d *Desk) pass(ctx context.Context, now time.Time) error {
	r := d.Rules
	markets, scanErr := d.Kalshi.Closing(ctx, now.Add(time.Duration(r.LeftMinutes)*time.Minute), now.Add(time.Duration(r.WithinHours*float64(time.Hour))))
	if scanErr != nil && len(markets) == 0 {
		d.set(func(s *Status) { s.State = STATE_ERROR })
		return fmt.Errorf("markets: %w", scanErr)
	}
	picks := []pick{}
	for _, m := range markets {
		if p, ok := favorite(m, r, now); ok {
			picks = append(picks, p)
		}
	}
	// Soonest to close first: the least time for the favorite to slip, and the fastest record.
	sort.Slice(picks, func(i, j int) bool { return picks[i].market.CloseAt.Before(picks[j].market.CloseAt) })
	note := Pass{Markets: len(markets), Candidates: len(picks)}

	d.mu.Lock()
	for _, m := range markets {
		d.Book.Mark(m.Ticker, m.Bid(sideOf(d.Book.Holding(m.Ticker))))
	}
	for _, p := range picks {
		if d.Book.Watched(p.market.Ticker) {
			continue
		}
		line := Line{At: now.UTC(), Kind: KIND_WATCH, Ticker: p.market.Ticker, Event: p.market.Event, Title: title(p.market),
			Side: p.side, Price: p.ask, CloseAt: p.market.CloseAt, Volume24h: p.market.Volume24h, Spread: p.spread}
		if _, err := d.Book.Append(line); err != nil {
			d.mu.Unlock()
			return err
		}
		note.Watched++
	}
	state, why := d.limits(now)
	d.mu.Unlock()

	if state == STATE_LOOKING {
		bought, err := d.buy(ctx, picks, now)
		note.Bought = bought
		if err != nil {
			log.Printf("%sbuy: %v", LOG_PREFIX, err)
		}
		state, why = d.limitsLocked(now)
	}
	settled, err := d.settle(ctx, now)
	note.Settled = settled
	d.set(func(s *Status) { s.State, s.Why, s.Pass = state, why, note })
	if note.Bought+note.Settled > 0 {
		log.Printf("%s%d markets, %d favorites, %d new, %d bought, %d settled", LOG_PREFIX, note.Markets, note.Candidates, note.Watched, note.Bought, note.Settled)
	}
	if scanErr != nil {
		err = errors.Join(fmt.Errorf("markets: %w", scanErr), err)
	}
	return err
}

func sideOf(p *Position) string {
	if p == nil {
		return SIDE_YES
	}
	return p.Buy.Side
}

// title is the market's question with the outcome it names, when Kalshi gives one.
func title(m Market) string {
	if m.YesTitle == "" || strings.Contains(m.Title, m.YesTitle) {
		return m.Title
	}
	return m.Title + " — " + m.YesTitle
}

// limits says whether the desk may open positions now. The caller holds d.mu.
func (d *Desk) limits(now time.Time) (string, string) {
	r, day := d.Rules, Day(now)
	if lost := d.Book.LostOn(day); lost >= r.DailyLossUSD {
		return STATE_PAUSED, fmt.Sprintf(WHY_PAUSED, lost)
	}
	if d.Book.StakedOn(day)+r.StakeUSD > r.DailyStakeUSD+CENT/2 {
		return STATE_FULL, fmt.Sprintf(WHY_FULL, r.DailyStakeUSD)
	}
	return STATE_LOOKING, ""
}

func (d *Desk) limitsLocked(now time.Time) (string, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.limits(now)
}

// buy fills up to the pass's share of new positions against the real order book.
func (d *Desk) buy(ctx context.Context, picks []pick, now time.Time) (int, error) {
	r, bought := d.Rules, 0
	for _, p := range picks {
		d.mu.Lock()
		account, events, day := d.Book.Account(), d.Book.OpenEvents(), Day(now)
		skip := d.Book.Holding(p.market.Ticker) != nil || events[p.market.Event] > 0
		stop := bought >= r.NewPerPass || account.Open >= r.MaxOpen || account.CashUSD < r.StakeUSD ||
			d.Book.StakedOn(day)+r.StakeUSD > r.DailyStakeUSD+CENT/2 || d.Book.LostOn(day) >= r.DailyLossUSD
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
		qty, cost, fee, levels := fillFrom(asks, r.MaxPrice, r.StakeUSD)
		if qty <= 0 || cost/qty < r.MinPrice-CENT/2 {
			continue
		}
		line := Line{At: time.Now().UTC(), Kind: KIND_BUY, Ticker: p.market.Ticker, Event: p.market.Event, Title: title(p.market),
			Side: p.side, Price: round(cost/qty, PRICE_DIGITS), Qty: qty, CostUSD: cost, FeeUSD: fee, CloseAt: p.market.CloseAt,
			Volume24h: p.market.Volume24h, Spread: p.spread, Note: fmt.Sprintf(NOTE_FILL, levels, asks[0].Price)}
		d.mu.Lock()
		_, err = d.Book.Append(line)
		d.mu.Unlock()
		if err != nil {
			return bought, err
		}
		bought++
		log.Printf("%sbought %.0f %s %s at %.4f, $%.2f + $%.2f fee", LOG_PREFIX, qty, strings.ToUpper(p.side), p.market.Ticker, line.Price, cost, fee)
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
		if !m.Finished() {
			d.Book.Mark(t, m.Bid(sideOf(d.Book.Holding(t))))
			continue
		}
		line := Line{At: now.UTC(), Kind: KIND_SETTLE, Ticker: t, Result: m.Result, Value: m.Payout(SIDE_YES)}
		if p := d.Book.Holding(t); p != nil && !p.Settled {
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
	s := d.state
	s.Venue, s.KeysSet, s.Rules = VENUE, KeysSet(), d.Rules
	if s.State == "" {
		s.State = STATE_LOOKING
	}
	s.Account, s.Bands = d.Book.Account(), d.Book.Bands()
	s.Open, s.Closed = d.Book.Positions(CLOSED_SHOWN)
	return s
}

// Handler serves the desk's status as JSON.
func (d *Desk) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d.Status())
	})
}
