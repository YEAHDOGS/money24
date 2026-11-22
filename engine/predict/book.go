package predict

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"sort"
	"time"
)

const (
	// Kalshi's taker fee on each fill: 7% of contracts × price × (1 − price), rounded up to the cent.
	// It is largest at 50¢ and small near the ends. A few series charge less; the desk charges every
	// market the standard rate, so its record is never flattered.
	FEE_RATE     = 0.07
	CENT         = 0.01
	PRICE_DIGITS = 4
	USD_DIGITS   = 2
	PERCENT      = 100.0
	LINE_MAX     = 1 << 20
	FILE_PERMS   = 0o644
	DAY_LAYOUT   = "2006-01-02"
	NEW_YORK     = "America/New_York"

	KIND_WATCH  = "watch"  // a market met the rule: followed to its result, traded or not
	KIND_BUY    = "buy"    // a paper fill against the real order book
	KIND_SETTLE = "settle" // the result: what every watched contract and every position paid
)

// BANDS are the favorite's price ranges the record is kept in, so each can prove itself on its own.
var BANDS = []float64{0.85, 0.88, 0.91, 0.95}

var newYork = func() *time.Location {
	loc, err := time.LoadLocation(NEW_YORK)
	if err != nil {
		return time.UTC
	}
	return loc
}()

// Line is one line of the book's journal.
type Line struct {
	ID        int64     `json:"id"`
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Ticker    string    `json:"ticker"`
	Event     string    `json:"event,omitempty"`
	Title     string    `json:"title,omitempty"`
	Side      string    `json:"side,omitempty"`
	Price     float64   `json:"price,omitempty"` // watch: the ask then; buy: the average fill
	Qty       float64   `json:"qty,omitempty"`
	CostUSD   float64   `json:"cost_usd,omitempty"`
	FeeUSD    float64   `json:"fee_usd,omitempty"`
	PayoutUSD float64   `json:"payout_usd,omitempty"`
	Result    string    `json:"result,omitempty"`
	Value     float64   `json:"value,omitempty"` // settle: what one YES contract paid
	CloseAt   time.Time `json:"close_at,omitzero"`
	Volume24h float64   `json:"volume_24h,omitempty"`
	Spread    float64   `json:"spread,omitempty"`
	Note      string    `json:"note,omitempty"`
}

// Position is a paper holding, open or settled.
type Position struct {
	Buy       Line      `json:"buy"`
	Mark      float64   `json:"mark"` // what the side bids now; the settlement payout once settled
	Settled   bool      `json:"settled"`
	PayoutUSD float64   `json:"payout_usd"`
	PnlUSD    float64   `json:"pnl_usd"`
	SettledAt time.Time `json:"settled_at,omitzero"`
	Result    string    `json:"result,omitempty"`
}

type watch struct {
	line    Line
	settled bool
	payout  float64 // per contract
}

// Account is the paper cash account of the prediction desk.
type Account struct {
	CapitalUSD  float64 `json:"capital_usd"`
	CashUSD     float64 `json:"cash_usd"`
	OpenUSD     float64 `json:"open_usd"` // open positions at cost, fees included
	MarkUSD     float64 `json:"mark_usd"` // open positions at what they bid now
	RealizedUSD float64 `json:"realized_usd"`
	FeesUSD     float64 `json:"fees_usd"`
	Open        int     `json:"open"`
	Settled     int     `json:"settled"`
	Won         int     `json:"won"`
}

// Band is the record of every watched favorite in one price range, as if one contract of each had
// been bought at the watched price with the fee: the honest test of the rule, traded or not.
type Band struct {
	From     float64 `json:"from"`
	To       float64 `json:"to"`
	Watched  int     `json:"watched"`
	Settled  int     `json:"settled"`
	Won      int     `json:"won"`
	WinPct   float64 `json:"win_pct"`
	AvgPrice float64 `json:"avg_price"` // the average ask paid, fee included: the win rate that breaks even
	EdgePct  float64 `json:"edge_pct"`  // what a dollar in every settled one made or lost
}

