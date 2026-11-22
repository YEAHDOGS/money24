package plays

import (
	"cmp"
	"time"

	"github.com/YEAHDOGS/money24/engine/scoreboard"
)

const (
	SCOREBOARD_SCOUT  = "options scout"
	SCOREBOARD_CRYPTO = "crypto"
	SCOREBOARD_FLOW   = "options flow"
	SCOREBOARD_FINDER = "stock finder"
	SCOREBOARD_MODEL  = "scout candidate returns"
	SCOREBOARD_RISK   = "high"
	SCOREBOARD_FILL   = "Validate returns with executable paper fills. Scout marks can use modeled prices."
)

var SCOREBOARD_NAMES = map[string]string{
	SETUP_ORB: "Opening range breakout", SETUP_VWAP: "VWAP reclaim", SETUP_FLOW: "Flow surge",
	SETUP_NEWS: "News catalyst", SETUP_LAST_HOUR: "Last half hour",
	CRYPTO_BREAKOUT: "Breakout", CRYPTO_RSI_REVERSION: "RSI reversion",
	CRYPTO_FOUR_HOUR: "Four-hour trend", CRYPTO_WEEKEND_FADE: "Weekend fade",
}

type scoutObservation struct {
	setup, playID string
	at            time.Time
	returnPct     float64
}

func (d *scoutDesk) keepObservation(c Candidate) {
	if c.ClosedAt.IsZero() && c.PlayID == "" {
		return
	}
	d.historyMu.Lock()
	defer d.historyMu.Unlock()
	if d.history == nil {
		d.history = make(map[string]scoutObservation)
	}
	d.history[c.ID] = scoutObservation{setup: c.Setup, playID: c.PlayID, at: c.ClosedAt, returnPct: c.ResultPct}
}

// Scoreboard snapshots every autonomous play strategy and the full scout candidate record.
func (s *Service) Scoreboard() []scoreboard.Strategy {
	rules := s.Store.Rules()
	scoutState, cryptoState := s.ScoutStatus().State, s.CryptoStatus().State
	if rules.Scout == nil || !rules.Scout.On {
		scoutState = FINDER_OFF
	}
	scout := make(map[string]*scoreboard.Strategy)
	setup := func(id string) *scoreboard.Strategy {
		if scout[id] == nil {
			scout[id] = &scoreboard.Strategy{Desk: SCOREBOARD_SCOUT, ID: id, Name: SCOREBOARD_NAMES[id], State: scoutState, Risk: SCOREBOARD_RISK,
				Basis: SCOREBOARD_MODEL, Blocker: SCOREBOARD_FILL, RealizedPnL: new(float64)}
		}
		return scout[id]
	}
	for _, id := range []string{SETUP_ORB, SETUP_VWAP, SETUP_FLOW, SETUP_NEWS, SETUP_LAST_HOUR} {
		setup(id)
	}
	playSetups := make(map[string]string)
	s.scout.historyMu.Lock()
	for _, observation := range s.scout.history {
		row := setup(observation.setup)
		if !observation.at.IsZero() {
			row.Results = append(row.Results, scoreboard.Result{At: observation.at, Return: observation.returnPct / PERCENT})
		}
		if observation.playID != "" {
			playSetups[observation.playID] = observation.setup
		}
	}
	s.scout.historyMu.Unlock()

	crypto := make(map[string]*scoreboard.Strategy)
	for _, id := range CRYPTO_STRATEGIES {
		state := scoreboard.STATE_INACTIVE
		if rules.Crypto != nil && cmp.Or(rules.Crypto.Strategy, CRYPTO_BREAKOUT) == id {
			state = cryptoState
			if !rules.Crypto.On {
				state = FINDER_OFF
			}
		}
		crypto[id] = &scoreboard.Strategy{Desk: SCOREBOARD_CRYPTO, ID: id, Name: SCOREBOARD_NAMES[id], State: state,
			Basis: scoreboard.BASIS_PAPER, RealizedPnL: new(float64)}
	}
	flow := scoreboard.Strategy{Desk: SCOREBOARD_FLOW, ID: FOUND_BY_FLOW, Name: FLOW_CATEGORY,
		State: s.FlowStatus().State, Risk: SCOREBOARD_RISK, Basis: scoreboard.BASIS_PAPER, RealizedPnL: new(float64)}
	finder := scoreboard.Strategy{Desk: SCOREBOARD_FINDER, ID: FOUND_BY_FINDER, Name: FINDER_CATEGORY,
		State: s.FinderStatus().State, Basis: scoreboard.BASIS_PAPER, RealizedPnL: new(float64)}
	if rules.Flow == nil || !rules.Flow.On {
		flow.State = FINDER_OFF
	}
	if rules.Finder == nil || !rules.Finder.On {
		finder.State = FINDER_OFF
	}

	for _, play := range s.Store.Plays() {
		var row *scoreboard.Strategy
		switch play.FoundBy {
		case FOUND_BY_SCOUT:
			if id := playSetups[play.ID]; id != "" {
				addPlayPnL(setup(id), play)
			}
			continue
		case FOUND_BY_CRYPTO:
			row = crypto[play.Category]
			if play.Category == CRYPTO_CATEGORY || play.Category == "" {
				row = crypto[CRYPTO_BREAKOUT]
			}
		case FOUND_BY_FLOW:
			row = &flow
		case FOUND_BY_FINDER:
			row = &finder
		}
		if row == nil {
			continue
		}
		addPlayPnL(row, play)
		if play.Result != nil && play.Result.Reason != REASON_VOID {
			rate := play.Result.PnLPct / PERCENT
			if play.Filled != nil && play.Filled.AmountUSD > 0 {
				rate = play.Result.PnLUSD / play.Filled.AmountUSD
			}
			row.Results = append(row.Results, scoreboard.Result{At: play.Result.At, Return: rate})
		}
	}
	rows := []scoreboard.Strategy{flow, finder}
	for _, group := range []map[string]*scoreboard.Strategy{scout, crypto} {
		for _, row := range group {
			rows = append(rows, *row)
		}
	}
	return rows
}

func addPlayPnL(row *scoreboard.Strategy, play Play) {
	if play.Result != nil {
		*row.RealizedPnL += play.Result.PnLUSD
		return
	}
	for _, fill := range play.Fills {
		*row.RealizedPnL += fill.PnLUSD
	}
}
