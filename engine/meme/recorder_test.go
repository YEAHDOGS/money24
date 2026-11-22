package meme

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	FIXTURE_LAUNCH    = "launch.json"
	FIXTURE_MIGRATION = "migration.json"
	FIXTURE_MARKETS   = "markets.json"
	FIXTURE_SAFETY    = "safety.json"
	FIXTURE_MINT      = "So11111111111111111111111111111111111111112"
	FIXTURE_OTHER     = "11111111111111111111111111111111"
	FIXTURE_MARKET    = "https://test.invalid/tokens/%s"
	FIXTURE_REPORT    = "https://test.invalid/report/%s"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func pumpFixture(t *testing.T, name string, now time.Time) Record {
	t.Helper()
	row, err := decodePump(fixture(t, name), now)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestFreeSources(t *testing.T) {
	now := time.Now().UTC()
	launch := pumpFixture(t, FIXTURE_LAUNCH, now)
	if launch.Kind != KIND_LAUNCH || launch.Mint != FIXTURE_MINT || launch.At != now ||
		launch.Launch.Name != "Fixture Coin" || launch.Launch.Symbol != "FIX" ||
		launch.Launch.Creator != FIXTURE_OTHER || *launch.Launch.InitialBuy != 0 ||
		*launch.Launch.PriceSOL != 30.0/1073000000 {
		t.Fatalf("bad launch: %+v %+v", launch, launch.Launch)
	}
	migration := pumpFixture(t, FIXTURE_MIGRATION, now)
	if migration.Kind != KIND_GRADUATION || migration.Graduation.Venue != "pump-amm" ||
		migration.Graduation.Pool != "" || migration.Signature == "" {
		t.Fatalf("bad migration: %+v %+v", migration, migration.Graduation)
	}
	client := &http.Client{Transport: fixtureTransport(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch req.URL.Path {
		case "/tokens/" + FIXTURE_MINT + "," + FIXTURE_OTHER:
			body = fixture(t, FIXTURE_MARKETS)
		case "/report/" + FIXTURE_MINT:
			body = fixture(t, FIXTURE_SAFETY)
		default:
			t.Fatalf("unexpected request %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
	markets, err := fetchMarket(t.Context(), client, FIXTURE_MARKET, []string{FIXTURE_MINT, FIXTURE_OTHER})
	if err != nil {
		t.Fatal(err)
	}
	pair, ok := markets[FIXTURE_MINT]
	if !ok || pair.Venue != "pumpswap" || *pair.PriceUSD != 0.00003 || *pair.MarketCap != 28000 ||
		*pair.Liquidity.USD != 9000 || pair.Volume["h24"] != 9000.9 || pair.Txns[WINDOW_FIVE_MIN].Sells != 10 {
		t.Fatalf("bad fixture market: %+v", pair)
	}
	if _, ok := markets[FIXTURE_OTHER]; ok {
		t.Fatal("token with no matching base pool received a quote")
	}
	report, err := fetchSafety(t.Context(), client, FIXTURE_REPORT, FIXTURE_MINT)
	if err != nil {
		t.Fatal(err)
	}
	if *report.TotalHolders != 150 || *report.TopHolders[0].Percent != 15.44 ||
		string(report.MintAuthority) != "null" || *report.Markets[0].LP.LockedPct != 100 {
		t.Fatalf("bad safety report: %+v", report)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, retry := range []string{"150", now.Add(150 * time.Second).Format(http.TimeFormat)} {
		err := &httpFailure{code: http.StatusTooManyRequests, retry: retry}
		if got := retryDelay(err, now); got != 150*time.Second {
			t.Errorf("retry delay for %q: %s", retry, got)
		}
	}
	if got := retryDelay(errors.New("transport failure"), now); got != HTTP_ERROR_DELAY {
		t.Fatalf("unclassified retry delay: %s", got)
	}
}

func TestCadenceAndGraduation(t *testing.T) {
	start := time.Now().UTC()
	tests := []struct{ age, next time.Duration }{
		{FIRST_SAMPLE, FIRST_SAMPLE + EARLY_INTERVAL},
		{EARLY_WINDOW - time.Second, EARLY_WINDOW},
		{EARLY_WINDOW, EARLY_WINDOW + MIDDLE_INTERVAL},
		{MIDDLE_WINDOW - time.Second, MIDDLE_WINDOW},
		{MIDDLE_WINDOW, MIDDLE_WINDOW + LATE_INTERVAL},
	}
	for _, tc := range tests {
		if got := nextSample(start, start.Add(tc.age)); got.Sub(start) != tc.next {
			t.Errorf("next sample at age %s: got %s, want %s", tc.age, got.Sub(start), tc.next)
		}
	}
	state := tracking{tokens: make(map[string]*trackedToken), recent: make(map[eventKey]struct{})}
	launch := pumpFixture(t, FIXTURE_LAUNCH, start)
	state.planEvent(&launch)
	state.applyEvent(launch)
	graduation := pumpFixture(t, FIXTURE_MIGRATION, start.Add(time.Minute))
	state.planEvent(&graduation)
	state.applyEvent(graduation)
	state.applySample(Record{Mint: launch.Mint}, start.Add(FIRST_SAMPLE))
	if due := state.marketDue(graduation.At); len(due) != 1 || due[0].mint != launch.Mint || due[0].start != start {
		t.Fatalf("the in-flight curve lookup displaced the first graduation sample: %+v", due)
	}
	state.applySample(Record{Mint: launch.Mint}, graduation.At)
	if due := state.marketDue(graduation.At.Add(MIGRATION_RETRY)); len(due) != 1 {
		t.Fatal("the market lookup did not retry while graduation liquidity indexed")
	}
	state.prune(start.Add(RECORD_WINDOW))
	if len(state.tokens) != 0 {
		t.Fatal("token survived the end of its tracking window")
	}
}

func TestCapacityAndRecovery(t *testing.T) {
	start := time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)
	r := &Recorder{Dir: t.TempDir()}
	run := recording{
		recorder: r, writer: recordWriter{dir: r.Dir},
		tracking: tracking{tokens: make(map[string]*trackedToken), recent: make(map[eventKey]struct{})},
	}
	launch := pumpFixture(t, FIXTURE_LAUNCH, start)
	run.stream(streamUpdate{row: launch})
	run.stream(streamUpdate{row: launch}) // duplicate
	graduation := pumpFixture(t, FIXTURE_MIGRATION, start.Add(FIRST_SAMPLE))
	run.stream(streamUpdate{row: graduation})
	if run.status.LastError != "" {
		t.Fatal(run.status.LastError)
	}
	run.save(Record{At: graduation.At, RequestedAt: graduation.At, Kind: KIND_SAFETY, Mint: launch.Mint, Safety: &Safety{}})
	if _, err := run.writer.file.WriteString(`{"partial":`); err != nil {
		t.Fatal(err)
	}
	if err := run.writer.file.Close(); err != nil {
		t.Fatal(err)
	}
	restored := recording{
		recorder: r, writer: recordWriter{dir: r.Dir},
		tracking: tracking{tokens: make(map[string]*trackedToken), recent: make(map[eventKey]struct{})},
	}
	restored.status.setDay(start)
	if err := restored.restore(context.Background(), start.Add(time.Minute)); err == nil {
		t.Fatal("expected a diagnostic for the partial line")
	}
	token := restored.tracking.tokens[launch.Mint]
	if token == nil || !token.safetyDone || token.graduated != graduation.At ||
		restored.status.SeenToday != 1 || restored.status.Graduated != 1 {
		t.Fatalf("bad recovery: %+v %+v", token, restored.status)
	}
	for i := 0; i < MAX_TRACKED-2; i++ {
		mint := time.Unix(int64(i), 0).Format(time.RFC3339)
		restored.tracking.tokens[mint] = &trackedToken{mint: mint, graduated: start, lastActivity: start}
	}
	restored.tracking.tokens[FIXTURE_OTHER] = &trackedToken{mint: FIXTURE_OTHER, lastActivity: start}
	extra := Record{Mint: "extra", Kind: KIND_LAUNCH, At: start}
	restored.tracking.planEvent(&extra)
	if extra.Tracking != TRACK_CAPACITY {
		t.Fatalf("launch should be skipped at cap: %+v", extra)
	}
	extra.Kind = KIND_GRADUATION
	restored.tracking.planEvent(&extra)
	restored.tracking.applyEvent(extra)
	if extra.Tracking != TRACK_ADMITTED || extra.Evicted != FIXTURE_OTHER ||
		len(restored.tracking.tokens) != MAX_TRACKED || restored.tracking.tokens[FIXTURE_OTHER] != nil {
		t.Fatalf("graduation should displace the ungraduated token: %+v", extra)
	}
}
