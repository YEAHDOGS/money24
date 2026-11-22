package plays

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/YEAHDOGS/money24/engine/coinbase"
	"github.com/YEAHDOGS/money24/engine/signals"
)

const (
	TEST_COIN       = "TEST-USD"
	TEST_HOURS      = 120
	TEST_FEE_PCT    = 0.6
	TEST_USUAL_VOL  = 100.0
	TEST_SURGE_VOL  = 300.0
	TEST_BREAK_JUMP = 2.0 // how far the breakout hour closes over the hour before it
	TEST_TOLERANCE  = 1e-6
)

var (
	TEST_WEEKDAY = time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC) // Friday 10:00 New York
	TEST_WEEKEND = time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC) // Saturday
	TEST_START   = time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)  // the first candle of every series here
)

// hourly builds n candles an hour apart, each opening where the last one closed.
func hourly(n int, at func(i int) (close, volume float64)) []coinbase.Candle {
	out := make([]coinbase.Candle, 0, n)
	open, _ := at(0)
	for i := range n {
		c, v := at(i)
		out = append(out, coinbase.Candle{T: TEST_START.Unix() + int64(i)*HOUR_SECONDS, O: open, H: math.Max(open, c) + 0.1, L: math.Min(open, c) - 0.1, C: c, V: v})
		open = c
	}
	return out
}

// choppy climbs (or with a negative slope, falls) a little every hour while swinging both ways, so RSI stays in the middle.
func choppy(slope float64) func(int) (float64, float64) {
	return func(i int) (float64, float64) {
		swing := 0.3
		if i%2 == 1 {
			swing = -0.3
		}
		return 100 + slope*float64(i) + swing, TEST_USUAL_VOL
	}
}

// breakout is a choppy climb whose last hour closes well over the range on volume times the usual.
func breakout(volume float64) []coinbase.Candle {
	base := choppy(0.05)
	return hourly(TEST_HOURS, func(i int) (float64, float64) {
		c, v := base(i)
		if i == TEST_HOURS-1 {
			before, _ := base(i - 1)
			return before + TEST_BREAK_JUMP, volume
		}
		return c, v
	})
}

func TestCryptoSetupFiresOnAVolumeBreakout(t *testing.T) {
	f := *DefaultCrypto()
	up, down := breakout(TEST_SURGE_VOL), hourly(TEST_HOURS, choppy(-0.05))
	s, why := cryptoSetup(TEST_COIN, up, up, f)
	if why != "" {
		t.Fatalf("a clean breakout was refused: %s", why)
	}
	if s.RelVolume < 2.9 || s.RelVolume > 3.1 || s.Close <= s.PriorHigh || s.Score <= s.RelVolume || s.ATR <= 0 {
		t.Errorf("setup %+v: want 3x volume, a close over the range, and a score raised by the margin", s)
	}
	if long, _ := cryptoSetup(TEST_COIN, append(hourly(500, choppy(0)), up...), up, f); long.Score != s.Score {
		t.Errorf("score %g with older candles in front, %g without: the window must not depend on how much history was loaded", long.Score, s.Score)
	}

	rising := hourly(TEST_HOURS, func(i int) (float64, float64) {
		if i == TEST_HOURS-1 {
			return 100 + 0.2*float64(i) + TEST_BREAK_JUMP, TEST_SURGE_VOL
		}
		return 100 + 0.2*float64(i), TEST_USUAL_VOL
	})
	refused := []struct {
		name    string
		candles []coinbase.Candle
		btc     []coinbase.Candle
		want    string
	}{
		{"usual volume", breakout(TEST_USUAL_VOL), up, "volume"},
		{"a downtrend", down, up, "no uptrend"},
		{"no close over the range", hourly(TEST_HOURS, choppy(0.05)), up, "no breakout"},
		{"never a down hour", rising, up, "RSI"},
		{"bitcoin under its average", up, down, "bitcoin"},
		{"too little history", up[TEST_HOURS-30:], up, "candles"},
	}
	for _, r := range refused {
		if _, why := cryptoSetup(TEST_COIN, r.candles, r.btc, f); !strings.Contains(why, r.want) {
			t.Errorf("%s: refused with %q, want a reason naming %q", r.name, why, r.want)
		}
	}
	if _, why := cryptoSetup(CRYPTO_BTC, up, down, f); why != "" {
		t.Errorf("bitcoin is not held back by its own filter: %s", why)
	}
}

