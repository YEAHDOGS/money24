package signals

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const TEST_CAPITAL = 1000.0

// cashRunner is a runner with a cash account of capital dollars and a $500 trade budget.
func cashRunner(t *testing.T, capital float64, quote QuoteFunc) *Runner {
	t.Helper()
	j, err := OpenJournal(t.TempDir() + "/journal.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	s := SETTINGS
	s.MaxUSDPerTrade = 500
	return NewRunner(&Catalog{Defaults: s, GoLive: GoLive{PaperCapitalUSD: capital}}, j, quote)
}

func coin(symbol, postID string, price float64) Signal {
	return Signal{SourceID: "x:caller", PostID: postID, Kind: KIND_ENTRY, Symbol: symbol, Market: MARKET_CRYPTO, Price: price, Confidence: CONFIDENCE_FULL, SeenAt: NOW}
}

// An option trade through the account: the buy leaves cash, a sold rung comes back but cannot be
// spent until the next session, and the stop returns what is left.
func TestAccountBuysSellsAndSettles(t *testing.T) {
	bid := 1.18
	quote := func(Signal) (Quote, bool) {
		return Quote{Bid: bid, Ask: bid + 0.04, OpenInterest: 5000, Volume: 900}, true
	}
	r := cashRunner(t, TEST_CAPITAL, quote)
	if a := r.Account(NOW); a.CashUSD != 1000 || a.BuyingPowerUSD != 1000 || a.EquityUSD != 1000 || a.Open != 0 || a.Settles == nil {
		t.Fatalf("empty account: %+v", a)
	}
	if e, _, _ := r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW); e.Plan == nil || e.Plan.Qty != 4 || e.Plan.CostUSD != 488 {
		t.Fatalf("plan: %+v", e)
	}
	if a := r.Account(NOW); a.CashUSD != 512 || a.BuyingPowerUSD != 512 || a.InvestedUSD != 488 || a.EquityUSD != 1000 || a.Open != 1 {
		t.Fatalf("after the buy: %+v", a)
	}

	// The first rung sells one contract at 1.59: $159 back, $37 of it gain, none of it settled yet.
	bid = 1.60
	sold := NOW.Add(2 * time.Minute)
	if out, _ := r.Tick(sold); len(out) != 1 || out[0].Action != ACTION_FILL {
		t.Fatalf("first target: %+v", out)
	}
	a := r.Account(sold)
	if a.CashUSD != 671 || a.UnsettledUSD != 159 || a.BuyingPowerUSD != 512 || a.InvestedUSD != 366 || a.RealizedUSD != 37 || a.EquityUSD != 1037 {
		t.Fatalf("after the rung: %+v", a)
	}
	open := time.Date(2026, 11, 10, SETTLE_HOUR, SETTLE_MINUTE, 0, 0, MARKET)
	if len(a.Settles) != 1 || !a.Settles[0].At.Equal(open) || a.Settles[0].USD != 159 {
		t.Fatalf("settles: %+v, want $159 at %s", a.Settles, open)
	}
	if a := r.Account(open.Add(-time.Minute)); a.BuyingPowerUSD != 512 {
		t.Fatalf("proceeds were spendable before they settled: %+v", a)
	}
	if a := r.Account(open); a.UnsettledUSD != 0 || a.BuyingPowerUSD != 671 || len(a.Settles) != 0 {
		t.Fatalf("at the next open: %+v", a)
	}

	// The stop sells the other three at 0.90: the account ends where the scoreboard does.
	bid = 0.90
	out, _ := r.Tick(open.Add(time.Minute))
	if len(out) != 1 || out[0].Action != ACTION_CLOSE || out[0].Result.PnLUSD != -59 {
		t.Fatalf("stop: %+v", out)
	}
	a = r.Account(open.Add(time.Minute))
	if a.CashUSD != 941 || a.UnsettledUSD != 270 || a.BuyingPowerUSD != 671 || a.InvestedUSD != 0 || a.RealizedUSD != -59 || a.EquityUSD != 941 || a.Open != 0 {
		t.Fatalf("after the stop: %+v", a)
	}
	if all := Scoreboard(r.journal.All(), r.catalog.GoLive)[0]; all.PnLUSD != a.RealizedUSD {
		t.Fatalf("scoreboard says %v, the account %v", all.PnLUSD, a.RealizedUSD)
	}
}

