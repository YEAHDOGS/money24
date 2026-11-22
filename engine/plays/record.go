package plays

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	// The markets a play can be filed under, as the record names them.
	RECORD_STOCK      = "stock"
	RECORD_OPTIONS    = "options"
	RECORD_FUTURES    = "futures"
	RECORD_FOREX      = "forex"
	RECORD_CRYPTO     = "crypto"
	RECORD_PREDICTION = "prediction"

	// Why something other than the price stop closed a play.
	OVERRIDE_VOLATILE = "extremely_volatile" // a gap or a halt: flattened on the calamity plan
	OVERRIDE_TIME     = "time_sl"            // the clock: the time exit or the sideways exit
	REASON_TIME       = "time"
	REASON_TIME_STOP  = "time stop"
	REASON_GAP        = "gap"
	REASON_STOP       = "stop"
	REASON_EXPIRY     = "expiry"
	REASON_OPEN       = "open"

	EXITS_SHOWN      = 3 // exit 1, exit 2, exit 3
	STOPS_SHOWN      = 3 // stop-loss 1, 2, 3
	EVENT_ENTRY      = "entry"
	EVENT_EXIT       = "exit%d"
	CURVE_MAX_POINTS = 240        // the page draws this many at most; longer timelines are thinned
	SOURCE_CONTRACT  = "contract" // replayed over the contract's own traded prices, one candle a day
	SOURCE_MODEL     = "model"    // replayed over prices worked out from the stock's candles
	BACKTEST_LAYOUT  = "2006-01-02"
	CATEGORY_MAX     = 40
)

// Record is a play in one fixed format, the same for every play, so plays can be compared.
type Record struct {
	Market         string    `json:"market"` // stock, options, futures, forex, crypto or prediction
	Category       string    `json:"category"`
	ExpectedEntry  float64   `json:"expected_entry"`      // the price the play planned to pay
	ActualEntry    float64   `json:"actual_entry"`        // the price it filled at; zero until it runs
	ExpectedAmount float64   `json:"expected_amount_usd"` // what the play planned to put in
	ActualAmount   float64   `json:"actual_amount_usd"`   // what it actually put in; zero until it runs
	Exits          []float64 `json:"exits"`               // exit 1, 2 and 3: the ladder, then the runner's target
	Stops          []float64 `json:"stops"`               // stop-loss 1, 2 and 3: the stop before any exit, after exit 1, after exit 2
	StopsByRule    bool      `json:"stops_by_rule"`       // stop-losses 2 and 3 come from the stop ladder rule, not from the play
	StopOverride   string    `json:"stop_override"`       // extremely_volatile or time_sl when that, not the stop, closed it
}

// exits are the prices the play sells at, in order: each rung, then the runner's target.
func (p Play) exits() []float64 {
	out := []float64{}
	for _, r := range p.Ladder {
		out = append(out, r.Price)
	}
	if p.Runner.Qty > 0 && p.Runner.Target > 0 {
		out = append(out, p.Runner.Target)
	}
	return out
}

// stopsAfter is where the stop moves as exits fill: the play's own stop-losses 2 and 3, or with
// the stop ladder rule on, the entry after exit 1 and exit 1's price after exit 2. Nil: it never moves.
func (p Play) stopsAfter(r Rules) []float64 {
	if len(p.StopsAfter) > 0 {
		return p.StopsAfter
	}
	exits := p.exits()
	if !r.rule(RULE_STOPS).Enabled || len(exits) == 0 || p.Entry <= p.Stop {
		return nil
	}
	return []float64{p.Entry, max(p.Entry, exits[0])}
}

// stops is every stop-loss in order: the play's stop, then where it moves as exits fill.
func (p Play) stops(r Rules) []float64 {
	return append([]float64{p.Stop}, p.stopsAfter(r)...)
}

