package plays

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/YEAHDOGS/money24/engine/alpaca"
	"github.com/YEAHDOGS/money24/engine/coinbase"
	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	// The replay walks old hourly candles with the desk's own rule, bracket, fee, size and limits, on one
	// cash account across every coin. It is how the rule is judged before it is trusted: a replay, not a promise.
	REPLAY_PAGE        = (coinbase.CANDLES_MAX - 1) * time.Hour // one request's span: both ends count, so 300 candles
	REPLAY_REQUEST_GAP = 150 * time.Millisecond                 // Coinbase's public limit is about ten requests a second
	REPLAY_RETRIES     = 4
	REPLAY_RETRY_WAIT  = 2 * time.Second
	REPLAY_LEAD        = CRYPTO_WINDOW * time.Hour // candles before the first day, so its first hour has its indicators
	REPLAY_TOTAL       = "full"
	REPLAY_EARLY       = "first-2/3"
	REPLAY_LATE        = "last-1/3"
	REPLAY_MARKET      = "market"
	REPLAY_LIMIT       = "limit"
	REPLAY_BOTH        = "both"
	REPLAY_LIMIT_PCT   = 0.10
	REPLAY_SHORT_DAYS  = 90
	REPLAY_SPLIT_PARTS = 3
	REPLAY_EARLY_PARTS = 2
	REASON_WINDOW      = "window close"
	REPLAY_HEAD        = "\nCrypto desk replay ending %s (exclusive) with $%.2f initial cash per segment.\n"
	REPLAY_METHOD      = "Alpaca tier 1: %.2f%% maker, %.2f%% taker. All exits are taker; no order book, spread or extra slippage.\nDrawdown uses hourly equity net of exit fees. Positions left at the boundary are sold at the last close.\n"
	REPLAY_COVERAGE    = "%-9s %s to %s: %d of %d hourly bars (including warm-up)\n"
	REPLAY_COLUMNS     = "%-16s %-6s %4s %-10s %6s %6s %8s %8s %11s %9s %9s\n"
	REPLAY_ROW         = "%-16s %-6s %4d %-10s %6d %6d %7.2f%% %+8.3f %+11.2f %+8.2f%% %8.2f%%\n"
)

var (
	errBacktestInput = errors.New("a replay needs at least one day, some cash and a coin to trade")
)

// CryptoReplayOptions controls the offline replay without changing the live desk.
type CryptoReplayOptions struct {
	Days    int
	End     time.Time // exclusive UTC hour; zero uses the most recent completed hour
	Entry   string
	Compare bool // all strategies and, when Days > 90, the trailing 90-day window
}

type replayExecution struct {
	entryFeePct float64
	exitFeePct  float64
	limit       bool
}

// coinTrade is one finished trade of the replay.
type coinTrade struct {
	Symbol  string
	EntryAt int64
	ExitAt  int64
	PnLUSD  float64
	R       float64 // the result in units of what the trade risked at its stop
	Reason  string
}

// coinPosition is one play the replay is holding.
type coinPosition struct {
	play       Play
	stops      []float64 // the stop in force after that many exits
	targets    []Rung
	next       int
	left       float64
	realized   float64
	riskUSD    float64
	enteredAt  int64
	sidewaysAt int64
}

// coinReplay is what a replay ended with.
type coinReplay struct {
	Trades         []coinTrade
	MaxDrawdownPct float64
	EndUSD         float64
}

// coinHistory pages a coin's hourly candles between two moments, oldest first, without repeats.
func coinHistory(ctx context.Context, client *http.Client, symbol string, from, to time.Time) ([]coinbase.Candle, error) {
	seen, out := map[int64]bool{}, []coinbase.Candle{}
	for end := to; end.After(from); end = end.Add(-REPLAY_PAGE) {
		start := end.Add(-REPLAY_PAGE)
		if start.Before(from) {
			start = from
		}
		var rows []coinbase.Candle
		var err error
		for try := 0; try < REPLAY_RETRIES; try++ {
			rows, err = coinbase.Candles(ctx, client, symbol, coinbase.GRANULARITY_HOUR, start, end)
			if !errors.Is(err, coinbase.ErrRateLimited) {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(REPLAY_RETRY_WAIT):
			}
		}
		if err != nil {
			return nil, err
		}
		for _, c := range rows {
			if c.T >= start.Unix() && c.T <= end.Unix() && !seen[c.T] {
				seen[c.T], out = true, append(out, c)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(REPLAY_REQUEST_GAP):
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].T < out[b].T })
	return out, nil
}

