package signals

import (
	"cmp"
	"slices"
	"strings"

	"github.com/YEAHDOGS/money24/engine/scoreboard"
)

const (
	SCOREBOARD_DESK         = "signals"
	SCOREBOARD_CONFIGURED   = "configured"
	SCOREBOARD_RECORDED     = "recorded"
	SCOREBOARD_REPLAY       = "live-recorded call replay"
	SCOREBOARD_TRUST        = "call exit or window close"
	SCOREBOARD_REPLAY_BLOCK = "Validate these replayed outcomes with paper fills."
	SCOREBOARD_TRUST_BLOCK  = "Validate these price moves with paper fills and execution costs."
	SCOREBOARD_NO_SOURCE    = "Configure this source before relying on its record."
)

type paperSource struct {
	id      string
	pnl     float64
	results []scoreboard.Result
}

// paperScore indexes completed results and partial exits from the full journal.
type paperScore struct {
	sources map[string]*paperSource
	banked  map[int64]float64
}

func (s *paperScore) apply(e Entry) {
	if e.Result == nil || (e.Action != ACTION_CLOSE && e.Action != ACTION_FILL) {
		return
	}
	if s.sources == nil {
		s.sources, s.banked = make(map[string]*paperSource), make(map[int64]float64)
	}
	id := strings.ToLower(e.Signal.SourceID)
	if s.sources[id] == nil {
		s.sources[id] = &paperSource{id: e.Signal.SourceID}
	}
	src := s.sources[id]
	src.pnl += e.Result.PnLUSD
	if e.Action == ACTION_FILL {
		s.banked[e.PlanID] += e.Result.PnLUSD
		return
	}
	src.pnl -= s.banked[e.PlanID]
	delete(s.banked, e.PlanID)
	src.results = append(src.results, scoreboard.Result{At: e.At, Return: e.Result.PnLPct / PERCENT})
}

// Scoreboard uses the strongest available evidence for each source: paper closes, then cached
// followable live-recorded crypto replays, then cached trust window closes or source exits.
// Reading it never triggers history requests or treats a hindsight peak as a return.
func (t *Tracker) Scoreboard(j *Journal, calls *CryptoBacktester, adoptedSource string) []scoreboard.Strategy {
	rows := make(map[string]*scoreboard.Strategy)
	ensure := func(id string) *scoreboard.Strategy {
		key := strings.ToLower(id)
		if rows[key] == nil {
			rows[key] = &scoreboard.Strategy{Desk: SCOREBOARD_DESK, ID: id, State: SCOREBOARD_RECORDED,
				Basis: scoreboard.BASIS_PAPER, Blocker: SCOREBOARD_NO_SOURCE, RealizedPnL: new(float64)}
		}
		return rows[key]
	}
	t.rec.mu.Lock()
	for _, post := range t.rec.posts {
		if post.SourceID != "" {
			ensure(post.SourceID)
		}
	}
	t.rec.mu.Unlock()
	for _, source := range t.catalog.Sources {
		if source.Role == ROLE_NEWS {
			continue
		}
		row := ensure(source.ID())
		row.ID, row.Name, row.State, row.Blocker = source.ID(), cmp.Or(source.Label, source.Handle, source.Venue, source.Type), SCOREBOARD_CONFIGURED, ""
	}
	j.mu.Lock()
	for _, paper := range j.score.sources {
		row := ensure(paper.id)
		pnl := paper.pnl
		row.RealizedPnL, row.Results = &pnl, slices.Clone(paper.results)
	}
	j.mu.Unlock()

	calls.doneMu.RLock()
	for post, o := range calls.done {
		if o.Call.Imported || !o.Followable || o.R == nil || o.ResolvedAt.IsZero() {
			continue
		}
		row := ensure(post.SourceID)
		if len(row.Results) > 0 && row.Unit != scoreboard.RETURN_R {
			continue
		}
		row.Basis, row.Unit, row.Blocker = SCOREBOARD_REPLAY, scoreboard.RETURN_R, SCOREBOARD_REPLAY_BLOCK
		row.Results = append(row.Results, scoreboard.Result{At: o.ResolvedAt, Return: *o.R})
	}
	calls.doneMu.RUnlock()

	// An entry can be cached both with and without a later exit. Keep one observation per post.
	t.mu.Lock()
	closed := make(map[string]Outcome)
	for _, o := range t.done {
		if !o.Scored || o.Open {
			continue
		}
		key := postIndex(RawPost{SourceID: o.Signal.SourceID, PostID: o.Signal.PostID, Text: o.Signal.Raw})
		prev, exists := closed[key]
		if !exists || o.HasExit && (!prev.HasExit || o.ExitAt.Before(prev.ExitAt)) {
			closed[key] = o
		}
	}
	t.mu.Unlock()
	for _, o := range closed {
		row := ensure(o.Signal.SourceID)
		if len(row.Results) > 0 && row.Basis != SCOREBOARD_TRUST {
			continue
		}
		window := WINDOW_MARKET
		if o.Signal.Market == MARKET_CRYPTO {
			window = WINDOW_CRYPTO
		}
		at, ret := o.At.Add(window), o.EndPct
		if o.HasExit {
			at, ret = o.ExitAt, o.ExitPct
		}
		row.Basis, row.Blocker = SCOREBOARD_TRUST, SCOREBOARD_TRUST_BLOCK
		row.Results = append(row.Results, scoreboard.Result{At: at, Return: ret / PERCENT})
	}

	delete(rows, strings.ToLower(adoptedSource))
	out := make([]scoreboard.Strategy, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	return out
}
