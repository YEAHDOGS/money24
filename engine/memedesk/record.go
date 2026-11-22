package memedesk

import (
	"maps"
	"slices"
	"time"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	DEFAULT_GRADUATION_VENUE = "pump-amm"
	PUMPSWAP_VENUE           = "pumpswap"
)

type PricePath struct {
	Samples      int       `json:"samples"`
	MissingPrice int       `json:"missing_price"`
	EmptyMarket  int       `json:"empty_market"`
	FirstUSD     float64   `json:"first_usd,omitempty"`
	LastUSD      float64   `json:"last_usd,omitempty"`
	HighUSD      float64   `json:"high_usd,omitempty"`
	LowUSD       float64   `json:"low_usd,omitempty"`
	LastAt       time.Time `json:"last_at,omitzero"`
}

func (p *PricePath) observe(row meme.Record, at time.Time) {
	p.Samples++
	if row.Market == nil {
		p.EmptyMarket++
	}
	value := price(row.Market)
	if value == 0 {
		p.MissingPrice++
		return
	}
	if p.FirstUSD == 0 {
		p.FirstUSD, p.LowUSD = value, value
	}
	p.LastUSD, p.LastAt = value, at
	p.HighUSD, p.LowUSD = max(p.HighUSD, value), min(p.LowUSD, value)
}

