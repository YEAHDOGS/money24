package signals

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

var NOW = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func TestParse(t *testing.T) {
	cases := []struct {
		text   string
		kind   Kind
		symbol string
		right  string
		strike float64
		expiry string
		price  float64
	}{
		{"BTO $SPY 600C 11/21 @ 1.20", KIND_ENTRY, "SPY", "C", 600, "2026-11-21", 1.20},
		{"$TSLA 250 puts 11/28 at 3.10 🚀", KIND_ENTRY, "TSLA", "P", 250, "2026-11-28", 3.10},
		{"Bought $nvda 145.5c 12/19/26 for .85", KIND_ENTRY, "NVDA", "C", 145.5, "2026-12-19", 0.85},
		{"$AAPL 230c 1/16", KIND_ENTRY, "AAPL", "C", 230, "2027-01-16", 0}, // past date rolls to next year
		{"STC $SPY 600C 11/21 @ 1.90 +58%", KIND_EXIT, "SPY", "C", 600, "2026-11-21", 1.90},
		{"Trimmed $SPY here, runners on", KIND_TRIM, "SPY", "", 0, "", 0},
		{"sold half $META", KIND_TRIM, "META", "", 0, "", 0},
		{"All out of $AMD", KIND_EXIT, "AMD", "", 0, "", 0},
		{"Adding $SPY 600C 11/21 @ 0.90", KIND_ADD, "SPY", "C", 600, "2026-11-21", 0.90},
		{"Long $BTC @ 96500", KIND_ENTRY, "BTC", "", 0, "", 96500},
		{"$SPY looking strong into the close", KIND_NOISE, "SPY", "", 0, "", 0},
		{"Good morning traders ☕", KIND_NOISE, "", "", 0, "", 0},
	}
	for _, c := range cases {
		got := Parse(c.text, NOW)
		if got.Kind != c.kind || got.Symbol != c.symbol || got.Right != c.right || got.Strike != c.strike || got.Expiry != c.expiry || got.Price != c.price {
			t.Errorf("%q\n got  %s %s %s %v %s %v\n want %s %s %s %v %s %v", c.text,
				got.Kind, got.Symbol, got.Right, got.Strike, got.Expiry, got.Price,
				c.kind, c.symbol, c.right, c.strike, c.expiry, c.price)
		}
	}
}

