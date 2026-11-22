package predict

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"testing"
	"time"
)

func dailyPoint(year int, month time.Month, day int, value float64) DailyPoint {
	return DailyPoint{Date: time.Date(year, month, day, 0, 0, 0, 0, time.UTC), Value: value}
}

func TestDailyEstimateKnownGapAndSpread(t *testing.T) {
	points := []DailyPoint{
		dailyPoint(2026, 11, 19, 3.60),
		dailyPoint(2026, 11, 20, 3.63),
		dailyPoint(2026, 11, 21, 3.61),
	}
	known, ok := estimate(points, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), DEFAULT_DAILY_LOOKBACK, DEFAULT_DAILY_SIGMA_FLOOR_FRAC)
	if !ok || !known.known || known.knownValue != 3.61 {
		t.Fatalf("known estimate %+v %v, want the published 3.61", known, ok)
	}
	// Moves are +0.03 and -0.02: RMS daily move sqrt((0.03^2+0.02^2)/2) = 0.0255, over the floor.
	wantDaily := math.Sqrt((0.03*0.03 + 0.02*0.02) / 2)
	ahead, ok := estimate(points, time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), DEFAULT_DAILY_LOOKBACK, DEFAULT_DAILY_SIGMA_FLOOR_FRAC)
	if !ok || ahead.known || ahead.gapDays != 1 || math.Abs(ahead.mean-3.61) > 1e-9 || math.Abs(ahead.sigma-wantDaily) > 1e-9 {
		t.Fatalf("one-day estimate %+v %v, want mean 3.61 sigma %.6f", ahead, ok, wantDaily)
	}
	twoDays, ok := estimate(points, time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), DEFAULT_DAILY_LOOKBACK, DEFAULT_DAILY_SIGMA_FLOOR_FRAC)
	if !ok || twoDays.gapDays != 2 || math.Abs(twoDays.sigma-wantDaily*math.Sqrt2) > 1e-9 {
		t.Fatalf("two-day estimate %+v %v, want sigma %.6f (spread grows with sqrt of days)", twoDays, ok, wantDaily*math.Sqrt2)
	}
	if _, ok := estimate(points, time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC), DEFAULT_DAILY_LOOKBACK, DEFAULT_DAILY_SIGMA_FLOOR_FRAC); ok {
		t.Fatal("a date before every publication got an estimate")
	}
	if _, ok := estimate(nil, time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), DEFAULT_DAILY_LOOKBACK, DEFAULT_DAILY_SIGMA_FLOOR_FRAC); ok {
		t.Fatal("no publications got an estimate")
	}
}

func TestDailySigmaFloorStopsQuietDaysOverconfidence(t *testing.T) {
	points := []DailyPoint{
		dailyPoint(2026, 11, 20, 100),
		dailyPoint(2026, 11, 21, 100),
	}
	est, ok := estimate(points, time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), DEFAULT_DAILY_LOOKBACK, 0.01)
	if !ok || math.Abs(est.sigma-1.0) > 1e-9 {
		t.Fatalf("quiet-day estimate %+v %v, want the 1%% floor sigma of 1.0", est, ok)
	}
}

func TestDailyYesProbUsesExactThresholds(t *testing.T) {
	// At the threshold itself the chance is a coin flip: these markets settle on the
	// exact published value, so no whole-unit half offset (Market.Range) may creep in.
	at := dailyEstimate{mean: 3.70, sigma: 0.03}
	greater := Market{Strike: STRIKE_GREATER, Floor: 3.70}
	if prob, ok := yesProb(at, greater); !ok || math.Abs(prob-0.5) > 1e-9 {
		t.Fatalf("greater at its threshold got %.6f %v, want 0.5", prob, ok)
	}
	equal := Market{Strike: STRIKE_GREATER_OR_EQUAL, Floor: 3.70}
	if prob, ok := yesProb(at, equal); !ok || math.Abs(prob-0.5) > 1e-9 {
		t.Fatalf("greater_or_equal at its threshold got %.6f %v, want 0.5", prob, ok)
	}
	between := Market{Strike: STRIKE_BETWEEN, Floor: 3.60, Cap: 3.70}
	wantBetween := normalCDF(0) - normalCDF((3.60-3.70)/0.03)
	if prob, ok := yesProb(at, between); !ok || math.Abs(prob-wantBetween) > 1e-9 {
		t.Fatalf("between got %.6f %v, want %.6f", prob, ok, wantBetween)
	}
	less := Market{Strike: STRIKE_LESS, Cap: 3.60}
	wantLess := normalCDF((3.60 - 3.70) / 0.03)
	if prob, ok := yesProb(at, less); !ok || math.Abs(prob-wantLess) > 1e-9 {
		t.Fatalf("less got %.6f %v, want %.6f", prob, ok, wantLess)
	}
	if _, ok := yesProb(at, Market{Strike: "sideways", Floor: 1}); ok {
		t.Fatal("an unknown strike got a chance")
	}
}

