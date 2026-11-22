package predict

import (
	"path/filepath"
	"testing"
)

func TestFeeRoundsUpAtFixedPointPriceBoundary(t *testing.T) {
	if got := Fee(3, 0.3909); got != 0.06 {
		t.Fatalf("fee = %v, want $0.06 for a raw fee of $0.0500004099", got)
	}
}

func TestDailyStakeCapHasNoHalfCentAllowance(t *testing.T) {
	book, err := OpenBook(filepath.Join(t.TempDir(), "book.jsonl"), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	d := &Desk{Book: book, rulebook: DefaultRulebook()}
	c := defaultFavorites()
	if _, err := book.Append(Line{At: testNow, Kind: KIND_BUY, Strategy: c.ID, Ticker: "T", CostUSD: 0.2}); err != nil {
		t.Fatal(err)
	}
	c.Limits.StakeUSD = 0.1
	c.Limits.DailyStakeUSD = 0.3
	if state, _ := d.limits(c, testNow); state != STATE_LOOKING {
		t.Errorf("state %s with exactly $0.10 remaining", state)
	}
	c.Limits.DailyStakeUSD = 0.299
	if state, _ := d.limits(c, testNow); state != STATE_FULL {
		t.Errorf("state %s with less than $0.10 remaining", state)
	}
}

func TestFillFromUsesExecutableLevelsWithinExactCaps(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		asks                         []Level
		limit, stake, qty, cost, fee float64
	}{
		{"strict price cap", []Level{{0.953, 100}}, 0.95, 20, 0, 0, 0},
		{"strict stake cap", []Level{{0.5, 2}}, 0.95, 0.519, 0, 0, 0},
		{"rounding cost up cannot exceed stake", []Level{{0.335, 3}}, 0.95, 1.057, 2, 0.67, 0.04},
		{"skip fractional-only level", []Level{{0.90, 0.75}, {0.91, 2.5}}, 0.95, 20, 2, 1.82, 0.02},
		{"cost rounds half cent up", []Level{{0.335, 3}}, 0.95, 2, 3, 1.01, 0.05},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qty, cost, fee, _ := fillFrom(tc.asks, tc.limit, tc.stake)
			if qty != tc.qty || cost != tc.cost || fee != tc.fee {
				t.Fatalf("got %v at $%.2f plus $%.2f, want %v at $%.2f plus $%.2f", qty, cost, fee, tc.qty, tc.cost, tc.fee)
			}
		})
	}
}

func TestPriceLimitKeepsEdgeAtSubcentPrices(t *testing.T) {
	c := Config{Entry: Entry{MaxPrice: 0.92, MinEdge: 0.08}}
	pick := candidate{prob: 0.9, ask: 0.807}
	limit := priceLimit(c, pick)
	if limit < pick.ask || pick.prob-limit-feeEach(limit) < c.Entry.MinEdge {
		t.Fatalf("limit %.6f must include the qualifying ask %.3f and keep the minimum edge", limit, pick.ask)
	}
}

func TestConsiderHonorsSpreadCap(t *testing.T) {
	r := build(t, defaultFavorites())
	m := Market{Type: MARKET_BINARY, CloseAt: testNow.Add(DAY), Volume24h: 500, YesAsk: 0.9, YesBid: 0.866}
	if _, ok := consider(r, m, testNow); ok {
		t.Fatal("accepted a spread of $0.034 with a $0.03 cap")
	}
	m.YesBid = 0.87
	if _, ok := consider(r, m, testNow); !ok {
		t.Fatal("rejected a spread of exactly $0.03")
	}
}
