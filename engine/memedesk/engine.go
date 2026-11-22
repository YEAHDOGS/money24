package memedesk

import (
	"container/list"
	"strings"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	REASON_OFF          = "strategy_off"
	REASON_REMOVED      = "strategy_removed"
	REASON_WATCH_ONLY   = "watch_only"
	REASON_NO_QUOTE     = "no_entry_quote"
	REASON_NO_FILL      = "entry_slippage_or_liquidity"
	REASON_ALREADY_OPEN = "mint_already_open"
	REASON_MAX_OPEN     = "open_position_limit"
	REASON_HOUR_LIMIT   = "hourly_entry_limit"
	REASON_DAILY_STAKE  = "daily_stake_limit"
	REASON_DAILY_LOSS   = "daily_loss_stop"
	REASON_CASH         = "paper_cash_limit"
	RECENT_LIMIT        = 30
	SHADOW_DELAY        = 15 * time.Second // allow a scheduled safety lookup before the shadow entry quote
)

type candidate struct {
	Candidate
	rule        configured
	fadeReady   time.Time
	followUntil time.Time
	followed    bool
}

type token struct {
	launch                     *meme.Launch
	safety                     *meme.Safety
	seenLaunch, seenGraduation bool
	expires                    time.Time
	candidates                 []*candidate
}

type engine struct {
	rulebook               Rulebook
	rules                  []configured
	states                 map[string]*strategyState
	tokens                 map[string]*token
	waiting, watching      list.List
	clock, nextPrune       time.Time
	day                    string
	dayRealized, cashDelta float64
	stopped                bool
}

func newEngine() *engine {
	return &engine{states: make(map[string]*strategyState), tokens: make(map[string]*token)}
}

func (e *engine) configure(rb Rulebook, rules []configured) {
	for _, s := range e.states {
		s.configured = false
	}
	for _, r := range rules {
		s := e.states[r.config.ID]
		if s == nil {
			s = &strategyState{open: make(map[string]*candidate), record: Record{Reasons: make(map[string]int)}}
			e.states[r.config.ID] = s
		}
		s.config, s.configured, s.canBuy = r.config, true, r.buy
	}
	e.rulebook, e.rules = rb, rules
	e.checkLossStop()
}

func (e *engine) advance(at time.Time) {
	if !at.After(e.clock) {
		return
	}
	e.clock = at
	day := at.UTC().Format(meme.DAY_LAYOUT)
	if day != e.day {
		e.day, e.dayRealized, e.stopped = day, 0, false
	}
	for node := e.waiting.Front(); node != nil; node = e.waiting.Front() {
		c := node.Value.(*candidate)
		if at.Before(c.ObservedAt.Add(ENTRY_WINDOW)) {
			break
		}
		if c.DecidedAt.IsZero() {
			t := e.tokens[c.Mint]
			e.decide(t, c, nil, c.ObservedAt.Add(ENTRY_WINDOW), true)
		}
		e.waiting.Remove(node)
	}
	for node := e.watching.Front(); node != nil; node = e.watching.Front() {
		c := node.Value.(*candidate)
		if at.Before(c.followUntil) {
			break
		}
		e.finishWatching(c)
		e.watching.Remove(node)
	}
	if !at.Before(e.nextPrune) {
		for mint, t := range e.tokens {
			if !at.Before(t.expires) && len(t.candidates) == 0 {
				delete(e.tokens, mint)
			}
		}
		e.nextPrune = at.Add(time.Minute)
	}
}

func (e *engine) observe(row meme.Record, admit bool) {
	e.advance(row.At)
	if row.Mint == "" {
		return
	}
	t := e.tokens[row.Mint]
	switch row.Kind {
	case meme.KIND_LAUNCH, meme.KIND_GRADUATION:
		if t == nil {
			t = &token{}
			e.tokens[row.Mint] = t
		}
		e.admit(t, row, admit)
	case meme.KIND_SAFETY:
		if row.Safety == nil || row.Error != "" {
			return
		}
		if t == nil {
			t = &token{expires: e.clock.Add(FOLLOW_WINDOW)}
			e.tokens[row.Mint] = t
		}
		t.safety = row.Safety
	case meme.KIND_SNAPSHOT:
		if t != nil && row.Error == "" {
			e.snapshot(t, row)
		}
	}
}

