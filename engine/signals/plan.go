package signals

import (
	"errors"
	"fmt"
	"math"
	"time"
	_ "time/tzdata" // market hours resolve the same on Windows, Linux and distroless
)

const (
	OPTION_MULTIPLIER = 100
	SHARE_MULTIPLIER  = 1
	PRICE_TICK        = 0.01
	PERCENT           = 100.0
	MARKET_TZ         = "America/New_York"
	MARKET_CLOSE_HOUR = 16
)

// MARKET is where US stock and option sessions are clocked.
var MARKET = func() *time.Location {
	loc, err := time.LoadLocation(MARKET_TZ)
	if err != nil {
		panic(err)
	}
	return loc
}()

// Rung is one ladder exit: sell PctOfPosition of the position at AtGainPct above entry.
type Rung struct {
	PctOfPosition float64 `json:"pct_of_position"`
	AtGainPct     float64 `json:"at_gain_pct"`
}

// Settings are a source's trade rules. Every signal from that source can override them.
type Settings struct {
	StopLossPct      float64 `json:"stop_loss_pct"`
	Ladder           []Rung  `json:"ladder"`
	FollowSourceExit bool    `json:"follow_source_exit"`
	FollowSourceTrim bool    `json:"follow_source_trim"`
	FollowSourceAdd  bool    `json:"follow_source_add"`
	MaxChasePct      float64 `json:"max_chase_pct"`
	MinConfidence    float64 `json:"min_confidence"`
	MaxUSDPerTrade   float64 `json:"max_usd_per_trade"`

	// Liquidity gate, checked when a live quote exists. Zero turns a rule off.
	MaxSpreadPct    float64 `json:"max_spread_pct"`    // bid-ask spread as a percent of the mid
	MinOpenInterest int     `json:"min_open_interest"` // options: contracts open
	MinVolume       int     `json:"min_volume"`        // traded today

	// Time rules. Zero turns a rule off.
	MaxSignalAgeSec        int      `json:"max_signal_age_sec"`        // skip callouts older than this when we see them
	MaxHoldMinutes         int      `json:"max_hold_minutes"`          // close whatever is left after this long
	TimeStop               TimeStop `json:"time_stop"`                 // not working yet? get out
	ExitDaysBeforeExpiry   int      `json:"exit_days_before_expiry"`   // options: dodge the last days of time decay
	ExitMinutesBeforeClose int      `json:"exit_minutes_before_close"` // options: out before the expiry-day close
}

// TimeStop closes the position if it isn't up MinGainPct after AfterMinutes.
type TimeStop struct {
	AfterMinutes int     `json:"after_minutes"`
	MinGainPct   float64 `json:"min_gain_pct"`
}

// Target is one resting limit sell.
type Target struct {
	Qty   int     `json:"qty"`
	Price float64 `json:"price"`
}

// Plan is the whole order, entry and exits together. It is never sent without its exits.
type Plan struct {
	Signal  Signal   `json:"signal"`
	Qty     int      `json:"qty"`
	Entry   float64  `json:"entry"`
	Stop    float64  `json:"stop"`
	Targets []Target `json:"targets"`
	Runner  int      `json:"runner"`
	CostUSD float64  `json:"cost_usd"`

	ExitBy        time.Time `json:"exit_by"`                   // hard time exit for everything left
	TimeStopAt    time.Time `json:"time_stop_at,omitzero"`     // check point for the time stop
	TimeStopAbove float64   `json:"time_stop_above,omitempty"` // must be above this at TimeStopAt, or close
}

var (
	ErrNotEntry       = errors.New("not an entry")
	ErrLowConfidence  = errors.New("confidence below source minimum")
	ErrNoPrice        = errors.New("no called price and no live quote")
	ErrChased         = errors.New("price already ran past max chase")
	ErrNoExits        = errors.New("no stop or no take-profit: no exits, no trade")
	ErrBudgetTooSmall = errors.New("max_usd_per_trade buys less than one unit")
	ErrStale          = errors.New("callout is older than max_signal_age_sec")
	ErrNoTime         = errors.New("no time left before the expiry exit")
)

