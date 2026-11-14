package plays

import (
	"fmt"
	"math"
	"time"
)

const (
	LEVEL_BREAKEVEN = "breakeven"
	LEVEL_STOP      = "stop"
	LEVEL_RUNNER    = "runner"
	LEVEL_RUNG      = "T%d"
	SCENARIO_STOP   = "stop"     // stopped out before any rung
	SCENARIO_RUNGS  = "rungs:%d" // that many rungs sold, the rest stopped out
	SCENARIO_ALL    = "all"      // every rung and the runner's target
	SCENARIO_ZERO   = "zero"     // calamity: the stop never filled and the contract went to nothing
	// With no exit time on a share play, the odds look this far ahead.
	DEFAULT_HORIZON = 7 * 24 * time.Hour
)

// Level is one price the play cares about, and the stock price it takes to get there.
type Level struct {
	Name        string  `json:"name"`         // breakeven, T1..Tn, runner, stop
	Price       float64 `json:"price"`        // the contract's (or share's) price at this level
	Qty         float64 `json:"qty"`          // units sold here
	PnLUSD      float64 `json:"pnl_usd"`      // what selling Qty here makes or loses
	SpotNow     float64 `json:"spot_now"`     // stock price that gets the contract here right now
	SpotByExit  float64 `json:"spot_by_exit"` // stock price that gets it here at the exit time (time decay included)
	SpotExpiry  float64 `json:"spot_expiry"`  // stock price that gets it here at expiry: intrinsic value only
	MovePct     float64 `json:"move_pct"`     // stock move from now to SpotByExit, signed
	Reachable   bool    `json:"reachable"`
	ChancePct   float64 `json:"chance_pct"` // the market's odds of the stock touching SpotByExit before the exit time
	AlreadyPast bool    `json:"already_past,omitempty"`
}

// Scenario is one way the play can end and what it pays.
type Scenario struct {
	Name   string  `json:"name"`
	PnLUSD float64 `json:"pnl_usd"`
	PnLPct float64 `json:"pnl_pct"`
}

// Payoff is the whole play in dollars.
type Payoff struct {
	CostUSD        float64    `json:"cost_usd"`
	PlannedLossUSD float64    `json:"planned_loss_usd"` // every unit sold at the stop
	WorstLossUSD   float64    `json:"worst_loss_usd"`   // the stop never fills: the whole premium
	MaxProfitUSD   float64    `json:"max_profit_usd"`   // every rung and the runner's target
	RewardRisk     float64    `json:"reward_risk"`      // max profit over planned loss
	ExpectedUSD    float64    `json:"expected_usd"`     // rough: scenarios weighted by the market's touch odds
	Scenarios      []Scenario `json:"scenarios"`
	ExitAt         time.Time  `json:"exit_at,omitzero"` // when the odds stop counting: exit time or expiry
	Vol            float64    `json:"vol"`              // the volatility the odds used
	VolSource      string     `json:"vol_source"`       // implied, historical or none
}

// Report is everything derived from a play: dollars, levels, odds and the rule checks.
type Report struct {
	Payoff  Payoff  `json:"payoff"`
	Sizing  Sizing  `json:"sizing"`
	Levels  []Level `json:"levels"`
	Odds    Odds    `json:"odds"`
	Checks  []Check `json:"checks"`
	Armable bool    `json:"armable"`
}

