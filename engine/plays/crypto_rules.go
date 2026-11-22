package plays

import (
	"math"
	"strconv"
	"strings"
)

const (
	// The crypto desk is the part of Money 24 that trades around the clock with nobody at the keyboard.
	// Every pass it reads each coin's hourly candles and, where a coin breaks out of its range on heavy
	// volume with the trend behind it, drafts the whole bracket and trades it. No model is asked:
	// the rule is arithmetic, so the same rule can be replayed over old candles (crypto_backtest.go).
	// Long only, cash only, no real money. It never trades through a blocking rule.
	FOUND_BY_CRYPTO  = "crypto-desk"
	CRYPTO_CATEGORY  = "Crypto breakout"
	CRYPTO_THEME     = "crypto"
	CRYPTO_SOURCE    = "Coinbase"
	CRYPTO_BTC       = "BTC-USD" // the coin the market filter reads
	CRYPTO_QTY_SCALE = 1e8       // coins are bought in fractions, to eight decimals
	CRYPTO_DIGITS    = 6         // significant digits kept in a coin price, and never fewer than cents
	// Every indicator reads this many of the latest completed hourly candles and no more, so a live
	// pass and a replay of the same hour always agree.
	CRYPTO_WINDOW      = 120
	CRYPTO_VOL_HOURS   = 20 // the candles a breakout candle's volume is measured against
	CRYPTO_ATR_HOURS   = 14
	CRYPTO_DAY_HOURS   = 24
	CRYPTO_STALE_HOURS = 2    // a newest completed candle older than this is a broken feed, not a signal
	QTY_DUST           = 1e-9 // less of a coin than this is nothing
)

// CryptoRules is the crypto desk's part of the rulebook.
type CryptoRules struct {
	On           bool     `json:"on"`
	Symbols      []string `json:"symbols"`            // US dollar pairs, as Coinbase names them: BTC-USD
	EveryMinutes int      `json:"every_minutes"`      // how often the desk looks
	Strategy     string   `json:"strategy,omitempty"` // empty selects breakout

	// The entry: an hourly close above the range, on volume, with the trend, and not already spent.
	BreakoutHours int     `json:"breakout_hours"`   // the close must clear the highest high of this many hours before it
	MinRelVolume  float64 `json:"min_rel_volume"`   // the breakout hour's volume over the average of the hours before it
	MaxRSI        float64 `json:"max_rsi"`          // hourly RSI(14) above this: the move is stretched
	MaxAboveMAPct float64 `json:"max_above_ma_pct"` // the close this far over its 20-hour average: the move is stretched
	BTCMustTrend  bool    `json:"btc_must_trend"`   // other coins wait while bitcoin is under its 50-hour average
	MaxSpreadPct  float64 `json:"max_spread_pct"`   // bid to ask, as a percent of the mid
	MaxChasePct   float64 `json:"max_chase_pct"`    // the ask this far over the breakout close: it already ran

	// The bracket, in R: one R is the distance from the entry to the stop, fees included.
	StopATR       float64   `json:"stop_atr"`     // the stop sits this many hourly ATRs under the ask
	MinStopPct    float64   `json:"min_stop_pct"` // and never closer than this
	MaxStopPct    float64   `json:"max_stop_pct"` // or further than this
	LadderR       []float64 `json:"ladder_r"`     // each rung sells an equal share at this many R over the entry
	RunnerR       float64   `json:"runner_r"`     // the last share rides to this many R
	HoldHours     int       `json:"hold_hours"`
	SidewaysHours int       `json:"sideways_hours"` // not above the entry after this long: out

	// The size, for a cash account: what is risked at the stop, never more than the cash that has settled.
	RiskPct        float64 `json:"risk_pct"`         // percent of the account lost if the stop fills
	MaxPositionPct float64 `json:"max_position_pct"` // the most of the account in one coin
	MinTradeUSD    float64 `json:"min_trade_usd"`
	WeekendSizePct float64 `json:"weekend_size_pct"` // Saturday and Sunday in New York: books are thinner

	MaxOpen       int     `json:"max_open"`
	MaxPerDay     int     `json:"max_per_day"`
	CooldownHours float64 `json:"cooldown_hours"`   // after a losing close on a coin
	MaxDayLossPct float64 `json:"max_day_loss_pct"` // of the account, closed today: no new entries until tomorrow
}

// DefaultCrypto is how the crypto desk runs until the rulebook says otherwise.
func DefaultCrypto() *CryptoRules {
	return &CryptoRules{
		On:           true,
		Symbols:      []string{"BTC-USD", "ETH-USD", "SOL-USD", "XRP-USD", "DOGE-USD", "ADA-USD", "AVAX-USD", "LINK-USD", "LTC-USD", "BCH-USD", "DOT-USD", "SUI-USD"},
		EveryMinutes: 5,

		BreakoutHours: 12, MinRelVolume: 1.5, MaxRSI: 75, MaxAboveMAPct: 8, BTCMustTrend: true, MaxSpreadPct: 0.25, MaxChasePct: 1,
		StopATR: 2, MinStopPct: 1.5, MaxStopPct: 6, LadderR: []float64{1.5, 3}, RunnerR: 5, HoldHours: 72, SidewaysHours: 12,
		RiskPct: 1, MaxPositionPct: 25, MinTradeUSD: 10, WeekendSizePct: 50,
		MaxOpen: 3, MaxPerDay: 8, CooldownHours: 12, MaxDayLossPct: 3,
	}
}

// Funds is what the cash account can spend. Nil on the Service: the rulebook bankroll stands in for both.
type Funds struct {
	EquityUSD      float64 `json:"equity_usd"`       // cash plus open positions at what they cost
	BuyingPowerUSD float64 `json:"buying_power_usd"` // cash that has settled: all a cash account may spend
}

// coinPrice rounds a coin price to CRYPTO_DIGITS significant digits, and never coarser than a cent.
func coinPrice(v float64) float64 {
	if v <= 0 {
		return 0
	}
	scale := math.Max(CENT, math.Pow(10, CRYPTO_DIGITS-1-math.Floor(math.Log10(v))))
	return math.Round(v*scale) / scale
}

// coinLabel writes coin prices with every digit they keep, "0.053463 → 0.05533": cents would
// round a coin under a dollar into nothing.
func coinLabel(prices ...float64) string {
	parts := make([]string, len(prices))
	for i, v := range prices {
		parts[i] = strconv.FormatFloat(coinPrice(v), 'f', -1, 64)
	}
	return strings.Join(parts, " → ")
}
