// Package scoreboard summarizes paper evidence. Its verdicts have no trading side effects.
package scoreboard

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"time"
)

const (
	ROUTE            = "GET /api/scoreboard/all"
	MIN_RESULTS      = 100
	MIN_DAYS         = 30
	DAY              = 24 * time.Hour
	RECENT_WINDOW    = 30 * DAY
	DECAY_MARGIN     = 0.05  // $0.05 per dollar invested, or 0.05 R per call
	DECAY_EPSILON    = 1e-12 // absorb roundoff when a recent mean lands on the permitted margin
	Z_95             = 1.96
	NOT_ENOUGH       = "not enough data"
	LOSING           = "losing"
	PROMISING        = "promising"
	PASSES           = "passes"
	RETURN_ON_COST   = "return_on_cost"
	RETURN_R         = "r"
	UNKNOWN          = "unknown"
	STATE_REMOVED    = "removed"
	STATE_INACTIVE   = "inactive"
	BASIS_PAPER      = "paper closes"
	HEADER_TYPE      = "Content-Type"
	HEADER_CACHE     = "Cache-Control"
	JSON_CONTENT     = "application/json"
	CACHE_CONTROL    = "no-store"
	REASON_RESULTS   = "%d more settled result%s"
	REASON_DAYS      = "%d more day%s of result history"
	PLURAL           = "s"
	REASON_BOTH      = "%s · %s"
	REASON_LOSING    = "Mean return is zero or negative. Improve returns before retesting."
	REASON_UNCERTAIN = "Positive mean, but the 95% lower bound has not cleared zero."
	REASON_NO_RECENT = "Record a result in the last 30 days to check for decay."
	REASON_DECAY     = "Recent mean trails the full record by more than 0.05. Retest current performance."
	REASON_PASSES    = "Clears the sample, positive lower bound and recent performance checks."
)

// Result is one completed observation, with Return in dollars per dollar or multiples of initial risk.
type Result struct {
	At     time.Time
	Return float64
}

// Strategy is a read-only desk snapshot. A blocker marks evidence that cannot support a pass.
type Strategy struct {
	Desk, ID, Name, Risk, State string
	Basis, Unit, Blocker        string
	Results                     []Result
	BreakEvenRate               *float64 // fraction, when the desk has a known break-even rate
	RealizedPnL                 *float64 // paper dollars, never hypothetical benchmark gains
}

// Row is the public summary. A null metric means the desk cannot report it.
type Row struct {
	Desk          string    `json:"desk"`
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Risk          string    `json:"risk"`
	State         string    `json:"state"`
	Basis         string    `json:"basis"`
	Unit          string    `json:"unit"`
	Settled       int       `json:"settled"`
	WinRate       *float64  `json:"win_rate"`
	BreakEvenRate *float64  `json:"break_even_rate"`
	Mean          *float64  `json:"mean_return"`
	Lower95       *float64  `json:"lower_95"`
	Upper95       *float64  `json:"upper_95"`
	RecentMean    *float64  `json:"recent_mean"`
	RecentSettled int       `json:"recent_settled"`
	RealizedPnL   *float64  `json:"realized_pnl_usd"`
	First         time.Time `json:"first_at,omitzero"`
	Latest        time.Time `json:"latest_at,omitzero"`
	HistoryDays   int       `json:"history_days"`
	Verdict       string    `json:"verdict"`
	Reason        string    `json:"reason"`
}

// Collect scores each supplied desk, including strategies with no results.
func Collect(now time.Time, desks ...func() []Strategy) []Row {
	rows := []Row{}
	for _, read := range desks {
		if read == nil {
			continue
		}
		for _, s := range read() {
			rows = append(rows, evaluate(s, now))
		}
	}
	slices.SortFunc(rows, func(a, b Row) int {
		return cmp.Or(cmp.Compare(a.Desk, b.Desk), cmp.Compare(a.ID, b.ID))
	})
	return rows
}

