package plays

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"time"

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
	REPLAY_TOTAL       = "TOTAL"
	REPLAY_TIME        = "2006-01-02 15:04 MST"
	REPLAY_HEAD        = "\nCrypto desk replay: %d days of hourly Coinbase candles to %s, %d coins, $%.2f of cash.\n"
	REPLAY_WARNING     = "A replay, not a promise. Entries fill at the next hour's open, a candle that touched both the stop and an exit\ncounts as stopped, and no order book is modelled. The rule was set before this was run, not fitted to it.\n"
	REPLAY_FEE_LINE    = "\nWith a %.2f%% fee on each side, in every price:\n"
	REPLAY_NO_FEE_LINE = "\nThe same replay with no fee, to show what trading costs:\n"
	REPLAY_COLUMNS     = "%-10s %6s %5s %5s %7s %10s %8s %9s\n"
	REPLAY_ROW         = "%-10s %6d %5d %4.0f%% %+7.2f %+10.2f %+7.2f%% %8.1fh\n"
	REPLAY_DRAWDOWN    = "Deepest fall of the account from a peak, on closed trades: %.1f%%. It ended at $%.2f.\n"
	REPLAY_EXITS       = "How trades ended: %s.\n"
	REPLAY_STILL_OPEN  = "Still open at the end: %d, worth %+.2f at the last price (not in the totals).\n"
	REPLAY_NO_TRADES   = "No trades: the rule never fired inside its limits.\n"
)