// payoff works out the dollars and the levels. The levels say, for each exit, what the stock has
// to do: the price it must reach right now, by the exit time, and at expiry.
func payoff(p Play, now time.Time) (Payoff, []Level) {
	mult := p.Multiplier()
	out := Payoff{CostUSD: math.Round(p.Entry*p.Qty*mult*CENT) / CENT}
	out.PlannedLossUSD = math.Round(math.Max(0, p.Entry-p.Stop)*p.Qty*mult*CENT) / CENT
	out.WorstLossUSD = out.CostUSD

	var m Market
	if p.Market != nil {
		m = *p.Market
	}
	expiry, hasExpiry := expiryClose(p.Contract.Expiry)
	exitAt := p.Failsafes.ExitBy
	switch {
	case hasExpiry && (exitAt.IsZero() || exitAt.After(expiry)):
		exitAt = expiry
	case exitAt.IsZero():
		exitAt = now.Add(DEFAULT_HORIZON)
	}
	out.ExitAt = exitAt
	out.Vol, out.VolSource = m.IV, "none"
	if m.IV > 0 {
		out.VolSource = map[bool]string{true: "implied", false: "historical"}[p.Contract.Market == MARKET_OPTION]
	}
	window := yearsUntil(now, exitAt)
	call := p.Contract.Right != RIGHT_PUT
	option := p.Contract.Market == MARKET_OPTION && hasExpiry && p.Contract.Strike > 0

	level := func(name string, price, qty float64) Level {
		l := Level{Name: name, Price: price, Qty: qty, PnLUSD: math.Round((price-p.Entry)*qty*mult*CENT) / CENT, Reachable: true}
		if !option {
			l.SpotNow, l.SpotByExit, l.SpotExpiry = price, price, price
		} else {
			var okNow, okExit bool
			l.SpotNow, okNow = spotFor(price, p.Contract.Strike, yearsUntil(now, expiry), m.IV, call)
			l.SpotByExit, okExit = spotFor(price, p.Contract.Strike, yearsUntil(exitAt, expiry), m.IV, call)
			l.SpotExpiry, _ = spotFor(price, p.Contract.Strike, 0, 0, call)
			l.Reachable = okNow && okExit
		}
		if m.Spot <= 0 || !l.Reachable {
			return l
		}
		l.MovePct = math.Round((l.SpotByExit/m.Spot-1)*PERCENT*10) / 10
		// A level on the play's side of the stock is already there.
		ahead := l.SpotByExit >= m.Spot
		if name == LEVEL_STOP {
			ahead = !ahead
		}
		if ahead == call {
			l.ChancePct = math.Round(touchChance(m.Spot, l.SpotByExit, window, m.IV)*PERCENT*10) / 10
			return l
		}
		l.AlreadyPast, l.ChancePct = true, PERCENT
		return l
	}

	levels := []Level{level(LEVEL_BREAKEVEN, p.Entry, 0)}
	rungs := make([]Level, 0, len(p.Ladder)+1)
	for i, r := range p.Ladder {
		rungs = append(rungs, level(fmt.Sprintf(LEVEL_RUNG, i+1), r.Price, r.Qty))
	}
	if p.Runner.Qty > 0 && p.Runner.Target > 0 {
		rungs = append(rungs, level(LEVEL_RUNNER, p.Runner.Target, p.Runner.Qty))
	}
	levels = append(levels, rungs...)
	levels = append(levels, level(LEVEL_STOP, p.Stop, p.Qty))

	// Scenarios: k rungs sell, the rest goes out at the stop. Weighted by the touch odds of each rung.
	pct := func(usd float64) float64 {
		if out.CostUSD == 0 {
			return 0
		}
		return math.Round(usd / out.CostUSD * PERCENT)
	}
	stopLeg := (p.Stop - p.Entry) * mult
	left, banked := p.Qty, 0.0
	scenario := func(name string, usd float64) Scenario {
		usd = math.Round(usd*CENT) / CENT
		return Scenario{Name: name, PnLUSD: usd, PnLPct: pct(usd)}
	}
	out.Scenarios = []Scenario{scenario(SCENARIO_STOP, left*stopLeg)}
	prevChance := 1.0
	expected := 0.0
	for k, r := range rungs {
		chance := r.ChancePct / PERCENT
		expected += (prevChance - chance) * out.Scenarios[len(out.Scenarios)-1].PnLUSD
		prevChance = chance
		banked += r.PnLUSD
		left -= r.Qty
		name := fmt.Sprintf(SCENARIO_RUNGS, k+1)
		if k == len(rungs)-1 {
			name = SCENARIO_ALL
		}
		out.Scenarios = append(out.Scenarios, scenario(name, banked+math.Max(0, left)*stopLeg))
	}
	expected += prevChance * out.Scenarios[len(out.Scenarios)-1].PnLUSD
	out.Scenarios = append(out.Scenarios, scenario(SCENARIO_ZERO, -out.CostUSD))
	out.MaxProfitUSD = math.Round(banked*CENT) / CENT
	if out.PlannedLossUSD > 0 {
		out.RewardRisk = math.Round(out.MaxProfitUSD/out.PlannedLossUSD*10) / 10
	}
	if out.VolSource != "none" && m.Spot > 0 {
		out.ExpectedUSD = math.Round(expected*CENT) / CENT
	}
	return out, levels
}