// Handler delivers a snapshot through the runner's existing read API middleware.
func Handler(desks ...func() []Strategy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(HEADER_TYPE, JSON_CONTENT)
		w.Header().Set(HEADER_CACHE, CACHE_CONTROL)
		json.NewEncoder(w).Encode(Collect(time.Now(), desks...))
	})
}

func evaluate(s Strategy, now time.Time) Row {
	row := Row{Desk: s.Desk, ID: s.ID, Name: cmp.Or(s.Name, s.ID), Risk: cmp.Or(s.Risk, UNKNOWN),
		State: cmp.Or(s.State, UNKNOWN), Basis: s.Basis, Unit: cmp.Or(s.Unit, RETURN_ON_COST),
		BreakEvenRate: s.BreakEvenRate, RealizedPnL: s.RealizedPnL}
	// Sort our own copy so online variance is reproducible even for desk snapshots built from maps.
	results := slices.Clone(s.Results)
	slices.SortFunc(results, func(a, b Result) int {
		return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.Return, b.Return))
	})
	cutoff := now.Add(-RECENT_WINDOW)
	var mean, m2, recent float64
	wins := 0
	for _, r := range results {
		if r.At.IsZero() || r.At.After(now) || math.IsNaN(r.Return) || math.IsInf(r.Return, 0) {
			continue
		}
		if row.Settled == 0 {
			row.First = r.At.UTC()
		}
		row.Latest = r.At.UTC()
		row.Settled++
		delta := r.Return - mean
		mean += delta / float64(row.Settled)
		m2 += delta * (r.Return - mean)
		if r.Return > 0 {
			wins++
		}
		if !r.At.Before(cutoff) {
			row.RecentSettled++
			recent += (r.Return - recent) / float64(row.RecentSettled)
		}
	}
	if row.Settled > 0 {
		row.HistoryDays = int(row.Latest.Sub(row.First) / DAY)
		winRate := float64(wins) / float64(row.Settled)
		row.Mean, row.WinRate = &mean, &winRate
	}
	if row.Settled > 1 {
		// Normal approximation for the mean of per-result returns: mean ± 1.96 ×
		// sample SD / √n. It assumes independent observations and can be optimistic
		// for correlated bets. No dollars or percentage points are pooled across desks.
		n := float64(row.Settled)
		half := Z_95 * math.Sqrt(max(0, m2)/(n-1)/n)
		lower, upper := mean-half, mean+half
		row.Lower95, row.Upper95 = &lower, &upper
	}
	if row.RecentSettled > 0 {
		row.RecentMean = &recent
	}
	row.Verdict, row.Reason = verdict(row, s.Blocker)
	return row
}

func verdict(row Row, blocker string) (string, string) {
	results, days := MIN_RESULTS-row.Settled, MIN_DAYS-row.HistoryDays
	resultSuffix, daySuffix := PLURAL, PLURAL
	if results == 1 {
		resultSuffix = ""
	}
	if days == 1 {
		daySuffix = ""
	}
	switch {
	case results > 0 && days > 0:
		return NOT_ENOUGH, fmt.Sprintf(REASON_BOTH, fmt.Sprintf(REASON_RESULTS, results, resultSuffix), fmt.Sprintf(REASON_DAYS, days, daySuffix))
	case results > 0:
		return NOT_ENOUGH, fmt.Sprintf(REASON_RESULTS, results, resultSuffix)
	case days > 0:
		return NOT_ENOUGH, fmt.Sprintf(REASON_DAYS, days, daySuffix)
	case *row.Mean <= 0:
		return LOSING, REASON_LOSING
	case blocker != "":
		return PROMISING, blocker
	case *row.Lower95 <= 0:
		return PROMISING, REASON_UNCERTAIN
	case row.RecentMean == nil:
		return PROMISING, REASON_NO_RECENT
	case *row.Mean-*row.RecentMean > DECAY_MARGIN+DECAY_EPSILON:
		return PROMISING, REASON_DECAY
	default:
		return PASSES, REASON_PASSES
	}
}