// replayCoins walks every coin's candles hour by hour from one moment to another as the live desk
// would have: the same setup, bracket, size and limits, one cash account. A setup on the hour that
// just closed is entered at the next hour's open. Pure arithmetic over the candles it is given.
func replayCoins(series map[string][]coinbase.Candle, rules Rules, from, to time.Time, capitalUSD float64, execution replayExecution) coinReplay {
	f := *rules.Crypto
	netEntry, netExit := 1-execution.entryFeePct/PERCENT, 1-execution.exitFeePct/PERCENT
	out := coinReplay{EndUSD: capitalUSD}
	cash, invested, peak := capitalUSD, 0.0, capitalUSD
	book := cryptoBook{open: map[string]bool{}, lostAt: map[string]time.Time{}}
	held, cursor, day := map[string]*coinPosition{}, map[string]int{}, ""
	lastClose := map[string]coinbase.Candle{}

	// sell books one sale: the cash comes back, and the account's closed result moves with it.
	sell := func(pos *coinPosition, qty, price float64) {
		gain := (price - pos.play.Entry) * qty
		pos.left, pos.realized = pos.left-qty, pos.realized+gain
		cash, invested, out.EndUSD = cash+qty*price, invested-qty*pos.play.Entry, out.EndUSD+gain
	}
	finish := func(pos *coinPosition, at int64, price float64, reason string) {
		symbol := pos.play.Contract.Symbol
		sell(pos, pos.left, price)
		out.Trades = append(out.Trades, coinTrade{Symbol: symbol, EntryAt: pos.enteredAt, ExitAt: at, PnLUSD: pos.realized, R: pos.realized / pos.riskUSD, Reason: reason})
		book.dayPnL += pos.realized
		if pos.realized < 0 {
			book.lostAt[symbol] = time.Unix(at, 0)
		}
		delete(held, symbol)
		delete(book.open, symbol)
	}

	for t := from.Unix(); t < to.Unix(); t += HOUR_SECONDS {
		now := time.Unix(t, 0)
		if today := now.In(MARKET_TZ).Format(market.EXPIRY_LAYOUT); today != day {
			day, book.today, book.dayPnL = today, 0, 0
		}
		for symbol, rows := range series {
			for cursor[symbol] < len(rows) && rows[cursor[symbol]].T < t {
				cursor[symbol]++
			}
		}
		// candleAt is the coin's candle for this hour, with the hours completed before it.
		candleAt := func(symbol string) (coinbase.Candle, []coinbase.Candle, bool) {
			rows, i := series[symbol], cursor[symbol]
			if i >= len(rows) || rows[i].T != t {
				return coinbase.Candle{}, nil, false
			}
			return rows[i], rows[:i], true
		}

		setups := []Setup{}
		for _, symbol := range f.Symbols {
			_, before, ok := candleAt(symbol)
			if !ok || len(before) == 0 || before[len(before)-1].T != t-HOUR_SECONDS {
				continue // no price to enter at, or the hour before it never traded
			}
			setup, why := cryptoSetup(symbol, before, series[CRYPTO_BTC][:cursor[CRYPTO_BTC]], f)
			if why == "" && f.barred(book, symbol, now) == "" {
				setups = append(setups, setup)
			}
		}
		sort.SliceStable(setups, func(a, b int) bool { return setups[a].Score > setups[b].Score })
		for _, setup := range setups {
			if state, _ := f.shut(book, cash+invested); state != FINDER_LOOKING {
				break
			}
			candle, _, _ := candleAt(setup.Symbol)
			entry, fills := replayEntryPrice(setup, candle, f, execution.limit)
			if !fills {
				continue
			}
			bracket := f.bracketAt(entry, setup.ATR, entry/netEntry, netExit)
			qty, small := f.size(bracket, Funds{EquityUSD: cash + invested, BuyingPowerUSD: cash}, now)
			if small != "" {
				continue
			}
			p := f.draft(setup, Market{}, bracket, qty, execution.exitFeePct, rules.Template, now)
			pos := &coinPosition{play: p, stops: p.stops(rules), left: qty, riskUSD: qty * bracket.R, enteredAt: t,
				sidewaysAt: t + int64(f.SidewaysHours)*HOUR_SECONDS}
			pos.targets = append(pos.targets, p.Ladder...)
			if p.Runner.Qty > 0 && p.Runner.Target > 0 {
				pos.targets = append(pos.targets, Rung{Qty: p.Runner.Qty, Price: p.Runner.Target})
			}
			cash, invested = cash-qty*bracket.Entry, invested+qty*bracket.Entry
			held[setup.Symbol], book.open[setup.Symbol] = pos, true
			book.today++
		}

		// Exits, a play entered this hour included. The clock first, then the stop, then the exits: the cautious order.
		for _, symbol := range f.Symbols {
			pos := held[symbol]
			candle, _, ok := candleAt(symbol)
			if !ok {
				continue
			}
			lastClose[symbol] = candle
			if pos == nil {
				continue
			}
			open, low, high := candle.O*netExit, candle.L*netExit, candle.H*netExit
			stop := pos.stops[min(pos.next, len(pos.stops)-1)]
			switch {
			case !now.Before(pos.play.Failsafes.ExitBy):
				finish(pos, t, open, REASON_TIME)
			case f.SidewaysHours > 0 && t == pos.sidewaysAt && open <= pos.play.Entry:
				finish(pos, t, open, REASON_TIME_STOP)
			case low <= stop:
				finish(pos, t, math.Min(stop, open), REASON_STOP) // a gap through the stop fills at the open, not at the stop
			case execution.limit && pos.enteredAt == t:
				// The high may have occurred before the resting buy filled.
			default:
				for pos.next < len(pos.targets) && pos.left > QTY_DUST && high >= pos.targets[pos.next].Price {
					target := pos.targets[pos.next]
					pos.next++
					if pos.left-target.Qty > QTY_DUST {
						sell(pos, target.Qty, target.Price)
						continue
					}
					finish(pos, t, target.Price, REASON_TARGET)
				}
			}
		}
		equity := cash
		for _, symbol := range f.Symbols {
			if pos := held[symbol]; pos != nil {
				equity += pos.left * lastClose[symbol].C * netExit
			}
		}
		peak = math.Max(peak, equity)
		out.MaxDrawdownPct = math.Max(out.MaxDrawdownPct, (peak-equity)/peak*PERCENT)
	}

	for _, symbol := range f.Symbols {
		if pos := held[symbol]; pos != nil {
			last := lastClose[symbol]
			finish(pos, last.T+HOUR_SECONDS, last.C*netExit, REASON_WINDOW)
		}
	}
	return out
}

