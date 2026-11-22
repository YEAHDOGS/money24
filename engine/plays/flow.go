package plays

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/calendar"
	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	// The flow desk follows the money into options. Every few minutes of the stock session it reads the
	// option chains of the most traded names, finds the contracts with the most premium changing hands
	// today on the side the crowd is leaning, and trades the top call and the top put with the
	// rulebook's whole bracket: entry, stop, ladder, runner, time exit and sideways exit. It never
	// trades through a blocking rule. The numbers come from the chains, never from a model: an AI read
	// is asked for afterwards, when a key is set, and it changes nothing on its own.
	FLOW_TIMEOUT       = 90 * time.Second
	FLOW_DEFAULT_EVERY = 5 * time.Minute
	FLOW_REQUEST_GAP   = 150 * time.Millisecond // between chain requests: Yahoo throttles bursts
	FOUND_BY_FLOW      = "flow"
	FLOW_CATEGORY      = "Order flow"
	FLOW_SOURCE        = "Option chains"
	FLOW_THEME_PREFIX  = "flow:"
	FLOW_LOG_PREFIX    = "flow desk: "

	WHY_FLOW_SESSION   = "the stock session is closed, or inside its opening or closing wait"
	WHY_FLOW_DAY_FULL  = "%d flow plays opened today, the most the rulebook allows"
	WHY_FLOW_OPEN_FULL = "%d flow plays are running, the most the rulebook allows"
	WHY_FLOW_DAY_LOSS  = "flow plays closed today lost $%.2f, past the $%.2f pause"
	WHY_FLOW_CHAIN     = "chain: %v"
	WHY_FLOW_NO_EXPIRY = "no expiry between %d and %d days out"
	WHY_FLOW_QUIET     = "nothing on it cleared %d contracts, %.1fx open interest and a %.0f%% spread"
	WHY_FLOW_BALANCED  = "calls %s and puts %s: neither side leads by %.1fx"
	WHY_FLOW_HELD      = "a play on it is already running"
	WHY_FLOW_TODAY     = "it already had a flow play today"
	WHY_FLOW_SIDE_DONE = "the pass already took its %s"
	WHY_FLOW_PICKED    = "top %s: %s, %s contracts, $%s premium, %.1fx open interest"
	WHY_FLOW_RUNNER_UP = "%s: %s, $%s premium, behind the pass's pick"
	EVIDENCE_FLOW      = "%s contracts traded today on this strike, %.1fx its open interest: about $%s of premium. %s calls %s, puts %s today"
	THESIS_FLOW        = "The most premium traded today on %s %s was this contract: %s contracts, %.1fx its open interest, $%s. The crowd is on the %s side (calls %s, puts %s across the next expiries). Entry at the mid with the rulebook's bracket."
	SIDE_CALLS         = "call"
	SIDE_PUTS          = "put"
	THOUSAND           = 1e3
	MILLION            = 1e6
)

// FlowRules is the flow desk's part of the rulebook.
type FlowRules struct {
	On           bool     `json:"on"`
	Symbols      []string `json:"symbols"`        // the names whose chains are read: the most traded options in the US
	EveryMinutes int      `json:"every_minutes"`  // how often it looks during the session
	MinDays      int      `json:"min_days"`       // expiries at least this far out: no lottery tickets on their last days
	MaxDays      int      `json:"max_days"`       // and no further than this
	Expiries     int      `json:"expiries"`       // how many expiries in that window are read per name
	MaxStrikePct float64  `json:"max_strike_pct"` // strikes within this percent of the stock, either side
	MinVolume    int      `json:"min_volume"`     // contracts traded today on the strike
	MinVolumeOI  float64  `json:"min_volume_oi"`  // today's volume over open interest: new money, not old positions trading
	SideRatio    float64  `json:"side_ratio"`     // one side's volume over the other's before it counts as the crowd's side
	MaxSpreadPct float64  `json:"max_spread_pct"` // bid-ask spread as a percent of the mid
	OpenWaitMin  int      `json:"open_wait_min"`  // minutes after the open before it looks: spreads are wide at the bell
	CloseWaitMin int      `json:"close_wait_min"`
	MaxPerDay    int      `json:"max_per_day"`
	MaxOpen      int      `json:"max_open"`
	// MaxDayLossUSD pauses the desk for the day once flow plays closed today have lost this much. 0 never pauses.
	MaxDayLossUSD float64 `json:"max_day_loss_usd"`
	AskAI         bool    `json:"ask_ai"` // after a play opens, have the first connected AI provider read it
}

