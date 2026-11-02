# Strategy evaluations

Claims found in the wild, read in full and verified before they're believed.
The standing pattern: read the actual artifact, check the math against the
source's own numbers, give a straight verdict, file it here.

## Perkins gold / NFP post (X, Oct 28, 2026) — engagement bait

Claim: a "dispatch rider" turned $50 into $200K trading NFP news releases;
the same account quotes its own "$0.92 to $72,000 using an AI Bot" post.
The video shows XAUUSD (spot gold) sells.

**Verdict: engagement bait, nothing portable.** A 4,000× return is lottery
math, not a strategy; there is zero proof (no statements, no verifiable
track record); and the instrument itself — spot gold on a phone app — is
the offshore CFD world, which is off the table for a US resident (see
[webull.md](webull.md) and [forex.md](forex.md)). Nothing here survives
contact with the fee/spread reality of news-release trading.

## GODMODE v4 indicator reel (Instagram, Oct 26, 2026) — reference only

A TradingView BTC/USD chart with an indicator named **GODMODE v4** plus a
second overlay, **Atlas Validation**, printing green/red BUY/SELL arrows,
cut with a screen of "insider" trades users can supposedly copy. Comments
reference a ~$50 price. Filed Oct 29, 2026 as a research reference — not
bought, not endorsed.

**Evaluation:**

- **What it most likely is:** indicators of this class are
  moving-average-based signal systems in costume. The reel publishes no
  rules, no settings, and no verifiable track record — there is no testable
  spec in it.
- **The standing technical risk is repainting:** signal indicators that
  redraw historical arrows after the fact, so chart history looks prophetic
  while the live signal at bar close was different or absent. Any
  evaluation that doesn't test signal-at-close vs. signal-later is
  worthless.
- **The "insider copy" half is unverifiable as shown** — names on a screen,
  no feed, no filings. (The legitimate version of that data class exists —
  congressional/insider transaction disclosures — but this reel is not
  that.)
- Community sentiment on the reel was negative-to-skeptical.

**Why it's filed anyway:** it's a specimen of the signal layer Money 24
needs — a primary signal indicator plus a **validation overlay that gates
it**, structurally the same idea as the paper desk's ENTRY gate (currently
placeholder heuristics; see [paper-desk.md](paper-desk.md)). If it's ever
tested, the honest route is the standing one: reconstruct the likely rule
(MA-family arrows), run it on the standalone backtester with real bid/ask
and slippage, entry rule kept pure, and a **repaint check first** — record
the signal at each bar close, then compare against what the indicator shows
a week later.

## Polymarket both-sides 5-minute BTC strategy — broken as described

Full evaluation in [prediction-markets.md](prediction-markets.md): the pair
math nets zero before costs, the "sell the loser" step re-opens directional
risk at fair price, and Polymarket's taker fee at 50¢ (~3.5% per pair)
exceeds the claimed edge. The featured account's own stats fit a
maker/rebate grind instead.
