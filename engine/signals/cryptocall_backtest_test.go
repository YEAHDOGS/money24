package signals

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
)

const (
	CRYPTO_CANDLE_FIXTURES = "testdata/crypto_candles.json"
	CRYPTO_FIXTURE_CALL    = "$BTC long | Entry 100 | TP 110 / 120 | SL 90"
	CRYPTO_FIXTURE_ROUTE   = "/api/trust/calls?id=fixture"
	CRYPTO_R_TOLERANCE     = 1e-9
)

type cryptoCandleFixture struct {
	Name          string
	Text          string
	Bars          []*[4]float64
	Flat          *[4]float64
	Status        string
	Entry         float64
	Targets       int
	R             *float64
	Hourly        bool
	Requests      int
	NowMinutes    int `json:"now_minutes"`
	Unlisted      bool
	NotFollowable bool `json:"not_followable"`
}

func TestCryptoBacktestCandles(t *testing.T) {
	start := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, fixture := range readCryptoFixtures[[]cryptoCandleFixture](t, CRYPTO_CANDLE_FIXTURES) {
		t.Run(fixture.Name, func(t *testing.T) {
			raw := fixture.Text
			if raw == "" {
				raw = CRYPTO_FIXTURE_CALL
			}
			call, ok := ParseCryptoCall(RawPost{Text: raw, SeenAt: start})
			if !ok {
				t.Fatalf("invalid fixture %q", raw)
			}
			now := start.Add(CRYPTO_HOLD_WINDOW + CRYPTO_ENTRY_WINDOW)
			if fixture.NowMinutes != 0 {
				now = start.Add(time.Duration(fixture.NowMinutes) * time.Minute)
			}
			requests, next := 0, start
			backend := CryptoBacktester{feePct: CRYPTO_FIXTURE_FEE}
			backend.history = func(_ context.Context, base string, step time.Duration, from, to time.Time) ([]coinbase.Candle, error) {
				requests++
				if base != call.Base || from != next || to.Sub(from) > coinbase.CANDLES_MAX*step || to.After(now) {
					t.Fatalf("invalid request %s %s %s to %s (next %s)", base, step, from, to, next)
				}
				next = to
				if fixture.Unlisted {
					return nil, coinbase.ErrNotFound
				}
				// These out-of-range bars exercise Coinbase's boundary behavior.
				bars := []coinbase.Candle{{T: from.Add(-step).Unix()}}
				for at := from; at.Before(to); at = at.Add(step) {
					i := int(at.Sub(start) / CRYPTO_FINE_BAR)
					ohlc := fixture.Flat
					if i < len(fixture.Bars) {
						ohlc = fixture.Bars[i]
					}
					if ohlc == nil {
						continue
					}
					bars = append(bars, coinbase.Candle{T: at.Unix(), O: ohlc[0], H: ohlc[1], L: ohlc[2], C: ohlc[3]})
				}
				return append(bars, coinbase.Candle{T: to.Unix()}), nil
			}
			got := backend.backtest(t.Context(), call, now)
			if got.Status != fixture.Status || got.Entry != fixture.Entry || got.TargetsHit != fixture.Targets ||
				got.Hourly != fixture.Hourly || got.Followable == fixture.NotFollowable || requests != fixture.Requests {
				t.Fatalf("got %+v (%d requests), want %+v", got, requests, fixture)
			}
			if (got.R == nil) != (fixture.R == nil) || got.R != nil && math.Abs(*got.R-*fixture.R) > CRYPTO_R_TOLERANCE {
				t.Errorf("unexpected R for %+v. Got %v, expected %v", got, got.R, fixture.R)
			}
			if got.Listed == nil || *got.Listed == fixture.Unlisted {
				t.Errorf("unexpected listing for %+v", got)
			}
		})
	}
}

func TestCryptoCallsRouteUsesFirstRecordedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "posts.jsonl")
	rec, err := OpenRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	seen := time.Now().UTC().Add(-CRYPTO_ENTRY_WINDOW).Truncate(CRYPTO_FINE_BAR).Add(7 * time.Minute)
	original := Signal{SourceID: "fixture", PostID: "valid", Raw: CRYPTO_FIXTURE_CALL, SeenAt: seen, PostedAt: seen.Add(-time.Hour)}
	edited := original
	edited.Raw, edited.SeenAt = "$BTC long | Entry 200 | TP 230 | SL 190", seen.Add(time.Hour)
	recap := original
	recap.PostID, recap.Raw = "recap", "TP2 hit! +80%"
	recapEdit := original
	recapEdit.PostID, recapEdit.SeenAt = recap.PostID, edited.SeenAt
	otherSource := original
	otherSource.SourceID = "different-source"
	for _, sig := range []Signal{original, edited, recap, recapEdit, otherSource} {
		if err := rec.Record(sig); err != nil {
			t.Fatal(err)
		}
	}

	b := NewCryptoBacktester(path, CRYPTO_FIXTURE_FEE)
	requests := 0
	b.history = func(_ context.Context, base string, step time.Duration, from, to time.Time) ([]coinbase.Candle, error) {
		requests++
		firstFull := seen.Truncate(step).Add(step)
		if base != "BTC" || from != firstFull {
			t.Fatalf("requested %s from %s, want BTC from %s", base, from, firstFull)
		}
		return []coinbase.Candle{
			{T: firstFull.Add(-step).Unix(), O: 100, H: 140, L: 80, C: 100}, // predates first recording
			{T: from.Unix(), O: 106, H: 130, L: 105, C: 125},                // target, but never entered
			{T: from.Add(step).Unix(), O: 100, H: 130, L: 89, C: 115},       // fill and stop
		}, nil
	}
	mux := http.NewServeMux()
	for pattern, h := range b.Routes(func(r *http.Request) bool { return r.Header.Get("Authorization") == "test" }) {
		mux.Handle(pattern, h)
	}
	for _, x := range []struct {
		path, auth string
		code       int
	}{
		{CRYPTO_FIXTURE_ROUTE, "", http.StatusUnauthorized},
		{"/api/trust/calls", "test", http.StatusBadRequest},
		{CRYPTO_FIXTURE_ROUTE, "test", http.StatusOK},
		{CRYPTO_FIXTURE_ROUTE, "test", http.StatusOK}, // a finished call uses the cached outcome
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, x.path, nil)
		r.Header.Set("Authorization", x.auth)
		mux.ServeHTTP(w, r)
		if w.Code != x.code {
			t.Fatalf("HTTP %d, expected %d: %s", w.Code, x.code, w.Body)
		}
		if x.code != http.StatusOK {
			continue
		}
		var record CryptoCallRecord
		if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if len(record.Outcomes) != 1 || record.All.Calls != 1 || record.Followable.Calls != 1 || !record.All.TooFewToJudge {
			t.Fatalf("unexpected record: %+v", record)
		}
		o := record.Outcomes[0]
		if o.Status != CRYPTO_STOPPED || o.Entry != 100 || !o.Call.SeenAt.Equal(seen) || !o.Call.PostedAt.Equal(original.PostedAt) {
			t.Fatalf("unexpected original outcome: %+v", o)
		}
	}
	if requests != 1 {
		t.Fatalf("expected one candle request, got %d", requests)
	}
}

