package plays

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	CRYPTO_TIMEOUT       = 60 * time.Second
	CRYPTO_REQUEST_GAP   = 120 * time.Millisecond // between candle requests: well inside Coinbase's public limit
	CRYPTO_DEFAULT_EVERY = 5 * time.Minute
	HOUR_SECONDS         = int64(time.Hour / time.Second)

	WHY_OPEN_FULL    = "%d coin plays are running, the most the rulebook allows"
	WHY_DAY_FULL     = "%d coin plays opened today, the most the rulebook allows"
	WHY_DAY_LOSS     = "coin plays closed today lost $%.2f, past the pause at %.1f%% of the account ($%.2f)"
	WHY_HELD         = "a play on it is already running"
	WHY_COOLDOWN     = "it closed at a loss %s ago: waiting %.0f hours"
	WHY_STALE        = "its newest completed candle is from %s: the feed is behind"
	WHY_SAME_SIGNAL  = "this breakout already has a play"
	WHY_NEXT_IN_LINE = "one play a pass: it is next in line"
	WHY_NO_BOOK      = "no two-sided market"
	WHY_WIDE_SPREAD  = "spread %.2f%% of the mid, max %.2f%%"
	WHY_CHASED       = "the ask %g is %.1f%% over the breakout close %g, max %.1f%%: it already ran"
	WHY_BLOCKED      = "blocked by "
	WHY_NO_FEED      = "no coin's candles could be read: %s"
	STALE_LAYOUT     = "Jan 2 15:04 MST"
)

// cryptoBook is what the desk's limits read: which coins have a play running, what today has done,
// and each coin's last losing close. The live desk builds it from the plays on file; the replay
// keeps its own as it goes.
type cryptoBook struct {
	open   map[string]bool
	today  int     // plays the desk opened today, New York time
	dayPnL float64 // every coin play closed today, in dollars
	lostAt map[string]time.Time
}

// shut says why the desk may not open anything right now, with the status word for it.
func (f CryptoRules) shut(b cryptoBook, equity float64) (state, why string) {
	pause := equity * f.MaxDayLossPct / PERCENT
	switch {
	case f.MaxOpen > 0 && len(b.open) >= f.MaxOpen:
		return FINDER_FULL, fmt.Sprintf(WHY_OPEN_FULL, len(b.open))
	case f.MaxPerDay > 0 && b.today >= f.MaxPerDay:
		return FINDER_FULL, fmt.Sprintf(WHY_DAY_FULL, b.today)
	case pause > 0 && b.dayPnL <= -pause:
		return FINDER_PAUSED, fmt.Sprintf(WHY_DAY_LOSS, -b.dayPnL, f.MaxDayLossPct, pause)
	}
	return FINDER_LOOKING, ""
}

// barred says why one coin may not be entered right now, or nothing when it may.
func (f CryptoRules) barred(b cryptoBook, symbol string, now time.Time) string {
	if b.open[symbol] {
		return WHY_HELD
	}
	wait := time.Duration(f.CooldownHours * float64(time.Hour))
	if lost, ok := b.lostAt[symbol]; ok && now.Sub(lost) < wait {
		return fmt.Sprintf(WHY_COOLDOWN, now.Sub(lost).Round(time.Minute), f.CooldownHours)
	}
	return ""
}

// CryptoNote is what the desk decided about one coin on its last pass.
type CryptoNote struct {
	Symbol  string  `json:"symbol"`
	Outcome string  `json:"outcome"` // opened, drafted or skipped
	Why     string  `json:"why,omitempty"`
	Score   float64 `json:"score,omitempty"` // set when the coin's last hour earned a play
	PlayID  string  `json:"play_id,omitempty"`
}

// CryptoStatus is the desk's last pass, for the app: nothing it does is hidden.
type CryptoStatus struct {
	BytesOnDisk int64        `json:"bytes_on_disk"`
	At          time.Time    `json:"at"`
	State       string       `json:"state"` // looking, off, full (the open or daily count), paused (the day's loss) or error
	Why         string       `json:"why,omitempty"`
	Open        int          `json:"open"`    // coin plays running now
	Today       int          `json:"today"`   // plays it opened today
	DayPnL      float64      `json:"day_pnl"` // every coin play closed today, in dollars
	Funds       Funds        `json:"funds"`   // the account the pass sized against
	FeePct      float64      `json:"fee_pct"` // the fee assumed on each side, already in every price
	Notes       []CryptoNote `json:"notes"`   // one per coin, in the rulebook's order
	NextAt      time.Time    `json:"next_at"`
	EveryMs     int64        `json:"every_ms"`
}

// hourCandles is one coin's completed hourly candles, good until the hour turns.
type hourCandles struct {
	hour int64
	rows []coinbase.Candle
}

// cryptoDesk holds the last pass for the API, and each coin's candles for the hour they were read in.
type cryptoDesk struct {
	mu      sync.Mutex
	last    CryptoStatus
	candles map[string]hourCandles
}