// replayEntryPrice models the next open or a one-hour buy limit that must first rest.
func replayEntryPrice(setup Setup, candle coinbase.Candle, f CryptoRules, limit bool) (float64, bool) {
	if limit {
		price := coinPrice(setup.Close * (1 - REPLAY_LIMIT_PCT/PERCENT))
		return price, price > 0 && candle.O > price && candle.L < price
	}
	chase := (candle.O/setup.Close - 1) * PERCENT
	return candle.O, candle.O > 0 && (f.MaxChasePct <= 0 || chase <= f.MaxChasePct)
}

// printReplay reports aggregate performance for one independently funded segment.
func printReplay(out io.Writer, r coinReplay, strategy, mode string, days int, segment string, capitalUSD float64) {
	wins, sumR := 0, 0.0
	for _, t := range r.Trades {
		if t.PnLUSD > 0 {
			wins++
		}
		sumR += t.R
	}
	pnl := r.EndUSD - capitalUSD
	n := float64(max(1, len(r.Trades)))
	fmt.Fprintf(out, REPLAY_ROW, strategy, mode, days, segment, len(r.Trades), wins,
		float64(wins)/n*PERCENT, sumR/n, pnl, pnl/capitalUSD*PERCENT, r.MaxDrawdownPct)
}

// replayWindow runs the full window and each of its chronological segments from fresh cash.
func replayWindow(out io.Writer, series map[string][]coinbase.Candle, rules Rules, mode string, days int, end time.Time, capitalUSD float64) {
	start := end.AddDate(0, 0, -days)
	splitHours := int(end.Sub(start)/time.Hour) * REPLAY_EARLY_PARTS / REPLAY_SPLIT_PARTS
	split := start.Add(time.Duration(splitHours) * time.Hour)
	execution := replayExecution{entryFeePct: alpaca.CRYPTO_TAKER_FEE_PCT, exitFeePct: alpaca.CRYPTO_TAKER_FEE_PCT}
	if mode == REPLAY_LIMIT {
		execution.entryFeePct, execution.limit = alpaca.CRYPTO_MAKER_FEE_PCT, true
	}
	for _, segment := range []struct {
		label string
		from  time.Time
		to    time.Time
	}{
		{REPLAY_TOTAL, start, end},
		{REPLAY_EARLY, start, split},
		{REPLAY_LATE, split, end},
	} {
		r := replayCoins(series, rules, segment.from, segment.to, capitalUSD, execution)
		printReplay(out, r, rules.Crypto.Strategy, mode, days, segment.label, capitalUSD)
	}
}

