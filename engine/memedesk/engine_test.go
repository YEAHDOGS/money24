package memedesk

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	TEST_MINT   = "fixture-mint"
	TEST_POOL   = "fixture-pool"
	TEST_LIQ    = 20000.0
	TEST_MARGIN = 1e-9
)

var testStart = time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)

func pointer[T any](v T) *T {
	return &v
}

func testEngine(t *testing.T, rb Rulebook) *engine {
	t.Helper()
	rules, err := rb.build()
	if err != nil {
		t.Fatal(err)
	}
	e := newEngine()
	e.configure(rb, rules)
	return e
}

func safetyRow(mint string, at time.Time, unsafe bool) meme.Record {
	freeze := json.RawMessage(JSON_NULL)
	if unsafe {
		freeze = json.RawMessage(`"fixture-authority"`)
	}
	return meme.Record{At: at, Kind: meme.KIND_SAFETY, Mint: mint, Safety: &meme.Safety{
		MintAuthority: json.RawMessage(JSON_NULL), FreezeAuthority: freeze, Rugged: pointer(false),
		TopHolders: []meme.HolderShare{{Percent: pointer(5.0)}, {Percent: pointer(7.0)}},
		Markets:    []meme.SafetyMarket{{Pool: TEST_POOL, LP: &meme.LPLock{LockedPct: pointer(100.0)}}},
	}}
}

func migration(mint string, at time.Time) meme.Record {
	return meme.Record{At: at, Kind: meme.KIND_GRADUATION, Mint: mint, Graduation: &meme.Graduation{
		Venue: DEFAULT_GRADUATION_VENUE, UniqueBuyers: pointer(DEFAULT_BUYERS),
		SameBlock: pointer(false), CreatorSold: pointer(false), DeployerFunded: pointer(false),
	}}
}

func snapshot(mint string, at time.Time, priceUSD float64) meme.Record {
	return meme.Record{At: at, RequestedAt: at, Kind: meme.KIND_SNAPSHOT, Mint: mint,
		Market: &meme.Market{Pool: TEST_POOL, Venue: PUMPSWAP_VENUE, PriceUSD: pointer(priceUSD),
			Liquidity: &meme.Liquidity{USD: pointer(TEST_LIQ)}},
	}
}

func observeGraduate(e *engine, mint string, at time.Time) {
	e.observe(safetyRow(mint, at, false), true)
	e.observe(migration(mint, at), true)
	e.observe(snapshot(mint, at.Add(SIMULATED_LATENCY), 1), true)
}

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > TEST_MARGIN {
		t.Fatalf("got %f, want %f", got, want)
	}
}

func TestFillsUseFeesAndAdverseSizeImpact(t *testing.T) {
	m := snapshot(TEST_MINT, testStart, 2).Market
	buy, ok := buyFill(m, DEFAULT_STAKE, testStart)
	if !ok {
		t.Fatal("missing buy fill")
	}
	rate := PUMP_FEE_RATE + PORTAL_FEE_RATE
	near(t, buy.FeeUSD, buy.Units*buy.PriceUSD*rate)
	near(t, buy.NetUSD, buy.Units*buy.PriceUSD+buy.FeeUSD)
	sell := sellFill(m, buy.Units, testStart, EXIT_TIME)
	near(t, sell.FeeUSD, sell.Units*sell.PriceUSD*rate)
	near(t, sell.NetUSD, sell.Units*sell.PriceUSD-sell.FeeUSD)
	if buy.PriceUSD <= 2 || sell.PriceUSD >= 2 || sell.NetUSD >= buy.NetUSD {
		t.Fatalf("fills did not move adversely: %+v %+v", buy, sell)
	}
	thinner := snapshot(TEST_MINT, testStart, 2).Market
	thinner.Liquidity.USD = pointer(TEST_LIQ / 2)
	other, ok := buyFill(thinner, DEFAULT_STAKE, testStart)
	if !ok || other.Slippage <= buy.Slippage {
		t.Fatalf("liquidity did not affect slippage: %+v %+v", buy, other)
	}
	if _, ok := buyFill(m, TEST_LIQ, testStart); ok {
		t.Fatal("entry filled despite breaching slippage cap")
	}
	m.PriceUSD = nil
	if _, ok := buyFill(m, DEFAULT_STAKE, testStart); ok {
		t.Fatal("entry filled with an absent price")
	}
}

func TestOrganicStopWaitsForNewQuote(t *testing.T) {
	e := testEngine(t, DefaultRulebook())
	observeGraduate(e, TEST_MINT, testStart)
	s := e.states[KIND_ORGANIC]
	p := s.open[TEST_MINT].Paper
	trigger := p.Buy.At.Add(time.Minute)
	e.observe(snapshot(TEST_MINT, trigger, 0.70), true)
	e.observe(snapshot(TEST_MINT, trigger.Add(time.Second), 0.60), true)
	if len(p.Sales) != 0 {
		t.Fatal("sold on the stop trigger or before latency elapsed")
	}
	late := snapshot(TEST_MINT, trigger.Add(SIMULATED_LATENCY), 0.45)
	late.RequestedAt = trigger
	e.observe(late, true)
	if len(p.Sales) != 0 {
		t.Fatal("filled from a lookup requested before the simulated order arrived")
	}
	late.RequestedAt = late.At
	e.observe(late, true)
	if p.ClosedAt.IsZero() || p.Sales[0].Reason != EXIT_STOP || p.Sales[0].ObservedUSD != 0.45 || p.PnLUSD >= 0 {
		t.Fatalf("unexpected stop exit: %+v", p)
	}
	near(t, e.dayRealized, p.PnLUSD)
	near(t, e.rulebook.CapitalUSD+e.cashDelta, DEFAULT_CAPITAL+p.PnLUSD)
}

