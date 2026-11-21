package signals

import (
	"errors"
	"math"
	"sort"
	"time"
)

const (
	// Stock and option proceeds settle the next business day (T+1): they are spendable from that
	// session's open, New York time. Coins settle the moment they sell.
	SETTLE_HOUR        = 9
	SETTLE_MINUTE      = 30
	CASH_TOLERANCE_USD = 0.01 // rounding room when a cost is checked against settled cash
	MIN_ORDER_USD      = 1.0  // less settled cash than this is not an order
	NOTE_CASH_SIZED    = "cash account: sized to $%.2f settled cash"
	CASH_SHORT         = "%w: $%.2f settled cash, needs $%.2f"
)

// ErrNoCash rejects an entry the cash account cannot pay for. There is no margin, and sale
// proceeds that have not settled do not count.
var ErrNoCash = errors.New("cash account")

// Account is the desk's cash account at one moment.
type Account struct {
	CapitalUSD     float64  `json:"capital_usd"`      // starting paper capital (catalog go_live.paper_capital_usd)
	CashUSD        float64  `json:"cash_usd"`         // capital - every buy + every sale, settled or not
	UnsettledUSD   float64  `json:"unsettled_usd"`    // sale proceeds not yet spendable
	BuyingPowerUSD float64  `json:"buying_power_usd"` // cash - unsettled, never below 0
	InvestedUSD    float64  `json:"invested_usd"`     // open positions at cost
	EquityUSD      float64  `json:"equity_usd"`       // cash + invested (at cost; marks are the UI's job)
	RealizedUSD    float64  `json:"realized_usd"`     // closed and partly sold trades, after slippage and fees
	Open           int      `json:"open"`             // positions held
	Settles        []Settle `json:"settles"`          // pending settlements, soonest first (never nil)
}

// Settle is sale proceeds that become spendable at At.
type Settle struct {
	At  time.Time `json:"at"`
	USD float64   `json:"usd"`
}

// outlay is what a plan's entry takes out of the account: the entry price, as rounded, on every
// unit. The watcher scores every sale against that same price, so buys and sales add up.
func (p Plan) outlay() float64 {
	multiplier := float64(SHARE_MULTIPLIER)
	if p.Signal.Right != "" {
		multiplier = OPTION_MULTIPLIER
	}
	return p.Entry * p.Qty * multiplier
}

// holding is one open position as the ledger sees it.
type holding struct {
	signal Signal
	unit   float64 // dollars one unit cost: the entry price times the multiplier
	cost   float64 // dollars still in the position
	banked float64 // what its sold rungs have made or lost so far
}

// sale is proceeds waiting to settle.
type sale struct {
	signal Signal
	at     time.Time
	usd    float64
}

// ledger adds the cash account up from journal lines, one at a time and in order. The journal feeds
// it every line it has ever written, so the totals hold however little of the journal is in memory.
type ledger struct {
	capital  float64
	flow     float64 // every sale minus every buy
	realized float64
	held     map[int64]*holding // by the plan's journal id
	pending  []sale
	settleAt func(Signal, time.Time) time.Time
}

func newLedger() *ledger {
	return &ledger{held: map[int64]*holding{}, settleAt: SettleNextWeekday}
}

// SettleNextWeekday is when a sale's proceeds can be spent again: at once for coins, and at the
// open of the next weekday for stocks and options. It knows no holidays; Runner.SetSettlement
// swaps in a calendar that does.
func SettleNextWeekday(sig Signal, soldAt time.Time) time.Time {
	if sig.Market == MARKET_CRYPTO || sig.Address != "" {
		return soldAt
	}
	day := soldAt.In(MARKET).AddDate(0, 0, 1)
	for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		day = day.AddDate(0, 0, 1)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), SETTLE_HOUR, SETTLE_MINUTE, 0, 0, MARKET)
}

