package plays

import (
	"math"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/alpaca"
	"github.com/YEAHDOGS/money24/engine/coinbase"
)

const (
	TEST_RSI_ASCENT       = 0.2
	TEST_RSI_DIP          = 7
	TEST_RSI_REBOUND      = 1.8
	TEST_TREND_FLAT       = 100
	TEST_TREND_CROSS      = 110
	TEST_SATURDAY_HOURS   = 6 * CRYPTO_DAY_HOURS
	TEST_FRIDAY_CLOSE     = 100
	TEST_SATURDAY_CLOSE   = 97
	TEST_MAKER_SIGNAL     = 100
	TEST_MAKER_ENTRY      = 99.90
	TEST_MAKER_LAST_CLOSE = 100.50
	TEST_REPLAY_CAPITAL   = 1000
)

func TestCryptoRSIReversionNeedsRecoveryInAnUptrend(t *testing.T) {
	f := *DefaultCrypto()
	f.Strategy = CRYPTO_RSI_REVERSION
	candles := hourly(TEST_HOURS+1, func(i int) (float64, float64) {
		if i < TEST_HOURS-2 {
			return TEST_TREND_FLAT + float64(i)*TEST_RSI_ASCENT, TEST_USUAL_VOL
		}
		dip := TEST_TREND_FLAT + (TEST_HOURS-3)*TEST_RSI_ASCENT - TEST_RSI_DIP
		if i == TEST_HOURS-2 {
			return dip, TEST_USUAL_VOL
		}
		return dip + TEST_RSI_REBOUND, TEST_USUAL_VOL
	})
	for _, end := range []int{TEST_HOURS - 1, TEST_HOURS, TEST_HOURS + 1} {
		s, why := cryptoSetup(CRYPTO_BTC, candles[:end], nil, f)
		if (why == "") != (end == TEST_HOURS) {
			t.Errorf("hour %d: RSI %g, SMA20 %g, SMA50 %g, reason %q", end, s.RSI, s.MA20, s.MA50, why)
		}
	}
	down := hourly(TEST_HOURS, choppy(-TEST_RSI_ASCENT))
	if _, why := cryptoSetup(TEST_COIN, candles[:TEST_HOURS], down, f); why != WHY_BTC_WEAK {
		t.Errorf("altcoin without BTC confirmation: %q", why)
	}
}

func TestCryptoFourHourTrendUsesCompleteUTCBars(t *testing.T) {
	f := *DefaultCrypto()
	f.Strategy = CRYPTO_FOUR_HOUR
	candles := hourly(CRYPTO_TREND_REQUIRED+CRYPTO_TREND_BAR, func(i int) (float64, float64) {
		if i >= CRYPTO_TREND_REQUIRED-2 {
			return TEST_TREND_CROSS, TEST_USUAL_VOL
		}
		return TEST_TREND_FLAT, TEST_USUAL_VOL
	})
	for _, end := range []int{CRYPTO_TREND_REQUIRED - 1, CRYPTO_TREND_REQUIRED, len(candles)} {
		s, why := cryptoSetup(CRYPTO_BTC, candles[:end], nil, f)
		if (why == "") != (end == CRYPTO_TREND_REQUIRED) {
			t.Errorf("hour %d: score %g, reason %q", end, s.Score, why)
		}
	}
	candles = append(candles[:1], candles[2:CRYPTO_TREND_REQUIRED]...)
	if _, why := cryptoSetup(CRYPTO_BTC, candles, nil, f); why != WHY_FOUR_HOUR {
		t.Errorf("missing hour inside a four-hour bar: %q", why)
	}
}

