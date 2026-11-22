For now, Nov. 21 2026, we are exclusively designing money24 around me, myself, and I alone. I have USD in a checking account.

That means concerns about liquidity should be minimal.

The main goal to backtest everything on: fund the accounts with $1,000, and it trades 24/7 and makes on average $100 every day. That's the target. 10% or more daily return. Compounding, aiming for maximum returns with minimal risk, for as long as we can until the amount starts impacting liquidity and other things.

We should think of everything in Plays. From start to finish, a play begins when money is moved from a money24 connected account into an equity. A play only ends when the money is converted back into USD, and is available for withdrawal (yes, unsettled funds after a trade means the play is still active). Plays have different phases, which must all be trackable and viewable in the money24 app. All plays must be saved to the user's history.

Google Gemini wrote the rest of this with my assistance:

# money24 Plays Lifecycle & Architecture Spec

A **Play** tracks capital end-to-end across global markets—spanning **stocks, options, futures, forex, crypto, and prediction markets**. A play begins when capital is drafted/reserved for an opportunity and only concludes once proceeds are converted back into USD (or base account cash) and settled for withdrawal. **Unsettled funds keep the play active.** Every phase is trackable and viewable in the money24 app, and every play is preserved in the user's history.

---

## 1. Play Phases

1. **Initialized** — A play has been drafted by the user or AI, and its capital is earmarked/reserved, but money24 has not yet confirmed that entry preconditions are good to go when the entry trigger or date arrives.
2. **Created** — The waiting or staging period before execution begins; the play is set up and money24 has confirmed liftoff will commence.
3. **Funding** — Capital is actively moving from the user's reserved/connected balance into the target position or market.
4. **In Play** — Capital is actively deployed in the position (stock, option contract, futures contract, forex pair, crypto asset, or prediction market contract).
5. **Settling** — The play is winding down: the position is being closed, sold, or converted, and proceeds are moving back toward USD/account cash while awaiting settlement or clearing. The play remains active during this window.
6. **Withdrawable** — Proceeds are settled in USD (or base account cash) and free for withdrawal—or eligible for deployment into a new play once any market-specific settlement rules are met.
7. **Error** — Something went wrong during the lifecycle. Must record an **error code** and **human-readable description** (plus context on which phase it occurred in) so the user or AI can diagnose and recover.
8. **Cancelled** — The user or AI cancelled the play prior to entering **In Play**. Should record a **cancellation reason** explaining why it was cancelled.

---

## 2. Core Capital & Lifecycle Rules

### Immediate Capital Reservation at `Initialized`
* The moment a play enters **Initialized**, that capital is treated as allocated/tied up in the play and excluded from available/uncommitted funds for other plays.
* Double-booking the same reserved dollars across competing plays is blocked.

### Cancellation Eligibility (`Not In Play`)
* A play can be cancelled by the user or AI at any point **before** it enters **In Play** (such as during **Initialized**, **Created**, or **Funding**, subject to order/transfer cancellation feasibility).
* Once **In Play**, exiting requires closing/unwinding the position rather than a pre-entry cancel.

### Post-Cancellation Reinvestment & Settlement Variance
When a play is **Cancelled**, whether freed capital is immediately reinvestable depends on the asset class, venue, and cash/settlement state:
* **Immediately Reinvestable:** If funds never left cash or have already settled/cleared on the venue (e.g., cancelled draft/staging reservation or instant-settlement rail/market where cash is free), those funds can return to available play capital right away.
* **Requires Settlement / Clearing Delay:** Certain instruments and venues—such as **options** or regulated markets with trade/settlement clearing cycles—may require a settlement period (e.g., T+1 or overnight clearing) before proceeds or released margin/cash are cleared for unrestricted reuse or withdrawal.

---

## 3. Additional Edge Cases & System Details

