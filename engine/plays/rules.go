package plays

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Rule ids. The first four are the DOGS rules; the rest are suggested defaults. Every one can be
// turned off, made a warning instead of a block, or retuned.
const (
	RULE_EXITS      = "exits_required" // DOGS: no exits, stop and failsafes, no play
	RULE_CROWD      = "crowd_evidence" // DOGS: other money must be in the trade
	RULE_NEWS       = "news_trust"     // DOGS: thin or untested news is ignored
	RULE_VOLUME     = "volume_first"   // DOGS: follow the whales, then watch the news
	RULE_LIQUIDITY  = "liquidity"      // a contract you can get out of
	RULE_LOTTO      = "lotto_guard"    // no penny lottery tickets far out of the money
	RULE_REWARD     = "reward_risk"    // the ladder must pay for the stop
	RULE_TIME       = "time_left"      // out before the last days of time decay
	RULE_EVENT      = "event_crush"    // earnings before expiry: the volatility drain
	RULE_OPEN       = "opening_wait"   // no entries in the opening rush
	RULE_CHASE      = "no_chase"       // don't buy the gap that already happened
	RULE_BANKROLL   = "bankroll_risk"  // one play can't sink the account
	RULE_ODDS       = "odds_floor"     // the first exit has to be plausible
	RULE_EXPECTANCY = "positive_edge"  // the weighted scenarios should not lose money
	RULE_THEME      = "theme_cap"      // AAL, DAL and UAL on one headline are one bet
	RULE_MAX_OPEN   = "max_open"       // a cap on armed plays
	RULE_FRESH      = "fresh_data"     // never arm on a stale quote
	RULE_LIMIT      = "limit_entry"    // pay the middle, not the ask
	RULE_COOLDOWN   = "cooldown"       // no revenge trade on a symbol that just stopped out
	RULE_STOPS      = "stop_ladder"    // the stop climbs as exits fill: a green trade never turns red

	P_MIN_ITEMS           = "min_items"
	P_MIN_SOURCES         = "min_sources"
	P_MIN_TRUST           = "min_trust_pct"
	P_MIN_REL_VOLUME      = "min_rel_volume"
	P_MIN_CONTRACT_VOLUME = "min_contract_volume"
	P_MAX_SPREAD          = "max_spread_pct"
	P_MIN_OPEN_INTEREST   = "min_open_interest"
	P_MIN_PREMIUM         = "min_premium"
	P_MAX_OTM             = "max_otm_pct"
	P_MIN_RATIO           = "min_ratio"
	P_MIN_DAYS            = "min_days_to_expiry"
	P_EXIT_DAYS           = "exit_days_before_expiry"
	P_AFTER_OPEN          = "minutes_after_open"
	P_BEFORE_CLOSE        = "minutes_before_close"
	P_MAX_GAP             = "max_gap_pct"
	P_MAX_PLANNED         = "max_planned_loss_pct"
	P_MAX_WORST           = "max_worst_loss_pct"
	P_MIN_ODDS            = "min_dogs_odds_pct"
	P_MIN_EXPECTED        = "min_expected_usd"
	P_MAX_PER_THEME       = "max_per_theme"
	P_MAX_ARMED           = "max_armed"
	P_MAX_AGE             = "max_age_minutes"
	P_MAX_OVER_MID        = "max_over_mid_pct"
	P_HOURS               = "hours"

	CHECK_PASS = "pass"
	CHECK_FAIL = "fail" // a blocking rule failed: cannot arm
	CHECK_WARN = "warn" // a warning rule failed: can arm
	CHECK_OFF  = "off"
	CHECK_INFO = "info"

	RULES_VERSION  = 1
	OPEN_HOUR      = 9
	OPEN_MINUTE    = 30
	CLOSE_HOUR     = 16
	MINUTES_PER_HR = 60
	HOURS_PER_DAY  = 24
)

