package plays

import (
	"math"
)

const (
	RULE_SIZE        = "size_impact" // a size the market can actually fill and take back
	P_MAX_PCT_VOLUME = "max_pct_of_volume"
	P_MAX_PCT_OI     = "max_pct_of_open_interest"
	// Fill model: up to SLIP_FREE of the day's contract volume fills at the mid; by SLIP_FULL it
	// fills at the ask. A model, not a book: Yahoo gives no depth.
	SLIP_FREE        = 0.05
	SLIP_FULL        = 0.25
	SLIDER_MIN_SPAN  = 10    // the slider reaches ten minimum deposits or the bankroll, whichever is more; the liquidity cap is only a mark on it
	SLIDER_MAX_UNITS = 10000 // and never past this many units
)

// Sizing is the play's stake: the minimum deposit, where the slider can go, and what the size
// does to the fill.
type Sizing struct {
	UnitCostUSD     float64 `json:"unit_cost_usd"`   // one contract (or share) at the entry
	MinUnits        float64 `json:"min_units"`       // one unit for every rung and the runner
	MinDepositUSD   float64 `json:"min_deposit_usd"` // the smallest stake that runs the whole bracket
	Units           float64 `json:"units"`
	StakeUSD        float64 `json:"stake_usd"`
	SliderMax       float64 `json:"slider_max"`       // units at the top of the stake slider
	PctOfVolume     float64 `json:"pct_of_volume"`    // the order against today's contract (or share) volume
	PctOfOI         float64 `json:"pct_of_oi"`        // against open interest, options only
	EstEntry        float64 `json:"est_entry"`        // the fill the size model expects
	EstSlipUSD      float64 `json:"est_slip_usd"`     // what that costs over the limit price
	LiquidityMax    float64 `json:"liquidity_max"`    // units the size rule allows, when LiquidityCapped
	LiquidityCapped bool    `json:"liquidity_capped"` // the size rule is on and has data
}

// parts are the bracket's pieces in order: each ladder rung, then the runner if it has units.
func parts(p Play) []float64 {
	out := make([]float64, 0, len(p.Ladder)+1)
	for _, r := range p.Ladder {
		out = append(out, r.Qty)
	}
	if p.Runner.Qty > 0 {
		out = append(out, p.Runner.Qty)
	}
	return out
}

// Resize scales a play to qty units, keeping the ladder's and runner's shares of the position and
// at least one unit in every piece. Prices, stop and failsafes stay; every dollar figure follows.
func Resize(p Play, qty float64) Play {
	if p.Contract.Market == MARKET_CRYPTO {
		return resizeCoins(p, qty)
	}
	pieces := parts(p)
	if len(pieces) == 0 {
		p.Qty = math.Max(1, math.Floor(qty))
		return p
	}
	qty = math.Max(float64(len(pieces)), math.Floor(qty))
	total := 0.0
	for _, x := range pieces {
		total += x
	}
	sized := make([]float64, len(pieces))
	sum, biggest := 0.0, 0
	for i, x := range pieces {
		share := 1 / float64(len(pieces))
		if total > 0 {
			share = x / total
		}
		sized[i] = math.Max(1, math.Floor(qty*share))
		sum += sized[i]
		if sized[i] > sized[biggest] {
			biggest = i
		}
	}
	// Rounding leftovers (or overshoot from the one-unit floor) go to or come off the biggest piece.
	sized[biggest] = math.Max(1, sized[biggest]+qty-sum)
	ladder := make([]Rung, len(p.Ladder))
	for i, r := range p.Ladder {
		ladder[i] = Rung{Qty: sized[i], Price: r.Price}
	}
	p.Ladder = ladder
	if p.Runner.Qty > 0 {
		p.Runner.Qty = sized[len(sized)-1]
	}
	p.Qty = 0
	for _, x := range sized {
		p.Qty += x
	}
	return p
}

// sizing works out the stake numbers for a play as it stands.
func sizing(p Play, r Rules) Sizing {
	var m Market
	if p.Market != nil {
		m = *p.Market
	}
	unit := p.Entry * p.Multiplier()
	s := Sizing{UnitCostUSD: math.Round(unit*CENT) / CENT, MinUnits: math.Max(1, float64(len(parts(p)))), Units: p.Qty}
	if desk := r.Crypto; p.Contract.Market == MARKET_CRYPTO && desk != nil && unit > 0 {
		// Coins are bought in fractions: the smallest stake is the desk's minimum trade, not one coin a rung.
		s.MinUnits = math.Ceil(desk.MinTradeUSD/unit*CRYPTO_QTY_SCALE) / CRYPTO_QTY_SCALE
	}
	s.MinDepositUSD = math.Round(s.MinUnits*unit*CENT) / CENT
	s.StakeUSD = math.Round(p.Qty*unit*CENT) / CENT

	s.EstEntry = p.Entry
	if m.Volume > 0 {
		s.PctOfVolume = math.Round(p.Qty/float64(m.Volume)*PERCENT*10) / 10
		mid := (m.Bid + m.Ask) / 2
		if mid > 0 && m.Ask > mid {
			slip := math.Max(0, math.Min(1, (p.Qty/float64(m.Volume)-SLIP_FREE)/(SLIP_FULL-SLIP_FREE)))
			s.EstEntry = math.Max(p.Entry, math.Round((mid+slip*(m.Ask-mid))*CENT)/CENT)
		}
	}
	if p.Contract.Market == MARKET_OPTION && m.OpenInterest > 0 {
		s.PctOfOI = math.Round(p.Qty/float64(m.OpenInterest)*PERCENT*10) / 10
	}
	s.EstSlipUSD = math.Round((s.EstEntry-p.Entry)*p.Qty*p.Multiplier()*CENT) / CENT

	byVolume, byOI := r.param(RULE_SIZE, P_MAX_PCT_VOLUME), r.param(RULE_SIZE, P_MAX_PCT_OI)
	caps := []float64{}
	if byVolume > 0 && m.Volume > 0 {
		caps = append(caps, math.Floor(float64(m.Volume)*byVolume/PERCENT))
	}
	if byOI > 0 && p.Contract.Market == MARKET_OPTION && m.OpenInterest > 0 {
		caps = append(caps, math.Floor(float64(m.OpenInterest)*byOI/PERCENT))
	}
	for i, c := range caps {
		if i == 0 || c < s.LiquidityMax {
			s.LiquidityMax = c
		}
	}
	s.LiquidityCapped = len(caps) > 0
	top := s.MinUnits * SLIDER_MIN_SPAN
	if unit > 0 && r.BankrollUSD > 0 {
		top = math.Max(top, math.Floor(r.BankrollUSD/unit))
	}
	s.SliderMax = math.Min(SLIDER_MAX_UNITS, math.Max(top, p.Qty))
	return s
}