// Real callouts, one per format we have seen. Every parser miss becomes a new line in the file.
func TestParseFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/callouts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Source   string    `json:"source"`
		PostID   string    `json:"post_id"`
		PostedAt time.Time `json:"posted_at"`
		Text     string    `json:"text"`
		Want     Signal    `json:"want"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		got, w := Parse(f.Text, f.PostedAt), f.Want
		if got.Kind != w.Kind || got.Symbol != w.Symbol || got.Right != w.Right || got.Strike != w.Strike ||
			got.Expiry != w.Expiry || got.Price != w.Price || got.Size != w.Size || got.Confidence != w.Confidence {
			t.Errorf("%s %s\n got  %s %s %s %v %s %v %q %v\n want %s %s %s %v %s %v %q %v", f.Source, f.PostID,
				got.Kind, got.Symbol, got.Right, got.Strike, got.Expiry, got.Price, got.Size, got.Confidence,
				w.Kind, w.Symbol, w.Right, w.Strike, w.Expiry, w.Price, w.Size, w.Confidence)
		}
	}
}

func TestParseExpiryFormats(t *testing.T) {
	cases := map[string]string{
		"$CSV 20 NOV 26 $30 CALL":          "2026-11-20", // 26 is the year
		"$CSV 20 NOV 26C @ 1.10":           "2026-11-20", // 26 is the strike
		"$SPY Nov 20 600C":                 "2026-11-20",
		"$SPY 600C November 20th, 2026":    "2026-11-20",
		"$SPY 600C 16JAN":                  "2027-01-16", // past date rolls to next year
		"$SPY 600C 20 NOV 19":              "",           // a year in the past is not an expiry
		"$SPY 600C looks decent 20 points": "",           // "dec" inside a word is not a month
	}
	for text, want := range cases {
		if got := Parse(text, NOW).Expiry; got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestParseConfidence(t *testing.T) {
	if c := Parse("BTO $SPY 600C 11/21 @ 1.20", NOW).Confidence; c != CONFIDENCE_FULL {
		t.Errorf("full callout: %v", c)
	}
	if c := Parse("$SPY 600C 11/21", NOW).Confidence; c != CONFIDENCE_NO_PRICE {
		t.Errorf("no price: %v", c)
	}
}

var SETTINGS = Settings{
	StopLossPct:    20,
	Ladder:         []Rung{{25, 30}, {50, 60}},
	MaxChasePct:    15,
	MinConfidence:  0.8,
	MaxUSDPerTrade: 1000,
}

func TestBuildBracket(t *testing.T) {
	sig := Parse("BTO $SPY 600C 11/21 @ 1.20", NOW)
	p, err := Build(sig, SETTINGS, 0, NOW)
	if err != nil {
		t.Fatal(err)
	}
	// $1000 / ($1.20 x 100) = 8 contracts: 2 at +30%, 4 at +60%, 2 runners, stop at -20%.
	if p.Qty != 8 || p.Stop != 0.96 || p.Runner != 2 || len(p.Targets) != 2 {
		t.Fatalf("plan %+v", p)
	}
	if p.Targets[0] != (Target{2, 1.56}) || p.Targets[1] != (Target{4, 1.92}) {
		t.Fatalf("targets %+v", p.Targets)
	}
	if p.CostUSD != 960 {
		t.Fatalf("cost %v", p.CostUSD)
	}
}

func TestBuildOneContractStillHasATakeProfit(t *testing.T) {
	s := SETTINGS
	s.MaxUSDPerTrade = 150
	p, err := Build(Parse("BTO $SPY 600C 11/21 @ 1.20", NOW), s, 0, NOW)
	if err != nil {
		t.Fatal(err)
	}
	if p.Qty != 1 || len(p.Targets) != 1 || p.Targets[0].Qty != 1 || p.Runner != 0 {
		t.Fatalf("plan %+v", p)
	}
}

func TestBuildRejects(t *testing.T) {
	entry := Parse("BTO $SPY 600C 11/21 @ 1.20", NOW)
	noExits := SETTINGS
	noExits.Ladder = nil
	noStop := SETTINGS
	noStop.StopLossPct = 0
	broke := SETTINGS
	broke.MaxUSDPerTrade = 50
	strict := SETTINGS
	strict.MinConfidence = 0.9

	cases := []struct {
		name  string
		sig   Signal
		s     Settings
		quote float64
		want  error
	}{
		{"exit is not an entry", Parse("STC $SPY 600C 11/21 @ 1.90", NOW), SETTINGS, 0, ErrNotEntry},
		{"no take-profit", entry, noExits, 0, ErrNoExits},
		{"no stop", entry, noStop, 0, ErrNoExits},
		{"chased past 15%", entry, SETTINGS, 1.40, ErrChased},
		{"budget too small", entry, broke, 0, ErrBudgetTooSmall},
		{"low confidence", Parse("$SPY 600C 11/21", NOW), strict, 1.20, ErrLowConfidence},
		{"chatter is not an entry", Parse("$SPY looking strong", NOW), SETTINGS, 0, ErrNotEntry},
		{"no price anywhere", Parse("$SPY 600C 11/21", NOW), SETTINGS, 0, ErrNoPrice},
	}
	for _, c := range cases {
		if _, err := Build(c.sig, c.s, c.quote, NOW); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestBuildUsesQuoteInsideChase(t *testing.T) {
	p, err := Build(Parse("BTO $SPY 600C 11/21 @ 1.20", NOW), SETTINGS, 1.30, NOW)
	if err != nil {
		t.Fatal(err)
	}
	if p.Entry != 1.30 || p.Stop != 1.04 {
		t.Fatalf("plan %+v", p)
	}
}

func TestTimeRules(t *testing.T) {
	s := SETTINGS
	s.MaxSignalAgeSec = 120
	s.MaxHoldMinutes = 240
	s.ExitMinutesBeforeClose = 30
	s.TimeStop = TimeStop{AfterMinutes: 45, MinGainPct: 0}
	sig := Parse("BTO $SPY 600C 11/21 @ 1.20", NOW)

	p, err := Build(sig, s, 0, NOW)
	if err != nil {
		t.Fatal(err)
	}
	if want := NOW.Add(240 * time.Minute); !p.ExitBy.Equal(want) {
		t.Errorf("max hold: exit by %v, want %v", p.ExitBy, want)
	}
	if want := NOW.Add(45 * time.Minute); !p.TimeStopAt.Equal(want) || p.TimeStopAbove != 1.20 {
		t.Errorf("time stop: %v above %v", p.TimeStopAt, p.TimeStopAbove)
	}

	// Close to expiry the expiry exit wins: 11/21 3:30pm New York.
	late := time.Date(2026, 11, 21, 13, 0, 0, 0, MARKET)
	sig.SeenAt = late
	p, err = Build(sig, s, 0, late)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 21, 15, 30, 0, 0, MARKET); !p.ExitBy.Equal(want) {
		t.Errorf("expiry exit: %v, want %v", p.ExitBy, want)
	}

	// After the expiry exit there is no time left to trade it.
	tooLate := time.Date(2026, 11, 21, 15, 45, 0, 0, MARKET)
	sig.SeenAt = tooLate
	if _, err := Build(sig, s, 0, tooLate); !errors.Is(err, ErrNoTime) {
		t.Errorf("after expiry exit: %v", err)
	}

	// A callout seen 3 minutes ago is stale at 120 seconds.
	sig.SeenAt = NOW.Add(-3 * time.Minute)
	if _, err := Build(sig, s, 0, NOW); !errors.Is(err, ErrStale) {
		t.Errorf("stale: %v", err)
	}
}

func TestDefaultCatalog(t *testing.T) {
	c, err := LoadCatalog("")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) < 20 || c.Defaults.StopLossPct != 20 || c.Defaults.MaxUSDPerTrade <= 0 {
		t.Fatalf("catalog: %d sources, defaults %+v", len(c.Sources), c.Defaults)
	}
	if got := c.SettingsFor("x:MATTSMATTED"); got.StopLossPct != 20 {
		t.Fatalf("settings %+v", got)
	}
}

func TestXStatusURL(t *testing.T) {
	tw, err := ParseXStatusURL("https://x.com/MATTSMATTED/status/1985841080431321088?s=46&t=abc")
	if err != nil {
		t.Fatal(err)
	}
	if tw.Handle != "MATTSMATTED" || tw.ID != "1985841080431321088" || !tw.At.Equal(time.UnixMilli(1762296383374)) {
		t.Fatalf("%+v", tw)
	}
	if got := FindXStatusURL("Check this out https://twitter.com/a_b/status/123456789 🔥"); got != "https://twitter.com/a_b/status/123456789" {
		t.Fatalf("find: %q", got)
	}
	if _, err := ParseXStatusURL("https://x.com/MATTSMATTED"); err == nil {
		t.Fatal("profile link is not a status")
	}
}

func TestTweetTextFromEmbed(t *testing.T) {
	embed := `<blockquote class="twitter-tweet"><p lang="en" dir="ltr">BTO <a href="https://twitter.com/search?q=%24SPY">$SPY</a> 600C 11/21 @ 1.20<br>Let&#39;s go &amp; size small</p>&mdash; Matt (@MATTSMATTED) <a href="https://twitter.com/x/status/1">November 9, 2026</a></blockquote>`
	if got, want := tweetTextFromEmbed(embed), "BTO $SPY 600C 11/21 @ 1.20\nLet's go & size small"; got != want {
		t.Fatalf("got %q", got)
	}
}