func TestCryptoBracketIsAfterFees(t *testing.T) {
	f := *DefaultCrypto()
	b := f.bracket(100, 1, TEST_FEE_PCT)
	// The ask is 100 and two hourly ranges are 2: buy at 100.6 after the fee, and the stop at 98 pays 97.412.
	if b.Entry != 100.6 || b.Stop != 97.412 || math.Abs(b.R-3.188) > TEST_TOLERANCE || math.Abs(b.StopPct-2) > TEST_TOLERANCE {
		t.Fatalf("bracket %+v, want entry 100.6, stop 97.412, R 3.188, stop 2%% under the ask", b)
	}
	if len(b.Rungs) != 2 || b.Rungs[0] != 105.382 || b.Rungs[1] != 110.164 || b.Runner != 116.54 {
		t.Errorf("exits %v and %g, want 1.5R, 3R and 5R over the entry: 105.382, 110.164, 116.54", b.Rungs, b.Runner)
	}
	if near, far := f.bracket(100, 0.1, 0), f.bracket(100, 10, 0); near.StopPct != f.MinStopPct || far.StopPct != f.MaxStopPct {
		t.Errorf("stop %.2f%% on a quiet coin and %.2f%% on a wild one, want the clamps %.1f%% and %.1f%%", near.StopPct, far.StopPct, f.MinStopPct, f.MaxStopPct)
	}
	if cheap := f.bracket(0.123456, 0.002, TEST_FEE_PCT); cheap.Stop >= cheap.Entry || cheap.Stop == math.Round(cheap.Stop*CENT)/CENT {
		t.Errorf("a coin under a dollar lost its digits: %+v", cheap)
	}
}

func TestCryptoSizeFitsACashAccount(t *testing.T) {
	f := *DefaultCrypto()
	b := f.bracket(100, 1, TEST_FEE_PCT)
	account := Funds{EquityUSD: 1000, BuyingPowerUSD: 1000}
	cost := func(funds Funds, now time.Time) float64 {
		qty, why := f.size(b, funds, now)
		if why != "" {
			t.Fatalf("size refused: %s", why)
		}
		return qty * b.Entry
	}
	// 1% of $1,000 at risk over a 3.188 stop would buy $315 of it; the position cap stops at 25% of the account.
	if got := cost(account, TEST_WEEKDAY); got > 250 || got < 249.99 {
		t.Errorf("weekday cost $%.4f, want the $250 position cap", got)
	}
	if got := cost(account, TEST_WEEKEND); got > 125 || got < 124.99 {
		t.Errorf("weekend cost $%.4f, want half: $125", got)
	}
	if got := cost(Funds{EquityUSD: 1000, BuyingPowerUSD: 40}, TEST_WEEKDAY); got > 40 || got < 39.99 {
		t.Errorf("cost $%.4f with $40 settled, want no more than the cash there is", got)
	}
	uncapped := f
	uncapped.MaxPositionPct = 100
	qty, _ := uncapped.size(b, account, TEST_WEEKDAY)
	if lost := qty * b.R; lost > 10 || lost < 9.99 {
		t.Errorf("loses $%.4f at the stop, want 1%% of the account: $10", lost)
	}
	if _, why := f.size(b, Funds{EquityUSD: 1000, BuyingPowerUSD: 5}, TEST_WEEKDAY); !strings.Contains(why, "minimum") {
		t.Errorf("$5 of settled cash: %q, want a refusal naming the minimum", why)
	}
}