// Rule is one check and its settings.
type Rule struct {
	ID       string             `json:"id"`
	Enabled  bool               `json:"enabled"`
	Blocking bool               `json:"blocking"` // a fail stops arming; otherwise it warns
	Params   map[string]float64 `json:"params"`
}

// Weights set how much each kind of evidence moves the odds.
type Weights struct {
	Volume       float64 `json:"volume"`
	News         float64 `json:"news"`
	OpenInterest float64 `json:"open_interest"`
	Technicals   float64 `json:"technicals"`
	Signals      float64 `json:"signals"`
}

// TemplateRung is one default ladder exit: sell PctOfPosition at GainPct over the entry.
type TemplateRung struct {
	PctOfPosition float64 `json:"pct_of_position"`
	GainPct       float64 `json:"gain_pct"`
}

// Template is the bracket a new play starts with, so a pasted link is a complete play at once.
type Template struct {
	BudgetUSD            float64        `json:"budget_usd"`
	StopPct              float64        `json:"stop_pct"`
	Ladder               []TemplateRung `json:"ladder"`
	RunnerGainPct        float64        `json:"runner_gain_pct"`
	ExitDaysBeforeExpiry int            `json:"exit_days_before_expiry"`
	ExitHour             int            `json:"exit_hour"` // New York time on the exit day
	HoldDays             int            `json:"hold_days"` // shares: exit after this many days
	SidewaysMinutes      int            `json:"sideways_minutes"`
	SidewaysMinGainPct   float64        `json:"sideways_min_gain_pct"`
	NoQuoteMinutes       int            `json:"no_quote_minutes"`
	GapPct               float64        `json:"gap_pct"`
	Calamity             string         `json:"calamity"`
}

// Rules is the whole rulebook: checks, weights, the news filter, the starting bracket and the
// desk switches. Saved as one document; reset brings back the DOGS defaults.
type Rules struct {
	Version                     int      `json:"version"`
	Name                        string   `json:"name"`
	BankrollUSD                 float64  `json:"bankroll_usd"`
	Rules                       []Rule   `json:"rules"`
	Weights                     Weights  `json:"weights"`
	NewsMinSources              int      `json:"news_min_sources"`
	NewsMinTrustPct             float64  `json:"news_min_trust_pct"`
	BreakingOverridesTechnicals bool     `json:"breaking_overrides_technicals"`
	Template                    Template `json:"template"`
	AutoPaper                   bool     `json:"auto_paper"`           // a pasted link is planned and paper traded at once
	PaperIgnoresBlocks          bool     `json:"paper_ignores_blocks"` // paper trades go through blocks, marked; live never would
	// Finder is how the engine opens plays on its own from the news. Nil in an older rulebook: the defaults apply.
	Finder *FinderRules `json:"finder,omitempty"`
}

