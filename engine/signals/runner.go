package signals

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	SEEN_POSTS_MAX      = 10000 // post ids remembered for dedupe
	POOL_SIDES          = 2     // a pool holds half its value in the token and half in what it trades against
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
	ErrThinPool     = errors.New("pool liquidity below min_liquidity_usd")
)

// Quote is the live market for one contract or symbol.
type Quote struct {
	Bid          float64 `json:"bid"`
	Ask          float64 `json:"ask"`
	OpenInterest int     `json:"open_interest"`
	Volume       int     `json:"volume"`
	// On-chain pools only. A pool has no order book: the price moves against every trade by the
	// trade's share of the pool, and the venue takes a fee. Zero liquidity means the size is unknown.
	LiquidityUSD float64 `json:"liquidity_usd,omitempty"`
	FeePct       float64 `json:"fee_pct,omitempty"`
}

// Sized returns the quote a trade of usd dollars would really get. Order-book quotes come back
// unchanged. In a pool, half the liquidity is on each side, so a trade moves the price by about
// its size over that half; the fee comes on top, both when buying and when selling.
func (q Quote) Sized(usd float64) Quote {
	slip := q.FeePct / PERCENT
	if q.LiquidityUSD > 0 {
		slip += POOL_SIDES * usd / q.LiquidityUSD
	}
	q.Bid, q.Ask = max(0, q.Bid*(1-slip)), q.Ask*(1+slip)
	return q
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
	if s.MinLiquidityUSD > 0 && q.LiquidityUSD < s.MinLiquidityUSD {
		return fmt.Errorf("%w: $%.0f", ErrThinPool, q.LiquidityUSD)
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
	order []string            // seen keys, oldest first, so the set stays bounded
	open  map[string]bool     // source + contract with a planned entry and no exit yet
	held  map[int64]*position // planned entries the watcher has not closed, by journal id
}

// NewRunner replays the journal so a restart keeps its dedupe set and open positions, and gives
// the journal's cash account the catalog's starting capital.
func NewRunner(c *Catalog, j *Journal, quote QuoteFunc) *Runner {
	r := &Runner{catalog: c, journal: j, quote: quote, seen: map[string]bool{}, open: map[string]bool{}, held: map[int64]*position{}}
	j.mu.Lock()
	j.ledger.capital = c.GoLive.PaperCapitalUSD
	j.mu.Unlock()
	for _, e := range j.Recent(0, JOURNAL_RECENT) {
		r.remember(e.Signal)
		r.track(e)
		r.hold(e)
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
	return fmt.Sprintf("%s|%s|%s|%v|%s|%s", strings.ToLower(sig.SourceID), sig.Symbol, sig.Right, sig.Strike, sig.Expiry, sig.Address)
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

	if r.catalog.IsCrypto(sig) {
		sig.Market = MARKET_CRYPTO
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
		plan, err := r.plan(planSig, settings, asOf, now, &e)
		if err != nil {
			e.Rejected = err.Error()
		} else {
			e.Plan = &plan
		}
	}
	r.track(e)
	e, err := r.journal.Append(e)
	r.hold(e)
	return e, true, err
}

// plan checks liquidity when a quote exists, then builds the bracket at the ask (what a buy pays).
// The cash account caps the size: an entry spends settled cash only, and is refused when that
// cannot buy one unit.
func (r *Runner) plan(sig Signal, s Settings, asOf, now time.Time, e *Entry) (Plan, error) {
	power, limited := r.cash(now)
	if limited && power < MIN_ORDER_USD {
		return Plan{}, fmt.Errorf(CASH_SHORT, ErrNoCash, power, MIN_ORDER_USD)
	}
	short := limited && power < s.MaxUSDPerTrade
	if short {
		s.MaxUSDPerTrade = power
	}
	price := 0.0
	q, ok := Quote{}, false
	if r.quote != nil {
		q, ok = r.quote(sig)
	}
	if !ok {
		e.Note = strings.TrimPrefix(e.Note+"; "+NOTE_NO_QUOTE, "; ")
	} else {
		if sig.Right == "" {
			s.MinOpenInterest = 0 // stocks have no open interest to check
		}
		if sig.Address == "" {
			s.MinLiquidityUSD = 0 // only on-chain tokens trade in a pool
		}
		q = q.Sized(s.MaxUSDPerTrade)
		if err := CheckLiquidity(q, s); err != nil {
			return Plan{}, err
		}
		price = q.Ask
	}
	paid := cmp.Or(price, sig.Price)
	p, err := Build(sig, s, price, asOf)
	if err == nil && limited && p.outlay() > power+CASH_TOLERANCE_USD {
		// The entry rounds to a tick, which can carry the cost past the cash: size again at the rounded price.
		short, s.MaxUSDPerTrade = true, power*paid/p.Entry
		p, err = Build(sig, s, price, asOf)
	}
	switch {
	case errors.Is(err, ErrBudgetTooSmall) && short:
		unit := paid
		if sig.Right != "" {
			unit *= OPTION_MULTIPLIER
		}
		return Plan{}, fmt.Errorf(CASH_SHORT, ErrNoCash, power, unit)
	case err != nil:
		return Plan{}, err
	case limited && p.outlay() > power+CASH_TOLERANCE_USD:
		return Plan{}, fmt.Errorf(CASH_SHORT, ErrNoCash, power, p.outlay())
	case short:
		e.Note = strings.TrimPrefix(e.Note+"; "+fmt.Sprintf(NOTE_CASH_SIZED, power), "; ")
	}
	return p, nil
}

// Adopt journals a bracket planned somewhere else (a play) as an entry, so the exit watcher
// prices and closes it like any other. It returns the journal id the watcher's lines will carry.
// A bracket that costs more than the settled cash is refused, and nothing is journaled.
func (r *Runner) Adopt(sig Signal, plan Plan, now time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if power, limited := r.cash(now); limited && plan.outlay() > power+CASH_TOLERANCE_USD {
		return 0, fmt.Errorf(CASH_SHORT, ErrNoCash, power, plan.outlay())
	}
	r.remember(sig)
	e := Entry{At: now.UTC(), Action: string(KIND_ENTRY), Signal: sig, Plan: &plan}
	r.track(e)
	e, err := r.journal.Append(e)
	if err != nil {
		return 0, err
	}
	r.hold(e)
	return e.ID, nil
}
