# The paper desk — predecessor scaffold

Location: `money-makers/ai-trading-desk/`. A 6-agent, 24/7 **paper-trading**
desk covering stocks, crypto, options, prediction markets, and futures.
Every "trade" is a simulated fill at the quoted price, logged with
timestamp, reasoning, and paper P&L. There is no order-placement code
anywhere in the project — it cannot trade real money, by construction, not
just by promise.

**This is a scaffold, not a strategy.** Money 24 inherits its plumbing
thinking; it is not the Money 24 engine.

## The 6 agents

| Agent | Job |
|---|---|
| SCANNER | Sweeps markets for unusual volume / candidates |
| SIZER | Fixed-fractional position sizing from risk config |
| ENTRY | Entry-timing confirmation gate |
| PREDICT | Prediction-market paper bets (Polymarket Gamma API, read-only) |
| BLOCKER | Bad-trade veto + risk-limit enforcement (last word before any fill) |
| COORDINATOR | Supervises, keeps the paper ledger, logs decisions, daily risk report |

Each loop cycle runs: SCANNER → SIZER → ENTRY → PREDICT → BLOCKER veto →
COORDINATOR decision log.

## Fail-safe gates

1. **Paper-only guard** — startup aborts if the config, env, or endpoints
   contain anything live-looking. Fail closed.
2. **No order code paths** — fills are simulated at the quoted price. There
   is literally no function that can submit an order.
3. **STOP file** — halts the loop immediately.
4. **Daily loss limit** — configurable (default −3% day); breached → no new
   entries until the next day.
5. **Position limits** — max % of equity per position, max open positions.
6. **BLOCKER veto** — every candidate passes a final veto; vetoes are logged
   too.
7. **Decision journal + paper ledger** — every cycle logged with reasoning;
   the ledger survives restarts.

## Strategies on the desk

- **Last Year's Winner** — every January, buy the prior year's #1 S&P 500
  performer and hold for the year. The entry rule is kept **pure** (always
  the prior year's #1, no trend filter, no fall-through — a filtered version
  was tried and rejected). Discipline lives only in the exits: a −35% hard
  stop and a trailing stop that arms at +50% and trails 35% below the peak.
  This is the standing testing rule for all strategies: entry pure,
  improvements only in exits and risk.
- An earnings put-spread module (see also the standalone backtest in
  [earnings-put-backtest.md](earnings-put-backtest.md)).

## Honest status

- **The signal layer is placeholder heuristics** with TODO markers where real
  research goes. The scanner, predictor, and entry gate are placeholders —
  **the desk's paper results currently mean nothing**, and it must never be
  cited as a working test system. Rebuilding the signal layer for real (or
  leaning on the standalone backtester plus manual logs at real quoted
  prices) is an open backlog item.
- Paper profits do not predict live results: simulated fills ignore spread,
  slippage, fees, and partial fills.
- The "6 AI agents trading 24/7" reel format that inspired the desk is
  unverified marketing — assume P&L screenshots in such reels are
  cherry-picked or fictional until proven otherwise.

## Worked example: FOMC, Oct 28, 2026

A paper-log sheet was prepped for the Oct 28 FOMC statement (a $15-wide SPY
bear put spread, $770/$755, expiring Oct 30, re-based from the Oct 27
close). The meeting **closed untraded**: no go was given, no live quote was
ever pulled, and the debit fields were left blank rather than back-filled.
The FOMC held at 3.75–4.00% (10–2), the S&P 500 rose ~0.28% on the day, and
the bearish thesis would not have paid. Nothing was lost beyond a prepped
sheet — which is what paper discipline is for.