func TestWebhookResolvesSharedLink(t *testing.T) {
	out := make(chan Signal, 1)
	resolve := func(_ context.Context, u string) (Tweet, error) {
		tw, _ := ParseXStatusURL(u)
		tw.Text = "BTO $SPY 600C 11/21 @ 1.20"
		return tw, nil
	}
	h := Webhook("s3cret", func(*http.Request) bool { return false }, resolve, func(Signal) {}, out)

	req := httptest.NewRequest(http.MethodPost, "/signals", strings.NewReader(`{"text":"https://x.com/MATTSMATTED/status/1985841080431321088","manual":true}`))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no secret: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/signals", strings.NewReader(`{"text":"https://x.com/MATTSMATTED/status/1985841080431321088","manual":true}`))
	req.Header.Set("Authorization", "Bearer s3cret")
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	sig := <-out
	if sig.SourceID != "x:MATTSMATTED" || sig.Kind != KIND_ENTRY || !sig.Manual || sig.PostedAt.IsZero() || sig.PostID != "1985841080431321088" {
		t.Fatalf("%+v", sig)
	}
}

func TestTrustsLocalDesk(t *testing.T) {
	o := APIOptions{Origins: []string{"http://localhost:5173"}, Hosts: []string{"127.0.0.1:8025"}}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8025/signals", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	if !o.TrustsLocalDesk(req) {
		t.Fatal("local desk should be trusted")
	}
	req.Header.Set("Origin", "https://evil.example")
	if o.TrustsLocalDesk(req) {
		t.Fatal("foreign origin trusted")
	}
	o.Hosts = nil // bound to a public interface: always need the secret
	req.Header.Set("Origin", "http://localhost:5173")
	if o.TrustsLocalDesk(req) {
		t.Fatal("non-loopback runner trusted")
	}
}