// Book is the append-only journal of the prediction desk and everything added up from it.
type Book struct {
	file      *os.File
	enc       *json.Encoder
	nextID    int64
	capital   float64
	watches   map[string]*watch
	positions map[string]*Position
	order     []string           // position tickers, oldest first
	staked    map[string]float64 // New York day -> dollars put into new positions
	lost      map[string]float64 // New York day -> dollars lost on positions settled that day
}

// Fee is Kalshi's taker fee for qty contracts bought at price.
func Fee(qty, price float64) float64 {
	return math.Ceil(round(FEE_RATE*qty*price*(DOLLAR-price)/CENT, PRICE_DIGITS)) * CENT
}

func round(n float64, digits int) float64 {
	scale := math.Pow(10, float64(digits))
	return math.Round(n*scale) / scale
}

// Day is the New York date of t, the day the desk's limits count by.
func Day(t time.Time) string { return t.In(newYork).Format(DAY_LAYOUT) }

// OpenBook opens the journal at path for appending and adds up what is in it.
func OpenBook(path string, capital float64) (*Book, error) {
	b := &Book{nextID: 1, capital: capital, watches: map[string]*watch{}, positions: map[string]*Position{},
		staked: map[string]float64{}, lost: map[string]float64{}}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, LINE_MAX), LINE_MAX)
		for sc.Scan() {
			var l Line
			if json.Unmarshal(sc.Bytes(), &l) != nil {
				continue
			}
			b.nextID = max(b.nextID, l.ID+1)
			b.apply(l)
		}
		f.Close()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, FILE_PERMS)
	if err != nil {
		return nil, err
	}
	b.file, b.enc = f, json.NewEncoder(f)
	return b, nil
}

// Close closes the journal.
func (b *Book) Close() error { return b.file.Close() }

// Append numbers l, writes it and adds it up.
func (b *Book) Append(l Line) (Line, error) {
	l.ID = b.nextID
	b.nextID++
	if err := b.enc.Encode(l); err != nil {
		return l, err
	}
	b.apply(l)
	return l, nil
}

func (b *Book) apply(l Line) {
	switch l.Kind {
	case KIND_WATCH:
		if b.watches[l.Ticker] == nil {
			b.watches[l.Ticker] = &watch{line: l}
		}
	case KIND_BUY:
		if b.positions[l.Ticker] != nil {
			return
		}
		b.positions[l.Ticker] = &Position{Buy: l, Mark: l.Price}
		b.order = append(b.order, l.Ticker)
		b.staked[Day(l.At)] += l.CostUSD + l.FeeUSD
	case KIND_SETTLE:
		b.settle(l)
	}
}

func (b *Book) settle(l Line) {
	yes := l.Value
	if w := b.watches[l.Ticker]; w != nil && !w.settled {
		w.settled, w.payout = true, payoutFor(w.line.Side, yes)
	}
	p := b.positions[l.Ticker]
	if p == nil || p.Settled {
		return
	}
	per := payoutFor(p.Buy.Side, yes)
	p.Settled, p.Mark, p.Result, p.SettledAt = true, per, l.Result, l.At
	p.PayoutUSD = round(per*p.Buy.Qty, USD_DIGITS)
	p.PnlUSD = round(p.PayoutUSD-p.Buy.CostUSD-p.Buy.FeeUSD, USD_DIGITS)
	if p.PnlUSD < 0 {
		b.lost[Day(l.At)] -= p.PnlUSD
	}
}

func payoutFor(side string, yesValue float64) float64 {
	if side == SIDE_YES {
		return yesValue
	}
	return DOLLAR - yesValue
}

// Watched says whether a market is already being followed.
func (b *Book) Watched(ticker string) bool { return b.watches[ticker] != nil }

// Holding returns the position in a market, nil when there is none.
func (b *Book) Holding(ticker string) *Position { return b.positions[ticker] }

// OpenEvents counts open positions per event: markets in one event move together, so the desk holds one.
func (b *Book) OpenEvents() map[string]int {
	out := map[string]int{}
	for _, p := range b.positions {
		if !p.Settled {
			out[p.Buy.Event]++
		}
	}
	return out
}

