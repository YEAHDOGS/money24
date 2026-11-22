package predict

import (
	"context"
	"encoding/json"
	"math"
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

func build(t *testing.T, c Config) running {
	t.Helper()
	runs, err := Rulebook{CapitalUSD: 1000, DailyLossUSD: 100, Mode: RISK_HIGH, Strategies: []Config{c}}.Build()
	if err != nil {
		t.Fatal(err)
	}
	return runs[0]
}

func TestFavoritesTakeOnlyTheSideInRange(t *testing.T) {
	r := build(t, defaultFavorites())
	m := Market{Ticker: "A", Type: MARKET_BINARY, CloseAt: testNow.Add(2 * time.Hour), Volume24h: 500,
		YesBid: 0.08, YesAsk: 0.10, NoBid: 0.89, NoAsk: 0.91}
	c, ok := consider(r, m, testNow)
	if !ok || c.side != SIDE_NO || c.ask != 0.91 {
		t.Fatalf("got %+v %v, want NO at 0.91", c, ok)
	}
	for name, change := range map[string]func(*Market){
		"thin":  func(m *Market) { m.Volume24h = 10 },
		"wide":  func(m *Market) { m.NoBid = 0.80 },
		"late":  func(m *Market) { m.CloseAt = testNow.Add(time.Minute) },
		"cheap": func(m *Market) { m.NoAsk, m.NoBid = 0.70, 0.69 },
	} {
		bad := m
		change(&bad)
		if _, ok := consider(r, bad, testNow); ok {
			t.Errorf("took a %s market", name)
		}
	}
}

func TestRulebookRejectsMistakes(t *testing.T) {
	good := DefaultRulebook()
	if _, err := good.Build(); err != nil {
		t.Fatalf("defaults do not build: %v", err)
	}
	cases := map[string]func(*Rulebook){
		"duplicate id": func(rb *Rulebook) { rb.Strategies = append(rb.Strategies, rb.Strategies[0]) },
		"unknown kind": func(rb *Rulebook) { rb.Strategies[0].Kind = "astrology" },
		"bad risk":     func(rb *Rulebook) { rb.Strategies[0].Risk = "yolo" },
		"bad prices":   func(rb *Rulebook) { rb.Strategies[0].Entry.MinPrice = 0.99 },
		"bad id":       func(rb *Rulebook) { rb.Strategies[0].ID = "Has Spaces" },
		"bad mode":     func(rb *Rulebook) { rb.Mode = "max" },
	}
	for name, change := range cases {
		rb := DefaultRulebook()
		change(&rb)
		if _, err := rb.Build(); err == nil {
			t.Errorf("%s: built anyway", name)
		}
	}
}

func TestRulebookIsWrittenThenRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), RULEBOOK_FILE)
	first, err := LoadRulebook(path)
	if err != nil {
		t.Fatal(err)
	}
	first.Strategies[0].On = false
	if err := SaveRulebook(path, first); err != nil {
		t.Fatal(err)
	}
	again, err := LoadRulebook(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Strategies[0].On || len(again.Strategies) != len(DefaultRulebook().Strategies) {
		t.Fatalf("read back %+v", again.Strategies[0])
	}
}

