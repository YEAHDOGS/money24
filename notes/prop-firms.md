# Prop firms: the long-term capital route (Nov 9, 2026)

**The goal:** Money 24 trades funded prop-firm accounts, so a proven
strategy runs on the firm's capital instead of our own. Our risk shrinks to
the evaluation fee. This is a long-term goal, and it comes after a strategy
has passed its paper period.

## What the rules allow (checked Nov 9, 2026 against third-party summaries; confirm with each firm)

| | Our own automated strategy | Copying someone else's signals |
|---|---|---|
| Topstep | Allowed. TopstepX has an API for bots | **Not allowed**: third-party signal mirroring is banned |
| Apex | Allowed in evaluations; full automation is restricted on funded/Performance accounts | **Not allowed** from external sources |
| Funded Futures Family, Blue Guardian, Tradeify | Bans or tight limits on full bots | Not allowed |
| Most firms | High-frequency trading, latency arbitrage and hedging across accounts are banned | Copying between accounts we own is often allowed |

## What that means for Money 24

- **Copy-trading callouts cannot go on prop accounts.** The callout pipeline
  stays on our own broker accounts.
- **What can go on prop accounts:** our own backtested and news-driven
  strategies, running through the same risk gate. Futures firms (Topstep
  first, since it has an API) fit best.
- **Prop rules become risk-gate rules.** Daily loss limit, trailing
  drawdown, consistency rules, news blackout windows and no weekend holds,
  loaded per account. Breaking one means losing the account, so the gate
  checks them before every order.
- **Rules change without notice.** Store each firm's rules as a dated
  config, re-check them monthly, and get written confirmation that
  automation is allowed before going live.

## The catch

Prop firms make most of their money on evaluation fees, because most
traders fail. The edge has to be proven on paper first. Passing a
challenge with an unproven bot just pays the firm.