var (
	errBacktestInput = errors.New("a replay needs at least one day, some cash and a coin to trade")
	REPLAY_REASONS   = []string{REASON_STOP, REASON_TARGET, REASON_TIME, REASON_TIME_STOP}
)

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
	EndUSD         float64 // starting cash plus every closed trade
	StillOpen      int
	OpenPnLUSD     float64
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
			if !seen[c.T] {
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
func replayCoins(series map[string][]coinbase.Candle, rules Rules, from, to time.Time, capitalUSD, feePct float64) coinReplay {
	f, fee := *rules.Crypto, feePct/PERCENT
	out := coinReplay{EndUSD: capitalUSD}
	cash, invested, peak := capitalUSD, 0.0, capitalUSD
	book := cryptoBook{open: map[string]bool{}, lostAt: map[string]time.Time{}}
	held, cursor, day := map[string]*coinPosition{}, map[string]int{}, ""

	// sell books one sale: the cash comes back, and the account's closed result moves with it.
	sell := func(pos *coinPosition, qty, price float64) {
		gain := (price - pos.play.Entry) * qty
		pos.left, pos.realized = pos.left-qty, pos.realized+gain
		cash, invested, out.EndUSD = cash+qty*price, invested-qty*pos.play.Entry, out.EndUSD+gain
		peak = math.Max(peak, out.EndUSD)
		out.MaxDrawdownPct = math.Max(out.MaxDrawdownPct, (peak-out.EndUSD)/peak*PERCENT)
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

	for t := from.Truncate(time.Hour).Unix(); t <= to.Unix(); t += HOUR_SECONDS {
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
			if f.MaxChasePct > 0 && (candle.O/setup.Close-1)*PERCENT > f.MaxChasePct {
				continue
			}
			bracket := f.bracket(candle.O, setup.ATR, feePct)
			qty, small := f.size(bracket, Funds{EquityUSD: cash + invested, BuyingPowerUSD: cash}, now)
			if small != "" {
				continue
			}
			p := f.draft(setup, Market{}, bracket, qty, feePct, rules.Template, now)
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
			if pos == nil || !ok {
				continue
			}
			open, low, high := candle.O*(1-fee), candle.L*(1-fee), candle.H*(1-fee)
			stop := pos.stops[min(pos.next, len(pos.stops)-1)]
			switch {
			case !now.Before(pos.play.Failsafes.ExitBy):
				finish(pos, t, open, REASON_TIME)
			case f.SidewaysHours > 0 && t == pos.sidewaysAt && open <= pos.play.Entry:
				finish(pos, t, open, REASON_TIME_STOP)
			case low <= stop:
				finish(pos, t, math.Min(stop, open), REASON_STOP) // a gap through the stop fills at the open, not at the stop
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
	}

	for symbol, pos := range held {
		rows := series[symbol]
		out.StillOpen++
		out.OpenPnLUSD += pos.realized + (rows[len(rows)-1].C*(1-fee)-pos.play.Entry)*pos.left
	}
	return out
}

// printReplay writes one table: a row a coin, then the total.
func printReplay(out io.Writer, r coinReplay, symbols []string, capitalUSD float64) {
	if len(r.Trades) == 0 {
		fmt.Fprint(out, REPLAY_NO_TRADES)
		return
	}
	fmt.Fprintf(out, REPLAY_COLUMNS, "coin", "trades", "wins", "win%", "avg R", "P&L $", "P&L %", "avg hold")
	row := func(name string, trades []coinTrade) {
		if len(trades) == 0 {
			return
		}
		wins, pnl, r, hours := 0, 0.0, 0.0, 0.0
		for _, t := range trades {
			if t.PnLUSD > 0 {
				wins++
			}
			pnl, r, hours = pnl+t.PnLUSD, r+t.R, hours+float64(t.ExitAt-t.EntryAt)/float64(HOUR_SECONDS)
		}
		n := float64(len(trades))
		fmt.Fprintf(out, REPLAY_ROW, name, len(trades), wins, float64(wins)/n*PERCENT, r/n, pnl, pnl/capitalUSD*PERCENT, hours/n)
	}
	ended := map[string]int{}
	for _, symbol := range symbols {
		own := []coinTrade{}
		for _, t := range r.Trades {
			if t.Symbol == symbol {
				own = append(own, t)
			}
		}
		row(symbol, own)
	}
	for _, t := range r.Trades {
		ended[t.Reason]++
	}
	row(REPLAY_TOTAL, r.Trades)
	fmt.Fprintf(out, REPLAY_DRAWDOWN, r.MaxDrawdownPct, r.EndUSD)
	how := ""
	for _, reason := range REPLAY_REASONS {
		how += fmt.Sprintf(", %d on the %s", ended[reason], reason)
	}
	fmt.Fprintf(out, REPLAY_EXITS, how[len(", "):])
	if r.StillOpen > 0 {
		fmt.Fprintf(out, REPLAY_STILL_OPEN, r.StillOpen, r.OpenPnLUSD)
	}
}

// BacktestCrypto replays the crypto desk's rulebook over the last days of hourly Coinbase candles and
// prints what it would have done, coin by coin and in total: once with the fee the desk
// assumes, once with none. It reads public candles and places nothing.
func BacktestCrypto(ctx context.Context, client *http.Client, rules Rules, days int, capitalUSD float64, out io.Writer) error {
	f := rules.Crypto
	if f == nil || len(f.Symbols) == 0 || days <= 0 || capitalUSD <= 0 {
		return errBacktestInput
	}
	to := time.Now().Truncate(time.Hour)
	from := to.AddDate(0, 0, -days)
	series := map[string][]coinbase.Candle{}
	for _, symbol := range append([]string{CRYPTO_BTC}, f.Symbols...) {
		if _, ok := series[symbol]; ok {
			continue
		}
		rows, err := coinHistory(ctx, client, symbol, from.Add(-REPLAY_LEAD), to)
		if err != nil {
			return fmt.Errorf("%s: %w", symbol, err)
		}
		series[symbol] = completed(rows, to)
	}
	fmt.Fprintf(out, REPLAY_HEAD, days, to.UTC().Format(REPLAY_TIME), len(f.Symbols), capitalUSD)
	fmt.Fprint(out, REPLAY_WARNING)
	fmt.Fprintf(out, REPLAY_FEE_LINE, coinbase.TAKER_FEE_PCT)
	printReplay(out, replayCoins(series, rules, from, to, capitalUSD, coinbase.TAKER_FEE_PCT), f.Symbols, capitalUSD)
	fmt.Fprint(out, REPLAY_NO_FEE_LINE)
	printReplay(out, replayCoins(series, rules, from, to, capitalUSD, 0), f.Symbols, capitalUSD)
	return nil
}