// stopsLabel is "0.07 → 0.13 → 0.26".
func stopsLabel(stops []float64) string {
	parts := make([]string, len(stops))
	for i, s := range stops {
		parts[i] = fmt.Sprintf("%.2f", s)
	}
	return strings.Join(parts, " → ")
}

// firstN is the first n values, padded with zeros: a record always has the same rows.
func firstN(values []float64, n int) []float64 {
	out := make([]float64, n)
	copy(out, values)
	return out
}

func recordMarket(c Contract) string {
	switch c.Market {
	case MARKET_OPTION:
		return RECORD_OPTIONS
	case MARKET_STOCK, "":
		return RECORD_STOCK
	}
	return c.Market
}

// record files the play in the fixed format.
func record(p Play, r Rules) Record {
	rec := Record{
		Market:         recordMarket(p.Contract),
		Category:       p.Category,
		ExpectedEntry:  p.Entry,
		ExpectedAmount: math.Round(p.Entry*p.Qty*p.Multiplier()*CENT) / CENT,
		Exits:          firstN(p.exits(), EXITS_SHOWN),
		Stops:          firstN(p.stops(r), STOPS_SHOWN),
		StopsByRule:    len(p.StopsAfter) == 0,
	}
	switch {
	case p.Filled != nil:
		rec.ActualEntry, rec.ActualAmount = p.Filled.Price, p.Filled.AmountUSD
	case p.Status != STATUS_DRAFT:
		// Armed before fills were recorded: an entry filled whole, at its limit.
		rec.ActualEntry, rec.ActualAmount = rec.ExpectedEntry, rec.ExpectedAmount
	}
	if p.Result == nil {
		return rec
	}
	switch p.Result.Reason {
	case REASON_TIME, REASON_TIME_STOP:
		rec.StopOverride = OVERRIDE_TIME
	case REASON_GAP:
		rec.StopOverride = OVERRIDE_VOLATILE
	}
	return rec
}

// CurvePoint is one moment of the play: the stock, the contract, and what the play was worth.
type CurvePoint struct {
	T      int64   `json:"t"` // Unix seconds
	Spot   float64 `json:"spot"`
	Price  float64 `json:"price"`   // the contract's (or share's) price
	Stop   float64 `json:"stop"`    // the stop in force
	PnLUSD float64 `json:"pnl_usd"` // banked so far plus what is still open, marked at Price
}

// CurveEvent is something the play did on the way: the entry, an exit, the stop, the clock.
type CurveEvent struct {
	T      int64   `json:"t"`
	Kind   string  `json:"kind"` // entry, exit1..n, stop, time, expiry
	Qty    float64 `json:"qty"`
	Price  float64 `json:"price"`
	PnLUSD float64 `json:"pnl_usd"`
}

// Curve is the play replayed over the whole timeline of prices since its entry: how its exit
// ladder and stop-losses would have played out, candle by candle.
type Curve struct {
	Modelled bool         `json:"modelled"` // contract prices are worked out from the stock's candles, not traded prices
	Source   string       `json:"source"`   // contract (its own daily candles) or model
	Backtest bool         `json:"backtest"` // entered at a chosen day's close, not at the play's own entry
	HoldPct  float64      `json:"hold_pct"` // entry to the last price with no exits and no stop: buy and hold
	Entry    float64      `json:"entry"`
	Exits    []float64    `json:"exits"`
	Stops    []float64    `json:"stops"`
	Points   []CurvePoint `json:"points"`
	Events   []CurveEvent `json:"events"`
	Reason   string       `json:"reason"` // how the replay ended; "open" while it has not
	PnLUSD   float64      `json:"pnl_usd"`
	PnLPct   float64      `json:"pnl_pct"`
	From     int64        `json:"from"`
	To       int64        `json:"to"`
}

// replay is one play being walked through its candles.
type replay struct {
	play     Play
	stops    []float64
	targets  []Rung
	left     float64
	next     int
	realized float64
	curve    *Curve
	direct   bool // the candles are the play's own prices: nothing to work out
}