func TestCryptoCallStats(t *testing.T) {
	start := time.Unix(0, 0)
	win, loss, zero := 2.0, -1.0, 0.0
	outcomes := []CryptoCallOutcome{
		{Entry: 100, R: &loss, ResolvedAt: start.Add(3 * time.Hour), Followable: true},
		{Entry: 100, R: &win, ResolvedAt: start.Add(time.Hour), Followable: true},
		{Entry: 100, R: &loss, ResolvedAt: start.Add(2 * time.Hour)},
		{Status: CRYPTO_NEVER_FILLED, R: &zero, Followable: true},
		{Status: CRYPTO_PENDING, Entry: 100, Followable: true},
		{Status: CRYPTO_UNAVAILABLE},
	}
	all := cryptoCallStats(outcomes, false)
	if all.Calls != 6 || all.Resolved != 4 || all.Filled != 4 || all.ResolvedFills != 3 || all.Wins != 1 ||
		all.WorstLossRun != 2 || all.MaxDrawdown != 200 || math.Abs(all.MaxDrawdownPct-PERCENT*200.0/10200) > CRYPTO_R_TOLERANCE ||
		all.AverageR != 0 || all.ExpectancyR != 0 || all.Pending != 1 || all.Unavailable != 1 {
		t.Fatalf("unexpected all-calls stats: %+v", all)
	}
	followable := cryptoCallStats(outcomes, true)
	if followable.Calls != 4 || followable.WorstLossRun != 1 || followable.ExpectancyR != 1.0/3 ||
		followable.AverageR != 0.5 || followable.WinRatePct != 50 {
		t.Fatalf("unexpected followable stats: %+v", followable)
	}
	atThreshold := make([]CryptoCallOutcome, CRYPTO_MIN_SAMPLE)
	for i := range atThreshold {
		atThreshold[i] = CryptoCallOutcome{Status: CRYPTO_NEVER_FILLED, R: &zero}
	}
	if cryptoCallStats(atThreshold, false).TooFewToJudge ||
		!cryptoCallStats(atThreshold[:CRYPTO_MIN_SAMPLE-1], false).TooFewToJudge {
		t.Fatal("incorrect sample threshold")
	}
}

func TestCryptoCandleErrorRemainsUnknown(t *testing.T) {
	now := time.Now()
	b := CryptoBacktester{history: func(_ context.Context, _ string, _ time.Duration, _, _ time.Time) ([]coinbase.Candle, error) {
		return nil, errors.New("temporary failure")
	}}
	call, _ := ParseCryptoCall(RawPost{Text: CRYPTO_FIXTURE_CALL, SeenAt: now.Add(-CRYPTO_ENTRY_WINDOW)})
	o := b.backtest(t.Context(), call, now)
	if o.Listed != nil || o.R != nil || o.Followable || o.Status != CRYPTO_UNAVAILABLE {
		t.Fatalf("temporary failure treated as a result: %+v", o)
	}
}

func TestCryptoBacktestLive(t *testing.T) {
	if os.Getenv(CRYPTO_LIVE_ENV) != "1" {
		t.Skip("set M24D_LIVE=1 to query Coinbase's public API")
	}
	b := NewCryptoBacktester("", CRYPTO_FIXTURE_FEE)
	for _, fixture := range readCryptoFixtures[[]cryptoCallFixture](t, CRYPTO_CALL_FIXTURES) {
		if fixture.LiveAt.IsZero() {
			continue
		}
		t.Run(fixture.Want.Base+"/"+fixture.LiveAt.Format(time.DateTime), func(t *testing.T) {
			call, ok := ParseCryptoCall(RawPost{Text: fixture.Text, SeenAt: fixture.LiveAt, PostedAt: fixture.LiveAt})
			if !ok {
				t.Fatal("live call fixture was rejected")
			}
			got := b.backtest(t.Context(), call, time.Now())
			if got.R == nil || got.Listed == nil || !*got.Listed {
				t.Fatalf("could not score call with Coinbase candles: %+v", got)
			}
			t.Logf("%s-%s %s: %s, entry %.4f, %d targets, %.4fR, followable=%t",
				call.Base, call.Quote, call.Direction, got.Status, got.Entry, got.TargetsHit, *got.R, got.Followable)
		})
	}
}
