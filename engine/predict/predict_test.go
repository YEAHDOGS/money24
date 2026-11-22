package predict

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)

func TestFee(t *testing.T) {
	cases := []struct{ qty, price, want float64 }{
		{1, 0.90, 0.01},   // 0.0063 rounds up to a cent
		{20, 0.90, 0.13},  // 0.126
		{100, 0.50, 1.75}, // the most a contract is charged: 1.75¢ at 50¢
		{10, 0.95, 0.04},  // 0.03325
	}
	for _, c := range cases {
		if got := Fee(c.qty, c.price); got != c.want {
			t.Errorf("Fee(%v, %v) = %v, want %v", c.qty, c.price, got, c.want)
		}
	}
}

func TestFillFromStaysInsideStakeAndPrice(t *testing.T) {
	asks := []Level{{Price: 0.90, Qty: 5}, {Price: 0.92, Qty: 100}, {Price: 0.97, Qty: 1000}}
	qty, cost, fee, levels := fillFrom(asks, 0.95, 20)
	if levels != 2 || qty != 21 {
		t.Fatalf("qty %v over %d levels, want 21 over 2", qty, levels)
	}
	if cost+fee > 20 {
		t.Fatalf("spent $%.2f + $%.2f, over the $20 stake", cost, fee)
	}
	if qty, _, _, _ := fillFrom([]Level{{Price: 0.97, Qty: 50}}, 0.95, 20); qty != 0 {
		t.Fatalf("bought %v above the price limit", qty)
	}
}

func TestFavoritePicksTheSideInRange(t *testing.T) {
	r := DefaultRules()
	m := Market{Ticker: "A", Type: MARKET_BINARY, CloseAt: testNow.Add(2 * time.Hour), Volume24h: 500,
		YesBid: 0.08, YesAsk: 0.10, NoBid: 0.89, NoAsk: 0.91}
	p, ok := favorite(m, r, testNow)
	if !ok || p.side != SIDE_NO || p.ask != 0.91 {
		t.Fatalf("got %+v %v, want NO at 0.91", p, ok)
	}
	thin := m
	thin.Volume24h = 10
	if _, ok := favorite(thin, r, testNow); ok {
		t.Fatal("took a market with almost no volume")
	}
	wide := m
	wide.NoBid = 0.80
	if _, ok := favorite(wide, r, testNow); ok {
		t.Fatal("took a 11¢ spread")
	}
	late := m
	late.CloseAt = testNow.Add(time.Minute)
	if _, ok := favorite(late, r, testNow); ok {
		t.Fatal("took a market a minute from its close")
	}
}

func TestBookSettlesAndReplays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.jsonl")
	b, err := OpenBook(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	lines := []Line{
		{At: testNow, Kind: KIND_WATCH, Ticker: "WIN", Side: SIDE_YES, Price: 0.90},
		{At: testNow, Kind: KIND_WATCH, Ticker: "LOSE", Side: SIDE_NO, Price: 0.86},
		{At: testNow, Kind: KIND_BUY, Ticker: "WIN", Event: "E1", Side: SIDE_YES, Price: 0.90, Qty: 20, CostUSD: 18, FeeUSD: 0.13},
		{At: testNow, Kind: KIND_BUY, Ticker: "LOSE", Event: "E2", Side: SIDE_NO, Price: 0.86, Qty: 20, CostUSD: 17.2, FeeUSD: 0.17},
		{At: testNow.Add(time.Hour), Kind: KIND_SETTLE, Ticker: "WIN", Result: RESULT_YES, Value: 1},
	}
	for _, l := range lines {
		if _, err := b.Append(l); err != nil {
			t.Fatal(err)
		}
	}
	a := b.Account()
	if a.Open != 1 || a.Settled != 1 || a.Won != 1 || a.RealizedUSD != 1.87 {
		t.Fatalf("account %+v", a)
	}
	if got := b.Unsettled(); len(got) != 1 || got[0] != "LOSE" {
		t.Fatalf("unsettled %v", got)
	}
	// YES won LOSE, so the NO position pays nothing.
	if _, err := b.Append(Line{At: testNow.Add(2 * time.Hour), Kind: KIND_SETTLE, Ticker: "LOSE", Result: RESULT_YES, Value: 1}); err != nil {
		t.Fatal(err)
	}
	b.Close()

	again, err := OpenBook(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	a = again.Account()
	if a.Open != 0 || a.Settled != 2 || a.RealizedUSD != 1.87-17.37 || a.CashUSD != 1000+1.87-17.37 {
		t.Fatalf("after replay %+v", a)
	}
	if lost := again.LostOn(Day(testNow)); lost != 17.37 {
		t.Fatalf("lost today %v, want 17.37", lost)
	}
	bands := again.Bands()
	if bands[0].Settled != 1 || bands[0].Won != 0 || bands[1].Settled != 1 || bands[1].Won != 1 {
		t.Fatalf("bands %+v", bands)
	}
}

func TestAsksTurnTheOtherSideIntoOffers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"orderbook_fp":{"no_dollars":[["0.0700","40.00"],["0.0900","25.00"]],"yes_dollars":[["0.8800","10.00"]]}}`))
	}))
	defer srv.Close()
	k := &Kalshi{HTTP: srv.Client(), Base: srv.URL}
	asks, err := k.Asks(context.Background(), "T", SIDE_YES)
	if err != nil {
		t.Fatal(err)
	}
	if len(asks) != 2 || asks[0].Price != 0.91 || asks[0].Qty != 25 || asks[1].Price != 0.93 {
		t.Fatalf("YES asks %+v, want 0.91 then 0.93", asks)
	}
}