// Unsettled lists every market still waiting for a result: watched ones and held ones.
func (b *Book) Unsettled() []string {
	out := []string{}
	for t, w := range b.watches {
		if !w.settled {
			out = append(out, t)
		}
	}
	for t, p := range b.positions {
		if !p.Settled && b.watches[t] == nil {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// Mark updates what an open position bids now.
func (b *Book) Mark(ticker string, bid float64) {
	if p := b.positions[ticker]; p != nil && !p.Settled && bid > 0 {
		p.Mark = bid
	}
}

// StakedOn and LostOn are the dollars put in and lost on one New York day.
func (b *Book) StakedOn(day string) float64 { return b.staked[day] }
func (b *Book) LostOn(day string) float64   { return b.lost[day] }

// Account adds up the cash account.
func (b *Book) Account() Account {
	a := Account{CapitalUSD: b.capital, CashUSD: b.capital}
	for _, p := range b.positions {
		cost := p.Buy.CostUSD + p.Buy.FeeUSD
		a.CashUSD -= cost
		a.FeesUSD += p.Buy.FeeUSD
		if !p.Settled {
			a.Open++
			a.OpenUSD += cost
			a.MarkUSD += p.Mark * p.Buy.Qty
			continue
		}
		a.CashUSD += p.PayoutUSD
		a.RealizedUSD += p.PnlUSD
		a.Settled++
		if p.PnlUSD > 0 {
			a.Won++
		}
	}
	a.CashUSD, a.OpenUSD, a.MarkUSD = round(a.CashUSD, USD_DIGITS), round(a.OpenUSD, USD_DIGITS), round(a.MarkUSD, USD_DIGITS)
	a.RealizedUSD, a.FeesUSD = round(a.RealizedUSD, USD_DIGITS), round(a.FeesUSD, USD_DIGITS)
	return a
}

// Bands is the rule's record by price range.
func (b *Book) Bands() []Band {
	out := make([]Band, len(BANDS)-1)
	paid, made := make([]float64, len(out)), make([]float64, len(out))
	for i := range out {
		out[i].From, out[i].To = BANDS[i], BANDS[i+1]
	}
	for _, w := range b.watches {
		i := bandOf(w.line.Price)
		if i < 0 {
			continue
		}
		out[i].Watched++
		if !w.settled {
			continue
		}
		out[i].Settled++
		// The fee per contract as it falls on an order of some size, before the cent rounding.
		cost := w.line.Price + FEE_RATE*w.line.Price*(DOLLAR-w.line.Price)
		paid[i] += cost
		made[i] += w.payout - cost
		if w.payout > w.line.Price {
			out[i].Won++
		}
	}
	for i := range out {
		if out[i].Settled == 0 {
			continue
		}
		n := float64(out[i].Settled)
		out[i].WinPct = round(float64(out[i].Won)/n*PERCENT, 1)
		out[i].AvgPrice = round(paid[i]/n, PRICE_DIGITS)
		out[i].EdgePct = round(made[i]/paid[i]*PERCENT, 2)
	}
	return out
}

func bandOf(price float64) int {
	for i := 0; i+1 < len(BANDS); i++ {
		if price >= BANDS[i] && (price < BANDS[i+1] || i+2 == len(BANDS) && price <= BANDS[i+1]) {
			return i
		}
	}
	return -1
}

// Positions returns the open positions soonest to close first, then up to closed settled ones, newest first.
func (b *Book) Positions(closed int) (open, done []Position) {
	open, done = []Position{}, []Position{}
	for i := len(b.order) - 1; i >= 0; i-- {
		p := b.positions[b.order[i]]
		if !p.Settled {
			open = append(open, *p)
			continue
		}
		done = append(done, *p)
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Buy.CloseAt.Before(open[j].Buy.CloseAt) })
	sort.SliceStable(done, func(i, j int) bool { return done[i].SettledAt.After(done[j].SettledAt) })
	return open, done[:min(len(done), closed)]
}