// Build turns an entry signal into a full bracket, price exits and time exits together.
// quote is the live price, or 0 when unknown. now is when we would place the order.
func Build(sig Signal, s Settings, quote float64, now time.Time) (Plan, error) {
	if sig.Kind != KIND_ENTRY {
		return Plan{}, ErrNotEntry
	}
	if sig.Confidence < s.MinConfidence {
		return Plan{}, ErrLowConfidence
	}
	if s.StopLossPct <= 0 || s.StopLossPct >= PERCENT || len(s.Ladder) == 0 {
		return Plan{}, ErrNoExits
	}
	if s.MaxSignalAgeSec > 0 && now.Sub(sig.SeenAt) > time.Duration(s.MaxSignalAgeSec)*time.Second {
		return Plan{}, ErrStale
	}

	entry := quote
	if entry <= 0 {
		entry = sig.Price
	}
	if entry <= 0 {
		return Plan{}, ErrNoPrice
	}
	if sig.Price > 0 && entry > sig.Price*(1+s.MaxChasePct/PERCENT) {
		return Plan{}, fmt.Errorf("%w: called %.2f, now %.2f", ErrChased, sig.Price, entry)
	}

	multiplier := float64(SHARE_MULTIPLIER)
	if sig.Right != "" {
		multiplier = OPTION_MULTIPLIER
	}
	qty := int(s.MaxUSDPerTrade / (entry * multiplier))
	if qty < 1 {
		return Plan{}, ErrBudgetTooSmall
	}

	plan := Plan{
		Signal:  sig,
		Qty:     qty,
		Entry:   math.Round((entry)/PRICE_TICK) * PRICE_TICK,
		Stop:    math.Round((entry*(1-s.StopLossPct/PERCENT))/PRICE_TICK) * PRICE_TICK,
		CostUSD: math.Round((entry*multiplier*float64(qty))/PRICE_TICK) * PRICE_TICK,
	}
	left := qty
	for _, r := range s.Ladder {
		n := min(int(float64(qty)*r.PctOfPosition/PERCENT), left)
		if n < 1 {
			continue
		}
		plan.Targets = append(plan.Targets, Target{Qty: n, Price: math.Round((entry*(1+r.AtGainPct/PERCENT))/PRICE_TICK) * PRICE_TICK})
		left -= n
	}
	// Too few units to split: the whole position exits at the first rung, so a take-profit always exists.
	if len(plan.Targets) == 0 {
		plan.Targets = []Target{{Qty: qty, Price: math.Round((entry*(1+s.Ladder[0].AtGainPct/PERCENT))/PRICE_TICK) * PRICE_TICK}}
		left = 0
	}
	plan.Runner = left

	if s.MaxHoldMinutes > 0 {
		plan.ExitBy = now.Add(time.Duration(s.MaxHoldMinutes) * time.Minute)
	}
	if sig.Expiry != "" {
		day, err := time.ParseInLocation(EXPIRY_LAYOUT, sig.Expiry, MARKET)
		if err == nil {
			last := day.AddDate(0, 0, -s.ExitDaysBeforeExpiry).Add(MARKET_CLOSE_HOUR*time.Hour - time.Duration(s.ExitMinutesBeforeClose)*time.Minute)
			if plan.ExitBy.IsZero() || last.Before(plan.ExitBy) {
				plan.ExitBy = last
			}
		}
	}
	if !plan.ExitBy.IsZero() && !plan.ExitBy.After(now) {
		return Plan{}, ErrNoTime
	}
	if s.TimeStop.AfterMinutes > 0 {
		plan.TimeStopAt = now.Add(time.Duration(s.TimeStop.AfterMinutes) * time.Minute)
		plan.TimeStopAbove = math.Round((entry*(1+s.TimeStop.MinGainPct/PERCENT))/PRICE_TICK) * PRICE_TICK
	}
	return plan, nil
}