### State Machine & Edge Cases
* **Partial Fills & Partial Exits:** Decide early whether a partial fill or partial close stays inside a single play (e.g., remaining in **Funding** or **In Play** / **Settling** with a tracked `filledQuantity` and `remainingQuantity`) or spawns a split/child play so cost basis and realized vs. unrealized P&L stay clean.
* **Reinvestment / Chaining Plays:** When funds hit **Withdrawable** in USD and the user or AI uses that USD to start a new play, decide whether the original play closes/archives into history and creates a new play ID with a `parentPlayId` / `chainedFromPlayId` link.
* **Error Recovery Paths:** **Error** should never be a dead end. Every error should retain `previousPhase` (e.g., failed during **Funding** vs. failed during **Settling**) and offer explicit recovery actions—such as **Retry**, **Cancel / Revert**, or **Escalate / Support**—so funds are never left in an ambiguous state.
* **Stale / Expired `Initialized` or `Created` Plays:** Define time-to-live (TTL) or market-session expiration rules for plays that never reach execution (e.g., a scheduled entry window passes or preconditions never clear), transitioning cleanly to **Cancelled** or **Error** with an explicit reason/code.
* **Corporate Actions, Expirations & Market Halts:** Handle edge cases while **In Play** or **Settling**, such as option expiration/assignment, contract rollovers/expiry in futures, trading halts, splits/dividends, or venue outages.

### History, Auditability & Accounting
* **Phase Timestamps & Transition Log:** Because all phases must be trackable and viewable in the app, maintain an ordered history of phase transitions (`phase`, `enteredAt`, `exitedAt`, `actor`: `user` | `ai` | `system`, `notes`) alongside top-level milestone timestamps (`initializedAt`, `createdAt`, `fundedAt`, `enteredPlayAt`, `settlingStartedAt`, `withdrawableAt`, `errorAt`, `cancelledAt`).
* **P&L Across Phases:**
  * **Initialized / Created / Funding:** P&L is N/A or `$0` (surface any transfer or order fees clearly).
  * **In Play:** Show live/unrealized P&L (and mark-to-market value).
  * **Settling:** Show estimated/pending net proceeds and lock in gross/expected exit figures while settlement finalizes.
  * **Withdrawable & History:** Show final realized P&L, total fees/slippage, and net withdrawable USD.
* **History Views & Filters:** Even after a play is withdrawn or moved out of active views, keep the full immutable record in **History** with filters for asset class, outcome (win/loss/cancelled/error), date range, and phases visited.

### App UX & Notifications
* **Balance Buckets in the UI:** Clearly separate **Available Cash**, **Reserved in Plays** (`Initialized` through `Settling` / `Withdrawable` until withdrawn/reallocated), and **Withdrawable USD** so reserved money never looks like a missing balance.
* **Settlement & Transfer Expectations:** Because bank rails, market hours, and settlement cycles (like T+1 or options clearing) create legitimate delays in **Funding** and **Settling**, display expected completion windows or countdown/status hints so users don't mistake normal clearing for a bug.
* **Actionable Alerts:** Notify users when a play enters **In Play**, reaches **Withdrawable**, or hits **Error** (especially when user/AI intervention is needed).

---

## 4. Suggested Play Data Model (Draft)

```json
{
  "playId": "play_01J...",
  "assetClass": "options",
  "symbol": "SPY",
  "venue": "globex_or_equities_venue",
  "phase": "Settling",
  "previousPhase": "In Play",
  "reservedAmountUsd": 2500.00,
  "withdrawableAmountUsd": 0.00,
  "reinvestable": false,
  "settlementEligibleAt": "2026-11-23T14:30:00Z",
  "error": {
    "code": null,
    "message": null,
    "phase": null
  },
  "cancellation": {
    "reason": null,
    "cancelledBy": null,
    "cancelledAt": null
  },
  "timestamps": {
    "initializedAt": "2026-11-22T15:00:00Z",
    "createdAt": "2026-11-22T15:01:00Z",
    "fundedAt": "2026-11-22T15:02:00Z",
    "enteredPlayAt": "2026-11-22T15:02:05Z",
    "settlingStartedAt": "2026-11-22T19:45:00Z",
    "withdrawableAt": null
  },
  "history": [
    { "phase": "Initialized", "at": "2026-11-22T15:00:00Z", "by": "ai" },
    { "phase": "Created", "at": "2026-11-22T15:01:00Z", "by": "system" },
    { "phase": "Funding", "at": "2026-11-22T15:02:00Z", "by": "system" },
    { "phase": "In Play", "at": "2026-11-22T15:02:05Z", "by": "system" },
    { "phase": "Settling", "at": "2026-11-22T19:45:00Z", "by": "user" }
  ]
}
```

