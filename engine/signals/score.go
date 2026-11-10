package signals

import (
	"fmt"
	"sort"
	"time"
)

const (
	ACTION_CLOSE = "close" // journal line for a finished paper trade; carries a Result
	ALL_SOURCES  = "all"
	DAY_LAYOUT   = "2006-01-02"
)

// Result is how one paper trade ended. PnLUSD is after slippage: fills are scored at the price
// we could really have had, never at the called price. On a close line PnLUSD is the whole trade,
// earlier fills included; on a fill line it is that rung alone.
type Result struct {
	PnLUSD float64 `json:"pnl_usd"`
	PnLPct float64 `json:"pnl_pct"`
	Reason string  `json:"reason,omitempty"` // stop, target, time, time stop, source exit
	Qty    int     `json:"qty,omitempty"`    // units sold on this line
	Price  float64 `json:"price,omitempty"`  // what they sold at
}

// GoLive is the bar paper trading must clear before real money. It is set before the results are in.
type GoLive struct {
	MinTrades       int     `json:"min_trades"`
	MinTradingDays  int     `json:"min_trading_days"`
	MaxDrawdownPct  float64 `json:"max_drawdown_pct"`
	PaperCapitalUSD float64 `json:"paper_capital_usd"` // starting equity drawdown is measured against
}

// Score is one source's paper record. Trades is the sample size; a win rate without it means nothing.
type Score struct {
	SourceID       string   `json:"source_id"`
	Posts          int      `json:"posts"`    // signals seen, noise excluded
	Planned        int      `json:"planned"`  // entries that passed the gate
	Rejected       int      `json:"rejected"` // entries the gate refused
	Trades         int      `json:"trades"`   // closed paper trades
	Wins           int      `json:"wins"`
	WinRatePct     float64  `json:"win_rate_pct"`
	PnLUSD         float64  `json:"pnl_usd"`
	MaxDrawdownPct float64  `json:"max_drawdown_pct"`
	TradingDays    int      `json:"trading_days"`
	GoLive         bool     `json:"go_live"`
	Missing        []string `json:"missing"` // what still stands between this record and real money
}

type tally struct {
	Score
	equity, peak float64
	days         map[string]bool
}

// Scoreboard scores every source in the journal, plus one "all" row, best P&L first.
func Scoreboard(entries []Entry, gate GoLive) []Score {
	tallies := map[string]*tally{}
	get := func(id string) *tally {
		if tallies[id] == nil {
			tallies[id] = &tally{Score: Score{SourceID: id}, equity: gate.PaperCapitalUSD, peak: gate.PaperCapitalUSD, days: map[string]bool{}}
		}
		return tallies[id]
	}
	for _, e := range entries {
		for _, t := range []*tally{get(e.Signal.SourceID), get(ALL_SOURCES)} {
			t.add(e)
		}
	}

	scores := make([]Score, 0, len(tallies))
	for _, t := range tallies {
		t.TradingDays = len(t.days)
		if t.Trades > 0 {
			t.WinRatePct = float64(t.Wins) / float64(t.Trades) * PERCENT
		}
		t.Missing = gate.missing(t.Score)
		t.GoLive = len(t.Missing) == 0
		scores = append(scores, t.Score)
	}
	sort.Slice(scores, func(a, b int) bool {
		if (scores[a].SourceID == ALL_SOURCES) != (scores[b].SourceID == ALL_SOURCES) {
			return scores[a].SourceID == ALL_SOURCES
		}
		if scores[a].PnLUSD != scores[b].PnLUSD {
			return scores[a].PnLUSD > scores[b].PnLUSD
		}
		return scores[a].SourceID < scores[b].SourceID
	})
	return scores
}

func (t *tally) add(e Entry) {
	if e.Action == ACTION_CLOSE && e.Result != nil {
		t.Trades++
		if e.Result.PnLUSD > 0 {
			t.Wins++
		}
		t.PnLUSD += e.Result.PnLUSD
		t.equity += e.Result.PnLUSD
		t.peak = max(t.peak, t.equity)
		if t.peak > 0 {
			t.MaxDrawdownPct = max(t.MaxDrawdownPct, (t.peak-t.equity)/t.peak*PERCENT)
		}
		t.days[e.At.In(time.UTC).Format(DAY_LAYOUT)] = true
		return
	}
	if e.Signal.Kind == KIND_NOISE || e.PlanID != 0 {
		return // watcher lines repeat the plan's signal: not a new post
	}
	t.Posts++
	switch {
	case e.Plan != nil:
		t.Planned++
	case e.Rejected != "":
		t.Rejected++
	}
}

// missing lists every part of the bar a record has not cleared. Empty means go.
func (g GoLive) missing(s Score) []string {
	out := []string{}
	if s.Trades < g.MinTrades {
		out = append(out, fmt.Sprintf("%d of %d trades", s.Trades, g.MinTrades))
	}
	if s.TradingDays < g.MinTradingDays {
		out = append(out, fmt.Sprintf("%d of %d trading days", s.TradingDays, g.MinTradingDays))
	}
	if s.PnLUSD <= 0 {
		out = append(out, "not profitable after slippage")
	}
	if g.MaxDrawdownPct > 0 && s.MaxDrawdownPct > g.MaxDrawdownPct {
		out = append(out, fmt.Sprintf("drawdown %.0f%% is over %.0f%%", s.MaxDrawdownPct, g.MaxDrawdownPct))
	}
	return out
}