// BacktestCrypto reads public candles once and replays the chosen modes and windows entirely offline.
func BacktestCrypto(ctx context.Context, client *http.Client, rules Rules, options CryptoReplayOptions, capitalUSD float64, out io.Writer) error {
	if rules.Crypto == nil || len(rules.Crypto.Symbols) == 0 || options.Days <= 0 || capitalUSD <= 0 {
		return errBacktestInput
	}
	f := *rules.Crypto
	modes := []string{options.Entry}
	switch options.Entry {
	case "", REPLAY_MARKET:
		modes[0] = REPLAY_MARKET
	case REPLAY_LIMIT:
	case REPLAY_BOTH:
		modes = []string{REPLAY_MARKET, REPLAY_LIMIT}
	default:
		return fmt.Errorf("unknown replay entry %q, want market, limit or both", options.Entry)
	}
	if f.Strategy == "" {
		f.Strategy = CRYPTO_BREAKOUT
	}
	strategies := []string{f.Strategy}
	windows := []int{options.Days}
	if options.Compare {
		strategies = CRYPTO_STRATEGIES[:]
		if options.Days > REPLAY_SHORT_DAYS {
			windows = append([]int{REPLAY_SHORT_DAYS}, windows...)
		}
	} else if !slices.Contains(CRYPTO_STRATEGIES[:], f.Strategy) {
		return fmt.Errorf(WHY_CRYPTO_STRATEGY, f.Strategy)
	}
	end := options.End.UTC()
	latest := time.Now().UTC().Truncate(time.Hour)
	if end.IsZero() {
		end = latest
	}
	if !end.Equal(end.Truncate(time.Hour)) || end.After(latest) {
		return fmt.Errorf("replay end %s must be a completed UTC hour boundary", end.Format(time.RFC3339))
	}
	from := end.AddDate(0, 0, -options.Days).Add(-REPLAY_LEAD)
	expected := int(end.Sub(from) / time.Hour)
	fmt.Fprintf(out, REPLAY_HEAD, end.Format(time.RFC3339), capitalUSD)
	fmt.Fprintf(out, REPLAY_METHOD, alpaca.CRYPTO_MAKER_FEE_PCT, alpaca.CRYPTO_TAKER_FEE_PCT)
	series := map[string][]coinbase.Candle{}
	for _, symbol := range append([]string{CRYPTO_BTC}, f.Symbols...) {
		if _, ok := series[symbol]; ok {
			continue
		}
		rows, err := coinHistory(ctx, client, symbol, from, end)
		if err != nil {
			return fmt.Errorf("%s: %w", symbol, err)
		}
		rows = completed(rows, end)
		if len(rows) == 0 {
			return fmt.Errorf("%s: %w", symbol, errNoCandles)
		}
		series[symbol] = rows
		fmt.Fprintf(out, REPLAY_COVERAGE, symbol, time.Unix(rows[0].T, 0).UTC().Format(time.RFC3339),
			time.Unix(rows[len(rows)-1].T, 0).UTC().Format(time.RFC3339), len(rows), expected)
	}
	rules.Crypto = &f
	fmt.Fprintf(out, REPLAY_COLUMNS, "strategy", "entry", "days", "segment", "trades", "wins", "win%", "avg R", "P&L $", "P&L %", "drawdown")
	for _, days := range windows {
		for _, strategy := range strategies {
			f.Strategy = strategy
			for _, mode := range modes {
				replayWindow(out, series, rules, mode, days, end, capitalUSD)
			}
		}
	}
	return nil
}
