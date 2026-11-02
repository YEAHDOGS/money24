# Treasuries — how an automated around-the-clock sleeve would work

**Status: candidate sleeve, not added.** Researched Nov 1, 2026. Bonds are
the strongest candidate for a seventh sleeve; adding them is the owner's
call, same as forex was.

## The honest headline

There is **no true 24/7 spot Treasury market** — not like crypto. Cash
Treasuries trade over-the-counter through dealers, and deep liquidity lives
in US hours. An "around-the-clock Treasury" sleeve decomposes into four
pieces:

## The four routes

1. **Cash bonds through Interactive Brokers — the real spot trade.** The API
   buys actual Treasuries by CUSIP: bills (weeks), notes (2–10yr), bonds
   (20–30yr), held in custody. Minimums around $1,000 par, settlement T+1,
   and the system must handle accrued interest (clean price + interest owed
   to the seller). Orders can rest overnight but fill when dealer liquidity
   shows up — realistically US and London hours, thin in the Asian session,
   dead on weekends.
2. **Treasury futures — the workhorse.** CME contracts (2yr ZT, 5yr ZF,
   10yr ZN, 30yr ZB, plus micros) trade ~23 hours a day, Sunday to Friday, on
   Globex. No CUSIP bookkeeping, no accrued interest, deep books, tiny
   spreads in US hours. This is how algos hold rates exposure around the
   clock in practice — and it rides the futures adapter Money 24 already has
   in the design. No new broker needed.
3. **Treasury ETFs — the easy proxy.** BIL/SHY (short), IEF (7–10yr), TLT
   (20yr+) trade like stocks through the stock API — and with overnight
   sessions (Webull runs one) they're close to 24/5. Less precise than bonds
   or futures, zero new plumbing.
4. **Tokenized Treasuries — the only literally 24/7 version.** Products like
   Ondo's OUSG/USDY and BlackRock's BUIDL are Treasury-backed tokens on
   crypto rails, so they move on weekends too — but they're gated (KYC,
   minimums, issuer-controlled redemption windows). A parking product, not a
   trading market.

## What the agents would do with it

- **Auto-ladder the idle cash.** Instead of capital sitting dead between
  trades, the system rolls it in a T-bill ladder — buy 4/8/13/17-week bills,
  auto-rebuy at maturity, clip roughly the risk-free rate (~3.6–4% at the
  Oct 2026 Fed funds range of 3.75–4.00%). Every idle dollar earns until a
  strategy calls it.
- **Trade the calendar.** Rates are the most scheduled market on earth — CPI
  prints, FOMC meetings, and Treasury auctions all happen at announced
  times. An agent never sleeps through an 8:30 AM release or a 1 PM auction
  tail. Duration trades (futures or TLT) sized around those events are the
  natural strategy shape.
- **Serve as the risk-off asset.** Treasuries are where the system parks
  during kill-switch events instead of going to zero-yield cash.

## The architecture stays honest

The *agents* run 24/7 — watching, queuing, laddering — while execution
follows each market's real hours, futures carrying the overnight load.
TreasuryDirect has no public API; the broker route is the only programmatic
one.
