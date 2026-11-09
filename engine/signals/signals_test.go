package signals

import (
	"errors"
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
	p, err := Build(sig, SETTINGS, 0)
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
	p, err := Build(Parse("BTO $SPY 600C 11/21 @ 1.20", NOW), s, 0)
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
		if _, err := Build(c.sig, c.s, c.quote); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestBuildUsesQuoteInsideChase(t *testing.T) {
	p, err := Build(Parse("BTO $SPY 600C 11/21 @ 1.20", NOW), SETTINGS, 1.30)
	if err != nil {
		t.Fatal(err)
	}
	if p.Entry != 1.30 || p.Stop != 1.04 {
		t.Fatalf("plan %+v", p)
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