// coinPlay is the play the desk would draft on the breakout series, with a live book around its close.
func coinPlay(now time.Time) (Play, Rules) {
	rules := DOGSRules()
	f := *rules.Crypto
	candles := breakout(TEST_SURGE_VOL)
	setup, _ := cryptoSetup(TEST_COIN, candles, candles, f)
	tick := coinbase.Ticker{Bid: setup.Close - 0.01, Ask: setup.Close + 0.01, Price: setup.Close}
	b := f.bracket(tick.Ask, setup.ATR, TEST_FEE_PCT)
	qty, _ := f.size(b, Funds{EquityUSD: 1000, BuyingPowerUSD: 1000}, now)
	return f.draft(setup, coinMarket(candles, tick, now), b, qty, TEST_FEE_PCT, rules.Template, now), rules
}

func TestCryptoPlayIsCompleteAndClearsTheRulebook(t *testing.T) {
	// Monday 9:35 New York: inside the stock opening wait, which a coin does not have.
	now := time.Date(2026, 11, 16, 14, 35, 0, 0, time.UTC)
	p, rules := coinPlay(now)
	sold := p.Runner.Qty
	for _, r := range p.Ladder {
		sold += r.Qty
	}
	if p.Qty <= 0 || math.Abs(sold-p.Qty) > QTY_DUST || len(p.Ladder) != 2 || p.Runner.Qty < p.Ladder[0].Qty {
		t.Fatalf("exits cover %.8f of %.8f coins: %+v %+v", sold, p.Qty, p.Ladder, p.Runner)
	}
	if p.Contract.Market != MARKET_CRYPTO || p.Multiplier() != 1 || p.FoundBy != FOUND_BY_CRYPTO || p.Stop >= p.Entry {
		t.Errorf("play %+v: want a coin play found by the desk, with a stop under the entry", p.Contract)
	}
	if hold := p.Failsafes.ExitBy.Sub(now); hold != 72*time.Hour || p.Failsafes.SidewaysMinutes != 12*60 {
		t.Errorf("time exit after %s, sideways after %d minutes; want 72 hours and 720 minutes", hold, p.Failsafes.SidewaysMinutes)
	}
	rep := Build(p, rules, Book{Others: []Play{p}, Now: now})
	for _, c := range rep.Checks {
		if c.Status == CHECK_FAIL {
			t.Errorf("blocked by %s: %s", c.ID, c.Detail)
		}
		if strings.Contains(strings.ToLower(c.Detail), "stock") && (c.ID == RULE_VOLUME || c.ID == RULE_CROWD || c.ID == RULE_OPEN) {
			t.Errorf("%s talks about a stock on a coin play: %s", c.ID, c.Detail)
		}
	}
	if rep.Payoff.RewardRisk < 2 || rep.Payoff.VolSource == "none" || rep.Record.Market != RECORD_CRYPTO {
		t.Errorf("reward to risk %.1f, volatility %q, filed under %q", rep.Payoff.RewardRisk, rep.Payoff.VolSource, rep.Record.Market)
	}
	half := Resize(p, p.Qty/2)
	if math.Abs(half.Qty-p.Qty/2) > 1/CRYPTO_QTY_SCALE || math.Abs(half.Ladder[0].Qty+half.Ladder[1].Qty+half.Runner.Qty-half.Qty) > QTY_DUST {
		t.Errorf("resized to %.8f coins with exits %+v %+v, want half of %.8f in the same shares", half.Qty, half.Ladder, half.Runner, p.Qty)
	}
}

