package plays

import (
	"fmt"
	"math"

	"github.com/YEAHDOGS/money24/engine/coinbase"
)

const (
	WHY_FEW_CANDLES = "only %d completed hourly candles, needs %d"
	WHY_NO_TREND    = "no uptrend: close %g, 20-hour average %g, 50-hour average %g"
	WHY_NO_BREAKOUT = "no breakout: close %g is not above the %d-hour high %g"
	WHY_THIN_VOLUME = "volume %.1fx the hours before it, needs %.1fx: nobody else is in it"
	WHY_HIGH_RSI    = "RSI %.0f is over %.0f: the move is stretched"
	WHY_FAR_FROM_MA = "%.1f%% over its 20-hour average, max %.0f%%: the move is stretched"
	WHY_NO_RANGE    = "no hourly range to set a stop from"
	WHY_BTC_WEAK    = "bitcoin is under its 50-hour average: other coins wait"
)

// Setup is a coin whose last completed hourly candle earned a play: a close above its range, on
// volume, with the trend behind it.
type Setup struct {
	Symbol    string  `json:"symbol"`
	At        int64   `json:"at"` // the breakout candle's opening time, Unix seconds
	Close     float64 `json:"close"`
	PriorHigh float64 `json:"prior_high"` // the highest high of the hours before it
	ATR       float64 `json:"atr"`        // average hourly range
	MA20      float64 `json:"ma20"`
	MA50      float64 `json:"ma50"`
	RSI       float64 `json:"rsi"`
	RelVolume float64 `json:"rel_volume"` // the breakout hour's volume over the average of the hours before it
	Score     float64 `json:"score"`      // relative volume, raised by how far past the range it closed
}

// lastCandles is the newest n candles, or all of them when there are fewer.
func lastCandles(candles []coinbase.Candle, n int) []coinbase.Candle {
	if len(candles) <= n {
		return candles
	}
	return candles[len(candles)-n:]
}

func closesOf(candles []coinbase.Candle) []float64 {
	out := make([]float64, len(candles))
	for i, c := range candles {
		out[i] = c.C
	}
	return out
}

// averageRange is the mean true range of the last n candles: each candle's high to low, stretched
// to the close before it when the price gapped. Zero with too few candles.
func averageRange(candles []coinbase.Candle, n int) float64 {
	if len(candles) <= n || n <= 0 {
		return 0
	}
	sum := 0.0
	for i := len(candles) - n; i < len(candles); i++ {
		c, before := candles[i], candles[i-1].C
		sum += math.Max(c.H-c.L, math.Max(math.Abs(c.H-before), math.Abs(c.L-before)))
	}
	return sum / float64(n)
}

// hourlyVol is the annualised standard deviation of hourly log returns: the volatility a coin
// play's odds are worked out from.
func hourlyVol(closes []float64) float64 {
	rets := make([]float64, 0, len(closes))
	mean := 0.0
	for i := 1; i < len(closes); i++ {
		if closes[i-1] <= 0 || closes[i] <= 0 {
			continue
		}
		r := math.Log(closes[i] / closes[i-1])
		rets, mean = append(rets, r), mean+r
	}
	if len(rets) < 2 {
		return 0
	}
	mean /= float64(len(rets))
	variance := 0.0
	for _, r := range rets {
		variance += (r - mean) * (r - mean)
	}
	return math.Sqrt(variance/float64(len(rets)-1)) * math.Sqrt(HOURS_PER_YEAR)
}

// cryptoSetup reads a coin's completed hourly candles, oldest first, and says whether the last one
// earned a play. btc is bitcoin's candles up to the same hour, for the market filter. When the coin
// did not earn one, why says so in words. Pure arithmetic: the live desk and the replay share it.
func cryptoSetup(symbol string, candles, btc []coinbase.Candle, f CryptoRules) (Setup, string) {
	candles = lastCandles(candles, CRYPTO_WINDOW)
	need := max(MA_LONG, CRYPTO_VOL_HOURS, f.BreakoutHours, CRYPTO_ATR_HOURS, RSI_PERIOD) + 1
	if len(candles) < need {
		return Setup{}, fmt.Sprintf(WHY_FEW_CANDLES, len(candles), need)
	}
	last, closes := candles[len(candles)-1], closesOf(candles)
	s := Setup{Symbol: symbol, At: last.T, Close: last.C, MA20: sma(closes, MA_SHORT), MA50: sma(closes, MA_LONG),
		RSI: rsi(closes, RSI_PERIOD), ATR: averageRange(candles, CRYPTO_ATR_HOURS)}
	before := candles[:len(candles)-1]
	for _, c := range lastCandles(before, f.BreakoutHours) {
		s.PriorHigh = math.Max(s.PriorHigh, c.H)
	}
	usual := 0.0
	for _, c := range lastCandles(before, CRYPTO_VOL_HOURS) {
		usual += c.V / CRYPTO_VOL_HOURS
	}
	if usual > 0 {
		s.RelVolume = last.V / usual
	}
	overMA := (s.Close/s.MA20 - 1) * PERCENT
	switch {
	case !(s.Close > s.MA20 && s.MA20 > s.MA50):
		return s, fmt.Sprintf(WHY_NO_TREND, coinPrice(s.Close), coinPrice(s.MA20), coinPrice(s.MA50))
	case s.Close <= s.PriorHigh:
		return s, fmt.Sprintf(WHY_NO_BREAKOUT, coinPrice(s.Close), f.BreakoutHours, coinPrice(s.PriorHigh))
	case s.RelVolume < f.MinRelVolume:
		return s, fmt.Sprintf(WHY_THIN_VOLUME, s.RelVolume, f.MinRelVolume)
	case f.MaxRSI > 0 && s.RSI > f.MaxRSI:
		return s, fmt.Sprintf(WHY_HIGH_RSI, s.RSI, f.MaxRSI)
	case f.MaxAboveMAPct > 0 && overMA > f.MaxAboveMAPct:
		return s, fmt.Sprintf(WHY_FAR_FROM_MA, overMA, f.MaxAboveMAPct)
	case s.ATR <= 0:
		return s, WHY_NO_RANGE
	case f.BTCMustTrend && symbol != CRYPTO_BTC && !aboveLongAverage(btc):
		return s, WHY_BTC_WEAK
	}
	s.Score = s.RelVolume * (1 + (s.Close-s.PriorHigh)/s.ATR)
	return s, ""
}

// aboveLongAverage says whether the last close is over the 50-hour average. Too few candles: no.
func aboveLongAverage(candles []coinbase.Candle) bool {
	closes := closesOf(lastCandles(candles, CRYPTO_WINDOW))
	long := sma(closes, MA_LONG)
	return long > 0 && closes[len(closes)-1] > long
}
