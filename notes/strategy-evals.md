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
## TradeCatalyst (tradecatalyst.com) — prediction-market prop challenge; real firm, hard math, payout-discretion smoke (evaluated Nov 7, 2026)

Source: an X ad link he sent Nov 7, 2026 (billboard campaign UTMs). Not a bot, not a signals service, not a download — the Nov 4-6 funnel pattern does NOT apply here. Read the actual site, its help center, launch press, and its Trustpilot page before writing this.

WHAT IT IS: "Catalyst" — a prop-firm-style challenge transplanted onto prediction markets. You pay a one-time fee to trade a SIMULATED account on live Polymarket prices. Hit the profit target inside the trade/day limits without breaching a trailing max loss, and a fixed cash payout lands in your Catalyst wallet, withdrawable immediately, no profit split. Founder/CEO is a named person (Melissa Footlick, former COO of Topstep — a real, established futures prop firm); launch press (Blockleaders, Aug 2026) reports a closed funding round and ~600 registered / ~300 completed challenges at that early stage, many via promos.

MECHANICS (help center, verified Nov 7, 2026):

| Tier | Fee | Sim account | Profit target | Payout | Payout multiple | Breakeven pass rate |
|------|-----|-------------|---------------|--------|-----------------|---------------------|
| Starter | $10 | $1,000 | $150 (+15%) | $100 | 10.0x | 10.0% |
| Pro | $59 | $10,000 | $1,500 (+15%) | $750 | 12.7x | 7.9% |
| Elite | $109 | $25,000 | $3,000 (+12%) | $1,500 | 13.8x | 7.3% |
| Max | $199 | $50,000 | $6,000 (+12%) | $3,000 | 15.1x | 6.6% |

Rules on top: 50-trade limit, 60 days, each day's profit capped at 35% of the target (so no one-day pass; minimum ~3 productive days — an anti-longshot design), a position cap (250 shares on Pro), and a TRAILING max loss (Pro: the balance may not fall more than $500 below its high-water mark — a 5% trailing drawdown against a 15% target).

THE MATH: this is the Topstep revenue model — the fee pool from failed attempts funds the payouts. The geometry is the filter: a 3:1 target-to-loss ratio under a TRAILING stop is much harder than the same numbers under a static stop, because every new high tightens your floor. On efficient-ish Polymarket prices, passing takes either a genuine pricing edge or variance luck disciplined enough to survive 50 trades. EV is negative without an edge and genuinely positive with one — it is a skill market wearing a lottery's clothes (the multiple is the marketing; the pass rate is the product).

REVIEWS — the problem, stated precisely: the homepage displays four glowing Trustpilot quotes (fast payouts, helpful named staff). The Trustpilot page itself carries positive reviews too — but its most detailed reviews are a cluster of near-identical 1-star accounts from the company's FREE World Cup / Legends Cup demo contests (Jul-Aug 2026): winner declared by email from the CEO -> KYC approved -> prize never arrived -> weeks later, a "coordinated activity" / rule-violation allegation -> appeals unanswered as of mid-August; contest pages taken offline; one reviewer reports being kicked from the Discord for asking about challenge requirements. SCOPE NOTE: every detailed denial story found concerns the free promo contests, NOT a paid challenge pass — no equivalent paid-challenge denial was found in this read. But the scope only softens it so far: the company's core pitch is "100% objective, rules don't change when you win," and the complaints allege exactly discretionary post-hoc denial. The aggregate TrustScore was not cleanly extractable from the page render; this eval characterizes the mix from the reviews actually read, not a headline rating.

NAMESAKE WARNING: a DIFFERENT product also called "TradeCatalyst" — a deposit-taking forex/crypto app ("1% cashback in Bitcoin") — is the subject of scam-warning videos (unregulated, withdrawal-fee complaints, possible impersonation). Same name, different beast. Never deposit with the app-shaped one; the challenge company takes fees only and holds no trading capital.

FIT FOR MONEY 24: (1) It is the prediction sleeve as an arena: a commercial paper-trading environment on Polymarket where passing pays real cash — conceptually adjacent to the indicator + paper-trading test bench, except the paper has stakes. A $10 Starter is the cheapest possible live-fire probe of a catalyst-reading edge (max downside $10). (2) It is NOT a copy-trading venue and does not change the strategy focus; simulated fills also ignore Polymarket book depth, so a pass proves price-reading, not executable size. (3) Automation warning: the review cluster shows the firm actively alleges "coordinated activity" — botting or multi-accounting a challenge is exactly what gets payouts denied. Human-paced, single account, or don't touch it.

STATUS: evaluated and filed at his order. No signup, no fee paid, no account — any probe (the $10 Starter is the only rational size) needs his explicit word first, per the standing no-spend rule.
