# Catalyst & Filings — how Money 24 catches moves like AAPL, Nov 4 2026

Design note, filed Nov 4, 2026. The question behind it: a stock moves 6% in
a day — what data would have shown it coming, in time to act? The honest
answer shapes four modules of the Money 24 design.

## The case study: Apple (AAPL), Nov 2–4, 2026

- **Mon Nov 2 — the print.** Fiscal Q4 earnings: EPS $2.10 vs $1.99
  consensus, revenue $114.5B vs $113.4B expected. A beat — with a catch:
  holiday-quarter gross margin guidance of 45–46% vs ~46.3–47% expected, and
  a soft China headline.
- **Tue Nov 3 — the dip.** The stock **fell −1.9%** to $331.90. Anyone who
  bought calls for the print was underwater first.
- **Wed Nov 4 — the rip.** The stock opened **+5.5%** and ran ~6% on the
  day. The driver was not new news: it was a *delayed institutional
  re-evaluation* of the annual filing. Digging into the 10-K, analysts
  surfaced a record active-device installed base, double-digit Greater
  China growth in the details, and a strategic surge in manufacturing
  procurement commitments (securing NAND flash supply) — read as demand
  confidence and a defense against memory-chip inflation. Price targets
  followed: Baird raised its target on Nov 3 (while the stock was still
  red), Goldman set $380 and Bank of America reiterated Buy on Nov 4.

## The lesson

**No data feed shows "this will skyrocket."** Direction on a catalyst is
not knowable in advance; even the right company dipped first. What *is*
knowable, in order:

1. **When** — the catalyst calendar. Earnings dates are public weeks ahead.
   Not knowing a catalyst is coming is the only unforgivable miss.
2. **How big** — the options-implied move. The earnings straddle price ÷
   spot is the move the market is pricing (for AAPL, typically ±4–5%). The
   6% day sat inside the priced range: the market predicted the *size*
   almost exactly. It never sells you the *sign*.
3. **What the filing actually says** — the fuel for the Nov 4 move was
   public on EDGAR Monday night, ~36 hours before the stock moved. The edge
   was reading it faster and weighting it differently than the institutions
   did. That is a machine-shaped job.
4. **Who is changing their mind** — the analyst revision cascade. A target
   raise into a post-earnings dip (Baird, Nov 3) was the earliest public
   tell that the street was turning.
5. **Confirmation, not prophecy** — the Nov 4 gap up on volume is an entry
   timing signal. Chasing it means paying spiked premiums; it confirms a
   move, it doesn't predict one.

And the standing result from the project's own backtest
([earnings-put-backtest.md](earnings-put-backtest.md)): directional bets
into earnings were a coin flip (puts won 50%), while the straddle — which
buys *magnitude* and doesn't care about direction — was the best performer.
The system trades the size of the move and reacts to the filing. It does
not pretend to know the sign in advance.

## The four modules

### 1. Filing-diff agent (the star)

Watches SEC EDGAR for every watchlist name. The second a 10-K, 10-Q, or
8-K posts, it parses the full filing and **diffs it against the prior
filing**, flagging the line items that move stocks:

- Purchase / procurement commitments (the exact item that moved AAPL)
- Share repurchase authorizations and changes
- Segment revenue, especially geographic splits (e.g. Greater China)
- Installed base / user / subscriber metrics
- Inventory buildups and drawdowns
- Guidance language changes, word by word
- New risk-factor language

Output: a short alert — what changed, why it matters, timestamp — within
minutes of the filing, not 36 hours later.

### 2. Catalyst calendar + implied-move engine

- Every watchlist name's scheduled catalysts: earnings, FOMC, CPI,
  auctions, FDA dates, investor days (see also
  [treasuries.md](treasuries.md) on calendar trading).
- Flags at T-7 and T-1.
- Implied move computed live from the options chain (ATM straddle ÷ spot),
  compared against the name's historical realized moves on the same
  catalyst. Flags when implied is *below* the typical realized move
  (magnitude is cheap) or far above it (priced for drama).

### 3. Analyst revision feed

Timestamped upgrades, downgrades, and target changes for watchlist names.
Weighting rule: a revision **against** the stock's most recent move (a
raise into a dip, a cut into a rip) scores higher than one moving with
momentum — it means someone changed their mind on new information.

### 4. Defined-risk execution rules

- No naked directional earnings bets by default. Structures are spreads
  and straddles: defined max loss, sized so the coin-flip outcome is
  survivable.
- IV-crush awareness: a pre-event position must beat the implied move, not
  just pick the direction; after the event, the *digestion trade* (act on
  the filing read + first revisions) is usually the better-priced entry
  than chasing the opening gap.
- Everything still passes the SIZER and the BLOCKER risk gate, and the
  paper rule applies: 60–90 days paper before live money is even discussed.

## Data sources

- **SEC EDGAR** — free; filings full-text and submissions API. The filing
  module needs no paid feed.
- **Options chains** — broker APIs (see [webull.md](webull.md) and
  [forex.md](forex.md) on IBKR): chains, greeks, IV, and flow aggregates
  for the implied-move engine.
- **Earnings / event calendars** — broker and market-data APIs.
- **Analyst action feeds** — market-data APIs and news wires; revisions are
  timestamped public events.

## Standing constraints

Design only. The Money 24 engine build remains stopped by the owner's
order (Oct 27, 2026) until the owner says go; no accounts opened, no live
trading, paper rule unchanged.

## The middle agent — catalyst brain between newsd and m24d (design, Nov 9, 2026)

Filed from the Nov 9, 2026 discussion (his direction, prompted by NVDA's post-election drawdown: 177.44, -22.61% past month on the chart he shared). Full design in Wordsmith money24 note b78c3227; summary here.

newsd as built predicts nothing: it is a pipe (25 sources -> watchlist tags -> SQLite -> SSE). The missing piece is a 24/7 analyst in the middle — the catalyst brain:

- **Inputs:** newsd SSE stream; a catalyst calendar (elections, Fed, earnings, export-control/tariff deadlines — scheduled events are the strongest input); Polymarket/Kalshi odds as the priced-in gauge (alert rule from the politics-indicator design: outcome crosses >60% or +15pts/week inside T-14d); and the per-ticker exposure map, drafted from 10-K risk factors and timestamped BEFORE events, citation-or-drop.
- **Processing:** headline -> structured catalyst record (event type, tickers, direction, magnitude band, horizon, confidence, citations); plus a "why is it moving" explainer for post-move autopsies that grade whether a call was right for the stated reason.
- **Output:** the brain is just another SOURCE to m24d — it emits intents through the same webhook contract as a callout, so the existing parser -> risk gate -> bracket planner -> paper journal -> scoreboard pipeline scores it like any human source: probation first, auto-disable if it loses, same go-live gate (100 trades / 30 trading days / max DD 20% / $1,000 paper capital). This is the ARCHITECTURE.md Thinker tier, productized as a source.
- **NVDA worked example:** calendar flags the Nov 3 midterms; odds show the Dem sweep climbing; the exposure map has NVDA high-negative on that outcome (China export controls, tariffs) -> paper intent before Nov 3, scored against the -22.61% month after. It scores scenarios and reacts in seconds; it does not prophesy, and magnitude remains the weak half.
- **Guardrails:** paper only; live stays behind funded accounts + the owner's explicit per-action approval; forward paper results are the only clean judge.