// DOGSRules is the default rulebook.
func DOGSRules() Rules {
	rule := func(id string, blocking bool, params map[string]float64) Rule {
		if params == nil {
			params = map[string]float64{}
		}
		return Rule{ID: id, Enabled: true, Blocking: blocking, Params: params}
	}
	return Rules{
		Version:     RULES_VERSION,
		Name:        "DOGS",
		BankrollUSD: 500,
		Rules: []Rule{
			rule(RULE_EXITS, true, nil),
			rule(RULE_CROWD, true, map[string]float64{P_MIN_ITEMS: 1}),
			rule(RULE_NEWS, false, nil),
			rule(RULE_VOLUME, false, map[string]float64{P_MIN_REL_VOLUME: 0.8, P_MIN_CONTRACT_VOLUME: 100}),
			rule(RULE_LIQUIDITY, true, map[string]float64{P_MAX_SPREAD: 15, P_MIN_OPEN_INTEREST: 500}),
			rule(RULE_LOTTO, true, map[string]float64{P_MIN_PREMIUM: 0.10, P_MAX_OTM: 15}),
			rule(RULE_REWARD, true, map[string]float64{P_MIN_RATIO: 2}),
			rule(RULE_TIME, true, map[string]float64{P_MIN_DAYS: 3, P_EXIT_DAYS: 1}),
			rule(RULE_EVENT, false, nil),
			rule(RULE_OPEN, true, map[string]float64{P_AFTER_OPEN: 15, P_BEFORE_CLOSE: 10}),
			rule(RULE_CHASE, false, map[string]float64{P_MAX_GAP: 3}),
			rule(RULE_BANKROLL, false, map[string]float64{P_MAX_PLANNED: 10, P_MAX_WORST: 30}),
			rule(RULE_ODDS, false, map[string]float64{P_MIN_ODDS: 30}),
			rule(RULE_EXPECTANCY, false, map[string]float64{P_MIN_EXPECTED: 0}),
			rule(RULE_THEME, false, map[string]float64{P_MAX_PER_THEME: 2}),
			rule(RULE_MAX_OPEN, true, map[string]float64{P_MAX_ARMED: 5}),
			rule(RULE_FRESH, true, map[string]float64{P_MAX_AGE: 20}),
			rule(RULE_LIMIT, false, map[string]float64{P_MAX_OVER_MID: 5}),
			rule(RULE_COOLDOWN, false, map[string]float64{P_HOURS: 24}),
			rule(RULE_STOPS, false, nil),
			rule(RULE_SIZE, false, map[string]float64{P_MAX_PCT_VOLUME: 10, P_MAX_PCT_OI: 5}),
		},
		Weights:                     Weights{Volume: 40, News: 25, OpenInterest: 15, Technicals: 15, Signals: 5},
		NewsMinSources:              2,
		NewsMinTrustPct:             60,
		BreakingOverridesTechnicals: true,
		Template: Template{
			BudgetUSD: 150, StopPct: 50, RunnerGainPct: 700,
			Ladder:               []TemplateRung{{PctOfPosition: 34, GainPct: 100}, {PctOfPosition: 33, GainPct: 300}},
			ExitDaysBeforeExpiry: 1, ExitHour: 15, HoldDays: 5,
			SidewaysMinutes: 1440, SidewaysMinGainPct: 0, NoQuoteMinutes: 15, GapPct: 10,
			Calamity: "Halt, gap or broken feed: cancel every resting exit, flatten at the bid when a two-sided market returns, and take no new plays on the symbol for a day.",
		},
		AutoPaper:          true,
		PaperIgnoresBlocks: true,
		Finder:             DefaultFinder(),
	}
}

// merged fills anything missing from a saved rulebook with the defaults, so new rules appear
// after an update without wiping the user's settings.
func (r Rules) merged() Rules {
	d := DOGSRules()
	have := map[string]bool{}
	for _, x := range r.Rules {
		have[x.ID] = true
	}
	for _, x := range d.Rules {
		if !have[x.ID] {
			r.Rules = append(r.Rules, x)
		}
	}
	for i := range r.Rules {
		def := d.rule(r.Rules[i].ID)
		if r.Rules[i].Params == nil {
			r.Rules[i].Params = map[string]float64{}
		}
		for k, v := range def.Params {
			if _, ok := r.Rules[i].Params[k]; !ok {
				r.Rules[i].Params[k] = v
			}
		}
	}
	if r.Template.BudgetUSD <= 0 {
		r.Template = d.Template
	}
	if r.Version == 0 {
		r.Version = RULES_VERSION
	}
	if r.Finder == nil {
		r.Finder = d.Finder
	}
	return r
}

func (r Rules) rule(id string) Rule {
	for _, x := range r.Rules {
		if x.ID == id {
			return x
		}
	}
	return Rule{ID: id}
}

// param is a rule's setting, or 0 when the rule is off.
func (r Rules) param(id, key string) float64 {
	x := r.rule(id)
	if !x.Enabled {
		return 0
	}
	return x.Params[key]
}