type Candidate struct {
	Mint       string    `json:"mint"`
	Event      string    `json:"event"`
	Signature  string    `json:"signature,omitempty"`
	Tracking   string    `json:"tracking,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	DecidedAt  time.Time `json:"decided_at,omitzero"`
	Decision   Decision  `json:"decision"`
	Path       PricePath `json:"price_path"`
	SpikeAt    time.Time `json:"spike_at,omitzero"`
	SpikeUSD   float64   `json:"spike_usd,omitempty"`
	Benchmark  *Position `json:"long_benchmark,omitempty"`
	Paper      *Position `json:"paper_position,omitempty"`
	event      meme.Record
	anchor     float64
	pool       string
}

type Record struct {
	Candidates           int            `json:"candidates"`
	Launches             int            `json:"launches"`
	Graduations          int            `json:"graduations"`
	Pending              int            `json:"pending"`
	Following            int            `json:"following"`
	Passed               int            `json:"passed"`
	Refused              int            `json:"refused"`
	Faded                int            `json:"faded"`
	Reasons              map[string]int `json:"reasons"`
	BenchmarkUnavailable int            `json:"benchmark_unavailable"`
	BenchmarkStarted     int            `json:"benchmark_started"`
	BenchmarkClosed      int            `json:"benchmark_closed"`
	BenchmarkCensored    int            `json:"benchmark_censored"`
	BenchmarkPnLUSD      float64        `json:"benchmark_closed_pnl_usd"`
	PassedBenchmarkUSD   float64        `json:"passed_benchmark_closed_pnl_usd"`
	LossesAvoided        int            `json:"losses_avoided"`
	LossesAvoidedUSD     float64        `json:"losses_avoided_usd"`
	WinnersMissed        int            `json:"winners_missed"`
	WinnersMissedUSD     float64        `json:"winners_missed_usd"`
	PaperEntries         int            `json:"paper_entries"`
	PaperClosed          int            `json:"paper_closed"`
	PaperWins            int            `json:"paper_wins"`
	PaperPnLUSD          float64        `json:"paper_closed_pnl_usd"`
}

type strategyState struct {
	config     Config
	configured bool
	canBuy     bool
	record     Record
	open       map[string]*candidate
	recent     []*candidate
	closed     []*candidate
	entries    []time.Time
	stakeDay   string
	stakeUSD   float64
}

func (s *strategyState) add(c *candidate) {
	s.record.Candidates++
	s.record.Pending++
	s.record.Following++
	if c.Event == meme.KIND_LAUNCH {
		s.record.Launches++
	} else {
		s.record.Graduations++
	}
	s.recent = appendRecent(s.recent, c)
}

func appendRecent(items []*candidate, c *candidate) []*candidate {
	if len(items) == RECENT_LIMIT {
		copy(items, items[1:])
		items = items[:len(items)-1]
	}
	return append(items, c)
}

func (c *candidate) paperOpen() bool {
	return c.Paper != nil && c.Paper.ClosedAt.IsZero()
}

func (s *strategyState) decided(c *candidate, d Decision, at time.Time) {
	c.DecidedAt, c.Decision = at, d
	s.record.Pending--
	switch d.Action {
	case DECISION_BUY, DECISION_PASS:
		s.record.Passed++
	case DECISION_FADE:
		s.record.Faded++
	default:
		s.record.Refused++
	}
	for _, reason := range d.Reasons {
		s.record.Reasons[reason]++
	}
	if c.Benchmark != nil {
		s.record.BenchmarkStarted++
	} else if d.Benchmark {
		s.record.BenchmarkUnavailable++
	}
}

func (s *strategyState) stake(day string, fill Fill) {
	if s.stakeDay != day {
		s.stakeDay, s.stakeUSD = day, 0
	}
	s.stakeUSD += fill.NetUSD
	s.entries = append(s.entries, fill.At)
	s.record.PaperEntries++
}

func (s *strategyState) pruneEntries(at time.Time) {
	cutoff := at.Add(-time.Hour)
	first := 0
	for first < len(s.entries) && !s.entries[first].After(cutoff) {
		first++
	}
	s.entries = s.entries[first:]
}

func (s *strategyState) staked(day string) float64 {
	if day != s.stakeDay {
		return 0
	}
	return s.stakeUSD
}

func (s *strategyState) settleBenchmark(c *candidate) {
	r := &s.record
	pnl := c.Benchmark.PnLUSD
	r.BenchmarkClosed++
	r.BenchmarkPnLUSD += pnl
	if c.Decision.Action == DECISION_PASS {
		r.PassedBenchmarkUSD += pnl
	}
	if c.Decision.Action != DECISION_SKIP && c.Decision.Action != DECISION_FADE {
		return
	}
	if pnl < 0 {
		r.LossesAvoided++
		r.LossesAvoidedUSD -= pnl
	} else if pnl > 0 {
		r.WinnersMissed++
		r.WinnersMissedUSD += pnl
	}
}

func (s *strategyState) settlePaper(c *candidate) {
	delete(s.open, c.Mint)
	s.closed = appendRecent(s.closed, c)
	s.record.PaperClosed++
	s.record.PaperPnLUSD += c.Paper.PnLUSD
	if c.Paper.PnLUSD > 0 {
		s.record.PaperWins++
	}
}

type StrategyStatus struct {
	Config     Config      `json:"config"`
	Configured bool        `json:"configured"`
	CanBuy     bool        `json:"can_buy"`
	Record     Record      `json:"record"`
	Open       []Position  `json:"open_positions"`
	Recent     []Position  `json:"recent_positions"`
	Candidates []Candidate `json:"recent_candidates,omitempty"`
}

type Status struct {
	PaperOnly     bool             `json:"paper_only"`
	AsOf          time.Time        `json:"as_of,omitzero"`
	Day           string           `json:"day,omitempty"`
	CapitalUSD    float64          `json:"capital_usd"`
	CashUSD       float64          `json:"cash_usd"`
	OpenMarkUSD   float64          `json:"open_mark_usd"`
	StaleMarks    int              `json:"stale_marks"`
	DayPnLUSD     float64          `json:"day_realized_pnl_usd"`
	DailyLossStop bool             `json:"daily_loss_stop"`
	Strategies    []StrategyStatus `json:"strategies"`
	RulebookError string           `json:"rulebook_error,omitempty"`
	JournalError  string           `json:"journal_error,omitempty"`
}

func (e *engine) status(details bool) Status {
	out := Status{PaperOnly: true, AsOf: e.clock, Day: e.day, CapitalUSD: e.rulebook.CapitalUSD,
		CashUSD: e.rulebook.CapitalUSD + e.cashDelta, DayPnLUSD: e.dayRealized, DailyLossStop: e.stopped}
	for _, id := range slices.Sorted(maps.Keys(e.states)) {
		state := e.states[id]
		result := StrategyStatus{Config: state.config, Configured: state.configured, CanBuy: state.canBuy,
			Record: state.record, Open: []Position{}, Recent: []Position{}}
		result.Record.Reasons = maps.Clone(state.record.Reasons)
		for _, mint := range slices.Sorted(maps.Keys(state.open)) {
			p := state.open[mint].Paper
			result.Open = append(result.Open, copyPosition(p))
			out.OpenMarkUSD += p.MarkUSD
			if p.MarkStale {
				out.StaleMarks++
			}
		}
		if details {
			for _, c := range slices.Backward(state.closed) {
				result.Recent = append(result.Recent, copyPosition(c.Paper))
			}
			for _, c := range slices.Backward(state.recent) {
				value := c.Candidate
				if c.Benchmark != nil {
					p := copyPosition(c.Benchmark)
					value.Benchmark = &p
				}
				if c.Paper != nil {
					p := copyPosition(c.Paper)
					value.Paper = &p
				}
				result.Candidates = append(result.Candidates, value)
			}
		}
		out.Strategies = append(out.Strategies, result)
	}
	return out
}

func copyPosition(p *Position) Position {
	out := *p
	out.Sales = slices.Clone(p.Sales)
	return out
}