func TestSettleNextWeekday(t *testing.T) {
	ny := func(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, MARKET) }
	option, share := Signal{Symbol: "SPY", Right: RIGHT_CALL}, Signal{Symbol: "SPY"}
	cases := []struct {
		name string
		sig  Signal
		sold time.Time
		want time.Time
	}{
		{"Monday option sale settles Tuesday", option, ny(9, 15, 0), ny(10, 9, 30)},
		{"Thursday evening settles Friday", share, ny(12, 20, 0), ny(13, 9, 30)},
		{"Friday sale settles Monday", option, ny(13, 15, 0), ny(16, 9, 30)},
		{"late Friday in New York is still Friday", share, ny(13, 23, 0).UTC(), ny(16, 9, 30)},
		{"Saturday settles Monday", share, ny(14, 12, 0), ny(16, 9, 30)},
		{"a coin settles at once", Signal{Symbol: "BTC", Market: MARKET_CRYPTO}, ny(14, 12, 0), ny(14, 12, 0)},
		{"an on-chain token settles at once", Signal{Symbol: "WIF", Address: "token"}, ny(13, 15, 0), ny(13, 15, 0)},
	}
	for _, c := range cases {
		if got := SettleNextWeekday(c.sig, c.sold); !got.Equal(c.want) {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// Coins settle at once: with the account fully spent, a sold rung funds the next buy the same minute.
func TestCryptoProceedsAreSpendableAtOnce(t *testing.T) {
	bid := 100.0
	r := cashRunner(t, TEST_CAPITAL, func(Signal) (Quote, bool) { return Quote{Bid: bid, Ask: bid, Volume: 900}, true })
	r.catalog.Defaults.MaxUSDPerTrade = 1000
	if e, _, _ := r.Handle(coin("BTC", "1", 100), NOW); e.Plan == nil || e.Plan.Qty != 10 {
		t.Fatalf("first buy: %+v", e)
	}
	if a := r.Account(NOW); a.CashUSD != 0 || a.BuyingPowerUSD != 0 || a.InvestedUSD != 1000 {
		t.Fatalf("fully invested: %+v", a)
	}
	e, _, err := r.Handle(coin("ETH", "2", 100), NOW)
	if err != nil || e.Plan != nil || !strings.HasPrefix(e.Rejected, ErrNoCash.Error()) {
		t.Fatalf("a buy with no cash went through: %+v %v", e, err)
	}

	bid = 130
	sold := NOW.Add(time.Minute)
	if out, _ := r.Tick(sold); len(out) != 1 || out[0].Action != ACTION_FILL || out[0].Result.Qty != 2.5 {
		t.Fatalf("first target: %+v", out)
	}
	a := r.Account(sold)
	if a.CashUSD != 325 || a.UnsettledUSD != 0 || a.BuyingPowerUSD != 325 || len(a.Settles) != 0 || a.RealizedUSD != 75 {
		t.Fatalf("coin proceeds: %+v", a)
	}
	// The next buy is cut to the cash there is, and says so.
	e, _, _ = r.Handle(coin("SOL", "3", 130), sold)
	if e.Plan == nil || e.Plan.CostUSD != 325 || !strings.Contains(e.Note, ErrNoCash.Error()) {
		t.Fatalf("buy sized to cash: %+v", e)
	}
	if a := r.Account(sold); a.CashUSD != 0 || a.Open != 2 {
		t.Fatalf("after the second buy: %+v", a)
	}
}

// A position voided for want of a quote was never scored: its cost comes back whole.
func TestVoidReturnsTheCost(t *testing.T) {
	r := cashRunner(t, TEST_CAPITAL, nil)
	r.catalog.Defaults.MaxHoldMinutes = 60
	if e, _, _ := r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW); e.Plan == nil || e.Plan.CostUSD != 480 {
		t.Fatalf("plan: %+v", e)
	}
	if a := r.Account(NOW); a.CashUSD != 520 || a.Open != 1 {
		t.Fatalf("after the buy: %+v", a)
	}
	late := NOW.Add(61 * time.Minute)
	if out, _ := r.Tick(late); len(out) != 1 || out[0].Action != ACTION_VOID {
		t.Fatalf("void: %+v", out)
	}
	if a := r.Account(late); a.CashUSD != 1000 || a.BuyingPowerUSD != 1000 || a.InvestedUSD != 0 || a.RealizedUSD != 0 || a.Open != 0 {
		t.Fatalf("after the void: %+v", a)
	}
}

func TestAdoptRefusesWhatCashCannotPay(t *testing.T) {
	r := cashRunner(t, TEST_CAPITAL, nil)
	bracket := func(postID string, cost float64) (Signal, Plan) {
		sig := coin("BTC", postID, 100)
		return sig, Plan{Signal: sig, Qty: cost / 100, Entry: 100, Stop: 80, Targets: []Target{{Qty: cost / 100, Price: 130}}, CostUSD: cost}
	}
	sig, plan := bracket("1", 1500)
	if _, err := r.Adopt(sig, plan, NOW); !errors.Is(err, ErrNoCash) {
		t.Fatalf("a $1,500 bracket on $1,000: %v", err)
	}
	if lines := r.journal.All(); len(lines) != 0 {
		t.Fatalf("a refused bracket was journaled: %+v", lines)
	}
	sig, plan = bracket("2", 600)
	if _, err := r.Adopt(sig, plan, NOW); err != nil {
		t.Fatal(err)
	}
	sig, plan = bracket("3", 600)
	if _, err := r.Adopt(sig, plan, NOW); !errors.Is(err, ErrNoCash) || !strings.Contains(err.Error(), "$400.00") {
		t.Fatalf("a second $600 bracket with $400 left: %v", err)
	}
	sig, plan = bracket("4", 400)
	if _, err := r.Adopt(sig, plan, NOW); err != nil {
		t.Fatalf("a bracket for exactly the cash left: %v", err)
	}
	if a := r.Account(NOW); a.CashUSD != 0 || a.InvestedUSD != 1000 || a.Open != 2 {
		t.Fatalf("account: %+v", a)
	}
}

// Cash never goes below zero, not even by the cent an entry gains when it rounds up to a tick.
func TestCashNeverGoesNegative(t *testing.T) {
	quote := func(Signal) (Quote, bool) { return Quote{Bid: 1.2, Ask: 1.226, OpenInterest: 5000, Volume: 900}, true }
	r := cashRunner(t, 368.80, quote)
	r.catalog.Defaults.MaxUSDPerTrade = 300
	// Two contracts at an ask of 1.226 fill at the rounded 1.23: $246 leaves the account.
	if e, _, _ := r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW); e.Plan == nil || e.Plan.Qty != 2 || e.Plan.Entry != 1.23 {
		t.Fatalf("first buy: %+v", e)
	}
	// $122.80 is left. One contract is $122.60 at the ask but $123 at the rounded entry: refused.
	e, _, _ := r.Handle(callout("BTO $AMD 150C 11/21 @ 1.20", "2"), NOW)
	if e.Plan != nil || !strings.HasPrefix(e.Rejected, ErrNoCash.Error()) {
		t.Fatalf("a buy a cent over the cash went through: %+v", e)
	}
	if a := r.Account(NOW); a.CashUSD != 122.80 || a.BuyingPowerUSD != 122.80 {
		t.Fatalf("account: %+v", a)
	}

	// Many buys at an awkward price: every one fits the cash there is, and the last ones are refused.
	r = cashRunner(t, TEST_CAPITAL, quote)
	r.catalog.Defaults.MaxUSDPerTrade = 300
	planned, refused := 0, 0
	for i := range 12 {
		e, _, err := r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", string(rune('a'+i))), NOW)
		if err != nil {
			t.Fatal(err)
		}
		if e.Plan != nil {
			planned++
		} else if strings.HasPrefix(e.Rejected, ErrNoCash.Error()) {
			refused++
		}
		if a := r.Account(NOW); a.CashUSD < 0 || a.BuyingPowerUSD < 0 || a.CashUSD+a.InvestedUSD != TEST_CAPITAL {
			t.Fatalf("buy %d: %+v", i, a)
		}
	}
	if planned != 4 || refused != 8 {
		t.Fatalf("planned %d, refused %d", planned, refused)
	}
}

