package alpaca

import (
	"testing"

	"github.com/YEAHDOGS/money24/engine/signals"
)

func TestSymbolFor(t *testing.T) {
	cases := []struct {
		sig  signals.Signal
		want string
		coin bool
	}{
		{signals.Signal{Symbol: "SPY", Right: "P", Strike: 700, Expiry: "2026-12-02"}, "SPY261202P00700000", false},
		{signals.Signal{Symbol: "aal", Right: "C", Strike: 12.5, Expiry: "2026-11-20"}, "AAL261120C00012500", false},
		{signals.Signal{Symbol: "BTC-USD", Market: signals.MARKET_CRYPTO}, "BTC/USD", true},
		{signals.Signal{Symbol: "DOGE", Market: signals.MARKET_CRYPTO}, "DOGE/USD", true},
		{signals.Signal{Symbol: "nvda"}, "NVDA", false},
	}
	for _, c := range cases {
		got, coin, err := symbolFor(c.sig)
		if err != nil || got != c.want || coin != c.coin {
			t.Errorf("%+v: %q coin=%v err=%v, want %q coin=%v", c.sig, got, coin, err, c.want, c.coin)
		}
	}
	if _, _, err := symbolFor(signals.Signal{Symbol: "PUMP", Address: "So11111111111111111111111111111111111111112"}); err == nil {
		t.Error("an on-chain token should not be sent to Alpaca")
	}
}

func TestLimitFor(t *testing.T) {
	opt := func(root string) signals.Signal { return signals.Signal{Symbol: root, Right: "C"} }
	cases := []struct {
		sig   signals.Signal
		price float64
		coin  bool
		want  string
	}{
		{opt("TSLA"), 2.333, false, "2.34"}, // under $3: pennies, rounded up
		{opt("TSLA"), 3.21, false, "3.25"},  // $3 and up: nickels, rounded up
		{opt("SPY"), 3.21, false, "3.21"},   // a penny name: pennies at any price
		{opt("TSLA"), 3.30, false, "3.30"},  // already on a nickel
		{signals.Signal{Symbol: "BTC-USD"}, 60154.961, true, "60154.97"},
		{signals.Signal{Symbol: "DOGE-USD"}, 0.0553412, true, "0.0553412"},
	}
	for _, c := range cases {
		if got := limitFor(c.sig, c.price, c.coin); got != c.want {
			t.Errorf("%s %.6f: %s, want %s", c.sig.Symbol, c.price, got, c.want)
		}
	}
}