// DefaultFlow is how the flow desk runs until the rulebook says otherwise.
func DefaultFlow() *FlowRules {
	return &FlowRules{On: true, Symbols: []string{"SPY", "QQQ", "IWM", "NVDA", "TSLA", "AAPL", "AMD", "META", "AMZN", "MSFT", "GOOGL", "PLTR"},
		EveryMinutes: 5, MinDays: 7, MaxDays: 45, Expiries: 3, MaxStrikePct: 10, MinVolume: 500, MinVolumeOI: 0.5, SideRatio: 1.3,
		MaxSpreadPct: 10, OpenWaitMin: 15, CloseWaitMin: 15, MaxPerDay: 6, MaxOpen: 4, MaxDayLossUSD: 150, AskAI: true}
}

// FlowNote is what one pass decided about one name.
type FlowNote struct {
	Symbol  string `json:"symbol"`
	Outcome string `json:"outcome"` // opened, drafted or skipped
	Why     string `json:"why,omitempty"`
	PlayID  string `json:"play_id,omitempty"`
}

// FlowStatus is the flow desk's last pass, for Home: nothing it decides is hidden.
type FlowStatus struct {
	BytesOnDisk int64      `json:"bytes_on_disk"`
	At          time.Time  `json:"at"`
	State       string     `json:"state"` // looking, off, closed, full, paused or error
	Why         string     `json:"why,omitempty"`
	Today       int        `json:"today"`
	Open        int        `json:"open"`
	DayPnL      float64    `json:"day_pnl"`
	Notes       []FlowNote `json:"notes"`
	NextAt      time.Time  `json:"next_at"`
	EveryMs     int64      `json:"every_ms"`
}

type flowDesk struct {
	mu   sync.Mutex
	last FlowStatus
}

// FlowStatus returns the flow desk's last pass.
func (s *Service) FlowStatus() FlowStatus {
	s.flow.mu.Lock()
	status := s.flow.last
	s.flow.mu.Unlock()
	status.BytesOnDisk = s.Store.bytesOnDisk()
	return status
}

// flowPick is one contract the crowd is buying, with what it traded today.
type flowPick struct {
	Symbol       string
	Right        string
	Strike       float64
	Expiry       string
	Volume       int
	OpenInterest int
	PremiumUSD   float64 // volume x mid x 100: the dollars that changed hands on it today
	CallVolume   int     // the name's call and put volume across the expiries read
	PutVolume    int
}

func (p flowPick) side() string {
	if p.Right == RIGHT_PUT {
		return SIDE_PUTS
	}
	return SIDE_CALLS
}

func (p flowPick) volumeOI() float64 {
	if p.OpenInterest <= 0 {
		return float64(p.Volume)
	}
	return float64(p.Volume) / float64(p.OpenInterest)
}

func (p flowPick) label() string {
	return fmt.Sprintf("%s %g%s %s", p.Symbol, p.Strike, p.Right, p.Expiry)
}

// short writes 1234567 as 1.2M, for notes a person reads at a glance.
func short(v float64) string {
	switch {
	case v >= MILLION:
		return fmt.Sprintf("%.1fM", v/MILLION)
	case v >= THOUSAND:
		return fmt.Sprintf("%.0fK", v/THOUSAND)
	}
	return fmt.Sprintf("%.0f", v)
}

// sessionOpen says whether the stock session is open, with the desk's waits at both ends.
func (f FlowRules) sessionOpen(now time.Time) bool {
	day := calendar.On(now)
	if day.Session == calendar.SESSION_CLOSED {
		return false
	}
	from := day.Opens.Add(time.Duration(f.OpenWaitMin) * time.Minute)
	until := day.Closes.Add(-time.Duration(f.CloseWaitMin) * time.Minute)
	return !now.Before(from) && now.Before(until)
}

