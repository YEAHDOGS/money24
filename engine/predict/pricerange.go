package predict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
)

const (
	// Prices at a set time: Kalshi's hourly and daily crypto markets ("bitcoin above $59,300 at 11 PM",
	// "between $59,200 and $59,299.99"). They settle on the 60-second average of CF Benchmarks' real-time
	// index before the hour, which follows the big exchanges closely, so Coinbase's live price stands in
	// for it. The chance of each range is the price's own recent minute-to-minute volatility, scaled to
	// the minutes left, with room for fat tails and for the gap between Coinbase and the index.
	KIND_PRICE        = "pricerange"
	PRICE_NAME        = "Crypto price ranges"
	PRICE_ABOUT       = "Kalshi's hourly bitcoin, ether, solana, XRP and dogecoin markets: the live price and its own recent volatility, scaled to the minutes left, give a chance for every range."
	PRICE_MODEL_ID    = "crypto-model"
	PRICE_MODEL_NAME  = "Crypto: volatility edge"
	PRICE_FAR_ID      = "crypto-far"
	PRICE_FAR_NAME    = "Crypto: far from the line"
	PRICE_EVERY       = 2 // minutes
	TICKER_EVERY      = 30 * time.Second
	VOL_EVERY         = 5 * time.Minute
	VOL_MIN_RETURNS   = 30 // fewer minute returns than this is no volatility
	PRICE_REASON      = "%s $%s · %.0fm left · ±%.2f%% · model %.0f%% YES"
	PRICE_REASON_FAR  = "%.1f spreads from the line"
	PRICE_DIGITS_SHOW = 6

	DEFAULT_VOL_MINUTES    = 240    // the minutes of one-minute candles volatility is measured over
	DEFAULT_VOL_MULTIPLIER = 1.3    // crypto has fatter tails and more jumps than a normal curve
	DEFAULT_VOL_FLOOR      = 0.0004 // the least one-minute spread, as a share of the price
	DEFAULT_BASIS_PCT      = 0.03   // Coinbase against the settling index, percent of the price
	DEFAULT_MAX_MINUTES    = 90
	DEFAULT_KNOWN_Z        = 4.0
)

// PriceCoin is one coin: its Coinbase symbol and the Kalshi series that settle on its price.
type PriceCoin struct {
	Symbol string   `json:"symbol"`
	Series []string `json:"series"`
}

// DEFAULT_COINS are the coins with hourly Kalshi markets and deep Coinbase books.
var DEFAULT_COINS = []PriceCoin{
	{Symbol: "BTC", Series: []string{"KXBTCD", "KXBTC"}},
	{Symbol: "ETH", Series: []string{"KXETHD", "KXETH"}},
	{Symbol: "SOL", Series: []string{"KXSOLD", "KXSOL"}},
	{Symbol: "XRP", Series: []string{"KXXRPD", "KXXRP"}},
	{Symbol: "DOGE", Series: []string{"KXDOGED", "KXDOGE"}},
}

// PriceParams are the price-range kind's own settings.
type PriceParams struct {
	Coins         []PriceCoin `json:"coins,omitempty"` // empty: DEFAULT_COINS
	MaxMinutes    float64     `json:"max_minutes"`     // only markets closing within this many minutes
	VolMinutes    int         `json:"vol_minutes"`
	VolMultiplier float64     `json:"vol_multiplier"`
	VolFloor      float64     `json:"vol_floor"`
	BasisPct      float64     `json:"basis_pct"`
	// FarOnly takes only ranges the price is at least KnownZ spreads inside or outside of: what would
	// take a move the price has not made in its recent hours.
	FarOnly bool    `json:"far_only"`
	KnownZ  float64 `json:"known_z"`
}

type coinState struct {
	price          float64
	priceAt, volAt time.Time
	minuteVol      float64 // spread of one-minute log returns
}

type priceRange struct {
	p      PriceParams
	series map[string]string // series -> coin symbol
	mu     sync.Mutex
	coins  map[string]*coinState
}

func init() {
	Register(Kind{ID: KIND_PRICE, Name: PRICE_NAME, About: PRICE_ABOUT, New: newPriceRange})
}

