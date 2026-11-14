package plays

import (
	"math"
	"testing"
	"time"
)

var TEST_NOW = time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC) // Monday 10:00 New York

// The AAL Nov 20 $12.50 put from the FAA chat, as Yahoo closed it Friday Nov 13.
func aalPut() Play {
	m := Market{Spot: 13.17, PreviousClose: 13.06, Bid: 0.12, Ask: 0.13, Last: 0.12, IV: 0.516, OpenInterest: 2320, Volume: 1746,
		DayVolume: 54305947, AvgVolume: 64775065, At: TEST_NOW}
	c := Contract{Symbol: "AAL", Market: MARKET_OPTION, Right: RIGHT_PUT, Strike: 12.5, Expiry: "2026-11-20"}
	return Draft(c, m, DOGSRules().Template, TEST_NOW)
}

func TestSpotForInvertsPrice(t *testing.T) {
	for _, call := range []bool{true, false} {
		s, ok := spotFor(1.25, 12.5, 0.02, 0.5, call)
		if !ok {
			t.Fatalf("call=%v: no solution", call)
		}
		if got := optionPrice(s, 12.5, 0.02, 0.5, call); math.Abs(got-1.25) > 0.01 {
			t.Errorf("call=%v: price at solved spot %.2f is %.3f, want 1.25", call, s, got)
		}
	}
	if _, ok := spotFor(20, 12.5, 0.02, 0.5, false); ok {
		t.Error("a $12.50 put can never be worth $20")
	}
}

func TestDraftIsCompleteAndSumsToQty(t *testing.T) {
	p := aalPut()
	sold := p.Runner.Qty
	for _, r := range p.Ladder {
		sold += r.Qty
	}
	if p.Qty != 11 || sold != p.Qty {
		t.Fatalf("qty %g, exits cover %g; want 11 and 11 ($150 at the $0.13 mid)", p.Qty, sold)
	}
	if p.Stop >= p.Entry || p.Failsafes.ExitBy.IsZero() || p.Failsafes.Calamity == "" {
		t.Fatalf("draft missing stop or failsafes: %+v", p)
	}
	if exit := p.Failsafes.ExitBy.In(MARKET_TZ); exit.Weekday() != time.Thursday || exit.Hour() != 15 {
		t.Errorf("exit %v, want Thursday 3 PM the day before expiry", exit)
	}
}

func TestResizeKeepsShapeAndMinimum(t *testing.T) {
	p := aalPut()
	small := Resize(p, 1)
	if small.Qty != 3 || small.Ladder[0].Qty != 1 || small.Ladder[1].Qty != 1 || small.Runner.Qty != 1 {
		t.Fatalf("minimum is one unit per piece: %+v %+v", small.Ladder, small.Runner)
	}
	big := Resize(p, 120)
	if big.Qty != 120 {
		t.Fatalf("resized to %g, want 120", big.Qty)
	}
	r := DOGSRules()
	sz := sizing(big, r)
	if sz.MinUnits != 3 || sz.MinDepositUSD != 39 {
		t.Errorf("min deposit %g units / $%.2f, want 3 / $39", sz.MinUnits, sz.MinDepositUSD)
	}
	a, b := Build(p, r, Book{Now: TEST_NOW}), Build(big, r, Book{Now: TEST_NOW})
	if a.Odds.MarketPct != b.Odds.MarketPct {
		t.Error("size must not change the market's odds")
	}
	if ratio, want := b.Payoff.MaxProfitUSD/a.Payoff.MaxProfitUSD, big.Qty/p.Qty; math.Abs(ratio-want) > 0.5 {
		t.Errorf("profit should scale with size: %.2fx vs %.2fx", ratio, want)
	}
}

func TestPutLevelsNeedTheStockToFall(t *testing.T) {
	rep := Build(aalPut(), DOGSRules(), Book{Now: TEST_NOW})
	for _, l := range rep.Levels {
		if l.Name == LEVEL_STOP || l.Name == LEVEL_BREAKEVEN || !l.Reachable {
			continue
		}
		if l.SpotByExit >= 13.17 || l.MovePct >= 0 {
			t.Errorf("%s: a put rung needs the stock below 13.17, got %.2f (%+.1f%%)", l.Name, l.SpotByExit, l.MovePct)
		}
		if l.SpotByExit > l.SpotNow {
			t.Errorf("%s: with time decay the exit-time level %.2f must be lower than right now %.2f", l.Name, l.SpotByExit, l.SpotNow)
		}
	}
	if rep.Payoff.WorstLossUSD != rep.Payoff.CostUSD || rep.Payoff.PlannedLossUSD >= rep.Payoff.CostUSD {
		t.Errorf("payoff: %+v", rep.Payoff)
	}
}