// Check is one rule's verdict on one play.
type Check struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Blocking bool   `json:"blocking"`
	Detail   string `json:"detail"`
}

// Book is what the checks need from outside the play: the other plays and the clock.
type Book struct {
	Others []Play
	Now    time.Time
}

// evaluate runs every rule on a play that already has its payoff, levels and odds.
func evaluate(p Play, r Rules, rep Report, b Book) []Check {
	var m Market
	if p.Market != nil {
		m = *p.Market
	}
	option := p.Contract.Market == MARKET_OPTION
	mid := (m.Bid + m.Ask) / 2
	expiry, hasExpiry := expiryClose(p.Contract.Expiry)
	nyNow := b.Now.In(MARKET_TZ)
	out := make([]Check, 0, len(r.Rules))

	for _, rule := range r.Rules {
		c := Check{ID: rule.ID, Status: CHECK_PASS, Blocking: rule.Blocking}
		if !rule.Enabled {
			c.Status = CHECK_OFF
			out = append(out, c)
			continue
		}
		fail := func(format string, a ...any) {
			c.Status, c.Detail = CHECK_WARN, fmt.Sprintf(format, a...)
			if rule.Blocking {
				c.Status = CHECK_FAIL
			}
		}
		pass := func(format string, a ...any) { c.Detail = fmt.Sprintf(format, a...) }
		v := rule.Params

		switch rule.ID {
		case RULE_EXITS:
			missing := []string{}
			if p.Stop <= 0 || p.Stop >= p.Entry {
				missing = append(missing, "stop loss below the entry")
			}
			if len(p.Ladder) == 0 {
				missing = append(missing, "ladder exits")
			}
			if p.Runner.Qty > 0 && p.Runner.Target <= p.Entry {
				missing = append(missing, "runner target")
			}
			f := p.Failsafes
			if f.ExitBy.IsZero() {
				missing = append(missing, "time exit")
			}
			if f.SidewaysMinutes <= 0 {
				missing = append(missing, "sideways exit")
			}
			if f.NoQuoteMinutes <= 0 {
				missing = append(missing, "glitch plan")
			}
			if f.GapPct <= 0 || strings.TrimSpace(f.Calamity) == "" {
				missing = append(missing, "calamity plan")
			}
			sold := p.Runner.Qty
			for _, x := range p.Ladder {
				sold += x.Qty
			}
			if p.Qty <= 0 || math.Abs(sold-p.Qty) > 1e-9 {
				missing = append(missing, fmt.Sprintf("exits for every unit (%g of %g planned)", sold, p.Qty))
			}
			if len(missing) > 0 {
				fail("Missing: %s.", strings.Join(missing, ", "))
				break
			}
			pass("Max profit $%.2f, planned loss $%.2f, worst case $%.2f.", rep.Payoff.MaxProfitUSD, rep.Payoff.PlannedLossUSD, rep.Payoff.WorstLossUSD)

		case RULE_CROWD:
			proof := []string{}
			if option && m.OpenInterest >= int(r.param(RULE_LIQUIDITY, P_MIN_OPEN_INTEREST)) && m.OpenInterest > 0 {
				proof = append(proof, fmt.Sprintf("%d open interest", m.OpenInterest))
			}
			if m.AvgVolume > 0 && m.DayVolume >= m.AvgVolume {
				proof = append(proof, fmt.Sprintf("stock volume %.1fx average", m.DayVolume/m.AvgVolume))
			}
			for _, e := range p.Evidence {
				counts := e.Kind == EV_VOLUME || e.Kind == EV_INTEREST || e.Kind == EV_CHAIN || e.Kind == EV_SIGNAL ||
					e.Kind == EV_NEWS && e.Verified && !e.AI
				if counts && e.Stance != STANCE_AGAINST && !e.Auto {
					proof = append(proof, e.Label)
				}
			}
			if float64(len(proof)) < v[P_MIN_ITEMS] {
				fail("No proof other traders are in it: no open interest, no volume surge, no flow, chain or signal evidence.")
				break
			}
			pass("%s.", strings.Join(proof, "; "))

		case RULE_NEWS:
			c.Status = CHECK_INFO
			pass("%d headline(s) ignored: fewer than %d sources or trust under %.0f.", len(rep.Odds.Ignored), r.NewsMinSources, r.NewsMinTrustPct)

		case RULE_VOLUME:
			if m.AvgVolume <= 0 {
				fail("No volume data: the most important input is missing.")
				break
			}
			rel := m.DayVolume / m.AvgVolume
			if rel < v[P_MIN_REL_VOLUME] {
				fail("Stock volume %.2fx its average; needs %.2fx.", rel, v[P_MIN_REL_VOLUME])
				break
			}
			if option && float64(m.Volume) < v[P_MIN_CONTRACT_VOLUME] {
				fail("%d contracts traded today; needs %.0f.", m.Volume, v[P_MIN_CONTRACT_VOLUME])
				break
			}
			pass("Stock volume %.2fx average, %d contracts today.", rel, m.Volume)

		case RULE_LIQUIDITY:
			if m.Bid <= 0 || m.Ask <= 0 {
				fail("No two-sided market: bid %.2f, ask %.2f. Nobody to sell to.", m.Bid, m.Ask)
				break
			}
			spread := (m.Ask - m.Bid) / mid * PERCENT
			if spread > v[P_MAX_SPREAD] {
				fail("Spread %.0f%% of the mid (%.2f x %.2f); max %.0f%%.", spread, m.Bid, m.Ask, v[P_MAX_SPREAD])
				break
			}
			if option && float64(m.OpenInterest) < v[P_MIN_OPEN_INTEREST] {
				fail("Open interest %d; needs %.0f.", m.OpenInterest, v[P_MIN_OPEN_INTEREST])
				break
			}
			pass("Spread %.0f%%, open interest %d.", spread, m.OpenInterest)

		case RULE_LOTTO:
			if !option {
				c.Status = CHECK_INFO
				pass("Shares: not an option.")
				break
			}
			if p.Entry < v[P_MIN_PREMIUM] {
				fail("Premium %.2f under %.2f: a penny contract often has no bid.", p.Entry, v[P_MIN_PREMIUM])
				break
			}
			if m.Spot > 0 {
				otm := (p.Contract.Strike/m.Spot - 1) * PERCENT
				if p.Contract.Right == RIGHT_PUT {
					otm = -otm
				}
				if otm > v[P_MAX_OTM] {
					fail("Strike %.1f%% out of the money; max %.0f%%.", otm, v[P_MAX_OTM])
					break
				}
				pass("Strike %.1f%% from the stock, premium %.2f.", otm, p.Entry)
			}

		case RULE_REWARD:
			if rep.Payoff.RewardRisk < v[P_MIN_RATIO] {
				fail("Reward to risk %.1f; needs %.1f.", rep.Payoff.RewardRisk, v[P_MIN_RATIO])
				break
			}
			pass("Reward to risk %.1f.", rep.Payoff.RewardRisk)

		case RULE_TIME:
			if !hasExpiry {
				c.Status = CHECK_INFO
				pass("No expiry.")
				break
			}
			days := expiry.Sub(b.Now).Hours() / HOURS_PER_DAY
			if days < v[P_MIN_DAYS] {
				fail("%.1f days to expiry; needs %.0f. Time decay goes vertical in the last days.", days, v[P_MIN_DAYS])
				break
			}
			if !p.Failsafes.ExitBy.IsZero() && expiry.Sub(p.Failsafes.ExitBy).Hours()/HOURS_PER_DAY < v[P_EXIT_DAYS]-0.5 {
				fail("Time exit is less than %.0f day(s) before expiry.", v[P_EXIT_DAYS])
				break
			}
			pass("%.1f days to expiry.", days)

		case RULE_EVENT:
			if m.NextEarnings.IsZero() || !hasExpiry || m.NextEarnings.After(expiry) {
				pass("No earnings before expiry on record.")
				break
			}
			fail("Earnings %s, before expiry: implied volatility collapses after the report.", m.NextEarnings.In(MARKET_TZ).Format("Mon Jan 2"))

		case RULE_OPEN:
			if nyNow.Weekday() == time.Saturday || nyNow.Weekday() == time.Sunday {
				pass("Market closed: a fill uses the last quote.")
				break
			}
			mins := nyNow.Hour()*MINUTES_PER_HR + nyNow.Minute()
			open, close := OPEN_HOUR*MINUTES_PER_HR+OPEN_MINUTE, CLOSE_HOUR*MINUTES_PER_HR
			if mins >= open && mins < open+int(v[P_AFTER_OPEN]) {
				fail("Inside the first %.0f minutes after the open: wait for the opening rush to settle.", v[P_AFTER_OPEN])
				break
			}
			if mins >= close-int(v[P_BEFORE_CLOSE]) && mins < close {
				fail("Inside the last %.0f minutes before the close.", v[P_BEFORE_CLOSE])
				break
			}
			pass("Outside the opening and closing rush.")

		case RULE_CHASE:
			if m.PreviousClose <= 0 || m.Spot <= 0 {
				pass("No previous close to compare.")
				break
			}
			move := (m.Spot/m.PreviousClose - 1) * PERCENT
			with := move
			if p.Contract.Right == RIGHT_PUT {
				with = -move
			}
			if with > v[P_MAX_GAP] {
				fail("The stock already moved %.1f%% the play's way today; max %.0f%%. The move may be spent.", with, v[P_MAX_GAP])
				break
			}
			pass("Moved %+.1f%% today.", move)

		case RULE_BANKROLL:
			if r.BankrollUSD <= 0 {
				pass("No bankroll set.")
				break
			}
			planned := rep.Payoff.PlannedLossUSD / r.BankrollUSD * PERCENT
			worst := rep.Payoff.WorstLossUSD / r.BankrollUSD * PERCENT
			if planned > v[P_MAX_PLANNED] || worst > v[P_MAX_WORST] {
				fail("Risks %.0f%% of the $%.0f bankroll at the stop, %.0f%% worst case; max %.0f%% and %.0f%%.", planned, r.BankrollUSD, worst, v[P_MAX_PLANNED], v[P_MAX_WORST])
				break
			}
			pass("%.0f%% of bankroll at the stop, %.0f%% worst case.", planned, worst)

		case RULE_ODDS:
			if rep.Odds.DOGSPct <= 0 {
				fail("No odds: needs the stock price and volatility.")
				break
			}
			if rep.Odds.DOGSPct < v[P_MIN_ODDS] {
				fail("DOGS odds of the first exit %.0f%%; needs %.0f%%.", rep.Odds.DOGSPct, v[P_MIN_ODDS])
				break
			}
			pass("DOGS odds of the first exit %.0f%%.", rep.Odds.DOGSPct)

		case RULE_EXPECTANCY:
			if rep.Payoff.VolSource == "none" {
				fail("No volatility: expected value unknown.")
				break
			}
			if rep.Payoff.ExpectedUSD < v[P_MIN_EXPECTED] {
				fail("Rough expected value $%.2f: the market's odds say this loses on average.", rep.Payoff.ExpectedUSD)
				break
			}
			pass("Rough expected value $%.2f.", rep.Payoff.ExpectedUSD)

		case RULE_THEME:
			if p.Theme == "" {
				pass("No theme set.")
				break
			}
			n := 0
			for _, o := range b.Others {
				if o.ID != p.ID && o.Status == STATUS_ARMED && strings.EqualFold(o.Theme, p.Theme) {
					n++
				}
			}
			if float64(n) >= v[P_MAX_PER_THEME] {
				fail("%d plays on %q are already armed; max %.0f. Same headline, same bet.", n, p.Theme, v[P_MAX_PER_THEME])
				break
			}
			pass("%d other armed on %q.", n, p.Theme)

		case RULE_MAX_OPEN:
			n := 0
			for _, o := range b.Others {
				if o.ID != p.ID && o.Status == STATUS_ARMED {
					n++
				}
			}
			if float64(n) >= v[P_MAX_ARMED] {
				fail("%d plays armed; max %.0f.", n, v[P_MAX_ARMED])
				break
			}
			pass("%d armed.", n)

		case RULE_FRESH:
			if p.Market == nil || m.At.IsZero() {
				fail("No market snapshot.")
				break
			}
			age := b.Now.Sub(m.At).Minutes()
			if age > v[P_MAX_AGE] {
				fail("Market snapshot is %.0f minutes old; max %.0f. Refresh it.", age, v[P_MAX_AGE])
				break
			}
			pass("Snapshot %.0f minutes old.", age)

		case RULE_LIMIT:
			if mid <= 0 {
				pass("No quote to compare.")
				break
			}
			over := (p.Entry/mid - 1) * PERCENT
			if over > v[P_MAX_OVER_MID] {
				fail("Entry %.2f is %.0f%% over the mid %.2f; max %.0f%%. Use a limit near the mid.", p.Entry, over, mid, v[P_MAX_OVER_MID])
				break
			}
			pass("Entry %.2f vs mid %.2f.", p.Entry, mid)

		case RULE_SIZE:
			sz := rep.Sizing
			if sz.LiquidityCapped && p.Qty > sz.LiquidityMax {
				fail("%g units is %.1f%% of today's volume and %.1f%% of open interest; the rule allows %g. Expect a fill near %.2f, not %.2f.", p.Qty, sz.PctOfVolume, sz.PctOfOI, sz.LiquidityMax, sz.EstEntry, p.Entry)
				break
			}
			pass("%g units: %.1f%% of today's volume, %.1f%% of open interest.", p.Qty, sz.PctOfVolume, sz.PctOfOI)

		case RULE_COOLDOWN:
			window := time.Duration(v[P_HOURS] * float64(time.Hour))
			for _, o := range b.Others {
				if o.ID == p.ID || o.Status != STATUS_CLOSED || o.Contract.Symbol != p.Contract.Symbol || o.Result == nil || o.Result.PnLUSD >= 0 {
					continue
				}
				if b.Now.Sub(o.Result.At) < window {
					fail("%s closed at a loss %s ago; wait %.0f hours.", o.Contract.Symbol, b.Now.Sub(o.Result.At).Round(time.Minute), v[P_HOURS])
					break
				}
			}
			if c.Detail == "" {
				pass("No recent loss on %s.", p.Contract.Symbol)
			}

		case RULE_STOPS:
			stops := p.stops(r)
			if len(stops) < 2 {
				fail("The stop stays at %.2f after exits fill: add stop-losses 2 and 3, or exits to climb to.", p.Stop)
				break
			}
			pass("Stop %s: it moves up as each exit fills.", stopsLabel(stops))
		}
		out = append(out, c)
	}
	return out
}

// Build derives the report for one play: payoff, levels, odds, checks, and whether it can arm.
func Build(p Play, r Rules, b Book) Report {
	pay, levels := payoff(p, b.Now)
	rep := Report{Payoff: pay, Levels: levels}
	rep.Sizing = sizing(p, r)
	rep.Odds = odds(p, r, levels)
	rep.Odds.WinPct, rep.Odds.WinAt, rep.Odds.FillPct = winOdds(rep.Odds, pay, levels, rep.Sizing)
	rep.Record = record(p, r)
	rep.Checks = evaluate(p, r, rep, b)
	rep.Armable = true
	for _, c := range rep.Checks {
		if c.Status == CHECK_FAIL {
			rep.Armable = false
		}
	}
	return rep
}