// The account counts every line ever written, though the journal keeps only its tail in memory,
// and a restart adds it up to the same cent.
func TestAccountSurvivesReopenAndALongJournal(t *testing.T) {
	path := t.TempDir() + "/journal.jsonl"
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &Catalog{Defaults: SETTINGS, GoLive: GoLive{PaperCapitalUSD: TEST_CAPITAL}}
	r := NewRunner(catalog, j, nil)
	const trips = JOURNAL_RECENT // two lines each: twice what memory keeps
	sig := coin("BTC", "", 10)
	for i := range trips {
		at := NOW.Add(time.Duration(i) * time.Minute)
		id, err := r.Adopt(sig, Plan{Signal: sig, Qty: 1, Entry: 10, Stop: 8, Targets: []Target{{Qty: 1, Price: 11}}, CostUSD: 10}, at)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := j.Append(Entry{At: at, Action: ACTION_CLOSE, Signal: sig, PlanID: id, Result: &Result{PnLUSD: 1, Reason: REASON_TARGET, Qty: 1, Price: 11}}); err != nil {
			t.Fatal(err)
		}
	}
	// One more left open, so the reopened account has a position to find.
	if _, err := r.Adopt(sig, Plan{Signal: sig, Qty: 2, Entry: 10, Stop: 8, Targets: []Target{{Qty: 2, Price: 11}}, CostUSD: 20}, NOW); err != nil {
		t.Fatal(err)
	}
	want := r.Account(NOW)
	if want.RealizedUSD != trips || want.CashUSD != TEST_CAPITAL+trips-20 || want.InvestedUSD != 20 || want.Open != 1 {
		t.Fatalf("account: %+v", want)
	}
	if kept := len(j.All()); kept != JOURNAL_RECENT {
		t.Fatalf("journal keeps %d lines in memory, want %d", kept, JOURNAL_RECENT)
	}
	j.Close()

	again, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	got := NewRunner(catalog, again, nil).Account(NOW)
	if got.CashUSD != want.CashUSD || got.RealizedUSD != want.RealizedUSD || got.InvestedUSD != want.InvestedUSD || got.EquityUSD != want.EquityUSD || got.Open != want.Open {
		t.Fatalf("after a restart: %+v, want %+v", got, want)
	}
}

// A calendar plugged in after the journal was read moves sales already waiting, holidays included.
func TestSetSettlementMovesWaitingSales(t *testing.T) {
	bid := 1.18
	quote := func(Signal) (Quote, bool) {
		return Quote{Bid: bid, Ask: bid + 0.04, OpenInterest: 5000, Volume: 900}, true
	}
	r := cashRunner(t, TEST_CAPITAL, quote)
	r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW)
	bid = 1.60
	r.Tick(NOW.Add(time.Minute))
	holiday := time.Date(2026, 11, 11, SETTLE_HOUR, SETTLE_MINUTE, 0, 0, MARKET)
	r.SetSettlement(func(Signal, time.Time) time.Time { return holiday })
	tuesday := time.Date(2026, 11, 10, 12, 0, 0, 0, MARKET)
	if a := r.Account(tuesday); a.UnsettledUSD != 159 || len(a.Settles) != 1 || !a.Settles[0].At.Equal(holiday) {
		t.Fatalf("with the calendar: %+v", a)
	}
	if a := r.Account(holiday); a.UnsettledUSD != 0 || a.BuyingPowerUSD != 671 {
		t.Fatalf("once settled: %+v", a)
	}
}