func TestBookSettlesAndReplaysPerStrategy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.jsonl")
	b, err := OpenBook(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	lines := []Line{
		{At: testNow, Kind: KIND_WATCH, Ticker: "WIN", Side: SIDE_YES, Price: 0.90}, // written before strategies had names
		{At: testNow, Kind: KIND_WATCH, Strategy: KNOWN_ID, Ticker: "WIN", Side: SIDE_YES, Price: 0.92, Prob: 0.98},
		{At: testNow, Kind: KIND_WATCH, Strategy: KNOWN_ID, Ticker: "LOSE", Side: SIDE_NO, Price: 0.86, Prob: 0.96},
		{At: testNow, Kind: KIND_BUY, Strategy: KNOWN_ID, Ticker: "WIN", Event: "E1", Side: SIDE_YES, Price: 0.90, Qty: 20, CostUSD: 18, FeeUSD: 0.13},
		{At: testNow, Kind: KIND_BUY, Strategy: KNOWN_ID, Ticker: "LOSE", Event: "E2", Side: SIDE_NO, Price: 0.86, Qty: 20, CostUSD: 17.2, FeeUSD: 0.17},
		{At: testNow.Add(time.Hour), Kind: KIND_SETTLE, Ticker: "WIN", Result: RESULT_YES, Value: 1},
		{At: testNow.Add(2 * time.Hour), Kind: KIND_SETTLE, Ticker: "LOSE", Result: RESULT_YES, Value: 1},
	}
	for _, l := range lines {
		if _, err := b.Append(l); err != nil {
			t.Fatal(err)
		}
	}
	b.Close()
	again, err := OpenBook(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	a := again.Account("")
	if a.Open != 0 || a.Settled != 2 || a.Won != 1 || math.Abs(a.CashUSD-(1000+1.87-17.37)) > 0.001 {
		t.Fatalf("account %+v", a)
	}
	if lost := again.LostOn(Day(testNow)); lost != 17.37 {
		t.Fatalf("lost today %v, want 17.37", lost)
	}
	known := again.Record(KNOWN_ID, nil)
	if known.Watched != 2 || known.Settled != 2 || known.Won != 1 || known.AvgProb != 97 {
		t.Fatalf("known record %+v", known)
	}
	fav := again.Record(FAVORITES_ID, []float64{0.85, 0.88, 0.91, 0.95})
	if fav.Watched != 1 || fav.Won != 1 || fav.Bands[1].Won != 1 {
		t.Fatalf("legacy line not counted as favorites: %+v", fav)
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

func TestMarketRangeAndDate(t *testing.T) {
	between := Market{Event: "KXHIGHNY-26NOV21", Strike: STRIKE_BETWEEN, Floor: 64, Cap: 65}
	lo, hi, ok := between.Range()
	if !ok || lo != 63.5 || hi != 65.5 {
		t.Fatalf("between range %v %v %v", lo, hi, ok)
	}
	if lo, _, _ := (Market{Strike: STRIKE_GREATER, Floor: 68}).Range(); lo != 68.5 {
		t.Fatalf("greater than 68 starts at %v, want 68.5 (69 or above)", lo)
	}
	if _, hi, _ := (Market{Strike: STRIKE_LESS, Cap: 61}).Range(); hi != 60.5 {
		t.Fatalf("less than 61 ends at %v, want 60.5 (60 or below)", hi)
	}
	day, err := between.EventDate()
	if err != nil || day.Format(DAY_LAYOUT) != "2026-11-21" || between.Series() != "KXHIGHNY" {
		t.Fatalf("date %v %v, series %s", day, err, between.Series())
	}
}

func TestWeatherJoinsTheRecordWithTheForecast(t *testing.T) {
	s, err := newWeather(nil)
	if err != nil {
		t.Fatal(err)
	}
	w := s.(*weather)
	info := w.series["KXHIGHNY"]
	day := time.Date(2026, 11, 22, 0, 0, 0, 0, info.loc)
	start, end := day.UTC(), day.Add(DAY).UTC()
	now := start.Add(16 * time.Hour) // 4 PM standard time
	// Every run says the rest of the day stays near 58°, but 63.2° was already recorded at 2 PM.
	ens := &Ensemble{}
	for h := start; h.Before(end); h = h.Add(time.Hour) {
		ens.Hours = append(ens.Hours, h)
	}
	for range MIN_RUNS * 2 {
		run := make([]float64, len(ens.Hours))
		for i := range run {
			run[i] = 58
		}
		ens.Members = append(ens.Members, run)
	}
	obs := &Readings{Times: []time.Time{start.Add(14 * time.Hour)}, TempsF: []float64{63.2}}
	model, _, ok := w.model(info, ens, obs, start, end, now)
	if !ok {
		t.Fatal("no model")
	}
	if p := model.prob(62.5, 64.5); p < 0.95 {
		t.Fatalf("63–64° got %.3f: the record should win over a cooler forecast", p)
	}
	if p := model.prob(math.Inf(-1), 60.5); p > 0.001 {
		t.Fatalf("60° or below got %.4f after 63° was recorded", p)
	}
	// The record decides 60° or below (the high is already past it), but not 63–64° (the day can still warm).
	if !model.decided(math.Inf(-1), 60.5, DEFAULT_KNOWN_MARGIN) || model.decided(62.5, 64.5, DEFAULT_KNOWN_MARGIN) {
		t.Fatal("known: wrong ranges decided by the record")
	}
	// Before the day starts nothing is recorded, and only the forecast speaks: about 58°.
	early, _, ok := w.model(info, ens, nil, start, end, start.Add(-6*time.Hour))
	if p := early.prob(56.5, 60.5); !ok || p < 0.55 || p > 0.85 {
		t.Fatalf("57–60° from the forecast alone got %.2f", p)
	}
}

func TestWeatherParamsLimitStations(t *testing.T) {
	// Settings left out keep their defaults.
	s, err := newWeather(json.RawMessage(`{"only": ["knyc"], "highs": true, "lows": false}`))
	if err != nil {
		t.Fatal(err)
	}
	scope := s.Scope().Series
	if len(scope) != 1 || scope[0] != "KXHIGHNY" {
		t.Fatalf("scope %v, want only New York's highs", scope)
	}
}