func newPriceRange(raw json.RawMessage) (Strategy, error) {
	p := PriceParams{MaxMinutes: DEFAULT_MAX_MINUTES, VolMinutes: DEFAULT_VOL_MINUTES, VolMultiplier: DEFAULT_VOL_MULTIPLIER,
		VolFloor: DEFAULT_VOL_FLOOR, BasisPct: DEFAULT_BASIS_PCT, KnownZ: DEFAULT_KNOWN_Z}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	if p.MaxMinutes <= 0 || p.VolMinutes < VOL_MIN_RETURNS || p.VolMinutes > coinbase.CANDLES_MAX || p.VolMultiplier <= 0 {
		return nil, fmt.Errorf("max_minutes must be above 0, vol_minutes %d to %d, vol_multiplier above 0", VOL_MIN_RETURNS, coinbase.CANDLES_MAX)
	}
	if len(p.Coins) == 0 {
		p.Coins = DEFAULT_COINS
	}
	r := &priceRange{p: p, series: map[string]string{}, coins: map[string]*coinState{}}
	for _, c := range p.Coins {
		for _, s := range c.Series {
			r.series[s] = strings.ToUpper(c.Symbol)
		}
	}
	return r, nil
}

func (r *priceRange) Scope() Scope {
	out := make([]string, 0, len(r.series))
	for s := range r.series {
		out = append(out, s)
	}
	return Scope{Series: out, WithinHours: r.p.MaxMinutes / float64(time.Hour/time.Minute)}
}

func (r *priceRange) Prepare(ctx context.Context, env *Env, markets []Market, now time.Time) error {
	needed := map[string]bool{}
	for _, m := range markets {
		if coin, ok := r.series[m.Series()]; ok {
			needed[coin] = true
		}
	}
	var errs []error
	for coin := range needed {
		if err := r.refresh(ctx, env, coin, now); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", coin, err))
		}
	}
	return errors.Join(errs...)
}

// refresh reads a coin's live price, and its volatility when that is stale.
func (r *priceRange) refresh(ctx context.Context, env *Env, coin string, now time.Time) error {
	r.mu.Lock()
	st := r.coins[coin]
	if st == nil {
		st = &coinState{}
		r.coins[coin] = st
	}
	priceDue, volDue := now.Sub(st.priceAt) >= TICKER_EVERY, now.Sub(st.volAt) >= VOL_EVERY
	r.mu.Unlock()
	if priceDue {
		t, err := coinbase.Fetch(ctx, env.HTTP, coin)
		if err != nil {
			return err
		}
		mid := t.Price
		if t.Bid > 0 && t.Ask >= t.Bid {
			mid = (t.Bid + t.Ask) / 2
		}
		r.mu.Lock()
		st.price, st.priceAt = mid, now
		r.mu.Unlock()
	}
	if !volDue {
		return nil
	}
	candles, err := coinbase.Candles(ctx, env.HTTP, coin, time.Minute, now.Add(-time.Duration(r.p.VolMinutes)*time.Minute), now)
	if err != nil {
		return err
	}
	vol, ok := minuteVol(candles)
	if !ok {
		return fmt.Errorf("%d one-minute candles: too few for a volatility", len(candles))
	}
	r.mu.Lock()
	st.minuteVol, st.volAt = vol, now
	r.mu.Unlock()
	return nil
}

// minuteVol is the spread of one-minute log returns between consecutive candles' closes.
func minuteVol(candles []coinbase.Candle) (float64, bool) {
	returns := []float64{}
	for i := 1; i < len(candles); i++ {
		if candles[i].C <= 0 || candles[i-1].C <= 0 {
			continue
		}
		gap := float64(candles[i].T-candles[i-1].T) / float64(time.Minute/time.Second)
		if gap <= 0 {
			continue
		}
		returns = append(returns, math.Log(candles[i].C/candles[i-1].C)/math.Sqrt(gap))
	}
	if len(returns) < VOL_MIN_RETURNS {
		return 0, false
	}
	mean := 0.0
	for _, x := range returns {
		mean += x
	}
	mean /= float64(len(returns))
	sum := 0.0
	for _, x := range returns {
		sum += (x - mean) * (x - mean)
	}
	return math.Sqrt(sum / float64(len(returns)-1)), true
}