Expand on this typedef as much as needed.

There are 3 mode to set the machine to based on risk profiles: Low, Medium, and High

The 3 Risk Modes ($1,000 Single-Account Execution)

There are 3 modes to set the machine to based on risk profiles: Low, Medium, and High.

1. Low Mode (Capital Preservation & Sleep-Well Baseline)

Objective: Protect the $1,000 principal during dead chop or macro uncertainty. Steady, low-volatility growth with zero day trading.

Instruments: Fractional broad-market ETFs (SPY, QQQ), short-term Treasury yield (SGOV), high-grade bond ETFs (LQD), 1yr+ fractional blue-chip holds, and delta-neutral stablecoin/cash yield.

Aim for the same yearly return as flat investment into the S&P 500 SPY.

Play Rules:

Zero options, zero leverage, zero intraday flipping.

Rebalances weekly based on macro trend regimes and seasonal liquidity windows.

Wide trailing stops to protect against systemic crashes while letting long-term compounders breathe.

2. Medium Mode (Catalyst Swings & 24/7 Range Capture)

Objective: Build steady daily momentum ($15–$50/day average) using multi-day swings and automated 24/7 crypto/FX movement without risking account blowup.

Instruments: Everything in Low Mode, plus:

Fractional Momentum Equities: Multi-day swing Plays on high-liquidity movers reacting to earnings guidance, sector rotations, and news catalysts.

24/7 Crypto Spot & Low-Leverage Perps (2x–3x max): Automated grid and mean-reversion Plays on BTC, ETH, and SOL capturing overnight and weekend volatility when US stock markets are closed.

Forex & Micro Futures: Trend-following Plays on major FX pairs (EUR/USD, USD/JPY) and micro index/commodity futures (MES, MNQ, MCL) sized strictly to a $1,000 margin footprint.

Light options trading: Options plays that take a week or more to mature.

Play Rules:

Max risk per Play capped at 3%–5% ($30–$50) of account equity.

Mandatory hard stop-losses and 2-tier ladder exits planned before order submission.

3. High Mode (The $100/Day Velocity Machine)

Objective: Full autonomous hunting for the 10% ($100/day) target. Built specifically for high-velocity directional moves and around-the-clock market coverage.

Instruments: Everything in Medium Mode, plus:

News-Driven Long Options Plays (Calls & Puts): Buying high-volume, near-term Call or Put contracts ($50–$250 per contract) on breaking catalysts (such as sudden FAA ground stops, contractor outages, or surprise earnings shocks). Pure velocity—profiting when a stock rips up or tanks down fast, without needing 100 shares of underlying capital.

24/7 Leveraged Crypto Perps (5x–15x isolated margin): Automated breakout and funding-rate Plays running through nights, weekends, and US holidays when Wall Street is offline.

Prediction Markets & Event Contracts: Automated directional bets on Polymarket / Kalshi / exchange event contracts when newsd spots mispriced real-world probabilities before odds adjust.

Options trading, high risk: 0DTE contracts, Friday hits, anything goes.

Intraday Momentum Scalping: Fast in-and-out Plays riding confirmed whale order flow on high-beta tickers and micro-futures.

Play Rules:

Isolated Margin Only: One bad Play can only lose its allocated stake ($50–$150), never the entire $1,000 account.

Aggressive Ladder Exits: First take-profit triggers quickly to pull initial risk off the table; remaining contracts ride velocity with tight trailing stops.

Daily Kill-Switch: If daily drawdown hits 15% ($150), m24d automatically halts High Mode for the rest of the 24-hour cycle and drops remaining capital into Low Mode preservation until the next session.