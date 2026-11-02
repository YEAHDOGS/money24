# Webull — what it actually offers an automated trading system

Verified Oct 30, 2026 against Webull's own API documentation. Webull comes up
constantly in Money 24 planning, so this file exists to keep the facts in one
place — including the one big thing Webull does **not** do.

## The headline: Webull API has NO forex

- Webull's OpenAPI covers **US stocks/ETFs, options, CME futures, crypto, and
  event contracts**. That is the whole list.
- Webull is **not a licensed retail forex broker**. It displays FX quotes for
  information only; there is no spot forex trading and no forex API.
- The only currency exposure available is indirect: currency ETFs (e.g. UUP,
  FXE) traded as ordinary stocks.
- Practical consequence: any "trade forex on Webull" claim — including viral
  XAUUSD (spot gold) content filmed in phone apps — is the offshore CFD
  world, not Webull and not available to a US-resident retail account. Spot
  gold (XAUUSD) is CFD-broker territory in the US; the clean route to gold is
  COMEX futures (GC / micro MGC).

## What Webull does offer Money 24

- **Stocks/ETFs + options** via API — the same asset classes the paper desk
  already gets quotes for elsewhere; a real execution candidate when the
  engine is built.
- **CME futures** via API — this is the important one. Futures through Webull
  include **CME FX futures** (6E euro, 6J yen, 6B pound, 6A Aussie, plus micro
  versions): exchange-traded currency exposure with **no spot forex broker
  required**. Webull can therefore carry the futures sleeve *and* a
  futures-based version of the forex sleeve.
- **Crypto** via API.
- **Event contracts** — exchange-listed, regulated prediction-style contracts.
  The regulated cousin of the prediction sleeve; see
  [prediction-markets.md](prediction-markets.md).
- **Overnight trading session** for US stocks — extended-hours access that
  makes the stock/ETF sleeve close to 24/5, including Treasury ETFs (BIL,
  SHY, IEF, TLT) as a rates proxy. See [treasuries.md](treasuries.md).
- **Paper trading** — Webull markets paper accounts heavily (including fixed
  income in its paper environment), which fits the standing paper rule:
  60–90 days paper before live money is even discussed.

## Where Webull fits — and where it doesn't

| Sleeve | Webull route |
|---|---|
| Stocks / ETFs | Yes — API |
| Options | Yes — API |
| Futures (incl. CME FX futures) | Yes — API |
| Crypto | Yes — API |
| Prediction | Partial — event contracts only; Polymarket/Kalshi are separate |
| Forex (spot) | **No — not offered, not licensed** |
| Bonds / Treasuries (cash) | Not the API route; Interactive Brokers is — see [treasuries.md](treasuries.md) |

## The one-account alternative

Interactive Brokers' API covers spot FX (100+ pairs) **plus** stocks,
options, futures, crypto, and cash Treasuries in a single account — the
consolidation route for nearly the whole pot. OANDA's v20 REST API is the
specialist route for a pure-forex sleeve. Full detail in
[forex.md](forex.md) and [treasuries.md](treasuries.md).

## Standing constraints

Nothing here changes the project rules: the engine build is stopped by the
owner's order (Oct 27, 2026), no broker accounts are opened without the
owner's word, and the paper rule applies to every sleeve.
