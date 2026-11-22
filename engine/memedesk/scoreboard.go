package memedesk

import (
	"slices"
	"strings"

	"github.com/YEAHDOGS/money24/engine/scoreboard"
)

const (
	SCOREBOARD_DESK      = "meme"
	SCOREBOARD_BENCHMARK = "long benchmark"
	SCOREBOARD_OBSERVE   = "Validate a paper entry and exit policy. This long benchmark cannot qualify."
	SCOREBOARD_REMOVED   = "Restore a strategy configuration and collect current results."
	SCOREBOARD_JOURNAL   = "Repair the paper journal before relying on its record."
	SCOREBOARD_OBSERVING = "observing"
	SCOREBOARD_PAPER     = "paper"
	SCOREBOARD_PAUSED    = "paused"
	SCOREBOARD_OFF       = "off"
)

// Scoreboard returns resolved observations from the full journal replay, beyond the recent list.
func (d *Desk) Scoreboard() []scoreboard.Strategy {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows := make([]scoreboard.Strategy, 0, len(d.model.states))
	for id, s := range d.model.states {
		pnl := s.record.PaperRealizedUSD
		row := scoreboard.Strategy{Desk: SCOREBOARD_DESK, ID: id, Name: strings.ReplaceAll(id, "-", " "),
			Risk: s.config.Risk, State: SCOREBOARD_PAPER, Basis: scoreboard.BASIS_PAPER, RealizedPnL: &pnl}
		if s.canBuy || len(s.paperResults) > 0 {
			row.Results = slices.Clone(s.paperResults)
		} else {
			row.State, row.Basis, row.Blocker = SCOREBOARD_OBSERVING, SCOREBOARD_BENCHMARK, SCOREBOARD_OBSERVE
			row.Results = slices.Clone(s.benchmarkResults)
		}
		switch {
		case !s.configured:
			row.State, row.Blocker = scoreboard.STATE_REMOVED, SCOREBOARD_REMOVED
		case !s.config.On:
			row.State = SCOREBOARD_OFF
		case s.canBuy && d.model.stopped:
			row.State = SCOREBOARD_PAUSED
		}
		if d.journalErr != nil {
			row.Blocker = SCOREBOARD_JOURNAL
		}
		rows = append(rows, row)
	}
	return rows
}
