package predict

import "github.com/YEAHDOGS/money24/engine/scoreboard"

const (
	SCOREBOARD_DESK  = "prediction"
	SCOREBOARD_BASIS = "fee-adjusted candidate returns"
	SCOREBOARD_OLD   = "Restore a strategy configuration and collect current results."
)

// Scoreboard snapshots all watched markets, including contracts the desk did not buy.
func (d *Desk) Scoreboard() []scoreboard.Strategy {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows := make(map[string]*scoreboard.Strategy)
	ensure := func(id string) *scoreboard.Strategy {
		if rows[id] == nil {
			rows[id] = &scoreboard.Strategy{Desk: SCOREBOARD_DESK, ID: id, State: scoreboard.STATE_REMOVED,
				Basis: SCOREBOARD_BASIS, Blocker: SCOREBOARD_OLD, RealizedPnL: new(float64)}
		}
		return rows[id]
	}
	for _, r := range d.strategies {
		cfg := r.cfg
		row := ensure(cfg.ID)
		row.Name, row.Risk, row.State, row.Blocker = cfg.Name, cfg.Risk, STATE_LOOKING, ""
		switch {
		case d.off || !cfg.On:
			row.State = STATE_OFF
		case d.runs[cfg.ID] != nil && d.runs[cfg.ID].state != "":
			row.State = d.runs[cfg.ID].state
		case RISK_ORDER[cfg.Risk] > RISK_ORDER[d.rulebook.Mode]:
			row.State = STATE_WATCH
		}
	}
	costs := make(map[string]float64)
	for _, w := range d.Book.watches {
		id := w.line.Strategy
		row := ensure(id)
		cost := w.line.Price + Fee(1, w.line.Price)
		if !w.settled || cost <= 0 || w.settledAt.IsZero() {
			continue
		}
		row.Results = append(row.Results, scoreboard.Result{At: w.settledAt, Return: (w.payout - cost) / cost})
		costs[id] += cost
	}
	for _, p := range d.Book.positions {
		row := ensure(p.Buy.Strategy)
		if p.Buy.Strategy == TIP_STRATEGY {
			// Tips have no rulebook entry: they are started by hand from Scribe.
			row.Name, row.State, row.Blocker = TIP_NAME, STATE_LOOKING, ""
		}
		if p.Settled {
			*row.RealizedPnL += p.PnlUSD
		}
	}
	out := make([]scoreboard.Strategy, 0, len(rows))
	for id, row := range rows {
		if len(row.Results) > 0 {
			rate := costs[id] / float64(len(row.Results))
			row.BreakEvenRate = &rate
		}
		out = append(out, *row)
	}
	return out
}