func TestTimeLimitWithoutQuoteAndLiquidityVanishing(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		disappear    bool
	}{
		{name: "quote returns", reason: EXIT_TIME},
		{name: "pool disappears", reason: EXIT_NO_LIQUIDITY, disappear: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine(t, DefaultRulebook())
			observeGraduate(e, TEST_MINT, testStart)
			p := e.states[KIND_ORGANIC].open[TEST_MINT].Paper
			due := p.Buy.At.Add(DEFAULT_HOLD_SECS * time.Second)
			noPrice := snapshot(TEST_MINT, due, 0)
			noPrice.Market.PriceUSD = nil
			e.observe(noPrice, true)
			if len(p.Sales) != 0 || !p.MarkStale {
				t.Fatalf("missing price should only stale the mark: %+v", p)
			}
			row := snapshot(TEST_MINT, due.Add(SIMULATED_LATENCY+time.Second), 1.05)
			if tc.disappear {
				row.Market = nil
			}
			e.observe(row, true)
			if p.ClosedAt.IsZero() || p.Sales[0].Reason != tc.reason {
				t.Fatalf("unexpected exit: %+v", p)
			}
			if tc.disappear {
				near(t, p.PnLUSD, -DEFAULT_STAKE)
				near(t, p.MarkUSD, 0)
				near(t, p.Sales[0].FeeUSD, 0)
			}
		})
	}
}

func TestShadowCountsAvoidedLossAndMissedWinnerWithoutCash(t *testing.T) {
	e := testEngine(t, DefaultRulebook())
	loser, winner, passed, unpriced := "loser", "winner", "passed", "unpriced"
	for _, mint := range []string{loser, winner, passed, unpriced} {
		e.observe(meme.Record{At: testStart, Kind: meme.KIND_LAUNCH, Mint: mint}, true)
		e.observe(safetyRow(mint, testStart, mint != passed), true)
	}
	entry := testStart.Add(SHADOW_DELAY)
	for _, mint := range []string{loser, winner, passed} {
		e.observe(snapshot(mint, entry, 1), true)
	}
	peak := entry.Add(time.Minute)
	e.observe(snapshot(winner, peak, 1.8), true)
	e.observe(snapshot(winner, peak.Add(SIMULATED_LATENCY), 1.7), true)
	e.observe(snapshot(winner, peak.Add(2*SIMULATED_LATENCY), 1.65), true)
	e.observe(meme.Record{At: peak.Add(3 * SIMULATED_LATENCY), Kind: meme.KIND_SNAPSHOT, Mint: loser}, true)
	e.observe(snapshot(passed, entry.Add(DEFAULT_HOLD_SECS*time.Second+SIMULATED_LATENCY), 1), true)
	e.advance(testStart.Add(FOLLOW_WINDOW))
	s := e.states[KIND_SHADOW]
	r := s.record
	if r.Candidates != 4 || r.Refused != 3 || r.Passed != 1 || r.LossesAvoided != 1 ||
		r.WinnersMissed != 1 || r.WinnersMissedUSD <= 0 || r.BenchmarkUnavailable != 1 ||
		r.BenchmarkClosed != 3 || r.PassedBenchmarkUSD >= 0 || r.Reasons[REASON_FREEZE_ACTIVE] != 3 {
		t.Fatalf("unexpected shadow record: %+v", r)
	}
	near(t, r.LossesAvoidedUSD, DEFAULT_STAKE)
	near(t, e.cashDelta, 0)
	if got := s.recent[1]; got.Path.HighUSD != 1.8 || len(got.Benchmark.Sales) != 2 {
		t.Fatalf("winner path or ladder missing: %+v", got)
	}
}

func TestMissingOrganicEvidenceAndDailyLossStop(t *testing.T) {
	rb := DefaultRulebook()
	rb.DailyLossUSD = DEFAULT_STAKE / 2
	e := testEngine(t, rb)
	observeGraduate(e, TEST_MINT, testStart)
	disappeared := testStart.Add(time.Minute)
	e.observe(meme.Record{At: disappeared, Kind: meme.KIND_SNAPSHOT, Mint: TEST_MINT}, true)
	observeGraduate(e, "blocked", disappeared)
	s := e.states[KIND_ORGANIC]
	if !e.stopped || s.open["blocked"] != nil || !slices.Contains(s.recent[1].Decision.Reasons, REASON_DAILY_LOSS) {
		t.Fatalf("desk did not stop entries after a loss: %+v", s.record)
	}
	nextDay := testStart.AddDate(0, 0, 1)
	observeGraduate(e, "next-day", nextDay)
	if e.stopped || s.open["next-day"] == nil {
		t.Fatal("daily loss stop did not reset at the UTC day boundary")
	}
	unknown := "missing-evidence"
	row := migration(unknown, nextDay.Add(time.Minute))
	row.Graduation.UniqueBuyers, row.Graduation.SameBlock = nil, nil
	e.observe(safetyRow(unknown, row.At, false), true)
	e.observe(row, true)
	e.observe(snapshot(unknown, row.At.Add(SIMULATED_LATENCY), 1), true)
	got := s.recent[len(s.recent)-1].Decision
	if got.Action != DECISION_SKIP || !slices.Contains(got.Reasons, REASON_BUYERS_UNKNOWN) ||
		!slices.Contains(got.Reasons, REASON_BLOCK_UNKNOWN) {
		t.Fatalf("missing data did not cause a refusal: %+v", got)
	}
}
