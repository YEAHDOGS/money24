package plays

import (
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/market"
)

func TestPlayFromSplitsTheBracket(t *testing.T) {
	now := time.Date(2026, 11, 23, 16, 0, 0, 0, time.UTC)
	base := Candidate{Symbol: "SPY", Contract: Contract{Symbol: "SPY", Market: MARKET_OPTION, Right: RIGHT_CALL, Strike: 692, Expiry: "2026-11-23"},
		Entry: 2, Stop: 1.3, Target1: 2.8, Target2: 4, ExitBy: now.Add(time.Hour)}
	for _, c := range []struct{ qty, ladder, runner float64 }{{3, 1, 2}, {1, 1, 0}, {4, 2, 2}} {
		cand := base
		cand.Qty = int(c.qty)
		p := playFrom(cand, now)
		if p.Ladder[0].Qty != c.ladder || p.Runner.Qty != c.runner || p.Ladder[0].Qty+p.Runner.Qty != p.Qty {
			t.Errorf("qty %g: ladder %g runner %g", c.qty, p.Ladder[0].Qty, p.Runner.Qty)
		}
		if p.Stop >= p.Entry || p.Failsafes.ExitBy.IsZero() {
			t.Errorf("qty %g: stop %g entry %g exit %v", c.qty, p.Stop, p.Entry, p.Failsafes.ExitBy)
		}
	}
}

func TestReadSessionAndSetups(t *testing.T) {
	opens := time.Date(2026, 11, 23, 14, 30, 0, 0, time.UTC)
	candles := []market.Candle{}
	price := 100.0
	for i := range 60 {
		v := 1000.0
		if i >= 55 {
			price += 0.3 // a late push out of the range on heavy volume
			v = 4000
		}
		candles = append(candles, market.Candle{T: opens.Unix() + int64(i*60), O: price, H: price + 0.1, L: price - 0.1, C: price, V: v})
	}
	sess, ok := readSession("SPY", candles, opens)
	if !ok || !sess.ORBDone || sess.RelVolume < 2 {
		t.Fatalf("session %+v", sess)
	}
	hit, ok := orbHit(sess, *DefaultScout())
	if !ok || hit.Direction != DIR_UP {
		t.Fatalf("breakout not found: %+v", hit)
	}
	if _, ok := flowHit("SPY", 100000, 900000); !ok {
		t.Error("a minute with $900K of puts against $100K of calls should be a surge")
	}
	if _, ok := flowHit("SPY", 100000, 150000); ok {
		t.Error("a small, balanced minute is not a surge")
	}
}