func TestDailyKnownValueDecidesExactly(t *testing.T) {
	market := Market{Strike: STRIKE_GREATER, Floor: 3.70}
	cases := []struct {
		value float64
		want  float64
	}{
		{3.7001, 1},
		{3.70, 0},
		{3.6999, 0},
	}
	for _, c := range cases {
		prob, ok := yesProb(dailyEstimate{known: true, knownValue: c.value}, market)
		if !ok || prob != c.want {
			t.Errorf("published %v against above-3.70 got %v, want %v", c.value, prob, c.want)
		}
	}
	equal := Market{Strike: STRIKE_GREATER_OR_EQUAL, Floor: 3200000}
	if prob, _ := yesProb(dailyEstimate{known: true, knownValue: 3200000}, equal); prob != 1 {
		t.Fatalf("at-least market at exactly its floor got %v, want 1", prob)
	}
}

func TestParseAAAReadsRegularAndDiesel(t *testing.T) {
	body := `<p>National average as of 11/21/26</p><table class="table-mob"><thead><tr><th></th><th>Regular</th><th>Mid-Grade</th><th>Premium</th><th>Diesel</th><th>E85</th></tr></thead>` +
		`<tbody><tr><td>Current Avg.</td><td>$3.6594</td><td>$4.1987</td><td>$4.5875</td><td>$5.8649</td><td>$2.7613</td></tr>` +
		`<tr><td>Yesterday Avg.</td><td>$3.6898</td><td>$4.2238</td><td>$4.6181</td><td>$5.8982</td><td>$2.7945</td></tr>` +
		`<tr><td>Week Ago Avg.</td><td>$3.8406</td><td>$4.3688</td><td>$4.7645</td><td>$6.0366</td><td>$2.9545</td></tr></tbody></table>` +
		// The real page's record-high table reuses the grade names in other columns.
		`<table><tr><td>Diesel</td><td></td><td></td><td>$6.5276</td><td>9/22/26</td></tr></table>`
	regular, diesel, err := parseAAA(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(regular) != 3 || len(diesel) != 3 {
		t.Fatalf("parsed %d regular and %d diesel points, want 3 each", len(regular), len(diesel))
	}
	latest := regular[len(regular)-1]
	if latest.Value != 3.6594 || latest.Date.Format(DAY_LAYOUT) != "2026-11-21" {
		t.Fatalf("latest regular %+v, want $3.6594 on 2026-11-21", latest)
	}
	oldest := diesel[0]
	if oldest.Value != 6.0366 || oldest.Date.Format(DAY_LAYOUT) != "2026-11-14" {
		t.Fatalf("oldest diesel %+v, want $6.0366 on 2026-11-14 (a week before)", oldest)
	}
	if _, _, err := parseAAA(`<p>nothing here</p>`); err == nil {
		t.Fatal("a page with no AAA table parsed anyway")
	}
}

func TestParseTSAReadsDailyCounts(t *testing.T) {
	body := `<table class="table"><tbody>` +
		`<tr><td class="text-align-center">11/19/2026</td><td class="text-align-center">2,609,107</td></tr>` +
		`<tr><td class="text-align-center">11/18/2026</td><td class="text-align-center">2,126,817</td></tr>` +
		`</tbody></table>`
	points, err := parseTSA(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 || points[0].Value != 2126817 || points[1].Value != 2609107 {
		t.Fatalf("TSA points %+v, want oldest first: 2,126,817 then 2,609,107", points)
	}
	if _, err := parseTSA(`<p>no table</p>`); err == nil {
		t.Fatal("a page with no TSA table parsed anyway")
	}
}

func dailyStrategy(t *testing.T, knownOnly bool, snapshots map[string]*DailySnapshot) *dailyCounts {
	t.Helper()
	raw := dailyParams(knownOnly)
	strategy, err := newDailyCounts(raw)
	if err != nil {
		t.Fatal(err)
	}
	d := strategy.(*dailyCounts)
	d.data = &dailyData{snapshots: snapshots}
	return d
}

func TestDailyEvaluateKnownAndForecast(t *testing.T) {
	now := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	tsa := &DailySnapshot{Points: []DailyPoint{
		dailyPoint(2026, 11, 18, 2126817),
		dailyPoint(2026, 11, 19, 2609107),
		dailyPoint(2026, 11, 21, 2900000),
	}}
	snapshots := map[string]*DailySnapshot{SOURCE_TSA: tsa}
	published := Market{Ticker: "KXTSAD-26NOV21-T2800000", Event: "KXTSAD-26NOV21", Type: MARKET_BINARY,
		Strike: STRIKE_GREATER_OR_EQUAL, Floor: 2800000, YesAsk: 0.90, YesBid: 0.88, NoAsk: 0.12, NoBid: 0.10}
	known := dailyStrategy(t, true, snapshots)
	op, ok := known.Evaluate(published, now)
	if !ok || op.Side != SIDE_YES || op.Prob != 1 {
		t.Fatalf("published market got %+v %v, want YES at 100%%", op, ok)
	}
	below := published
	below.Floor = 3000000
	op, ok = known.Evaluate(below, now)
	if !ok || op.Side != SIDE_NO || op.Prob != 0 {
		t.Fatalf("published-below market got %+v %v, want NO with YES at 0%%", op, ok)
	}
	// A market about a day not yet published is no business of the known strategy.
	unpublished := Market{Ticker: "KXTSAD-26NOV22-T2800000", Event: "KXTSAD-26NOV22", Type: MARKET_BINARY,
		Strike: STRIKE_GREATER_OR_EQUAL, Floor: 2800000, YesAsk: 0.50, YesBid: 0.48, NoAsk: 0.52, NoBid: 0.50}
	if _, ok := known.Evaluate(unpublished, now); ok {
		t.Fatal("the known strategy took an unpublished day")
	}
	forecast := dailyStrategy(t, false, snapshots)
	op, ok = forecast.Evaluate(unpublished, now)
	if !ok || op.Prob <= 0 || op.Prob >= 1 {
		t.Fatalf("next-day market got %+v %v, want a chance strictly inside 0 and 1", op, ok)
	}
}

func TestDailyParamsLimitSeries(t *testing.T) {
	raw, _ := json.Marshal(DailyParams{Only: []string{"kxtsad"}, SigmaFloorFrac: DEFAULT_DAILY_SIGMA_FLOOR_FRAC, Lookback: DEFAULT_DAILY_LOOKBACK, MaxGapDays: DEFAULT_DAILY_MAX_GAP_DAYS})
	strategy, err := newDailyCounts(raw)
	if err != nil {
		t.Fatal(err)
	}
	scope := strategy.Scope().Series
	if len(scope) != 1 || scope[0] != SERIES_TSA_DAILY {
		t.Fatalf("scope %v, want only %s", scope, SERIES_TSA_DAILY)
	}
	if _, err := newDailyCounts(json.RawMessage(`{"lookback": 0}`)); err == nil {
		t.Fatal("a lookback of 0 built anyway")
	}
}

// TestLiveDailyCounts reads the real AAA and TSA pages and the real open Kalshi
// markets once: M24D_LIVE=1 go test ./predict -run TestLiveDailyCounts -v
func TestLiveDailyCounts(t *testing.T) {
	if os.Getenv("M24D_LIVE") == "" {
		t.Skip("set M24D_LIVE=1 to read Kalshi, AAA and TSA")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	env := &Env{HTTP: client}
	strategy, err := newDailyCounts(dailyParams(false))
	if err != nil {
		t.Fatal(err)
	}
	d := strategy.(*dailyCounts)
	kalshi := &Kalshi{HTTP: client}
	ctx, cancel := context.WithTimeout(context.Background(), PASS_TIMEOUT)
	defer cancel()
	markets := []Market{}
	for _, series := range d.Scope().Series {
		found, err := kalshi.InSeries(ctx, series, time.Time{})
		if err != nil {
			t.Fatalf("%s: %v", series, err)
		}
		t.Logf("%s: %d open markets", series, len(found))
		markets = append(markets, found...)
		if err := pause(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := strategy.Prepare(ctx, env, markets, time.Now()); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for source, snap := range d.data.snapshots {
		latest, ok := snap.Latest(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
		t.Logf("source %s: %d published points, latest %v on %s", source, len(snap.Points), latest.Value, latest.Date.Format(DAY_LAYOUT))
		if !ok || len(snap.Points) == 0 {
			t.Errorf("source %s published nothing", source)
		}
	}
	opinions := 0
	for _, m := range markets {
		op, ok := strategy.Evaluate(m, time.Now())
		if !ok {
			continue
		}
		opinions++
		if opinions <= 10 {
			t.Logf("%s: %s %.0f%% YES (%v)", m.Ticker, op.Side, op.Prob*PERCENT, op.Reasons)
		}
	}
	t.Logf("%d markets, %d opinions", len(markets), opinions)
	if len(markets) == 0 || opinions == 0 {
		t.Fatal("the live pass found markets but formed no opinions")
	}
}
