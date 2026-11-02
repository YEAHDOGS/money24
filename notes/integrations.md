# Integrations

## What the paper desk actually connects to today

All read-only, by construction — the desk scaffold contains **no
order-placement code anywhere** (see [paper-desk.md](paper-desk.md)).

- **Alpaca paper data API** — stocks / crypto / options quotes. Read-only
  module; no order methods exist in it. Keys via environment variables only.
- **Polymarket public Gamma API** — prediction-market quotes, no key needed.
- **Coinbase + Kraken public tickers** — crypto quotes, no key needed.
- **Futures feed** — a **simulated** random-walk feed, every quote labeled
  `SIMULATED` so it can never be mistaken for a real price.

## Candidate integrations (owner's list, as filed)

- pump.fun
- Polymarket
- web3 (on-chain rails generally)
- Robinhood
- Webull
- MetaMask wallet
- Kalshi

This is a wish list, not a set of connections — none of these are integrated
into Money 24 today, and opening any account or connection is the owner's
call.

## Verified broker routes for the engine, when it's built

- **Interactive Brokers API** — spot FX (100+ pairs) plus stocks, options,
  futures, crypto, and cash Treasuries in one account. The consolidation
  route. See [forex.md](forex.md), [treasuries.md](treasuries.md).
- **OANDA v20 REST API** — the specialist spot-forex route (CFTC/NFA).
  See [forex.md](forex.md).
- **Webull OpenAPI** — stocks/ETFs, options, CME futures (including FX
  futures), crypto, event contracts. **No forex.** See
  [webull.md](webull.md).
- **CME** — futures across rates, FX, equity index, energy, metals, and
  volatility, reachable through any futures API above.
