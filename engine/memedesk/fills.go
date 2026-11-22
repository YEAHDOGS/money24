package memedesk

import (
	"math"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	PUMP_FEE_RATE     = 0.0125
	PORTAL_FEE_RATE   = 0.005
	ADVERSE_SLIPPAGE  = 0.005
	MAX_BUY_SLIPPAGE  = 0.03
	SIMULATED_LATENCY = 3 * time.Second
	ENTRY_WINDOW      = 5 * time.Minute
	QUOTE_POOL_SHARE  = 0.5
	FRACTION_EPSILON  = 1e-12
	EXIT_STOP         = "hard_stop"
	EXIT_TIME         = "time_limit"
	EXIT_TARGET       = "take_profit"
	EXIT_NO_LIQUIDITY = "liquidity_vanished"
)

type Fill struct {
	At          time.Time `json:"at"`
	Reason      string    `json:"reason"`
	ObservedUSD float64   `json:"observed_usd"`
	PriceUSD    float64   `json:"price_usd"`
	Liquidity   float64   `json:"liquidity_usd"`
	Units       float64   `json:"units"`
	FeeUSD      float64   `json:"fee_usd"`
	Slippage    float64   `json:"slippage"`
	NetUSD      float64   `json:"net_usd"` // all-in cost on entry, net proceeds on exit
}

type Position struct {
	Strategy   string    `json:"strategy"`
	Mint       string    `json:"mint"`
	Event      string    `json:"event"`
	Buy        Fill      `json:"buy"`
	Sales      []Fill    `json:"sales,omitempty"`
	Units      float64   `json:"remaining_units"`
	Proceeds   float64   `json:"proceeds_usd"`
	MarkUSD    float64   `json:"mark_usd"`
	MarkAt     time.Time `json:"mark_at,omitzero"`
	MarkStale  bool      `json:"mark_stale"`
	ClosedAt   time.Time `json:"closed_at,omitzero"`
	PnLUSD     float64   `json:"pnl_usd"`
	policy     Exit
	nextRung   int
	exitReason string
	exitAt     time.Time
}

func positive(v float64) bool {
	return v > 0 && !math.IsInf(v, 0)
}

func price(m *meme.Market) float64 {
	if m == nil || m.PriceUSD == nil || !positive(*m.PriceUSD) {
		return 0
	}
	return *m.PriceUSD
}

func liquidity(m *meme.Market) float64 {
	if m == nil || m.Liquidity == nil || m.Liquidity.USD == nil || !positive(*m.Liquidity.USD) {
		return 0
	}
	return *m.Liquidity.USD
}

func buyFill(m *meme.Market, stake float64, at time.Time) (Fill, bool) {
	p, liq := price(m), liquidity(m)
	if p == 0 || liq == 0 || !positive(stake) {
		return Fill{}, false
	}
	beforeFee := stake / (1 + PUMP_FEE_RATE + PORTAL_FEE_RATE)
	multiplier := (1 + beforeFee/(liq*QUOTE_POOL_SHARE)) * (1 + ADVERSE_SLIPPAGE)
	units := beforeFee / (p * multiplier)
	if multiplier-1 > MAX_BUY_SLIPPAGE || !positive(units) {
		return Fill{}, false
	}
	return Fill{At: at, Reason: DECISION_BUY, ObservedUSD: p, PriceUSD: p * multiplier, Liquidity: liq,
		Units: units, FeeUSD: stake - beforeFee, Slippage: multiplier - 1, NetUSD: stake}, true
}

func sellFill(m *meme.Market, units float64, at time.Time, reason string) Fill {
	p, liq := price(m), liquidity(m)
	reserve := liq * QUOTE_POOL_SHARE
	gross := reserve / (1 + reserve/(units*p)) * (1 - ADVERSE_SLIPPAGE)
	fillPrice := gross / units
	fee := gross * (PUMP_FEE_RATE + PORTAL_FEE_RATE)
	return Fill{At: at, Reason: reason, ObservedUSD: p, PriceUSD: fillPrice,
		Liquidity: liq, Units: units, FeeUSD: fee, Slippage: 1 - fillPrice/p, NetUSD: gross - fee}
}