func (e *engine) admit(t *token, row meme.Record, admit bool) {
	if row.Kind == meme.KIND_LAUNCH {
		if t.seenLaunch {
			return
		}
		t.seenLaunch, t.launch = true, row.Launch
	} else {
		if t.seenGraduation {
			return
		}
		t.seenGraduation = true
	}
	t.expires = e.clock.Add(FOLLOW_WINDOW)
	if !admit {
		return
	}
	for _, rule := range e.rules {
		c := &candidate{Candidate: Candidate{
			Mint: row.Mint, Event: row.Kind, Signature: row.Signature, ObservedAt: e.clock,
			Tracking: row.Tracking, Decision: Decision{Action: DECISION_WAIT},
		}, rule: rule, followUntil: t.expires}
		c.event = row
		t.candidates = append(t.candidates, c)
		s := e.states[rule.config.ID]
		s.add(c)
		e.waiting.PushBack(c)
		e.watching.PushBack(c)
		if !rule.config.On || (rule.config.Kind == KIND_ORGANIC && row.Kind != meme.KIND_GRADUATION) {
			e.decide(t, c, nil, e.clock, false)
		}
	}
}

func (e *engine) snapshot(t *token, row meme.Record) {
	for _, c := range t.candidates {
		if !quoteAfter(row, row.At, c.event.At) {
			continue
		}
		if c.followed && !c.paperOpen() {
			continue
		}
		if !c.followed {
			c.Path.observe(row, e.clock)
		}
		if !c.matchMarket(row.Market) {
			if c.paperOpen() {
				c.Paper.MarkStale = true
			}
			if c.Benchmark != nil && c.Benchmark.ClosedAt.IsZero() {
				c.Benchmark.MarkStale = true
			}
			continue
		}
		e.updatePositions(c, row)
	}
	for _, c := range t.candidates {
		delay := SIMULATED_LATENCY
		if c.rule.config.Kind == KIND_SHADOW {
			delay = SHADOW_DELAY
		}
		if !c.DecidedAt.IsZero() || !c.matchMarket(row.Market) || price(row.Market) == 0 || liquidity(row.Market) == 0 ||
			!quoteAfter(row, row.At, c.ObservedAt.Add(delay)) {
			continue
		}
		if !c.fadeReady.IsZero() && !quoteAfter(row, row.At, c.fadeReady) {
			continue
		}
		e.decide(t, c, row.Market, e.clock, false)
	}
	e.trim(t)
}

func (c *candidate) matchMarket(m *meme.Market) bool {
	if c.Event != meme.KIND_GRADUATION || m == nil {
		return true
	}
	bond := c.event.Graduation
	if bond != nil && bond.Pool != "" {
		return m.Pool == bond.Pool
	}
	if c.pool != "" {
		return m.Pool == c.pool
	}
	venue := ""
	if bond != nil {
		venue = bond.Venue
	}
	if venue == "" {
		venue = DEFAULT_GRADUATION_VENUE
	}
	return normalizeVenue(venue) == normalizeVenue(m.Venue)
}

func normalizeVenue(venue string) string {
	key := strings.ToLower(strings.ReplaceAll(venue, "_", "-"))
	if key == DEFAULT_GRADUATION_VENUE {
		return PUMPSWAP_VENUE
	}
	return key
}

func (e *engine) decide(t *token, c *candidate, m *meme.Market, at time.Time, expired bool) {
	s := e.states[c.rule.config.ID]
	if !c.rule.config.On {
		s.decided(c, Decision{Action: DECISION_SKIP, Reasons: []string{REASON_OFF}}, at)
		return
	}
	if !c.fadeReady.IsZero() {
		if expired || !c.fadeReady.After(at) {
			e.enter(c, s, m, at, Decision{Action: DECISION_FADE, Benchmark: true})
		}
		return
	}
	o := Observation{Event: c.event, Launch: t.launch, Safety: t.safety, Market: m, FirstPrice: c.anchor, Expired: expired}
	decision := c.rule.rule.Evaluate(o)
	if c.anchor == 0 {
		c.anchor = price(m)
	}
	if decision.Action == DECISION_WAIT && !expired {
		return
	}
	if decision.Action == DECISION_WAIT {
		decision.Action = DECISION_SKIP
	}
	if decision.Action == DECISION_FADE && !expired {
		c.fadeReady = at.Add(SIMULATED_LATENCY)
		c.SpikeAt, c.SpikeUSD = at, price(m)
		return
	}
	e.enter(c, s, m, at, decision)
}

