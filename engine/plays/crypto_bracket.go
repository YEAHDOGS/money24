package plays

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/YEAHDOGS/money24/engine/calendar"
	"github.com/YEAHDOGS/money24/engine/coinbase"
)

const (
	WHY_NO_STOP     = "no room between the entry and the stop"
	WHY_TOO_SMALL   = "the account can put $%.2f in (equity $%.2f, settled cash $%.2f), under the $%.2f minimum"
	NOTE_CANDLES    = "candles: "
	NOTE_QUOTE      = "quote: "
	THESIS_CRYPTO   = "%s closed the hour at %g, above its %d-hour high of %g, on %.1fx the volume of the hours before it, with its 20-hour average over its 50-hour. The stop is %.1f%% under the ask. Every price is after a %.2f%% fee a side."
	EVIDENCE_VOLUME = "Breakout hour traded %.1fx the %d hours before it"
	EVIDENCE_BREAK  = "Hourly close %g cleared the %d-hour high %g"
	EVIDENCE_TREND  = "Close above the 20-hour average %g, which is above the 50-hour %g"
)

var errNoCandles = errors.New("no completed hourly candles")

// cryptoBracket is a coin play's prices. Every one is after the fee: the entry is what a buy
// really costs, and the stop and exits are what a sell really pays.
type cryptoBracket struct {
	Entry   float64
	Stop    float64
	Rungs   []float64 // the ladder's prices, nearest first
	Runner  float64   // the runner's target, 0 when it rides to the stop or the clock
	R       float64   // entry minus stop: what one coin loses at the stop
	StopPct float64   // the stop's distance under the ask before fees, in percent
}

// bracket sets the stop from the coin's hourly range, clamped, and every exit a multiple of the
// risk above the entry. ask is the raw price; feePct is charged on each side.
func (f CryptoRules) bracket(ask, atr, feePct float64) cryptoBracket {
	fee := feePct / PERCENT
	b := f.bracketAt(ask, atr, ask*(1+fee), 1-fee)
	return b
}

// bracketAt takes the cost per credited coin and the fraction of gross exit proceeds kept.
func (f CryptoRules) bracketAt(ask, atr, entry, netExit float64) cryptoBracket {
	distance := math.Min(math.Max(f.StopATR*atr, ask*f.MinStopPct/PERCENT), ask*f.MaxStopPct/PERCENT)
	b := cryptoBracket{Entry: coinPrice(entry), Stop: coinPrice((ask - distance) * netExit), StopPct: distance / ask * PERCENT}
	b.R = b.Entry - b.Stop
	for _, r := range f.LadderR {
		b.Rungs = append(b.Rungs, coinPrice(b.Entry+r*b.R))
	}
	if f.RunnerR > 0 {
		b.Runner = coinPrice(b.Entry + f.RunnerR*b.R)
	}
	return b
}

// size is how many coins a cash account buys: what loses RiskPct of the account at the stop, capped
// at MaxPositionPct of the account and at the cash that has settled. Weekends trade smaller. When
// that is too little to bother with, why says so.
func (f CryptoRules) size(b cryptoBracket, funds Funds, now time.Time) (qty float64, why string) {
	if b.R <= 0 || b.Entry <= 0 {
		return 0, WHY_NO_STOP
	}
	share := 1.0
	// A day the stock session is shut (a weekend, a holiday) is a day banks and CME are shut too: thin books.
	if f.WeekendSizePct > 0 && calendar.On(now).Session == calendar.SESSION_CLOSED {
		share = f.WeekendSizePct / PERCENT
	}
	atRisk := funds.EquityUSD * f.RiskPct / PERCENT * share / b.R * b.Entry
	usd := math.Min(atRisk, math.Min(funds.EquityUSD*f.MaxPositionPct/PERCENT*share, funds.BuyingPowerUSD))
	// Whole cents, rounded down, so the cost never passes what the account has by a rounding.
	usd = math.Floor(usd*CENT) / CENT
	if usd < f.MinTradeUSD || usd <= 0 {
		return 0, fmt.Sprintf(WHY_TOO_SMALL, math.Max(0, usd), funds.EquityUSD, funds.BuyingPowerUSD, f.MinTradeUSD)
	}
	return math.Floor(usd/b.Entry*CRYPTO_QTY_SCALE) / CRYPTO_QTY_SCALE, ""
}

// pieces splits qty across the ladder and the runner in whole hundred-millionths of a coin, so the
// exits add up to exactly what was bought. The runner takes what the equal shares leave over.
func (f CryptoRules) pieces(qty float64) (rungs []float64, runner float64) {
	units := int64(math.Round(qty * CRYPTO_QTY_SCALE))
	each := units / int64(len(f.LadderR)+1)
	for range f.LadderR {
		rungs = append(rungs, float64(each)/CRYPTO_QTY_SCALE)
	}
	return rungs, float64(units-each*int64(len(f.LadderR))) / CRYPTO_QTY_SCALE
}

// resizeCoins scales a coin play to qty coins, to eight decimals, keeping each rung's and the
// runner's share of the position. Prices, stop and failsafes stay.
func resizeCoins(p Play, qty float64) Play {
	if p.Qty <= 0 || qty <= 0 {
		return p
	}
	units, sold := int64(math.Round(qty*CRYPTO_QTY_SCALE)), int64(0)
	ladder := make([]Rung, len(p.Ladder))
	for i, r := range p.Ladder {
		n := int64(float64(units) * r.Qty / p.Qty)
		ladder[i], sold = Rung{Qty: float64(n) / CRYPTO_QTY_SCALE, Price: r.Price}, sold+n
	}
	p.Ladder, p.Runner.Qty, p.Qty = ladder, float64(units-sold)/CRYPTO_QTY_SCALE, float64(units)/CRYPTO_QTY_SCALE
	return p
}