// apply books one journal line: a plan buys, a fill or a close sells, a void hands the cost back.
// Dollars sold come from the line's own result, so the account and the scoreboard always agree.
func (l *ledger) apply(e Entry) {
	if e.Plan != nil {
		h := &holding{signal: e.Plan.Signal, cost: e.Plan.outlay()}
		if e.Plan.Qty > 0 {
			h.unit = h.cost / e.Plan.Qty
		}
		l.flow -= h.cost
		l.held[e.ID] = h
		return
	}
	h := l.held[e.PlanID]
	if h == nil {
		return
	}
	switch {
	case e.Action == ACTION_VOID, e.Action == ACTION_CLOSE && e.Result == nil:
		// Never scored: the money comes back as if the trade had not happened.
		l.flow += h.cost
		delete(l.held, e.PlanID)
	case e.Action == ACTION_CLOSE:
		// A close line carries the whole trade's result: what is new is the part no fill banked.
		l.sell(h, e.At, h.cost, e.Result.PnLUSD-h.banked)
		delete(l.held, e.PlanID)
	case e.Action == ACTION_FILL && e.Result != nil:
		basis := min(h.cost, e.Result.Qty*h.unit)
		l.sell(h, e.At, basis, e.Result.PnLUSD)
		h.cost, h.banked = h.cost-basis, h.banked+e.Result.PnLUSD
	}
}

// sell returns basis dollars of a position plus its gain to cash, and queues the proceeds when
// they do not settle at once.
func (l *ledger) sell(h *holding, at time.Time, basis, gain float64) {
	proceeds := basis + gain
	l.flow += proceeds
	l.realized += gain
	if proceeds > 0 && l.settleAt(h.signal, at).After(at) {
		l.pending = append(l.pending, sale{signal: h.signal, at: at, usd: proceeds})
	}
}

// snapshot is the account at now. Proceeds that have settled by then leave the queue for good.
func (l *ledger) snapshot(now time.Time) Account {
	cash, invested := l.capital+l.flow, 0.0
	for _, h := range l.held {
		invested += h.cost
	}
	waiting, unsettled := l.pending[:0], 0.0
	due := map[int64]float64{} // by settlement time, so one day's sales read as one line
	for _, s := range l.pending {
		at := l.settleAt(s.signal, s.at)
		if !at.After(now) {
			continue
		}
		waiting, unsettled = append(waiting, s), unsettled+s.usd
		due[at.Unix()] += s.usd
	}
	l.pending = waiting

	a := Account{
		CapitalUSD:     l.capital,
		CashUSD:        math.Round(cash*PRICE_SCALE) / PRICE_SCALE,
		UnsettledUSD:   math.Round(unsettled*PRICE_SCALE) / PRICE_SCALE,
		BuyingPowerUSD: max(0, math.Round((cash-unsettled)*PRICE_SCALE)/PRICE_SCALE),
		InvestedUSD:    math.Round(invested*PRICE_SCALE) / PRICE_SCALE,
		EquityUSD:      math.Round((cash+invested)*PRICE_SCALE) / PRICE_SCALE,
		RealizedUSD:    math.Round(l.realized*PRICE_SCALE) / PRICE_SCALE,
		Open:           len(l.held),
		Settles:        make([]Settle, 0, len(due)),
	}
	for at, usd := range due {
		a.Settles = append(a.Settles, Settle{At: time.Unix(at, 0).UTC(), USD: math.Round(usd*PRICE_SCALE) / PRICE_SCALE})
	}
	sort.Slice(a.Settles, func(i, j int) bool { return a.Settles[i].At.Before(a.Settles[j].At) })
	return a
}

// Account is the desk's cash account right now, added up from every line the journal ever wrote.
func (r *Runner) Account(now time.Time) Account {
	r.journal.mu.Lock()
	defer r.journal.mu.Unlock()
	return r.journal.ledger.snapshot(now)
}

// SetSettlement replaces the rule for when a sale's proceeds settle, so a calendar that knows the
// market holidays can stand in for SettleNextWeekday. Sales already waiting follow the new rule.
func (r *Runner) SetSettlement(settleAt func(sig Signal, soldAt time.Time) time.Time) {
	r.journal.mu.Lock()
	defer r.journal.mu.Unlock()
	r.journal.ledger.settleAt = settleAt
}

// cash is the settled cash an entry may spend. limited is false when the catalog sets no starting
// capital: then there is no account to run out, and nothing is enforced.
func (r *Runner) cash(now time.Time) (usd float64, limited bool) {
	a := r.Account(now)
	return a.BuyingPowerUSD, a.CapitalUSD > 0
}
