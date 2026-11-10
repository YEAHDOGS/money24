package signals

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	ACTION_FILL        = "fill" // one take-profit rung sold; the position is still open
	ACTION_VOID        = "void" // dropped unscored: there was never a price to close it at
	REASON_STOP        = "stop"
	REASON_TARGET      = "target"
	REASON_TIME        = "time"
	REASON_TIME_STOP   = "time stop"
	REASON_SOURCE_EXIT = "source exit"
	NOTE_VOID          = "no live quote by the exit time: not scored"
	// The time stop is judged once, in this window after its check point. A runner that was down
	// for the whole window skips it rather than judging the trade at some later price.
	TIME_STOP_WINDOW = 5 * time.Minute
)

// position is one planned entry the watcher still holds. It is rebuilt from the journal on start.
type position struct {
	planID     int64
	plan       Plan
	left       int     // units not sold yet
	next       int     // index of the next unfilled target
	realized   float64 // USD banked by filled targets
	exitCalled bool    // the source said it is out, and we follow its exits
}

func (p *position) pnl(price float64, qty int) float64 {
	multiplier := float64(SHARE_MULTIPLIER)
	if p.plan.Signal.Right != "" {
		multiplier = OPTION_MULTIPLIER
	}
	return (price - p.plan.Entry) * float64(qty) * multiplier
}

func (p *position) line(action string, now time.Time, res *Result, note string) Entry {
	if res != nil {
		res.PnLUSD = math.Round(res.PnLUSD/PRICE_TICK) * PRICE_TICK
		if p.plan.CostUSD > 0 {
			res.PnLPct = math.Round(res.PnLUSD / p.plan.CostUSD * PERCENT)
		}
	}
	return Entry{At: now.UTC(), Action: action, Signal: p.plan.Signal, PlanID: p.planID, Result: res, Note: note}
}

// step returns the journal lines one quote causes, without changing the position: Runner.track
// applies them, so a live tick and a replay after restart end in the same state.
// Sells are scored at the bid, except a target, which is a resting limit and fills at its price.
func (p *position) step(q Quote, priced bool, now time.Time) []Entry {
	due := !p.plan.ExitBy.IsZero() && !now.Before(p.plan.ExitBy)
	if !priced {
		if !due {
			return nil
		}
		return []Entry{p.line(ACTION_VOID, now, nil, NOTE_VOID)}
	}
	left, realized, out := p.left, p.realized, []Entry{}
	closeAll := func(reason string) []Entry {
		res := &Result{PnLUSD: realized + p.pnl(q.Bid, left), Reason: reason, Qty: left, Price: q.Bid}
		return append(out, p.line(ACTION_CLOSE, now, res, ""))
	}
	if q.Bid <= p.plan.Stop {
		return closeAll(REASON_STOP)
	}
	for _, t := range p.plan.Targets[p.next:] {
		if q.Bid < t.Price {
			break
		}
		qty := min(t.Qty, left)
		gain := p.pnl(t.Price, qty)
		left, realized = left-qty, realized+gain
		if left == 0 {
			return append(out, p.line(ACTION_CLOSE, now, &Result{PnLUSD: realized, Reason: REASON_TARGET, Qty: qty, Price: t.Price}, ""))
		}
		out = append(out, p.line(ACTION_FILL, now, &Result{PnLUSD: gain, Reason: REASON_TARGET, Qty: qty, Price: t.Price}, ""))
	}
	stopAt := p.plan.TimeStopAt
	switch {
	case p.exitCalled:
		return closeAll(REASON_SOURCE_EXIT)
	case due:
		return closeAll(REASON_TIME)
	case !stopAt.IsZero() && !now.Before(stopAt) && now.Before(stopAt.Add(TIME_STOP_WINDOW)) && q.Bid <= p.plan.TimeStopAbove:
		return closeAll(REASON_TIME_STOP)
	}
	return out
}

// hold applies one journal line to the held positions. Called with r.mu held.
func (r *Runner) hold(e Entry) {
	switch {
	case e.Plan != nil:
		r.held[e.ID] = &position{planID: e.ID, plan: *e.Plan, left: e.Plan.Qty}
	case e.Action == ACTION_FILL && e.Result != nil && r.held[e.PlanID] != nil:
		p := r.held[e.PlanID]
		p.left, p.next, p.realized = p.left-e.Result.Qty, p.next+1, p.realized+e.Result.PnLUSD
	case e.Action == ACTION_CLOSE || e.Action == ACTION_VOID:
		delete(r.held, e.PlanID)
		delete(r.open, positionKey(e.Signal))
	case e.Signal.Kind == KIND_EXIT && r.catalog.SettingsFor(e.Signal.SourceID).FollowSourceExit:
		// Same match as track: an exit that names only the symbol covers every contract on it.
		for _, p := range r.held {
			held := p.plan.Signal
			sameSource := strings.EqualFold(held.SourceID, e.Signal.SourceID)
			if sameSource && (positionKey(held) == positionKey(e.Signal) || e.Signal.Right == "" && held.Symbol == e.Signal.Symbol) {
				p.exitCalled = true
			}
		}
	}
}

// Tick prices every held position once and journals what happened: target fills, and closes on
// the stop, the last target, the source's exit or the clock. Quotes are fetched outside the lock,
// so a slow quote source never holds up incoming signals.
func (r *Runner) Tick(now time.Time) ([]Entry, error) {
	r.mu.Lock()
	ids := make([]int64, 0, len(r.held))
	sigs := map[int64]Signal{}
	for id, p := range r.held {
		ids, sigs[id] = append(ids, id), p.plan.Signal
	}
	r.mu.Unlock()
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })

	quotes := map[int64]Quote{}
	for _, id := range ids {
		if r.quote == nil {
			break
		}
		if q, ok := r.quote(sigs[id]); ok && q.Bid > 0 {
			quotes[id] = q
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Entry{}
	for _, id := range ids {
		p := r.held[id]
		if p == nil {
			continue
		}
		q, priced := quotes[id]
		for _, e := range p.step(q, priced, now) {
			e, err := r.journal.Append(e)
			if err != nil {
				return out, err
			}
			r.hold(e)
			out = append(out, e)
		}
	}
	return out, nil
}
