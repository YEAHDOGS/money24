# Plays — Money 24's core loop

Status: built (Nov 14, 2026). Engine: `engine/plays/`. Screen: `plays.html` (the launch page on
every shell).

A **play** is one trade with everything it needs to run without a person: the contract, the entry,
ladder exits, a runner, a stop, failsafes, the evidence that other money is in the same trade, and
the market snapshot it was planned against. The loop is:

```
paste a link / AI finds one → resolve the contract → load the market → draft the full bracket
→ score the odds → check the rulebook → trade (instantly) → exit watcher closes it → record
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

### Desk switches

- **Auto trade** (on): a pasted link is planned and traded in the same moment.
- **Trade through blocks** (on): trades go through blocking rules and record which ones they
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
 │ Desk:    Runner.Adopt → journal → exit watcher                                                 │
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
3. **Web search, and no other tools.** Prompt-injection containment (ARCHITECTURE §6.3): headlines
   and links arrive as quoted JSON; the answer must match a JSON schema. Claude and ChatGPT may search
   the web and read pages, and nothing else; prices still come only from the engine (docs/AI.md).
4. **Unverified until verified.** Evidence a model suggests is filed with `ai: true` and is never
   weighed in the odds until a person marks it verified.
5. **Your own plans, no keys.** A play goes to an AI app signed in on the desk machine (Claude Code,
   Codex, Antigravity, OpenCode) on your own subscription or a free tier. Money 24 pays for no AI and
   holds no AI keys (docs/AI.md).
6. **The finder** (built Nov 18, 2026, `engine/plays/finder.go`; no model in it): every five minutes
   of the stock session m24d reads newsd's stories board (`engine/README.md`, Stories). A story earns a
   play when its coverage-against-volume reading is `early` or `confirmed`, its attention and validity
   clear the rulebook's bars, at least three headlines take the angle's side, and the stock is not
   moving against that angle. The finder then picks the option itself (calls when bullish, puts when
   bearish; the first expiry at least a week out; the two-sided strike nearest 2% past the price),
   drafts the full bracket from the template, files the story as news evidence with its outlet count,
   and trades it **only if no blocking rule fails**. A blocked play waits as a draft. One play a
   pass, six a day, never two on one symbol or one story. Found plays carry `found_by: "finder"` and
   the story's id, and show a Found chip on the board. The finder pauses for the day once the
   plays closed that day have lost $150. Settings are on the Rules tab; its last pass, with what it
   decided about each loud story and why, is at `GET /api/plays/finder` (not on a page yet).
7. **The flow desk** (built Nov 21, 2026, `engine/plays/flow.go`; no model in it): every five minutes of
   the stock session (after a 15-minute wait at the open, stopping 15 minutes before the close) it reads
   the option chains of the most traded names (SPY, QQQ, IWM, NVDA, TSLA, AAPL, AMD, META, AMZN, MSFT,
   GOOGL, PLTR) for the next three expiries 7 to 45 days out. A name counts only when one side leads:
   calls at least 1.3x puts by volume, or the reverse. On that side it takes the contract with the most
   premium traded today (volume x mid x 100), within 10% of the stock, at least 500 contracts, at least
   half its open interest traded today (new money, not old positions changing hands), spread under 10%.
   The top call and the top put of the pass get the rulebook's whole bracket and trade **only if no
   blocking rule fails**. One flow play per name a day, four running at once, six a day, and a pause
   once the day's closed flow plays have lost $150. When an AI app is signed in, the first one
   reads each new play; the read changes nothing. Its last pass is at `GET /api/plays/flow` and on Home.
   Option volume has no free history, so this desk cannot be replayed: it is proven or not by its own
   record, and its trades can be hedges it is following rather than bets.
8. **Where AI goes next** (not built): a model reading each loud story once to replace the word-count
   angle, and a scan of the catalyst calendar for scheduled events (a launch date, an earnings day)
   so a play can be opened before the coverage starts (the "middle agent" in
   notes/catalyst-and-filings.md). Both stay desk first and are scored like any source.

A play read is a deep ask: each app's stronger model at medium effort, up to four minutes. Each app's
model can be changed with `M24D_AI_<APP>_MODEL_DEEP` (docs/AI.md).

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

With no runner set up, the public site (money24.dogs.red) opens every app page on a recorded
snapshot in `public/demo/`, labelled Demo: real plays and reports from a runner (the stake dial
recorded at every size, so every number still moves), the journal and source catalog, 400
headlines with the calendar and network meter, and six symbols' quotes and charts. Writing (trades,
links, rules, AI reads, callouts) says to run Money 24 locally. `?demo` forces it anywhere.
Source trust records and stored posts are left out of the public snapshot on purpose. A first visit
gets the work-in-progress notice and then a short tour (`src/lib/tour/`); the footer's Tour button
or `?tour` shows it again. Re-record it with the services going:

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
- fills ignore fees and most slippage. See the Terms of Use §5.

*Options trading involves substantial risk of losing 100% of your capital; this is probability and
market-structure analysis, not financial advice.*

## 8. The play record, the stop ladder and backtests

Every play is filed in one format (`engine/plays/record.go`, shown as **Play record** on the play):

| Row | Where it comes from |
|---|---|
| Market | stock, options, futures, forex, crypto or prediction |
| Category | set by hand on the play ("News play"); shown on its card. `POST /api/plays/{id}/category` |
| Expected entry, expected amount in | the entry price and cost the play planned |
| Actual entry, actual amount in | what filled once it runs. An entry fills whole, at its limit |
| Exit 1, 2, 3 | the ladder's rungs, then the runner's target |
| Stop-loss 1, 2, 3 | the stop before any exit, after exit 1, after exit 2 |
| Stop-loss override | *Time stop-loss* when the clock closed it, *Extremely volatile* when a gap did |

**The stop climbs** (`stop_ladder`, on by default, in the Rules tab): after exit 1 the stop moves to
the entry, after exit 2 to exit 1's price, so a green trade never turns red. A play can carry its own
stop-losses 2 and 3 instead (`stops_after`). The exit watcher moves the stop as rungs fill.

**How the play went** is the play replayed candle by candle, with its exits and stops:

- *Since entry* (`GET /api/plays/{id}/curve`) walks the stock's candles from the play's entry. For an
  option the contract's price is worked out from the stock at the planned volatility.
- *Backtest* (`?from=YYYY-MM-DD`) enters at that day's close and keeps the bracket in proportion. It
  runs over the contract's own daily candles when Yahoo has them (real traded prices, one candle a
  day), otherwise over the worked-out prices.

Both report how the bracket ended and, beside it, what simply holding would have made. They are
replays, not promises: a candle that touched both the stop and an exit counts as stopped, fills are
assumed at the stop and exit prices, and a thin contract may not give them.

## 9. Orders, active plays and history

Every play lists its orders with a status and an exact time (`src/plays/orders.js`): when it was made,
the entry, each exit, the stop-loss, the time exit and how it ended. Statuses are *Not placed* (a
draft), *Working* or *Scheduled* (a live play), *Filled*, *Triggered* and *Cancelled*.

- **Active** plays (trading now) are marked in the list and open on a panel that says when they
  began, the amount in, what they are worth right now (from the replay, read again every 30 seconds)
  and where they end for a win or a loss.
- **History** is the list of finished plays. One that lost says what went wrong: how it ended, the
  blocking rules it was let through as a trade, and any gap between the planned and actual loss.

## 8. Crowd room, levels, the Pit and the deck (Nov 23, 2026)

Built from the Nov 23 discussion tickets. UI and client logic only; the engine is untouched.

- **Crowd room** (`src/plays/crowd.js`, `CrowdPanel.svelte`, shown on every play): how many of the 10,000 players a play's contract can take. Impact is per contract, so the limits are ~5% of open interest and ~1% of the contract's day volume, one contract per player, and a ±10% entry collar for late tranches. When one contract cannot hold the crowd, the verdict says how many matching contracts (other strikes, or the names that moved with it) would. Seat order is the hand-picked list, then the top tier, then lower tiers. Starting values, to be tuned on paper. The engine's `crowd_evidence` and `liquidity` rules do not know about it yet: when the engine build resumes, this becomes a blocking rule.
- **Levels** (`src/markets/levels.js`, `Levels.svelte`): support and resistance from six months of daily candles (swing turns clustered by ATR), the pivot, a normal day's reach, the nearest lines that turn a symbol positive or negative, and a lean (bullish, neutral, bearish) from the 20/50-day averages and five-day move. Drawn on the chart (toggle). Not yet wired: levels the news outlets publish (needs newsd).
- **The Pit** (`src/lib/pit.svelte.js`, `PitBell.svelte`): a bell in every header. It polls the runner, logs each new play and each change of phase on the device, counts unread, and can raise a system notification when the tab is in the background. The demo has no live desk, so it stays empty there.
- **Strategy deck** (`src/deck/`, `deck.html`, filed under Plays): strategies as editable cards (AI read, rule, catalyst, spread, prediction), kept on the device, seeded with DOGS's starter cards, JSON import and export so an AI can write cards in. Grading by the scoreboard and running a card as a play need the engine.

### Nov 24 additions

- **Draft only** on the link inbox: a per-link switch (`draft: true` on `POST /api/plays/link`) that builds the full bracket but skips the rulebook's auto-trade for that one link, so a play can be reviewed and edited before it trades.
- **Stretch and three outcomes** (`src/markets/levels.js`): how far a symbol has run (a month's move, distance from its 20-day average in ATRs, RSI), and up / flat / down odds for the next session and the next five. A stretched symbol tilts against its run, the trend tilts with it. Starting weights, to be tuned against the scoreboard; a lean, not a forecast.
- **Storylines** (`src/feed/storylines.svelte.js`): your own grouping of news stories on this device, over the news service's own.
- **Expected entry and exit** (`engine/plays/schedule.go`): every play carries `expected_entry_at`, `expected_exit_at` and a `schedule_mode`. Strategy plays that enter at once are `immediate`. Plays made or edited by hand are `planned`: a day ahead by default, at least three hours, and started by hand once the entry is due. The expected exit is the play's time stop. Plays saved before this are `legacy` and need times before they can trade.
- **Scribe** (`engine/plays/scribe*.go`, `src/plays/ScribePanel.svelte`): type what you heard ("a key player may miss tomorrow's game") with a planned entry and exit. One signed-in AI checks the claim against sources, corrects mistaken names and dates, and proposes Kalshi contracts. The engine confirms every ticker and price with the exchange itself; the AI's numbers are never shown. `GET`/`POST /api/plays/scribe`, history in `plays/scribe.json`, one request at a time.
- **Scribe to the prediction desk** (`engine/predict/tip.go`): a contract the exchange confirmed can be started on paper once its entry is due (`POST /api/plays/scribe/paper`). It fills one $20 stake from Kalshi's live order book, is held to settlement, obeys the desk's switch, cash and daily loss stop, and is scored as the "Scribe tips" strategy.
- **Phone lock** (`src/lib/app-lock/`, `src/tauri/src/mobile_lock.rs`): the Android app asks for a password of 12 or more characters, encrypts the runner address and secret with it (PBKDF2, AES-GCM), and locks whenever it leaves the foreground. There is no password recovery.