func newPosition(c *candidate, fill Fill) *Position {
	return &Position{Strategy: c.rule.config.ID, Mint: c.event.Mint, Event: c.event.Kind,
		Buy: fill, Units: fill.Units, policy: c.rule.config.Exit}
}

func (p *Position) mark(m *meme.Market, at time.Time) {
	p.MarkUSD, p.MarkAt, p.MarkStale = sellFill(m, p.Units, at, "").NetUSD, at, false
	p.PnLUSD = p.Proceeds + p.MarkUSD - p.Buy.NetUSD
}

func (p *Position) sell(fill Fill) {
	p.Units -= fill.Units
	p.Proceeds += fill.NetUSD
	p.Sales = append(p.Sales, fill)
	if p.Units == 0 {
		p.ClosedAt = fill.At
		p.MarkUSD, p.MarkAt, p.MarkStale = 0, fill.At, false
		p.PnLUSD = p.Proceeds - p.Buy.NetUSD
	}
}

func (p *Position) update(row meme.Record, at time.Time) {
	if !p.ClosedAt.IsZero() || !quoteAfter(row, row.At, p.Buy.At) {
		return
	}
	if row.Market == nil || (row.Market.Liquidity != nil && row.Market.Liquidity.USD != nil && *row.Market.Liquidity.USD == 0) {
		p.sell(Fill{At: at, Units: p.Units, Reason: EXIT_NO_LIQUIDITY, ObservedUSD: price(row.Market)})
		return
	}
	p.triggerExit(row, at)
	if price(row.Market) == 0 || liquidity(row.Market) == 0 {
		p.MarkStale = true
		return
	}
	p.execute(row, at)
}

func (p *Position) triggerExit(row meme.Record, at time.Time) {
	if p.exitReason == EXIT_STOP || p.exitReason == EXIT_TIME {
		return
	}
	deadline := p.Buy.At.Add(time.Duration(p.policy.HoldSeconds) * time.Second)
	if !at.Before(deadline) {
		p.exitReason, p.exitAt = EXIT_TIME, deadline
		return
	}
	if price(row.Market) > 0 && price(row.Market) <= p.Buy.PriceUSD*(1-p.policy.StopPct) {
		p.exitReason, p.exitAt = EXIT_STOP, at
		return
	}
	if p.exitReason != "" {
		return
	}
	if p.nextRung < len(p.policy.TakeProfit) &&
		price(row.Market) >= p.Buy.PriceUSD*(1+p.policy.TakeProfit[p.nextRung].Gain) {
		p.exitReason, p.exitAt = EXIT_TARGET, at
	}
}

func (p *Position) execute(row meme.Record, at time.Time) {
	if p.exitReason == "" || !quoteAfter(row, row.At, p.exitAt.Add(SIMULATED_LATENCY)) {
		p.mark(row.Market, at)
		return
	}
	if p.exitReason == EXIT_TARGET {
		rung := p.policy.TakeProfit[p.nextRung]
		p.nextRung++
		units := min(p.Units, p.Buy.Units*rung.Fraction)
		if p.nextRung == len(p.policy.TakeProfit) && p.Units-units <= p.Buy.Units*FRACTION_EPSILON {
			units = p.Units
		}
		p.sell(sellFill(row.Market, units, at, EXIT_TARGET))
		p.exitReason = ""
		if p.Units > 0 {
			p.mark(row.Market, at)
			p.triggerExit(row, at)
		}
		return
	}
	p.sell(sellFill(row.Market, p.Units, at, p.exitReason))
}

func quoteAfter(row meme.Record, at, ready time.Time) bool {
	return !at.Before(ready) && (row.RequestedAt.IsZero() || !row.RequestedAt.Before(ready))
}
