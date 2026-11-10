package news

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const (
	EVENT_PLAN  = "plan"
	EVENT_CYCLE = "cycle"
	// A round reports once every source it started has answered, or at this share of the interval,
	// whichever comes first. Slower sources count in the round they finish in.
	REPORT_SHARE = 0.8
	REPORT_MAX   = 15 * time.Second
	// A floored source polls on the tick nearest its floor rather than one full interval late.
	FLOOR_SLACK_SHARE = 0.5
)

// Plan is when the last round started and when the next one will. The UI's ring fills between them.
type Plan struct {
	LastAt  time.Time `json:"last_at"`
	NextAt  time.Time `json:"next_at"`
	EveryMs int64     `json:"every_ms"`
	Now     time.Time `json:"now"`
}

// Cycle is one round's report: what the sources returned and the data it cost.
type Cycle struct {
	At           time.Time `json:"at"`
	Now          time.Time `json:"now"`
	Started      int       `json:"started"`       // sources started this round
	Answered     int       `json:"answered"`      // sources that answered (this round or a late one)
	Changed      int       `json:"changed"`       // answered with new content (not a 304)
	Unchanged    int       `json:"unchanged"`     // answered 304 Not Modified
	Errors       int       `json:"errors"`        // failed
	SourcesAdded int       `json:"sources_added"` // sources that brought at least one new article
	Added        int       `json:"added"`         // new articles stored
	Pending      int       `json:"pending"`       // still running when the report went out
	BytesIn      int64     `json:"bytes_in"`
	BytesOut     int64     `json:"bytes_out"`
}

func (c *Cycle) count(o pollOutcome) {
	c.Answered++
	c.Added += o.added
	switch {
	case o.err != nil:
		c.Errors++
	case o.unchanged:
		c.Unchanged++
	default:
		c.Changed++
	}
	if o.added > 0 {
		c.SourcesAdded++
	}
}

// pollOutcome is how one source's poll ended.
type pollOutcome struct {
	id        string
	unchanged bool
	added     int
	err       error
}

// clock keeps the latest plan and report so a newly opened window can sync its ring at once.
type clock struct {
	mu        sync.Mutex
	plan      Plan
	last      Cycle
	hasReport bool
}

func (c *clock) setPlan(p Plan) {
	c.mu.Lock()
	c.plan = p
	c.mu.Unlock()
}

func (c *clock) setReport(r Cycle) {
	c.mu.Lock()
	c.last, c.hasReport = r, true
	c.mu.Unlock()
}

// snapshot returns the current plan and, if one exists, the last report.
func (c *clock) snapshot() (Plan, *Cycle) {
	c.mu.Lock()
	defer c.mu.Unlock()
	plan := c.plan
	plan.Now = time.Now().UTC()
	if !c.hasReport {
		return plan, nil
	}
	report := c.last
	return plan, &report
}

// cycleLoop is the engine's one clock: every tick it starts each due source, then reports the round.
func (s *Service) cycleLoop(ctx context.Context) {
	results := make(chan pollOutcome, len(s.sources))
	busy := map[string]bool{}
	lastStart := map[string]time.Time{}
	carry := Cycle{} // results that arrived between rounds

	for {
		tick := time.Now().UTC()
		every, _ := s.schedule.Every()
		s.announcePlan(Plan{LastAt: tick, NextAt: tick.Add(every), EveryMs: every.Milliseconds()})
		in0, out0 := s.monitor.groupBytes(GROUP_NEWS)

		report := carry
		report.At = tick
		carry = Cycle{}
		for _, src := range s.sources {
			if busy[src.ID] || !isDue(src, tick, lastStart[src.ID], every) {
				continue
			}
			busy[src.ID], lastStart[src.ID] = true, tick
			report.Started++
			go func(src Source) { results <- s.pollOnce(ctx, src) }(src)
		}

		deadline := time.NewTimer(min(time.Duration(float64(every)*REPORT_SHARE), REPORT_MAX))
		for len(busy) > 0 && !s.collect(ctx, results, busy, &report, deadline.C) {
		}
		deadline.Stop()
		if ctx.Err() != nil {
			return
		}
		in1, out1 := s.monitor.groupBytes(GROUP_NEWS)
		report.BytesIn, report.BytesOut = in1-in0, out1-out0
		report.Pending = len(busy)
		s.announceCycle(report)

		if !s.waitTick(ctx, tick, results, busy, &carry) {
			return
		}
	}
}

// collect takes one result. It returns true when the round should stop waiting (deadline or shutdown).
func (s *Service) collect(ctx context.Context, results <-chan pollOutcome, busy map[string]bool, report *Cycle, deadline <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return true
	case <-deadline:
		return true
	case o := <-results:
		delete(busy, o.id)
		report.count(o)
		return false
	}
}

// waitTick sleeps until the next tick, re-planning at once if the slider moves, and keeps
// absorbing late results so their sources are free for the next round.
func (s *Service) waitTick(ctx context.Context, tick time.Time, results <-chan pollOutcome, busy map[string]bool, carry *Cycle) bool {
	for {
		every, changed := s.schedule.Every()
		wait := time.Until(tick.Add(every))
		if wait <= 0 {
			return true
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-changed:
			timer.Stop()
			next, _ := s.schedule.Every()
			s.announcePlan(Plan{LastAt: tick, NextAt: maxTime(tick.Add(next), time.Now().UTC()), EveryMs: next.Milliseconds()})
		case o := <-results:
			timer.Stop()
			delete(busy, o.id)
			carry.count(o)
		case <-timer.C:
			return true
		}
	}
}

func isDue(src Source, tick, last time.Time, every time.Duration) bool {
	if last.IsZero() || src.MinEvery <= every {
		return true
	}
	slack := time.Duration(float64(every) * FLOOR_SLACK_SHARE)
	return tick.Sub(last)+slack >= src.MinEvery
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (s *Service) announcePlan(p Plan) {
	s.clock.setPlan(p)
	p.Now = time.Now().UTC()
	data, _ := json.Marshal(p)
	s.hub.broadcast(Message{Event: EVENT_PLAN, Data: data})
}

func (s *Service) announceCycle(r Cycle) {
	r.Now = time.Now().UTC()
	s.clock.setReport(r)
	data, _ := json.Marshal(r)
	s.hub.broadcast(Message{Event: EVENT_CYCLE, Data: data})
}