func (rp *replay) stop() float64 { return rp.stops[min(rp.next, len(rp.stops)-1)] }

func (rp *replay) sell(t int64, kind string, qty, price float64) {
	gain := math.Round((price-rp.play.Entry)*qty*rp.play.Multiplier()*CENT) / CENT
	rp.left, rp.realized = rp.left-qty, rp.realized+gain
	rp.curve.Events = append(rp.curve.Events, CurveEvent{T: t, Kind: kind, Qty: qty, Price: price, PnLUSD: gain})
}

func (rp *replay) closeAll(t int64, reason string, price float64) {
	rp.sell(t, reason, rp.left, price)
	rp.curve.Reason = reason
}

// priceAt is the play's own price when the stock is at spot. Options are priced by Black-Scholes at
// the volatility the play was planned on, with the time left at that moment.
func (rp *replay) priceAt(spot float64, at time.Time) float64 {
	c := rp.play.Contract
	if rp.direct || c.Market != MARKET_OPTION {
		return spot
	}
	closeAt, ok := expiryClose(c.Expiry)
	if !ok || rp.play.Market == nil {
		return 0
	}
	return optionPrice(spot, c.Strike, yearsUntil(at, closeAt), rp.play.Market.IV, c.Right == RIGHT_CALL)
}

// Replay walks the play through the stock's candles from its entry. Within a candle the stop is
// judged before the exits, so a candle that touched both counts as stopped: the cautious reading.
// Exits fill at their own price, the stop at the stop's price, the clock at that candle's open.
func Replay(p Play, r Rules, candles []market.Candle, now time.Time) Curve {
	from := p.ArmedAt
	if from.IsZero() {
		from = p.CreatedAt
	}
	return replayFrom(p, r, candles, from, now, false)
}

// Backtest enters the play at the close of a chosen day and replays it from there with the same
// bracket in proportion: a stop half way down stays half way down, whatever the entry was.
// It runs over the contract's own daily candles when there are any (real traded prices, one
// candle a day), otherwise over prices worked out from the stock's candles.
func Backtest(p Play, r Rules, day time.Time, contract, stock []market.Candle, now time.Time) (Curve, error) {
	closed := day.Add(24 * time.Hour) // candles are stamped at their open; the day ends a day later
	entry, direct := 0.0, false
	for _, c := range contract {
		if c.T < closed.Unix() && c.C > 0 {
			entry, direct = c.C, true
		}
	}
	candles := contract
	if !direct {
		candles = stock
		probe := &replay{play: p}
		for _, c := range stock {
			if c.T < closed.Unix() {
				entry = probe.priceAt(c.C, time.Unix(c.T, 0))
			}
		}
	}
	if entry <= 0 || p.Entry <= 0 {
		return Curve{}, fmt.Errorf("no price on %s to enter at", day.Format(BACKTEST_LAYOUT))
	}
	curve := replayFrom(p.scaled(entry/p.Entry), r, candles, closed, now, direct)
	curve.Backtest = true
	return curve, nil
}

// scaled is the play with every price of its bracket multiplied by k, so it can be entered at another price.
func (p Play) scaled(k float64) Play {
	tick := func(v float64) float64 { return math.Round(v*k*CENT) / CENT }
	q := p
	q.Entry, q.Stop, q.Runner.Target = tick(p.Entry), tick(p.Stop), tick(p.Runner.Target)
	q.Ladder, q.StopsAfter = make([]Rung, len(p.Ladder)), nil
	for i, rung := range p.Ladder {
		q.Ladder[i] = Rung{Qty: rung.Qty, Price: tick(rung.Price)}
	}
	for _, s := range p.StopsAfter {
		q.StopsAfter = append(q.StopsAfter, tick(s))
	}
	return q
}