func TestCryptoWeekendFadeNeedsSaturdayDropAndUTCBoundary(t *testing.T) {
	f := *DefaultCrypto()
	f.Strategy = CRYPTO_WEEKEND_FADE
	candles := hourly(TEST_SATURDAY_HOURS+1, func(i int) (float64, float64) {
		if i >= TEST_SATURDAY_HOURS-2 {
			return TEST_SATURDAY_CLOSE, TEST_USUAL_VOL
		}
		return TEST_FRIDAY_CLOSE, TEST_USUAL_VOL
	})
	for _, end := range []int{TEST_SATURDAY_HOURS - 1, TEST_SATURDAY_HOURS, TEST_SATURDAY_HOURS + 1} {
		s, why := cryptoSetup(CRYPTO_BTC, candles[:end], nil, f)
		if (why == "") != (end == TEST_SATURDAY_HOURS) {
			t.Errorf("hour %d: score %g, reason %q", end, s.Score, why)
		}
	}
	candles[TEST_SATURDAY_HOURS-1].C++
	if _, why := cryptoSetup(CRYPTO_BTC, candles[:TEST_SATURDAY_HOURS], nil, f); why != WHY_WEEKEND_FADE {
		t.Errorf("drop below threshold: %q", why)
	}
}

func TestReplayLimitMustRestAndTradeThrough(t *testing.T) {
	f := *DefaultCrypto()
	setup := Setup{Close: TEST_MAKER_SIGNAL}
	for _, tc := range []struct {
		name string
		open float64
		low  float64
		fill bool
	}{
		{"trade through", TEST_MAKER_SIGNAL, TEST_MAKER_ENTRY - 0.01, true},
		{"touch", TEST_MAKER_SIGNAL, TEST_MAKER_ENTRY, false},
		{"opening gap through limit", TEST_MAKER_ENTRY - 0.01, TEST_MAKER_ENTRY - 0.02, false},
	} {
		price, fills := replayEntryPrice(setup, coinbase.Candle{O: tc.open, L: tc.low}, f, true)
		if price != TEST_MAKER_ENTRY || fills != tc.fill {
			t.Errorf("%s: price %g, fills %t", tc.name, price, fills)
		}
	}
}

func TestReplayMakerCreditsNetCoinsAndClosesAtBoundary(t *testing.T) {
	rules := DOGSRules()
	rules.Crypto.Symbols = []string{CRYPTO_BTC}
	history := breakout(TEST_SURGE_VOL)
	last := history[len(history)-1]
	hour := last.T + HOUR_SECONDS
	entry := coinPrice(last.C * (1 - REPLAY_LIMIT_PCT/PERCENT))
	candle := coinbase.Candle{T: hour, O: last.C, H: last.C * 2, L: entry - 0.01, C: TEST_MAKER_LAST_CLOSE}
	history = append(history, candle)
	now := time.Unix(hour, 0)
	execution := replayExecution{entryFeePct: alpaca.CRYPTO_MAKER_FEE_PCT, exitFeePct: alpaca.CRYPTO_TAKER_FEE_PCT, limit: true}
	replayed := replayCoins(map[string][]coinbase.Candle{CRYPTO_BTC: history}, rules, now, now.Add(time.Hour), TEST_REPLAY_CAPITAL, execution)
	if len(replayed.Trades) != 1 || replayed.Trades[0].Reason != REASON_WINDOW || replayed.Trades[0].ExitAt != hour+HOUR_SECONDS {
		t.Fatalf("want only a boundary exit, got %+v", replayed.Trades)
	}
	setup, _ := cryptoSetup(CRYPTO_BTC, history[:len(history)-1], nil, *rules.Crypto)
	wantCost := coinPrice(entry / (1 - alpaca.CRYPTO_MAKER_FEE_PCT/PERCENT))
	netExit := 1 - alpaca.CRYPTO_TAKER_FEE_PCT/PERCENT
	bracket := rules.Crypto.bracketAt(entry, setup.ATR, wantCost, netExit)
	qty, _ := rules.Crypto.size(bracket, Funds{EquityUSD: TEST_REPLAY_CAPITAL, BuyingPowerUSD: TEST_REPLAY_CAPITAL}, now)
	want := qty * (candle.C*netExit - wantCost)
	if math.Abs(replayed.Trades[0].PnLUSD-want) > TEST_TOLERANCE || math.Abs(replayed.EndUSD-TEST_REPLAY_CAPITAL-want) > TEST_TOLERANCE {
		t.Errorf("maker fill P&L %g, ending equity %g, want P&L %g", replayed.Trades[0].PnLUSD, replayed.EndUSD, want)
	}
}