func (r *priceRange) Evaluate(m Market, now time.Time) (Opinion, bool) {
	coin, ok := r.series[m.Series()]
	lo, hi, okRange := m.ContinuousRange()
	left := m.CloseAt.Sub(now).Minutes()
	if !ok || !okRange || left <= 0 || left > r.p.MaxMinutes {
		return Opinion{}, false
	}
	r.mu.Lock()
	st := r.coins[coin]
	var price, vol float64
	if st != nil && now.Sub(st.priceAt) < TICKER_EVERY && now.Sub(st.volAt) < VOL_EVERY {
		price, vol = st.price, st.minuteVol
	}
	r.mu.Unlock()
	if price <= 0 || vol <= 0 {
		return Opinion{}, false
	}
	spread := r.spread(vol, left)
	prob := priceProb(price, lo, hi, spread)
	if r.p.FarOnly && !far(price, lo, hi, spread, r.p.KnownZ) {
		return Opinion{}, false
	}
	side := SIDE_YES
	if (DOLLAR-prob)-m.NoAsk > prob-m.YesAsk {
		side = SIDE_NO
	}
	reasons := []string{fmt.Sprintf(PRICE_REASON, coin, trimPrice(price), left, spread*PERCENT, prob*PERCENT)}
	if r.p.FarOnly {
		reasons = append(reasons, fmt.Sprintf(PRICE_REASON_FAR, nearestZ(price, lo, hi, spread)))
	}
	return Opinion{Side: side, Prob: prob, HasProb: true, Reasons: reasons}, true
}

// spread is the spread of the log price at the close: the minute volatility, made fatter, over the
// minutes left, with the gap between Coinbase and the settling index on top.
func (r *priceRange) spread(minuteVol, minutes float64) float64 {
	v := math.Max(minuteVol, r.p.VolFloor) * r.p.VolMultiplier
	basis := r.p.BasisPct / PERCENT
	return math.Sqrt(v*v*minutes + basis*basis)
}

// priceProb is the chance the price at the close lands in (lo, hi], the log price spread normally around today's.
func priceProb(price, lo, hi, spread float64) float64 {
	return math.Max(0, logCDF(price, hi, spread)-logCDF(price, lo, spread))
}

// logCDF is the chance the closing price ends at or under level.
func logCDF(price, level, spread float64) float64 {
	switch {
	case math.IsInf(level, 1):
		return 1
	case level <= 0 || math.IsInf(level, -1):
		return 0
	}
	return normalCDF(math.Log(level/price) / spread)
}

// nearestZ is how many spreads the price sits from the nearest edge of its range.
func nearestZ(price, lo, hi, spread float64) float64 {
	z := math.Inf(1)
	for _, edge := range []float64{lo, hi} {
		if edge > 0 && !math.IsInf(edge, 0) {
			z = math.Min(z, math.Abs(math.Log(edge/price))/spread)
		}
	}
	return z
}

// far says whether the price is at least z spreads from every edge of the range.
func far(price, lo, hi, spread, z float64) bool { return nearestZ(price, lo, hi, spread) >= z }

// trimPrice shows a price to a sensible number of digits: 59,180 for bitcoin, 0.9199 for XRP.
func trimPrice(p float64) string {
	digits := max(0, PRICE_DIGITS_SHOW-int(math.Floor(math.Log10(p)))-1)
	text := fmt.Sprintf("%.*f", digits, p)
	if digits == 0 {
		return text
	}
	return strings.TrimRight(strings.TrimRight(text, "0"), ".")
}

func priceParams(farOnly bool, maxMinutes float64) json.RawMessage {
	raw, _ := json.Marshal(PriceParams{MaxMinutes: maxMinutes, VolMinutes: DEFAULT_VOL_MINUTES, VolMultiplier: DEFAULT_VOL_MULTIPLIER,
		VolFloor: DEFAULT_VOL_FLOOR, BasisPct: DEFAULT_BASIS_PCT, FarOnly: farOnly, KnownZ: DEFAULT_KNOWN_Z})
	return raw
}

func defaultPriceModel() Config {
	return Config{ID: PRICE_MODEL_ID, Kind: KIND_PRICE, Name: PRICE_MODEL_NAME, On: true, Risk: RISK_MEDIUM, EveryMinutes: PRICE_EVERY,
		Entry:  Entry{MinPrice: 0.05, MaxPrice: 0.95, MaxSpread: 0.04, MinVolume24h: 20, LeftMinutes: 3, MinEdge: 0.07, MaxEdge: 0.25},
		Limits: Limits{StakeUSD: 10, MaxOpen: 10, NewPerPass: 3, DailyStakeUSD: 100}, Params: priceParams(false, DEFAULT_MAX_MINUTES)}
}

func defaultPriceFar() Config {
	return Config{ID: PRICE_FAR_ID, Kind: KIND_PRICE, Name: PRICE_FAR_NAME, On: true, Risk: RISK_LOW, EveryMinutes: PRICE_EVERY,
		Entry:  Entry{MinPrice: 0.85, MaxPrice: 0.98, MaxSpread: 0.03, LeftMinutes: 2, MinEdge: 0.015, MinProb: 0.97},
		Limits: Limits{StakeUSD: 15, MaxOpen: 10, NewPerPass: 3, DailyStakeUSD: 150}, Params: priceParams(true, 30)}
}
