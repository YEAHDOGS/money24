# Money 24 — Notes

Research and design notes for **Money 24 by DOGS**: one AI trading system that
watches the markets around the clock — scan, size, risk-gate, execute, and log
every fill to a ledger. Nothing off the books.

Landing: https://money24.dogs.red — status: **In Development**.

## Status, stated plainly

- **The landing page is the only shipped artifact.** The trading engine does
  not exist yet. Its build was stopped by the owner's order on Oct 27, 2026
  ("Don't build anything yet") and stays stopped until the owner says go.
- **Paper rule:** 60–90 days of paper trading before live money is even
  discussed. The clock runs from the paper desk going live (Oct 11, 2026), so
  mid-December 2026 is the earliest the live-money conversation can start.
- **No live trading** without funded accounts *and* the owner's explicit
  per-action approval. A vague standing instruction does not authorize money
  movement.
- **No performance claims** are made anywhere. Nothing here is financial
  advice.

## Three things that must not be conflated

1. **Money 24** — the named system: a landing page and a plan. Not built.
2. **The paper desk** (`money-makers/ai-trading-desk/`) — a predecessor
   scaffold with real plumbing (risk limits, kill switch, paper ledger,
   decision journal) and **placeholder signal logic**. Its paper results
   currently mean nothing. See [paper-desk.md](paper-desk.md).
3. **The earnings-put backtester** (`money-makers/earnings-put-backtest/`) —
   a standalone, finished research artifact. Research only. See
   [earnings-put-backtest.md](earnings-put-backtest.md).

## The market sleeves

Current pot (six): **crypto, options, stocks, futures, prediction, forex.**
Forex was added Oct 30, 2026 — see [forex.md](forex.md).

Candidate, not yet added: **bonds / Treasuries** — see
[treasuries.md](treasuries.md).

Already inside futures, no new sleeve needed: commodities (gold, oil,
grains), volatility (VIX), carbon credits. Not real algo markets: real estate
(no liquid API market — REITs are just stocks) and collectibles (no
exchange-grade API or liquidity).

## Index

- [webull.md](webull.md) — Webull API: what it actually supports, what it
  doesn't (no forex), and where it fits Money 24.
- [forex.md](forex.md) — verified forex routes (OANDA, Interactive Brokers,
  CME FX futures) and the US rules the design must respect.
- [treasuries.md](treasuries.md) — how an automated around-the-clock Treasury
  sleeve would work; why "24/7 spot" is really four different things.
- [prediction-markets.md](prediction-markets.md) — venues, the fee math, a
  full evaluation of a viral both-sides strategy, and reference repos.
- [catalyst-and-filings.md](catalyst-and-filings.md) — how the system
  catches moves like AAPL's 6% day: catalyst calendar + implied move,
  a filing-diff agent that reads EDGAR faster than the institutions,
  the analyst revision cascade, and defined-risk structures.
- [integrations.md](integrations.md) — what the desk scaffold actually
  connects to today, plus the owner's candidate integration list.
- [paper-desk.md](paper-desk.md) — the predecessor paper-trading scaffold:
  agents, fail-safes, strategies, honest status.
- [earnings-put-backtest.md](earnings-put-backtest.md) — the standalone
  earnings put-buying backtest: full results and verdict.
- [strategy-evals.md](strategy-evals.md) — evaluations of strategies and
  claims found in the wild. Verified before believed.
- [research-leads.md](research-leads.md) — open research leads on file.