func testRunner(t *testing.T, quote QuoteFunc) *Runner {
	t.Helper()
	j, err := OpenJournal(t.TempDir() + "/journal.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	return NewRunner(&Catalog{Defaults: SETTINGS}, j, quote)
}

func callout(text, postID string) Signal {
	sig := Parse(text, NOW)
	sig.SourceID, sig.PostID = "x:caller", postID
	return sig
}

func TestRunnerDropsDuplicatePosts(t *testing.T) {
	r := testRunner(t, nil)
	if _, fresh, _ := r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW); !fresh {
		t.Fatal("first post was dropped")
	}
	if _, fresh, _ := r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW); fresh {
		t.Fatal("the same post traded twice")
	}
	// A restart replays the journal and still knows the post.
	again := NewRunner(r.catalog, r.journal, nil)
	if _, fresh, _ := again.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW); fresh {
		t.Fatal("the same post traded again after a restart")
	}
}

func TestRunnerAddWithoutPositionIsAnEntry(t *testing.T) {
	r := testRunner(t, nil)
	add := "ADDING HEAVY\n\n$CSV\n20 NOV 26 $30 CALL\n$0.75"
	e, _, _ := r.Handle(callout(add, "1"), NOW)
	if e.Plan == nil || !strings.Contains(e.Note, "treated as entry") {
		t.Fatalf("add with no position: plan %v note %q rejected %q", e.Plan, e.Note, e.Rejected)
	}
	// Now the position is open, so a second add is a real add, and adds are off by default.
	e, _, _ = r.Handle(callout(add, "2"), NOW)
	if e.Plan != nil || e.Rejected != REJECT_NOT_FOLLOWED {
		t.Fatalf("add with a position: plan %v rejected %q", e.Plan, e.Rejected)
	}
	// The source exits; the next add is an entry again.
	r.Handle(callout("All out of $CSV", "3"), NOW)
	if e, _, _ = r.Handle(callout(add, "4"), NOW); e.Plan == nil {
		t.Fatalf("add after exit: rejected %q", e.Rejected)
	}
}

func TestLiquidityGate(t *testing.T) {
	s := SETTINGS
	s.MaxSpreadPct, s.MinOpenInterest, s.MinVolume = 10, 100, 50
	cases := []struct {
		name string
		q    Quote
		want error
	}{
		{"tight and liquid", Quote{1.18, 1.22, 5000, 900}, nil},
		{"wide spread", Quote{0.60, 0.90, 5000, 900}, ErrWideSpread},
		{"thin open interest", Quote{1.18, 1.22, 12, 900}, ErrThinInterest},
		{"thin volume", Quote{1.18, 1.22, 5000, 3}, ErrThinVolume},
		{"no bid", Quote{0, 0.75, 5000, 900}, ErrNoMarket},
	}
	for _, c := range cases {
		if err := CheckLiquidity(c.q, s); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}

	// With a quote the runner fills at the ask and rejects what fails the gate.
	wide := func(Signal) (Quote, bool) { return Quote{0.60, 0.90, 5000, 900}, true }
	r := testRunner(t, wide)
	r.catalog.Defaults = s
	if e, _, _ := r.Handle(callout("BTO $SPY 600C 11/21 @ 0.75", "1"), NOW); e.Plan != nil || !strings.Contains(e.Rejected, "spread") {
		t.Fatalf("wide spread was traded: %+v", e)
	}
	tight := func(Signal) (Quote, bool) { return Quote{1.18, 1.22, 5000, 900}, true }
	r = testRunner(t, tight)
	r.catalog.Defaults = s
	if e, _, _ := r.Handle(callout("BTO $SPY 600C 11/21 @ 1.20", "1"), NOW); e.Plan == nil || e.Plan.Entry != 1.22 {
		t.Fatalf("liquid contract: %+v", e)
	}
}