// CryptoStatus returns the crypto desk's last pass.
func (s *Service) CryptoStatus() CryptoStatus {
	s.crypto.mu.Lock()
	status := s.crypto.last
	s.crypto.mu.Unlock()
	status.BytesOnDisk = s.Store.bytesOnDisk()
	return status
}

// coinCandles returns a coin's completed hourly candles. Completed candles only change when the hour
// turns, so they are read once an hour per coin; a read that came back without the hour just ended
// is not kept, and the next pass asks again.
func (s *Service) coinCandles(ctx context.Context, symbol string, now time.Time) ([]coinbase.Candle, error) {
	hour := now.Truncate(time.Hour).Unix()
	s.crypto.mu.Lock()
	kept, ok := s.crypto.candles[symbol]
	s.crypto.mu.Unlock()
	if ok && kept.hour == hour {
		return kept.rows, nil
	}
	rows, err := coinbase.Candles(ctx, s.HTTP, symbol, coinbase.GRANULARITY_HOUR, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	rows = completed(rows, now)
	if len(rows) == 0 {
		return nil, errNoCandles
	}
	if rows[len(rows)-1].T == hour-HOUR_SECONDS {
		s.crypto.mu.Lock()
		if s.crypto.candles == nil {
			s.crypto.candles = map[string]hourCandles{}
		}
		s.crypto.candles[symbol] = hourCandles{hour: hour, rows: rows}
		s.crypto.mu.Unlock()
	}
	select {
	case <-ctx.Done():
	case <-time.After(CRYPTO_REQUEST_GAP):
	}
	return rows, nil
}

// bookFrom reads the desk's limits from the plays on file, and when each coin last got a play.
func bookFrom(all []Play, now time.Time) (cryptoBook, map[string]time.Time) {
	b := cryptoBook{open: map[string]bool{}, lostAt: map[string]time.Time{}}
	newest := map[string]time.Time{}
	day := now.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT)
	for _, p := range all {
		if p.Contract.Market != MARKET_CRYPTO {
			continue
		}
		symbol := p.Contract.Symbol
		if p.CreatedAt.After(newest[symbol]) {
			newest[symbol] = p.CreatedAt
		}
		if p.Status == STATUS_ARMED {
			b.open[symbol] = true
		}
		if p.FoundBy == FOUND_BY_CRYPTO && p.CreatedAt.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			b.today++
		}
		if p.Result == nil {
			continue
		}
		if p.Result.At.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT) == day {
			b.dayPnL += p.Result.PnLUSD
		}
		if p.Result.PnLUSD < 0 && p.Result.At.After(b.lostAt[symbol]) {
			b.lostAt[symbol] = p.Result.At
		}
	}
	return b, newest
}

// openCrypto checks the live book for one setup, then drafts the play and trades it when no
// blocking rule fails. A play nobody looked at never trades through a block: it waits as a draft.
// With no play made, the outcome is skipped and why says what stopped it.
func (s *Service) openCrypto(ctx context.Context, setup Setup, candles []coinbase.Candle, rules Rules, funds Funds, now time.Time) (p Play, outcome, why string) {
	f := *rules.Crypto
	tick, err := coinbase.Fetch(ctx, s.HTTP, setup.Symbol)
	if err != nil {
		return Play{}, FINDER_SKIPPED, err.Error()
	}
	if tick.Bid <= 0 || tick.Ask < tick.Bid {
		return Play{}, FINDER_SKIPPED, WHY_NO_BOOK
	}
	spread := (tick.Ask - tick.Bid) / ((tick.Ask + tick.Bid) / 2) * PERCENT
	chase := (tick.Ask/setup.Close - 1) * PERCENT
	switch {
	case f.MaxSpreadPct > 0 && spread > f.MaxSpreadPct:
		return Play{}, FINDER_SKIPPED, fmt.Sprintf(WHY_WIDE_SPREAD, spread, f.MaxSpreadPct)
	case f.MaxChasePct > 0 && chase > f.MaxChasePct:
		return Play{}, FINDER_SKIPPED, fmt.Sprintf(WHY_CHASED, coinPrice(tick.Ask), chase, coinPrice(setup.Close), f.MaxChasePct)
	}
	bracket := f.bracket(tick.Ask, setup.ATR, coinbase.TAKER_FEE_PCT)
	qty, small := f.size(bracket, funds, now)
	if small != "" {
		return Play{}, FINDER_SKIPPED, small
	}
	p = f.draft(setup, coinMarket(candles, tick, now), bracket, qty, coinbase.TAKER_FEE_PCT, rules.Template, now)
	if err := s.Store.Save(p); err != nil {
		return Play{}, FINDER_SKIPPED, err.Error()
	}
	blocked := ""
	for _, check := range Build(p, rules, Book{Others: s.Store.Plays(), Now: now}).Checks {
		if check.Status == CHECK_FAIL {
			blocked += ", " + check.ID
		}
	}
	if blocked != "" {
		return p, FINDER_DRAFTED, WHY_BLOCKED + blocked[len(", "):]
	}
	armed, err := s.paper(p, now)
	if err != nil {
		return p, FINDER_DRAFTED, err.Error()
	}
	return armed, FINDER_OPENED, ""
}

