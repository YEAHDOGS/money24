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
	h := Webhook("s3cret", func(*http.Request) bool { return false }, resolve, out)

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