func TestRecorderKeepsEveryPostAndReplays(t *testing.T) {
	path := t.TempDir() + "/posts.jsonl"
	rec, err := OpenRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	posts := []string{"BTO $SPY 600C 11/21 @ 1.20", "Good morning traders", "BTO $SPY 600C 11/21 @ 1.25"} // third is an edit
	for i, text := range posts {
		sig := callout(text, []string{"1", "2", "1"}[i])
		if err := rec.Record(sig); err != nil {
			t.Fatal(err)
		}
	}
	rec.Close()

	kinds, prices := []Kind{}, []float64{}
	err = Replay(path, func(p RawPost, sig Signal) {
		kinds, prices = append(kinds, sig.Kind), append(prices, sig.Price)
		if p.SourceID != "x:caller" || sig.SourceID != p.SourceID {
			t.Errorf("source lost: %+v", p)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 3 || kinds[0] != KIND_ENTRY || kinds[1] != KIND_NOISE || prices[2] != 1.25 {
		t.Fatalf("replay: %v %v", kinds, prices)
	}
}

func TestScoreboardAndGoLiveGate(t *testing.T) {
	gate := GoLive{MinTrades: 3, MinTradingDays: 2, MaxDrawdownPct: 20, PaperCapitalUSD: 1000}
	day := func(d int) time.Time { return NOW.AddDate(0, 0, d) }
	closed := func(source string, at time.Time, pnl float64) Entry {
		return Entry{At: at, Action: ACTION_CLOSE, Signal: Signal{SourceID: source, Kind: KIND_ENTRY}, Result: &Result{PnLUSD: pnl}}
	}
	entries := []Entry{
		{Signal: Signal{SourceID: "x:good", Kind: KIND_ENTRY}, Plan: &Plan{}},
		{Signal: Signal{SourceID: "x:good", Kind: KIND_ENTRY}, Rejected: "chased"},
		{Signal: Signal{SourceID: "x:good", Kind: KIND_NOISE}},
		closed("x:good", day(0), 130), closed("x:good", day(0), -50), closed("x:good", day(1), 130),
		closed("x:bad", day(0), 100), closed("x:bad", day(1), -300), closed("x:bad", day(2), 50), // 1100 -> 800: 27% drawdown
		closed("x:lucky", day(0), 500), // one big win is not a record
	}
	byID := map[string]Score{}
	scores := Scoreboard(entries, gate)
	for _, s := range scores {
		byID[s.SourceID] = s
	}
	if scores[0].SourceID != ALL_SOURCES {
		t.Fatalf("first row is %s, want the all row", scores[0].SourceID)
	}
	good := byID["x:good"]
	if !good.GoLive || good.Posts != 2 || good.Planned != 1 || good.Rejected != 1 || good.Trades != 3 || good.Wins != 2 || good.PnLUSD != 210 {
		t.Fatalf("good: %+v", good)
	}
	bad := byID["x:bad"]
	if bad.GoLive || bad.MaxDrawdownPct < 27 || bad.MaxDrawdownPct > 28 || len(bad.Missing) != 2 {
		t.Fatalf("bad: %+v", bad)
	}
	if lucky := byID["x:lucky"]; lucky.GoLive || lucky.WinRatePct != 100 || len(lucky.Missing) != 2 {
		t.Fatalf("lucky: a 1-for-1 source must not pass: %+v", lucky)
	}
}