func TestPaperTradesACoinAsACoin(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var got signals.Plan
	s := &Service{Store: store, Paper: func(sig signals.Signal, plan signals.Plan) (int64, error) {
		got = plan
		return 7, nil
	}}
	p, _ := coinPlay(TEST_WEEKDAY)
	p.Entry, p.Stop = 0.123456, 0.119 // a coin under a dollar: its thresholds must keep their digits
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	armed, err := s.paper(p, TEST_WEEKDAY)
	if err != nil {
		t.Fatal(err)
	}
	if got.Signal.Market != signals.MARKET_CRYPTO || got.Signal.Symbol != TEST_COIN || got.Signal.Right != "" {
		t.Errorf("signal %+v: the exit watcher would not price it as a coin", got.Signal)
	}
	if got.TimeStopAbove != 0.123456 || got.TimeStopAt.Sub(TEST_WEEKDAY) != 12*time.Hour || len(got.Targets) != 3 || len(got.StopsAfter) != 2 {
		t.Errorf("plan %+v: want the sideways check at the entry after 12 hours, three resting exits and a stop that climbs", got)
	}
	if armed.Status != STATUS_ARMED || armed.JournalID != 7 {
		t.Errorf("play status %s, journal %d", armed.Status, armed.JournalID)
	}
}

func TestCryptoLimits(t *testing.T) {
	f := *DefaultCrypto()
	now := TEST_WEEKDAY
	book := cryptoBook{open: map[string]bool{"BTC-USD": true}, lostAt: map[string]time.Time{"ETH-USD": now.Add(-2 * time.Hour), "SOL-USD": now.Add(-13 * time.Hour)}}
	if state, why := f.shut(book, 1000); state != FINDER_LOOKING {
		t.Fatalf("one play open and nothing lost: %s %s", state, why)
	}
	for symbol, want := range map[string]string{"BTC-USD": "already running", "ETH-USD": "loss", "SOL-USD": "", "XRP-USD": ""} {
		if why := f.barred(book, symbol, now); (want == "") != (why == "") || !strings.Contains(why, want) {
			t.Errorf("%s: %q, want %q", symbol, why, want)
		}
	}
	book.open["ETH-USD"], book.open["SOL-USD"] = true, true
	if state, _ := f.shut(book, 1000); state != FINDER_FULL {
		t.Errorf("three plays open: %s, want full", state)
	}
	if state, _ := f.shut(cryptoBook{today: 8}, 1000); state != FINDER_FULL {
		t.Errorf("eight plays today: %s, want full", state)
	}
	if state, _ := f.shut(cryptoBook{dayPnL: -30}, 1000); state != FINDER_PAUSED {
		t.Errorf("3%% of the account lost today: %s, want paused", state)
	}

	held, _ := coinPlay(now.Add(-time.Hour))
	held.Status = STATUS_ARMED
	lost, _ := coinPlay(now.Add(-3 * time.Hour))
	lost.ID, lost.Contract.Symbol, lost.Status = "lost", "ETH-USD", STATUS_CLOSED
	lost.Result = &Result{At: now.Add(-2 * time.Hour), PnLUSD: -12}
	share := Play{Contract: Contract{Symbol: "AAL", Market: MARKET_STOCK}, Status: STATUS_ARMED, CreatedAt: now}
	got, newest := bookFrom([]Play{held, lost, share}, now)
	if len(got.open) != 1 || !got.open[TEST_COIN] || got.today != 2 || got.dayPnL != -12 || got.lostAt["ETH-USD"].IsZero() {
		t.Errorf("book %+v: want one coin open, two desk plays today, $12 lost, and a stock play ignored", got)
	}
	if !newest[TEST_COIN].Equal(held.CreatedAt) {
		t.Errorf("newest play on %s at %v, want %v", TEST_COIN, newest[TEST_COIN], held.CreatedAt)
	}
}

func TestOlderRulebookGetsTheCryptoDesk(t *testing.T) {
	var old Rules
	if err := json.Unmarshal([]byte(`{"version":1,"name":"mine","bankroll_usd":900,"rules":[]}`), &old); err != nil {
		t.Fatal(err)
	}
	got := old.merged()
	if got.Crypto == nil || !got.Crypto.On || len(got.Crypto.Symbols) != 12 || got.Crypto.Symbols[0] != CRYPTO_BTC || got.BankrollUSD != 900 {
		t.Errorf("crypto rules after merging: %+v", got.Crypto)
	}
}