func (e *engine) enter(c *candidate, s *strategyState, m *meme.Market, at time.Time, d Decision) {
	fill, fillable := buyFill(m, c.rule.config.Limits.StakeUSD, at)
	if d.Benchmark {
		switch {
		case price(m) == 0:
			d.Reasons = append(d.Reasons, REASON_NO_QUOTE)
		case !fillable:
			d.Reasons = append(d.Reasons, REASON_NO_FILL)
		default:
			c.Benchmark = newPosition(c, fill)
			c.Benchmark.mark(m, at)
			c.pool = m.Pool
		}
	}
	if d.Action == DECISION_BUY {
		if !fillable {
			d.Action = DECISION_SKIP
			if !d.Benchmark {
				d.Reasons = append(d.Reasons, REASON_NO_FILL)
			}
		} else if why := e.limit(c, s, fill.NetUSD); why != "" {
			d.Action, d.Reasons = DECISION_SKIP, append(d.Reasons, why)
		} else {
			e.cashDelta -= fill.NetUSD
			s.stake(e.day, fill)
			c.Paper = newPosition(c, fill)
			c.Paper.mark(m, at)
			s.open[c.Mint] = c
		}
	}
	s.decided(c, d, at)
}

func (e *engine) limit(c *candidate, s *strategyState, cost float64) string {
	l := s.config.Limits
	s.pruneEntries(e.clock)
	switch {
	case !s.configured:
		return REASON_REMOVED
	case !s.config.On:
		return REASON_OFF
	case !s.canBuy || !c.rule.buy:
		return REASON_WATCH_ONLY
	case e.stopped:
		return REASON_DAILY_LOSS
	case s.open[c.Mint] != nil:
		return REASON_ALREADY_OPEN
	case len(s.open) >= l.MaxOpen:
		return REASON_MAX_OPEN
	case len(s.entries) >= l.NewPerHour:
		return REASON_HOUR_LIMIT
	case cost > l.StakeUSD || cost+s.staked(e.day) > l.DailyStakeUSD:
		return REASON_DAILY_STAKE
	case cost > e.rulebook.CapitalUSD+e.cashDelta:
		return REASON_CASH
	}
	return ""
}

func (e *engine) updatePositions(c *candidate, row meme.Record) {
	s := e.states[c.rule.config.ID]
	if c.Benchmark != nil && !c.followed && c.Benchmark.ClosedAt.IsZero() {
		c.Benchmark.update(row, e.clock)
		if !c.Benchmark.ClosedAt.IsZero() {
			s.settleBenchmark(c)
		}
	}
	if !c.paperOpen() {
		return
	}
	p := c.Paper
	before := len(p.Sales)
	p.update(row, e.clock)
	for _, fill := range p.Sales[before:] {
		realized := fill.NetUSD - fill.Units/p.Buy.Units*p.Buy.NetUSD
		e.cashDelta += fill.NetUSD
		e.dayRealized += realized
		s.record.PaperRealizedUSD += realized
		e.checkLossStop()
	}
	if !p.ClosedAt.IsZero() {
		s.settlePaper(c)
	}
}

func (e *engine) checkLossStop() {
	if e.rulebook.DailyLossUSD > 0 && e.dayRealized <= -e.rulebook.DailyLossUSD {
		e.stopped = true
	}
}

func (e *engine) finishWatching(c *candidate) {
	c.followed = true
	record := &e.states[c.rule.config.ID].record
	if c.Benchmark != nil && c.Benchmark.ClosedAt.IsZero() {
		c.Benchmark.MarkStale = true
		record.BenchmarkCensored++
		record.BenchmarkOpen--
	}
	if c.paperOpen() {
		c.Paper.MarkStale = true
	}
	if c.Path.FirstUSD == 0 {
		record.UnpricedFinished++
	}
	record.Following--
	e.trim(e.tokens[c.Mint])
}

func (e *engine) trim(t *token) {
	keep := t.candidates[:0]
	for _, c := range t.candidates {
		if !c.followed || c.paperOpen() {
			keep = append(keep, c)
		}
	}
	clear(t.candidates[len(keep):])
	t.candidates = keep
}
