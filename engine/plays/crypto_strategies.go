package plays

import (
	"fmt"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
)

const (
	CRYPTO_BREAKOUT       = "breakout"
	CRYPTO_RSI_REVERSION  = "rsi-reversion"
	CRYPTO_FOUR_HOUR      = "four-hour-trend"
	CRYPTO_WEEKEND_FADE   = "weekend-fade"
	CRYPTO_RSI_THRESHOLD  = 30
	CRYPTO_TREND_BAR      = 4
	CRYPTO_TREND_FAST     = 6
	CRYPTO_TREND_SLOW     = 24
	CRYPTO_TREND_REQUIRED = (CRYPTO_TREND_SLOW + 1) * CRYPTO_TREND_BAR
	CRYPTO_WEEKEND_DROP   = 3
	WHY_RSI_REVERSION     = "no hourly RSI recovery through 30 with SMA20 above SMA50"
	WHY_FOUR_HOUR         = "no completed four-hour 6/24 SMA bullish crossover"
	WHY_WEEKEND_FADE      = "no Saturday decline of at least 3% at Sunday 00:00 UTC"
	WHY_CRYPTO_STRATEGY   = "unknown crypto strategy %q"
	THESIS_RSI_REVERSION  = "%s hourly RSI recovered through 30 with SMA20 above SMA50."
	THESIS_FOUR_HOUR      = "%s four-hour SMA6 crossed above SMA24."
	THESIS_WEEKEND_FADE   = "%s fell %.2f%% from Friday's UTC close to Saturday's UTC close."
	THESIS_CRYPTO_STOP    = " The stop is %.1f%% under the ask. Every price is after a %.2f%% fee a side."
)

var CRYPTO_STRATEGIES = [...]string{CRYPTO_BREAKOUT, CRYPTO_RSI_REVERSION, CRYPTO_FOUR_HOUR, CRYPTO_WEEKEND_FADE}

// alternativeCryptoSetup evaluates only the selected signal, then applies the shared BTC and ATR checks.
func alternativeCryptoSetup(s Setup, candles []coinbase.Candle, closes []float64, btc []coinbase.Candle, f CryptoRules) (Setup, string) {
	switch f.Strategy {
	case CRYPTO_RSI_REVERSION:
		previous := rsi(closes[:len(closes)-1], RSI_PERIOD)
		if previous > CRYPTO_RSI_THRESHOLD || s.RSI <= CRYPTO_RSI_THRESHOLD || s.MA20 <= s.MA50 {
			return s, WHY_RSI_REVERSION
		}
		s.Score = s.RSI
	case CRYPTO_FOUR_HOUR:
		s.Score = fourHourCross(candles)
		if s.Score <= 0 {
			return s, WHY_FOUR_HOUR
		}
	case CRYPTO_WEEKEND_FADE:
		s.Score = weekendDrop(candles)
		if s.Score < CRYPTO_WEEKEND_DROP {
			return s, WHY_WEEKEND_FADE
		}
	default:
		return s, fmt.Sprintf(WHY_CRYPTO_STRATEGY, f.Strategy)
	}
	if s.ATR <= 0 {
		return s, WHY_NO_RANGE
	}
	if f.BTCMustTrend && s.Symbol != CRYPTO_BTC && !aboveLongAverage(btc) {
		return s, WHY_BTC_WEAK
	}
	return s, ""
}

// consecutiveHours checks that no hourly candle is missing in a fixed-length signal window.
func consecutiveHours(candles []coinbase.Candle, count int) bool {
	if len(candles) < count {
		return false
	}
	window := candles[len(candles)-count:]
	for i, c := range window {
		if c.T != window[0].T+int64(i)*HOUR_SECONDS {
			return false
		}
	}
	return true
}

// fourHourCross reads only fully observed four-hour bars and signals once at the UTC boundary.
func fourHourCross(candles []coinbase.Candle) float64 {
	end := candles[len(candles)-1].T + HOUR_SECONDS
	if end%(CRYPTO_TREND_BAR*HOUR_SECONDS) != 0 || !consecutiveHours(candles, CRYPTO_TREND_REQUIRED) {
		return 0
	}
	window := candles[len(candles)-CRYPTO_TREND_REQUIRED:]
	closes := make([]float64, 0, CRYPTO_TREND_SLOW+1)
	for i := CRYPTO_TREND_BAR - 1; i < len(window); i += CRYPTO_TREND_BAR {
		closes = append(closes, window[i].C)
	}
	prior := closes[:len(closes)-1]
	fast, slow := sma(closes, CRYPTO_TREND_FAST), sma(closes, CRYPTO_TREND_SLOW)
	if fast <= slow || slow <= 0 || sma(prior, CRYPTO_TREND_FAST) > sma(prior, CRYPTO_TREND_SLOW) {
		return 0
	}
	return fast / slow
}

// weekendDrop measures Friday close to Saturday close once per weekend in UTC.
func weekendDrop(candles []coinbase.Candle) float64 {
	last := candles[len(candles)-1]
	end := time.Unix(last.T+HOUR_SECONDS, 0).UTC()
	if end.Weekday() != time.Sunday || end.Hour() != 0 || !consecutiveHours(candles, CRYPTO_DAY_HOURS+1) {
		return 0
	}
	friday := candles[len(candles)-CRYPTO_DAY_HOURS-1].C
	if friday <= 0 {
		return 0
	}
	return (friday - last.C) / friday * PERCENT
}