// best reads one name's chains and returns its top contract on the side the crowd leans to, or why there is none.
func (s *Service) flowBest(ctx context.Context, symbol string, f FlowRules, now time.Time) (flowPick, string) {
	near, err := s.Resolver.Market.Chain(ctx, symbol, 0)
	if err != nil {
		return flowPick{}, fmt.Sprintf(WHY_FLOW_CHAIN, err)
	}
	expiries := []int64{}
	for _, e := range near.Expiries {
		days := float64(e-now.Unix()) / DAY_SECONDS
		if days >= float64(f.MinDays) && days <= float64(f.MaxDays) && len(expiries) < f.Expiries {
			expiries = append(expiries, e)
		}
	}
	if len(expiries) == 0 {
		return flowPick{}, fmt.Sprintf(WHY_FLOW_NO_EXPIRY, f.MinDays, f.MaxDays)
	}
	var best flowPick
	calls, puts := 0, 0
	candidates := map[string]*flowPick{}
	for _, e := range expiries {
		time.Sleep(FLOW_REQUEST_GAP)
		chain, err := s.Resolver.Market.Chain(ctx, symbol, e)
		if err != nil || chain.Spot <= 0 {
			continue
		}
		expiry := time.Unix(chain.Expiry, 0).UTC().Format(market.EXPIRY_LAYOUT)
		for _, side := range []struct {
			right string
			rows  []market.OptionRow
			total *int
		}{{RIGHT_CALL, chain.Calls, &calls}, {RIGHT_PUT, chain.Puts, &puts}} {
			for _, row := range side.rows {
				*side.total += row.Volume
				mid := (row.Bid + row.Ask) / 2
				if row.Bid <= 0 || row.Ask < row.Bid || row.Volume < f.MinVolume || mid <= 0 {
					continue
				}
				pick := flowPick{Symbol: symbol, Right: side.right, Strike: row.Strike, Expiry: expiry, Volume: row.Volume,
					OpenInterest: row.OpenInterest, PremiumUSD: float64(row.Volume) * mid * OPTION_MULT}
				spread := (row.Ask - row.Bid) / mid * PERCENT
				if spread > f.MaxSpreadPct || math.Abs(row.Strike/chain.Spot-1)*PERCENT > f.MaxStrikePct || pick.volumeOI() < f.MinVolumeOI {
					continue
				}
				if top := candidates[side.right]; top == nil || pick.PremiumUSD > top.PremiumUSD {
					candidates[side.right] = &pick
				}
			}
		}
	}
	right := ""
	switch {
	case float64(calls) >= f.SideRatio*float64(puts):
		right = RIGHT_CALL
	case float64(puts) >= f.SideRatio*float64(calls):
		right = RIGHT_PUT
	default:
		return flowPick{}, fmt.Sprintf(WHY_FLOW_BALANCED, short(float64(calls)), short(float64(puts)), f.SideRatio)
	}
	top := candidates[right]
	if top == nil {
		return flowPick{}, fmt.Sprintf(WHY_FLOW_QUIET, f.MinVolume, f.MinVolumeOI, f.MaxSpreadPct)
	}
	best = *top
	best.CallVolume, best.PutVolume = calls, puts
	return best, ""
}

// openFlow drafts the play for one pick and trades it when no blocking rule fails.
func (s *Service) openFlow(ctx context.Context, pick flowPick, rules Rules, now time.Time) (Play, string, string) {
	c := Contract{Symbol: pick.Symbol, Market: MARKET_OPTION, Right: pick.Right, Strike: pick.Strike, Expiry: pick.Expiry}
	p := Draft(c, s.Resolver.Snapshot(ctx, c, now), rules.Template, now)
	p.ID, p.FoundBy, p.Category, p.Theme = NewID(now), FOUND_BY_FLOW, FLOW_CATEGORY, FLOW_THEME_PREFIX+pick.Symbol
	p.Thesis = fmt.Sprintf(THESIS_FLOW, pick.Symbol, pick.side()+"s", short(float64(pick.Volume)), pick.volumeOI(), short(pick.PremiumUSD),
		pick.side(), short(float64(pick.CallVolume)), short(float64(pick.PutVolume)))
	p.Evidence = append(p.Evidence, Evidence{Kind: EV_VOLUME, Stance: STANCE_FOR, Source: FLOW_SOURCE, Verified: true, At: now.UTC(),
		Value: float64(pick.Volume), Label: fmt.Sprintf(EVIDENCE_FLOW, short(float64(pick.Volume)), pick.volumeOI(), short(pick.PremiumUSD),
			pick.Symbol, short(float64(pick.CallVolume)), short(float64(pick.PutVolume)))})
	if err := s.Store.Save(p); err != nil {
		return Play{}, FINDER_SKIPPED, err.Error()
	}
	blocked := []string{}
	for _, check := range Build(p, rules, Book{Others: s.Store.Plays(), Now: now}).Checks {
		if check.Status == CHECK_FAIL {
			blocked = append(blocked, check.ID)
		}
	}
	if len(blocked) > 0 {
		return p, FINDER_DRAFTED, WHY_BLOCKED + strings.Join(blocked, ", ")
	}
	armed, err := s.paper(p, now)
	if err != nil {
		return p, FINDER_DRAFTED, err.Error()
	}
	return armed, FINDER_OPENED, ""
}

