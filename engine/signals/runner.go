package signals

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	SEEN_POSTS_MAX      = 10000 // post ids remembered for dedupe
	NOTE_ADD_AS_ENTRY   = "add with no open position: treated as entry (the source's cost is lower than ours)"
	NOTE_NO_QUOTE       = "no live quote: chase and liquidity unchecked"
	REJECT_DUPLICATE    = "duplicate post"
	REJECT_NOT_FOLLOWED = "add ignored: follow_source_add is off"
)

var (
	ErrWideSpread   = errors.New("spread wider than max_spread_pct")
	ErrThinInterest = errors.New("open interest below min_open_interest")
	ErrThinVolume   = errors.New("volume below min_volume")
	ErrNoMarket     = errors.New("no bid or no ask")
)

// Quote is the live market for one contract or symbol.
type Quote struct {
	Bid          float64 `json:"bid"`
	Ask          float64 `json:"ask"`
	OpenInterest int     `json:"open_interest"`
	Volume       int     `json:"volume"`
}

// QuoteFunc looks up the live market for a signal. ok is false when there is no quote source yet.
type QuoteFunc func(Signal) (q Quote, ok bool)

// CheckLiquidity rejects contracts we could not get out of cleanly: a caller's own followers
// can move a thin option, and a wide spread alone can trip the stop.
func CheckLiquidity(q Quote, s Settings) error {
	if q.Bid <= 0 || q.Ask <= 0 || q.Ask < q.Bid {
		return ErrNoMarket
	}
	spreadPct := (q.Ask - q.Bid) / ((q.Ask + q.Bid) / 2) * PERCENT
	if s.MaxSpreadPct > 0 && spreadPct > s.MaxSpreadPct {
		return fmt.Errorf("%w: %.0f%%", ErrWideSpread, spreadPct)
	}
	if s.MinOpenInterest > 0 && q.OpenInterest < s.MinOpenInterest {
		return fmt.Errorf("%w: %d", ErrThinInterest, q.OpenInterest)
	}
	if s.MinVolume > 0 && q.Volume < s.MinVolume {
		return fmt.Errorf("%w: %d", ErrThinVolume, q.Volume)
	}
	return nil
}

// Runner is the risk gate between parsed signals and the journal. It drops posts it has already
// seen, tracks which positions are open per source, and plans every entry with its exits.
type Runner struct {
	catalog *Catalog
	journal *Journal
	quote   QuoteFunc

	mu    sync.Mutex
	seen  map[string]bool
	order []string        // seen keys, oldest first, so the set stays bounded
	open  map[string]bool // source + contract with a planned entry and no exit yet
}

// NewRunner replays the journal so a restart keeps its dedupe set and open positions.
func NewRunner(c *Catalog, j *Journal, quote QuoteFunc) *Runner {
	r := &Runner{catalog: c, journal: j, quote: quote, seen: map[string]bool{}, open: map[string]bool{}}
	for _, e := range j.Recent(0, JOURNAL_RECENT) {
		r.remember(e.Signal)
		r.track(e)
	}
	return r
}

func postKey(sig Signal) string {
	if sig.PostID == "" {
		return ""
	}
	return strings.ToLower(sig.SourceID) + "|" + sig.PostID
}

func positionKey(sig Signal) string {
	return fmt.Sprintf("%s|%s|%s|%v|%s", strings.ToLower(sig.SourceID), sig.Symbol, sig.Right, sig.Strike, sig.Expiry)
}

// remember records a post id and reports whether it was new.
func (r *Runner) remember(sig Signal) bool {
	key := postKey(sig)
	if key == "" {
		return true
	}
	if r.seen[key] {
		return false
	}
	r.seen[key] = true
	r.order = append(r.order, key)
	if len(r.order) > SEEN_POSTS_MAX {
		delete(r.seen, r.order[0])
		r.order = r.order[1:]
	}
	return true
}

// track opens a position on a planned entry and closes it on the source's exit.
func (r *Runner) track(e Entry) {
	switch {
	case e.Plan != nil:
		r.open[positionKey(e.Signal)] = true
	case e.Signal.Kind == KIND_EXIT:
		// An exit that names only the symbol closes every contract we hold on it from that source.
		prefix := strings.ToLower(e.Signal.SourceID) + "|" + e.Signal.Symbol + "|"
		for key := range r.open {
			if key == positionKey(e.Signal) || e.Signal.Right == "" && strings.HasPrefix(key, prefix) {
				delete(r.open, key)
			}
		}
	}
}

// Handle gates one signal and journals the result. now is when we would place the order.
// It returns false for a duplicate post, which is dropped without a journal line.
func (r *Runner) Handle(sig Signal, now time.Time) (Entry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.remember(sig) {
		return Entry{Rejected: REJECT_DUPLICATE, Signal: sig}, false, nil
	}

	settings := r.catalog.SettingsFor(sig.SourceID)
	e := Entry{At: now.UTC(), Action: string(sig.Kind), Signal: sig}
	// A manual share is planned as of its post time, so the clock rules start when the source posted.
	asOf, planSig := now, sig
	if sig.Manual && !sig.PostedAt.IsZero() {
		asOf, planSig.SeenAt = sig.PostedAt, sig.PostedAt
		e.AsOf = asOf.UTC()
	}

	if sig.Kind == KIND_ADD {
		switch {
		case !r.open[positionKey(sig)]:
			planSig.Kind, e.Note = KIND_ENTRY, NOTE_ADD_AS_ENTRY
		case !settings.FollowSourceAdd:
			e.Rejected = REJECT_NOT_FOLLOWED
		}
	}
	if planSig.Kind == KIND_ENTRY {
		plan, err := r.plan(planSig, settings, asOf, &e)
		if err != nil {
			e.Rejected = err.Error()
		} else {
			e.Plan = &plan
		}
	}
	r.track(e)
	e, err := r.journal.Append(e)
	return e, true, err
}

// plan checks liquidity when a quote exists, then builds the bracket at the ask (what a buy pays).
func (r *Runner) plan(sig Signal, s Settings, asOf time.Time, e *Entry) (Plan, error) {
	price := 0.0
	q, ok := Quote{}, false
	if r.quote != nil {
		q, ok = r.quote(sig)
	}
	if !ok {
		e.Note = strings.TrimPrefix(e.Note+"; "+NOTE_NO_QUOTE, "; ")
	} else {
		if err := CheckLiquidity(q, s); err != nil {
			return Plan{}, err
		}
		price = q.Ask
	}
	return Build(sig, s, price, asOf)
}