// coinMarket is the snapshot a coin play is planned against: the live top of book, and volume,
// averages, RSI and volatility from the completed hourly candles.
func coinMarket(candles []coinbase.Candle, tick coinbase.Ticker, now time.Time) Market {
	m := Market{Source: CRYPTO_SOURCE, At: now.UTC(), Spot: tick.Price, Last: tick.Price, Bid: tick.Bid, Ask: tick.Ask}
	candles = lastCandles(candles, CRYPTO_WINDOW)
	if len(candles) == 0 {
		return m
	}
	closes, total := closesOf(candles), 0.0
	for _, c := range candles {
		total += c.V
	}
	day := lastCandles(candles, CRYPTO_DAY_HOURS)
	for _, c := range day {
		m.DayVolume += c.V
	}
	m.PreviousClose = day[0].O
	m.AvgVolume = total / float64(len(candles)) * CRYPTO_DAY_HOURS
	m.MA20, m.MA50, m.RSI14, m.IV = sma(closes, MA_SHORT), sma(closes, MA_LONG), rsi(closes, RSI_PERIOD), hourlyVol(closes)
	return m
}

// completed drops the candle that is still forming and any candle from the future.
func completed(candles []coinbase.Candle, now time.Time) []coinbase.Candle {
	for len(candles) > 0 && candles[len(candles)-1].T+int64(time.Hour/time.Second) > now.Unix() {
		candles = candles[:len(candles)-1]
	}
	return candles
}

// coinSnapshot loads a coin's market from Coinbase, for a play being refreshed. What fails is noted
// on the snapshot instead of failing the play.
func (r Resolver) coinSnapshot(ctx context.Context, symbol string, now time.Time) Market {
	candles, err := coinbase.Candles(ctx, r.HTTP, symbol, coinbase.GRANULARITY_HOUR, time.Time{}, time.Time{})
	note := ""
	if err != nil {
		note = NOTE_CANDLES + err.Error()
	}
	tick, err := coinbase.Fetch(ctx, r.HTTP, symbol)
	if err != nil {
		note = NOTE_QUOTE + err.Error()
	}
	m := coinMarket(completed(candles, now), tick, now)
	m.Note = note
	return m
}

// draft builds the whole coin play from a setup, its bracket and its size: exits for every coin
// bought, the clock, and the failsafes the rulebook asks of every play.
func (f CryptoRules) draft(s Setup, m Market, b cryptoBracket, qty, feePct float64, t Template, now time.Time) Play {
	p := Play{ID: NewID(now), CreatedAt: now.UTC(), UpdatedAt: now.UTC(), Status: STATUS_DRAFT, Title: s.Symbol,
		Contract: Contract{Symbol: s.Symbol, Market: MARKET_CRYPTO}, Market: &m,
		FoundBy: FOUND_BY_CRYPTO, Category: CRYPTO_CATEGORY, Theme: CRYPTO_THEME,
		Qty: qty, Entry: b.Entry, Stop: b.Stop}
	rungs, runner := f.pieces(qty)
	for i, n := range rungs {
		p.Ladder = append(p.Ladder, Rung{Qty: n, Price: b.Rungs[i]})
	}
	p.Runner = Runner{Qty: runner, Target: b.Runner}
	p.Failsafes = Failsafes{ExitBy: now.Add(time.Duration(f.HoldHours) * time.Hour).UTC(), SidewaysMinutes: f.SidewaysHours * MINUTES_PER_HR,
		NoQuoteMinutes: t.NoQuoteMinutes, GapPct: t.GapPct, Calamity: t.Calamity}
	p = immediateSchedule(p, now)
	line := func(kind, stance, label string, value float64) Evidence {
		return Evidence{Kind: kind, Label: label, Value: value, Stance: stance, Source: CRYPTO_SOURCE, Verified: true, At: now.UTC()}
	}
	switch f.Strategy {
	case CRYPTO_RSI_REVERSION:
		p.Thesis = fmt.Sprintf(THESIS_RSI_REVERSION, s.Symbol)
	case CRYPTO_FOUR_HOUR:
		p.Thesis = fmt.Sprintf(THESIS_FOUR_HOUR, s.Symbol)
	case CRYPTO_WEEKEND_FADE:
		p.Thesis = fmt.Sprintf(THESIS_WEEKEND_FADE, s.Symbol, s.Score)
	}
	if p.Thesis != "" {
		p.Category = f.Strategy
		p.Evidence = []Evidence{line(EV_TECHNICAL, STANCE_NEUTRAL, p.Thesis, s.Score)}
		p.Thesis += fmt.Sprintf(THESIS_CRYPTO_STOP, b.StopPct, feePct)
		return p
	}
	p.Thesis = fmt.Sprintf(THESIS_CRYPTO, s.Symbol, coinPrice(s.Close), f.BreakoutHours, coinPrice(s.PriorHigh), s.RelVolume, b.StopPct, feePct)
	// The trend line is neutral: the odds already read the averages from the snapshot, and it must not count twice.
	p.Evidence = []Evidence{
		line(EV_VOLUME, STANCE_FOR, fmt.Sprintf(EVIDENCE_VOLUME, s.RelVolume, CRYPTO_VOL_HOURS), s.RelVolume),
		line(EV_TECHNICAL, STANCE_FOR, fmt.Sprintf(EVIDENCE_BREAK, coinPrice(s.Close), f.BreakoutHours, coinPrice(s.PriorHigh)), s.Close),
		line(EV_TECHNICAL, STANCE_NEUTRAL, fmt.Sprintf(EVIDENCE_TREND, coinPrice(s.MA20), coinPrice(s.MA50)), s.MA20),
	}
	return p
}