// after continues the breakout series for more hours, moving step dollars an hour on usual volume.
func after(hours int, step float64) []coinbase.Candle {
	start := breakout(TEST_SURGE_VOL)
	last := start[len(start)-1].C
	return hourly(TEST_HOURS+hours, func(i int) (float64, float64) {
		if i < TEST_HOURS {
			return start[i].C, start[i].V
		}
		return last + step*float64(i-TEST_HOURS+1), TEST_USUAL_VOL
	})
}

func TestReplayCoinsRunsTheSameBracket(t *testing.T) {
	rules := DOGSRules()
	rules.Crypto.Symbols, rules.Crypto.BTCMustTrend = []string{TEST_COIN}, false
	run := func(candles []coinbase.Candle, fee float64) coinReplay {
		end := time.Unix(candles[len(candles)-1].T+HOUR_SECONDS, 0)
		return replayCoins(map[string][]coinbase.Candle{TEST_COIN: candles}, rules, TEST_START, end, 1000,
			replayExecution{entryFeePct: fee, exitFeePct: fee})
	}

	won := run(after(30, 1), TEST_FEE_PCT)
	if len(won.Trades) != 1 {
		t.Fatalf("a breakout that kept climbing: %d trades: %+v", len(won.Trades), won.Trades)
	}
	// A third at 1.5R, a third at 3R and the rest at 5R is 3.17R on the whole position.
	if w := won.Trades[0]; w.Reason != REASON_TARGET || math.Abs(w.R-(1.5+3+5)/3) > 0.01 || w.EntryAt != TEST_START.Unix()+TEST_HOURS*HOUR_SECONDS {
		t.Errorf("winner %+v: want every exit filled for about 3.17R, entered at the open of the hour after the breakout", w)
	}
	if math.Abs(won.EndUSD-1000-won.Trades[0].PnLUSD) > TEST_TOLERANCE {
		t.Errorf("account ended at $%.2f after a $%.2f winner, drawdown %.2f%%", won.EndUSD, won.Trades[0].PnLUSD, won.MaxDrawdownPct)
	}

	lost := run(after(30, -1), TEST_FEE_PCT)
	if len(lost.Trades) != 1 {
		t.Fatalf("a breakout that failed: %d trades: %+v", len(lost.Trades), lost.Trades)
	}
	// Risk is 1% of the account, but the 25% position cap binds first, so the loss is under $10 and exactly 1R.
	if l := lost.Trades[0]; l.Reason != REASON_STOP || math.Abs(l.R+1) > TEST_TOLERANCE || l.PnLUSD >= 0 || l.PnLUSD < -10 {
		t.Errorf("loser %+v: want stopped out for exactly 1R", l)
	}
	// The same failed breakout with no fee: the same $250 position, stopped the same distance down, loses less.
	if free := run(after(30, -1), 0); len(free.Trades) != 1 || free.Trades[0].PnLUSD <= lost.Trades[0].PnLUSD {
		t.Errorf("no fee lost $%.2f, the fee lost $%.2f: the fee must cost something", free.Trades[0].PnLUSD, lost.Trades[0].PnLUSD)
	}
	if lost.MaxDrawdownPct <= 0 || lost.EndUSD >= 1000 {
		t.Errorf("after a loss the account is at $%.2f with a %.2f%% drawdown", lost.EndUSD, lost.MaxDrawdownPct)
	}

	flat := run(after(30, 0), TEST_FEE_PCT)
	if len(flat.Trades) != 1 || flat.Trades[0].Reason != REASON_TIME_STOP || flat.Trades[0].ExitAt-flat.Trades[0].EntryAt != 12*HOUR_SECONDS {
		t.Errorf("a breakout that went nowhere: %+v, want out on the sideways exit after 12 hours", flat.Trades)
	}
}