func replayFrom(p Play, r Rules, candles []market.Candle, from, now time.Time, direct bool) Curve {
	curve := Curve{Modelled: !direct && p.Contract.Market == MARKET_OPTION, Source: SOURCE_MODEL, Entry: p.Entry, Exits: p.exits(), Stops: p.stops(r),
		Reason: REASON_OPEN, From: from.Unix(), To: now.Unix(), Points: []CurvePoint{}, Events: []CurveEvent{}}
	if direct {
		curve.Source = SOURCE_CONTRACT
	}
	rp := &replay{play: p, stops: curve.Stops, left: p.Qty, curve: &curve, direct: direct}
	rp.targets = append(rp.targets, p.Ladder...)
	if p.Runner.Qty > 0 && p.Runner.Target > 0 {
		rp.targets = append(rp.targets, Rung{Qty: p.Runner.Qty, Price: p.Runner.Target})
	}
	curve.Events = append(curve.Events, CurveEvent{T: from.Unix(), Kind: EVENT_ENTRY, Qty: p.Qty, Price: p.Entry})
	call := p.Contract.Right != RIGHT_PUT
	exitBy := p.Failsafes.ExitBy

	for _, c := range candles {
		at := time.Unix(c.T, 0)
		if at.Before(from) || rp.left <= 0 {
			continue
		}
		// A call is dearest at the candle's high; a put at its low.
		best, worst := c.H, c.L
		if !call && !direct {
			best, worst = c.L, c.H
		}
		high, low, last := rp.priceAt(best, at), rp.priceAt(worst, at), rp.priceAt(c.C, at)
		switch {
		case !exitBy.IsZero() && !at.Before(exitBy):
			rp.closeAll(c.T, REASON_TIME, rp.priceAt(c.O, at))
		case low <= rp.stop():
			rp.closeAll(c.T, REASON_STOP, rp.stop())
		default:
			for rp.next < len(rp.targets) && rp.left > 0 && high >= rp.targets[rp.next].Price {
				t := rp.targets[rp.next]
				rp.next++
				rp.sell(c.T, fmt.Sprintf(EVENT_EXIT, rp.next), min(t.Qty, rp.left), t.Price)
			}
			if rp.left <= 0 {
				curve.Reason = REASON_TARGET
			}
		}
		open := math.Round((last-p.Entry)*rp.left*p.Multiplier()*CENT) / CENT
		curve.Points = append(curve.Points, CurvePoint{T: c.T, Spot: c.C, Price: math.Round(last*CENT*CENT) / (CENT * CENT), Stop: rp.stop(), PnLUSD: rp.realized + open})
	}
	// Still holding past expiry: what is left is worth what it is in the money by.
	if closeAt, ok := expiryClose(p.Contract.Expiry); ok && rp.left > 0 && now.After(closeAt) && len(candles) > 0 {
		rp.closeAll(closeAt.Unix(), REASON_EXPIRY, rp.priceAt(candles[len(candles)-1].C, closeAt))
	}
	if n := len(candles); n > 0 && p.Entry > 0 {
		last := candles[n-1]
		curve.HoldPct = math.Round((rp.priceAt(last.C, time.Unix(last.T, 0)) - p.Entry) / p.Entry * PERCENT)
	}
	curve.PnLUSD = rp.realized
	if n := len(curve.Points); n > 0 && rp.left > 0 {
		curve.PnLUSD = curve.Points[n-1].PnLUSD
	}
	if cost := p.Entry * p.Qty * p.Multiplier(); cost > 0 {
		curve.PnLPct = math.Round(curve.PnLUSD / cost * PERCENT)
	}
	curve.Points = thin(curve.Points, CURVE_MAX_POINTS)
	return curve
}

// thin keeps at most n points, evenly spaced, always with the last one.
func thin(points []CurvePoint, n int) []CurvePoint {
	if len(points) <= n {
		return points
	}
	out := make([]CurvePoint, 0, n)
	step := float64(len(points)-1) / float64(n-1)
	for i := range n {
		out = append(out, points[int(math.Round(float64(i)*step))])
	}
	return out
}
