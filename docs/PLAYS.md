# Plays — Money 24's core loop

Status: built (Nov 14, 2026). Engine: `engine/plays/`. Screen: `plays.html` (the launch page on
every shell). Paper only.

A **play** is one trade with everything it needs to run without a person: the contract, the entry,
ladder exits, a runner, a stop, failsafes, the evidence that other money is in the same trade, and
the market snapshot it was planned against. The loop is:

```
paste a link / AI finds one → resolve the contract → load the market → draft the full bracket
→ score the odds → check the rulebook → paper trade (instantly) → exit watcher closes it → record
```

This replaces the manual version of the job: a person asking a chatbot where open interest is,
which contract to buy, what the stop is, and how to turn $150 into $1,000.

## 1. What every play shows

| Block | What it says |
|---|---|
| **What must happen** | For each exit: the stock price needed right now, by the exit time (time decay included), and at expiry; the move in percent; the options market's odds of touching that price before the exit time. The stop is shown the same way, including when time decay alone reaches it. |
| **Payoff** | Cost, planned loss (everything sold at the stop), worst loss (the stop never fills: the whole premium), max profit (every rung and the runner), reward to risk, and a rough expected value. Scenarios: stopped out, T1 then stop, T1+T2 then stop, everything, total calamity. |
| **Odds** | *Market odds*: the chance the stock touches the T1 price before the exit time, from implied volatility alone (driftless, reflection principle). *DOGS odds*: market odds × (0.5 + evidence score / 100), capped 1–95%. |
| **Success rate** | Big, top right of every play: the DOGS odds that the whole play ends in profit at the size on the dial. It is the touch odds of the first exit where selling that rung (the rest stopped out, the estimated slippage paid) is a net gain, times the evidence multiplier, times the share of the order the market can fill (the size rule's volume and open-interest limit over the order). Inside the limit it holds; past it, it falls with every unit. |
| **Stake** | Minimum deposit (one unit for every rung and the runner), a slider up to the larger of 10× the minimum, the bankroll, or the liquidity cap. Every number on the play follows it. The first exit's odds don't change with size; the success rate, dollars, bankroll share, share of volume and the expected fill do. |
| **Checks** | Every rule's verdict: fail (blocking), warn, pass, info, off. |

Prices are per contract (×100 for options). Levels are found by inverting Black-Scholes with the
contract's implied volatility; share plays use 20-day historical volatility.

## 2. The rulebook

One document (`rules.json` in the data folder), editable on the Rules tab, reset to DOGS defaults
with one button. Every rule can be turned off, switched between **Block** and **Warn**, and retuned.

### DOGS rules (the owner's)

| Rule | Default | What it enforces |
|---|---|---|
| `exits_required` | Block | No play runs without a stop below the entry, ladder exits, exits for every unit, a time exit, a sideways exit, a glitch plan (no quote / halt) and a written calamity plan with a gap trigger. So every play has a known max profit and max loss for any stake. |
| `crowd_evidence` | Block | Proof other traders are in it: open interest over the liquidity bar, stock volume at or above average, or hand-entered flow, on-chain, signal or verified news evidence. |
| `news_trust` | Filter | News is ignored unless it has ≥ 2 independent sources and ≥ 60 source trust. AI-suggested news is never weighed until a person verifies it. |
| `volume_first` | Warn | Volume is the biggest weight (40 of 100). Stock volume ≥ 0.8× average and ≥ 100 contracts traded today. Trusted *breaking* news takes the indicators' weight entirely — but never volume's. |

### Suggested rules (on by default, from the FAA chat and the project's own research)

| Rule | Default | Why |
|---|---|---|
| `liquidity` | Block, spread ≤ 15%, OI ≥ 500 | The $0.00 × $0.01 trap: a contract nobody bids can't be sold. The Robinhood link in the chat (DAL 80P **11/27**) had OI 72 and a 73% spread. |
| `lotto_guard` | Block, premium ≥ $0.10, strike ≤ 15% OTM | The $500 SPY puts: ~0 delta, no bid, theta eats them. |
| `reward_risk` | Block, ≥ 2.0 | The ladder has to pay for the stop. |
| `time_left` | Block, ≥ 3 days, exit ≥ 1 day before expiry | "Never hold past Wednesday/Thursday" — vertical decay. |
| `event_crush` | Warn | Earnings before expiry (NVDA Nov 17): implied volatility collapses after the report. |
| `opening_wait` | Block, 15 min after open, 10 before close | "Wait 15–30 minutes after the open; don't buy puts on a red gap." |
| `no_chase` | Warn, 3% | The stock already moved the play's way today; the move may be spent. |
| `bankroll_risk` | Warn, 10% at stop / 30% worst | One play can't sink the account. Set the bankroll on the Rules tab. |
| `odds_floor` | Warn, DOGS odds ≥ 30% | T1 has to be plausible, not a lottery. |
| `positive_edge` | Warn, expected ≥ $0 | The scenarios weighted by the market's own odds shouldn't lose on average. |
| `theme_cap` | Warn, 2 per theme | AAL, DAL and UAL on one FAA headline are one bet, not three. |
| `max_open` | Block, 5 | A cap on plays running at once. |
| `fresh_data` | Block, 20 min | Never plan on a stale quote. |
| `limit_entry` | Warn, ≤ 5% over mid | "Limit order at the mid, never a market order." |
| `cooldown` | Warn, 24 h | No revenge trade on a symbol that just stopped out. |
| `size_impact` | Warn, ≤ 10% of day volume, ≤ 5% of OI | A size the market can fill and take back. The stake slider shows the expected fill as size grows. |

### The starting bracket (template)

$150 budget, entry at the mid, stop −50%, sell ⅓ at +100%, ⅓ at +300%, runner to +700%, exit the
trading day before expiry at 3 PM New York, sideways exit after a day, glitch plan after 15 minutes
without a quote, calamity flatten on a 10% gap. All editable.

### Paper switches

- **Auto paper** (on): a pasted link is planned and paper traded in the same moment.
- **Paper ignores blocks** (on): paper trades go through blocking rules and record which ones they
  broke (`broke_rules`), so the scoreboard can show whether the rules earn their keep — the same
  ablation idea as ARCHITECTURE §4.3. Live trading would never go through a block.

## 3. Where AI lives

```
                     ┌──────────────── browser / app (plays.html) ────────────────┐
                     │ shows plays, edits rules; never sees a key, never calls AI  │
                     └───────────────▲─────────────────────────────────────────────┘
                                     │ localhost API (origin + host checked)
 ┌───────────────────────────────────┴──── m24d (Go, your machine or VPS) ────────────────────────┐
 │ Resolver: Robinhood instrument API, OCC symbols, Webull quote links, plain text                │
 │ Market:  Yahoo option chain + candles (keyless) · Webull when keyed                            │
 │ Math:    levels, odds, payoff, sizing — deterministic Go, no AI                                │
 │ Rules:   the rulebook — deterministic Go, no AI                                                │
 │ AI read: play + report + newsd headlines as quoted JSON → provider → fixed JSON schema         │
 │ Paper:   Runner.Adopt → journal → exit watcher                                                 │
 └────────────▲────────────────────────────────────────────────────────▲──────────────────────────┘
              │ HTTPS, the user's own key, no tools, no web            │ headlines (newsd)
      Claude · OpenAI · Grok · Gemini                            newsd (Go)
```

Rules for the AI layer:

1. **AI reads and drafts; Go decides.** A model returns a verdict, a must-happen sentence, risks,
   evidence and an optional bracket suggestion. It cannot arm, size, or change a play. Applying a
   suggestion is a button a person presses, and the result goes back through the rulebook.
2. **Math is never asked of a model.** Levels, odds, payoff and sizing are computed in Go. The model
   gets them as input and is told not to invent prices.
3. **No tools, no browsing.** Prompt-injection containment (ARCHITECTURE §6.3): headlines and links
   arrive as quoted JSON fields; the answer must match a JSON schema.
4. **Unverified until verified.** Evidence a model suggests is filed with `ai: true` and is never
   weighed in the odds until a person marks it verified.
5. **Bring your own key.** Keys live in `.env` today (OS keychain planned), go only to their own
   provider, and never reach the browser or DOGS.
6. **Where AI goes next** (not built): a *finder* that scans newsd, the catalyst calendar, unusual
   volume and open-interest changes, and proposes plays into the same inbox (the "middle agent" in
   notes/catalyst-and-filings.md) — scored like any source, paper first, auto-disabled if it loses.

Claude calls use `claude-opus-5-5` with structured output (`output_config.format`), effort
`medium`, and server-side refusal fallback (`fallbacks: "default"`). Every provider's model can be
changed with `M24D_<PROVIDER>_MODEL`.

## 4. Links

| Input | How it resolves |
|---|---|
| `robinhood.com/options/<id>_L1` | Robinhood's public instrument record (no login): symbol, strike, expiry, type |
| `robinhood.com/stocks/AAL`, `webull.com/quote/nasdaq-aal` | The stock |
| `AAL261120P00012500` anywhere (OCC symbol) | The contract |
| `AAL 12.5P 11/20`, `$DAL 80 put 11/27/26` | The contract (no year: the next such date) |
| A callout (`BTO $SPY 750P 11/20 @ 1.20`) | The callout parser |

Every input is kept in the link history with what it became, or why it failed.

## 5. API

See `Routes()` in `engine/plays/service.go`. Reads are open to the allowed origins; writes need
the local desk or the webhook secret, like the rest of m24d.

## 6. The public demo

With no runner set up, the public site (money24.dogs.red) opens the plays board on a recorded
snapshot in `public/demo/`, labelled Demo: real plays and reports from a runner, with the stake dial
recorded at every size so every number still moves. Writing (paper trading, links, rules, AI reads)
says to run Money 24 locally. `?demo` forces it anywhere. Re-record it with a runner going:

```sh
npm start                # paste the plays you want to show
npm run demo:snapshot    # writes public/demo/, then commit and push
```

## 7. Honest limits

- Yahoo data is delayed up to 15 minutes and unofficial. Open interest is as of the last close.
- Touch odds assume no drift and constant volatility. They're the market's price of the move,
  not a forecast.
- The expected value is rough: scenarios weighted by touch odds, losses assumed at the stop.
- The fill model for size is a model; there's no order-book depth.
- Paper fills ignore fees and most slippage. See the Terms of Use §5.

*Options trading involves substantial risk of losing 100% of your capital; this is probability and
market-structure analysis, not financial advice.*
