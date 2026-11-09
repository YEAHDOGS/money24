package signals

import (
	"errors"
	"fmt"
	"math"
)

const (
	OPTION_MULTIPLIER = 100
	SHARE_MULTIPLIER  = 1
	PRICE_TICK        = 0.01
	PERCENT           = 100.0
)

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
}

var (
	ErrNotEntry       = errors.New("not an entry")
	ErrLowConfidence  = errors.New("confidence below source minimum")
	ErrNoPrice        = errors.New("no called price and no live quote")
	ErrChased         = errors.New("price already ran past max chase")
	ErrNoExits        = errors.New("no stop or no take-profit: no exits, no trade")
	ErrBudgetTooSmall = errors.New("max_usd_per_trade buys less than one unit")
)

// Build turns an entry signal into a full bracket. quote is the live price, or 0 when unknown.
func Build(sig Signal, s Settings, quote float64) (Plan, error) {
	if sig.Kind != KIND_ENTRY {
		return Plan{}, ErrNotEntry
	}
	if sig.Confidence < s.MinConfidence {
		return Plan{}, ErrLowConfidence
	}
	if s.StopLossPct <= 0 || s.StopLossPct >= PERCENT || len(s.Ladder) == 0 {
		return Plan{}, ErrNoExits
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
	return plan, nil
}
