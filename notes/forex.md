# Forex — added to the pot (Oct 30, 2026)

Owner's decision, Oct 30, 2026: Money 24's AI agents trade forex
automatically too, alongside crypto, options, stocks, futures, and
prediction. The landing page was updated the same night — the market list now
reads six: CRYPTO, OPTIONS, STOCKS, FUTURES, PREDICTION, FOREX.

**Status: design addition only.** The engine build remains stopped; no broker
accounts have been opened. The paper rule applies to forex like every other
class: 60–90 days paper before live money is even discussed, and no live
trading without funded accounts plus explicit per-action approval.

## Verified routes (checked Oct 30, 2026 against broker docs)

- **OANDA v20 REST API** — the standard retail algo-forex route. US-regulated
  (CFTC/NFA). REST + streaming prices, token auth, a practice (paper) account
  spins up in minutes, no minimum deposit, fractional position sizing. Best
  first integration for a pure-forex sleeve.
- **Interactive Brokers API** (TWS API / Web API / FIX) — spot FX, 100+
  pairs, **plus** stocks, options, futures, and crypto in the same account.
  The one-integration route for nearly the whole Money 24 pot (prediction
  markets stay separate).
- **CME FX futures** (6E euro, 6J yen, 6B pound, 6A Aussie + micro versions)
  through any futures API, including Webull's futures side — exchange-traded
  currency exposure, no spot forex broker needed.
- **Webull API has NO forex.** Full breakdown in [webull.md](webull.md).

## US rules the design has to respect

- **50:1 max leverage** on major pairs (20:1 on minors).
- **FIFO closing** (NFA rule 2-43(b)): the oldest position on a pair closes
  first — position management must be designed around it.
- **No hedging** the same pair inside one account.
- Only ~5 CFTC/NFA-regulated retail forex brokers remain in the US.
- Offshore CFD brokers are off the table for a US resident.

## Gold note

XAUUSD spot is CFD-broker territory in the US. The clean Money 24 route to
gold is COMEX futures (GC / micro MGC), inside the futures sleeve.
