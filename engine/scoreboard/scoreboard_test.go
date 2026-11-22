package scoreboard

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	FIXTURE_COUNT = 100
	FIXTURE_SPAN  = 60 * DAY
	TOLERANCE     = 1e-12
)

var FIXTURE_NOW = time.Date(2026, time.November, 21, 12, 0, 0, 0, time.UTC)

func fixture(n int, span, age time.Duration, rate func(int) float64) []Result {
	rows := make([]Result, n)
	for i := range rows {
		at := FIXTURE_NOW.Add(-age - span + span*time.Duration(i)/time.Duration(max(1, n-1)))
		rows[i] = Result{At: at, Return: rate(i)}
	}
	return rows
}

func TestVerdicts(t *testing.T) {
	gain := func(_ int) float64 { return 0.2 }
	loss := func(_ int) float64 { return -0.2 }
	uncertain := func(i int) float64 {
		if i%2 == 0 {
			return 1.1
		}
		return -1
	}
	decay := func(i int) float64 {
		if i < FIXTURE_COUNT/2 {
			return 0.3
		}
		return 0.1
	}
	for _, tc := range []struct {
		name                  string
		n                     int
		span, age             time.Duration
		rate                  func(int) float64
		blocker, word, reason string
	}{
		{"empty", 0, 0, 0, gain, "", NOT_ENOUGH, "100 more settled results"},
		{"insufficient count", 99, FIXTURE_SPAN, 0, gain, "", NOT_ENOUGH, "1 more settled result"},
		{"insufficient span", FIXTURE_COUNT, 29 * DAY, 0, gain, "", NOT_ENOUGH, "1 more day"},
		{"passes exact minimum", FIXTURE_COUNT, 30 * DAY, 0, gain, "", PASSES, REASON_PASSES},
		{"loses", FIXTURE_COUNT, FIXTURE_SPAN, 0, loss, "", LOSING, REASON_LOSING},
		{"zero", FIXTURE_COUNT, FIXTURE_SPAN, 0, func(_ int) float64 { return 0 }, "", LOSING, REASON_LOSING},
		{"uncertainty", FIXTURE_COUNT, FIXTURE_SPAN, 0, uncertain, "", PROMISING, REASON_UNCERTAIN},
		{"stale", FIXTURE_COUNT, FIXTURE_SPAN, 31 * DAY, gain, "", PROMISING, REASON_NO_RECENT},
		{"decay", FIXTURE_COUNT, FIXTURE_SPAN, 0, decay, "", PROMISING, REASON_DECAY},
		{"benchmark", FIXTURE_COUNT, FIXTURE_SPAN, 0, gain, "benchmark only", PROMISING, "benchmark only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := evaluate(Strategy{Results: fixture(tc.n, tc.span, tc.age, tc.rate), Blocker: tc.blocker}, FIXTURE_NOW)
			if row.Verdict != tc.word || !strings.Contains(row.Reason, tc.reason) {
				t.Fatalf("got %q (%s), want %q (%s)", row.Verdict, row.Reason, tc.word, tc.reason)
			}
		})
	}
}

func TestPerResultInterval(t *testing.T) {
	at := FIXTURE_NOW.Add(-RECENT_WINDOW)
	row := evaluate(Strategy{Unit: RETURN_R, Results: []Result{
		{FIXTURE_NOW, 3}, {at, 1}, {at, math.NaN()}, {FIXTURE_NOW.Add(DAY), 10}, {time.Time{}, 50},
	}}, FIXTURE_NOW)
	if row.Settled != 2 || row.RecentSettled != 2 || *row.Mean != 2 || *row.WinRate != 1 || row.Unit != RETURN_R ||
		math.Abs(*row.Lower95-0.04) > TOLERANCE || math.Abs(*row.Upper95-3.96) > TOLERANCE ||
		!row.First.Equal(at) || !row.Latest.Equal(FIXTURE_NOW) {
		t.Fatalf("unexpected interval summary: %+v", row)
	}
}

func TestAggregation(t *testing.T) {
	reads := 0
	pnl := -8.5
	handler := Handler(
		func() []Strategy {
			reads++
			return []Strategy{{Desk: "prediction", ID: "shared"}, {Desk: "prediction", ID: "alternate", Risk: "high"}}
		}, nil,
		func() []Strategy {
			reads++
			return []Strategy{{Desk: "crypto", ID: "shared", Name: "Crypto", RealizedPnL: &pnl}}
		},
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scoreboard/all", nil))
	var rows []Row
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || len(rows) != 3 || rec.Header().Get(HEADER_CACHE) != CACHE_CONTROL {
		t.Fatalf("expected one read per desk and three rows, got %d reads, %+v", reads, rows)
	}
	if rows[0].Desk != "crypto" || rows[0].Name != "Crypto" || *rows[0].RealizedPnL != pnl ||
		rows[1].ID != "alternate" || rows[1].Risk != "high" || rows[2].ID != "shared" || rows[2].WinRate != nil ||
		rows[2].Verdict != NOT_ENOUGH {
		t.Fatalf("incorrect strategy aggregation: %+v", rows)
	}
}