// scanFlow runs one pass: every name's top contract, then the best call and the best put get a play.
func (s *Service) scanFlow(ctx context.Context, now time.Time) FlowStatus {
	s.sync()
	rules := s.Store.Rules()
	f := rules.Flow
	every := FLOW_DEFAULT_EVERY
	if f != nil && f.EveryMinutes > 0 {
		every = time.Duration(f.EveryMinutes) * time.Minute
	}
	st := FlowStatus{At: now.UTC(), State: FINDER_LOOKING, Notes: []FlowNote{}, NextAt: now.Add(every).UTC(), EveryMs: every.Milliseconds()}
	if f == nil || !f.On || s.Resolver.Market == nil {
		st.State = FINDER_OFF
		return st
	}
	day := now.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT)
	held, todays := map[string]bool{}, map[string]bool{}
	for _, p := range s.Store.Plays() {
		if p.FoundBy != FOUND_BY_FLOW {
			continue
		}
		if p.Status == STATUS_ARMED {
			held[p.Contract.Symbol] = true
			st.Open++
		}
		if p.CreatedAt.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			todays[p.Contract.Symbol] = true
			st.Today++
		}
		if p.Result != nil && p.Result.At.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			st.DayPnL += p.Result.PnLUSD
		}
	}
	switch {
	case !f.sessionOpen(now):
		st.State, st.Why = FINDER_CLOSED, WHY_FLOW_SESSION
	case f.MaxPerDay > 0 && st.Today >= f.MaxPerDay:
		st.State, st.Why = FINDER_FULL, fmt.Sprintf(WHY_FLOW_DAY_FULL, st.Today)
	case f.MaxOpen > 0 && st.Open >= f.MaxOpen:
		st.State, st.Why = FINDER_FULL, fmt.Sprintf(WHY_FLOW_OPEN_FULL, st.Open)
	case f.MaxDayLossUSD > 0 && st.DayPnL <= -f.MaxDayLossUSD:
		st.State, st.Why = FINDER_PAUSED, fmt.Sprintf(WHY_FLOW_DAY_LOSS, -st.DayPnL, f.MaxDayLossUSD)
	}
	if st.State != FINDER_LOOKING {
		return st
	}
	picks := []flowPick{}
	for _, symbol := range f.Symbols {
		note := FlowNote{Symbol: symbol, Outcome: FINDER_SKIPPED}
		switch {
		case held[symbol]:
			note.Why = WHY_FLOW_HELD
		case todays[symbol]:
			note.Why = WHY_FLOW_TODAY
		default:
			pick, why := s.flowBest(ctx, symbol, *f, now)
			note.Why = why
			if why == "" {
				picks = append(picks, pick)
				note.Why = fmt.Sprintf(WHY_FLOW_RUNNER_UP, pick.label(), pick.side(), short(pick.PremiumUSD))
			}
		}
		st.Notes = append(st.Notes, note)
		if ctx.Err() != nil {
			break
		}
	}
	// The most premium first; one call and one put a pass, and never past the open-play cap.
	sort.Slice(picks, func(a, b int) bool { return picks[a].PremiumUSD > picks[b].PremiumUSD })
	taken := map[string]bool{}
	for _, pick := range picks {
		note := flowNoteFor(st.Notes, pick.Symbol)
		if taken[pick.Right] || (f.MaxOpen > 0 && st.Open >= f.MaxOpen) {
			if taken[pick.Right] {
				note.Why = fmt.Sprintf(WHY_FLOW_SIDE_DONE, pick.side()) + "; " + note.Why
			}
			continue
		}
		taken[pick.Right] = true
		p, outcome, why := s.openFlow(ctx, pick, rules, now)
		note.Outcome, note.PlayID = outcome, p.ID
		note.Why = fmt.Sprintf(WHY_FLOW_PICKED, pick.side(), pick.label(), short(float64(pick.Volume)), short(pick.PremiumUSD), pick.volumeOI())
		if why != "" {
			note.Why += "; " + why
		}
		if outcome == FINDER_OPENED {
			st.Open++
			s.askAI(p, *f)
		}
		log.Printf(FLOW_LOG_PREFIX+"%s %s %s", pick.label(), outcome, why)
	}
	return st
}

// askAI has the first connected provider read a play the desk just opened. The read is filed on the
// play for a person to look at; it moves nothing on its own.
func (s *Service) askAI(p Play, f FlowRules) {
	if !f.AskAI {
		return
	}
	for _, x := range Providers() {
		if !x.Connected {
			continue
		}
		s.Store.Update(p.ID, func(q *Play) { q.Analyzing = x.ID })
		go s.analyze(p.ID, x.ID, "")
		return
	}
}

// flowNoteFor is the pass's note on one name, to finish once its pick was acted on.
func flowNoteFor(notes []FlowNote, symbol string) *FlowNote {
	for i := range notes {
		if notes[i].Symbol == symbol {
			return &notes[i]
		}
	}
	return &FlowNote{}
}

// RunFlow looks for flow plays until ctx ends: once at the start, then as often as the rulebook says.
func (s *Service) RunFlow(ctx context.Context) {
	for {
		pass, cancel := context.WithTimeout(ctx, FLOW_TIMEOUT)
		st := s.scanFlow(pass, time.Now())
		cancel()
		s.flow.mu.Lock()
		s.flow.last = st
		s.flow.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(st.EveryMs) * time.Millisecond):
		}
	}
}