// scanCrypto runs one pass and returns what it decided about every coin. The best setup that may be
// entered gets a play: one play a pass, so a busy hour cannot spend the day's count at once.
func (s *Service) scanCrypto(ctx context.Context, now time.Time) CryptoStatus {
	s.sync() // plays the exit watcher closed since the last pass free their slots and count in the day's result
	rules := s.Store.Rules()
	f := rules.Crypto
	every := CRYPTO_DEFAULT_EVERY
	if f != nil && f.EveryMinutes > 0 {
		every = time.Duration(f.EveryMinutes) * time.Minute
	}
	st := CryptoStatus{At: now.UTC(), State: FINDER_LOOKING, Notes: []CryptoNote{}, FeePct: coinbase.TAKER_FEE_PCT,
		NextAt: now.Add(every).UTC(), EveryMs: every.Milliseconds()}
	if f == nil || !f.On {
		st.State = FINDER_OFF
		return st
	}
	st.Funds = Funds{EquityUSD: rules.BankrollUSD, BuyingPowerUSD: rules.BankrollUSD}
	if s.Funds != nil {
		st.Funds = s.Funds(now)
	}
	book, newest := bookFrom(s.Store.Plays(), now)
	st.Open, st.Today, st.DayPnL = len(book.open), book.today, book.dayPnL
	if st.State, st.Why = f.shut(book, st.Funds.EquityUSD); st.State != FINDER_LOOKING {
		return st
	}

	// Bitcoin's candles are read first: with the market filter on, every other coin's setup needs them.
	btc, btcErr := s.coinCandles(ctx, CRYPTO_BTC, now)
	bySymbol, setups, read, lastErr := map[string][]coinbase.Candle{}, []Setup{}, 0, btcErr
	for _, symbol := range f.Symbols {
		note := CryptoNote{Symbol: symbol, Outcome: FINDER_SKIPPED}
		candles, err := btc, btcErr
		if symbol != CRYPTO_BTC {
			candles, err = s.coinCandles(ctx, symbol, now)
		}
		if err != nil {
			note.Why, lastErr = err.Error(), err
			st.Notes = append(st.Notes, note)
			continue
		}
		read++
		setup, why := cryptoSetup(symbol, candles, btc, *f)
		closedAt := time.Unix(candles[len(candles)-1].T+HOUR_SECONDS, 0)
		barred := f.barred(book, symbol, now)
		switch {
		case now.Sub(closedAt) > CRYPTO_STALE_HOURS*time.Hour:
			why = fmt.Sprintf(WHY_STALE, closedAt.UTC().Format(STALE_LAYOUT))
		case why != "":
		case barred != "":
			why = barred
		case !newest[symbol].Before(closedAt):
			why = WHY_SAME_SIGNAL
		}
		note.Why, note.Score = why, setup.Score
		if why == "" {
			setups, bySymbol[symbol] = append(setups, setup), candles
		}
		st.Notes = append(st.Notes, note)
	}
	if read == 0 && lastErr != nil {
		st.State, st.Why = FINDER_ERROR, fmt.Sprintf(WHY_NO_FEED, lastErr)
		return st
	}

	sort.SliceStable(setups, func(a, b int) bool { return setups[a].Score > setups[b].Score })
	done := false
	for _, setup := range setups {
		note := noteFor(st.Notes, setup.Symbol)
		if done {
			note.Why = WHY_NEXT_IN_LINE
			continue
		}
		p, outcome, why := s.openCrypto(ctx, setup, bySymbol[setup.Symbol], rules, st.Funds, now)
		note.Outcome, note.Why, note.PlayID = outcome, why, p.ID
		done = outcome != FINDER_SKIPPED
		if done {
			log.Printf("crypto desk: %s %s %s (score %.1f, %g coins at %g, stop %g)", setup.Symbol, outcome, why, setup.Score, p.Qty, p.Entry, p.Stop)
		}
	}
	return st
}

// noteFor is the pass's note on one coin, to finish once its setup was acted on.
func noteFor(notes []CryptoNote, symbol string) *CryptoNote {
	for i := range notes {
		if notes[i].Symbol == symbol {
			return &notes[i]
		}
	}
	return &CryptoNote{}
}

// RunCrypto looks for coin plays around the clock until ctx ends: once at the start, then as often
// as the rulebook says. Coins never close, so unlike the stock finder it has no session to wait for.
func (s *Service) RunCrypto(ctx context.Context) {
	for {
		pass, cancel := context.WithTimeout(ctx, CRYPTO_TIMEOUT)
		st := s.scanCrypto(pass, time.Now())
		cancel()
		if st.State == FINDER_ERROR && ctx.Err() == nil {
			log.Printf("crypto desk: %s", st.Why)
		}
		s.crypto.mu.Lock()
		s.crypto.last = st
		s.crypto.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(st.EveryMs) * time.Millisecond):
		}
	}
}
