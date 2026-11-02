# Prediction markets

## Venues

- **Polymarket** — the paper desk already reads it: public Gamma API,
  read-only, no key needed. Crypto-settled, runs 24/7.
- **Kalshi** — US-regulated exchange for event contracts; on the owner's
  candidate integration list (see [integrations.md](integrations.md)).
- **Event contracts on brokers** — Webull and Interactive Brokers list
  exchange-regulated event contracts: the same product family inside a
  brokerage account. See [webull.md](webull.md).

## Fee math that matters (Polymarket)

Polymarket's crypto taker fee = shares × 0.07 × p × (1−p). It **peaks at
exactly 50¢**: 1.75¢ per share per leg, ≈3.5% on a two-leg pair bought at
the mid. Makers pay no fee and share rebates (makers receive 20% of taker
fees). Any strategy design that ignores which side of that line it sits on
is fiction.

## Eval: the viral both-sides 5-minute BTC strategy — broken as described

Source: X post by Tommy Peterson (@TommyP5), Oct 30, 2026, labeled PAID
PARTNERSHIP, with a referral link to a Polymarket profile
("arcsine-barrister"). Claim: in 5-minute BTC Up/Down markets, buy BOTH
sides at 50¢ each ($1/pair); near settlement sell the losing side for
3–10¢; the winner settles at $1 — recovering $1.03–$1.10 per $1,
"direction-free," repeated across 11,460 markets for $62,784 in one month.

**Verdict: does not work as described. Three independent reasons.**

1. **The math double-counts.** A pair bought for $1.00 pays exactly $1.00 —
   zero gross edge. The entire claimed profit is step 2, selling the "loser"
   for 3–10¢. But a side trading at 3–10¢ near settlement still carries a
   real 3–10% chance of winning; selling it is cashing a live ticket at fair
   value, not harvesting free money. When the late reversal hits (constant
   in 5-minute BTC), the side you dumped pays $1 and your kept side pays $0:
   roughly −90¢ on the pair, wiping out ~15–30 good trades. Expected value
   of the recipe before costs is ~zero.
2. **Fees kill the taker version outright.** Taking both legs at 50¢ costs
   ~3.5% per pair (fee formula above) against a $1.00 guaranteed payout —
   the pair starts underwater by more than the claimed low-end edge. The
   recipe only survives as a *maker* strategy, which is a different game.
3. **The post's own evidence contradicts it.** The featured profile shows
   +$64.2K total PnL on $5.9M volume = ~1.1% of volume, not 3–10% per
   trade; the profile total doesn't match the post's $62,784; and "11,460
   markets in one month" exceeds the 8,640 BTC 5-minute markets that exist
   in 30 days (288/day) — 11,460 is the account's all-time count since
   joining May 2026.

**What the account most likely actually does:** high-volume market making /
liquidity provision — maker rebates plus liquidity rewards on $5.9M of
volume plausibly produce a steady ~1% margin. Speed-and-infrastructure
income, not a copyable trick.

**The real kernel:** binary complement arbitrage — buy both sides when
their *combined* price is under $1 — is genuine, but post-fee-rollout it
must clear ~3.5% at the mid, and a UCLA study of 75M order-book snapshots
found 7 executable episodes with a 3.6-second median life. Owned by
sub-second bots; not retail-executable.

**Money 24 takeaway:** prediction stays in the pot. The honest paper-test
candidate is a maker-side quoting / sub-$1 arb scanner on 5-minute crypto
markets **with the real fee formula modeled** — entry rule pure, per the
standing testing rule. Nothing built; the engine stop order and paper rule
are unchanged.

## Reference: 10 free GitHub repos for trading on Polymarket

Captured Oct 27, 2026 from a post by Recogard (@recogard) —
https://x.com/recogard/status/2114812867249602750 — as research input for
Money 24's prediction arm. Vet before any use; nothing installed.

1. **AutoPilotPM** — AI trading bot for prediction markets, 118 ready-to-use
   strategies. https://github.com/recogardtech/AutoPilotPM
2. **prediction-market-analysis** — largest Polymarket dataset: 36GB+, 72M+
   trades (Jon Becker, Coinbase).
   https://github.com/Jon-Becker/prediction-market-analysis
3. **prediction-market-backtesting** — backtesting simulator on real
   historical markets. https://github.com/evan-kolberg/prediction-market-backtesting
4. **polymarket_lp_tool** — manages Polymarket limit orders to maximize
   liquidity rewards. https://github.com/lihanyu81/polymarket_lp_tool
5. **polybot** — breaks down how any Polymarket trader trades: history and
   pattern analysis. https://github.com/ent0n29/polybot
6. **Prediction-Markets-Trading-Bot-Toolkits** — multi-strategy bot:
   arbitrage, copy trading, market making, whale alerts, spread farming.
   https://github.com/HarrierOnChain/Prediction-Markets-Trading-Bot-Toolkits
7. **PolyWeather** — weather-forecast + airport/METAR data bot for weather
   markets. https://github.com/yangyuan-zhen/PolyWeather
8. **polymarket-mcp-server** — Claude-powered terminal connected directly to
   Polymarket. https://github.com/caiovicentino/polymarket-mcp-server
9. **TradingAgents** — multi-agent AI trading dashboard (TauricResearch).
   https://github.com/TauricResearch/TradingAgents
10. **Awesome-Prediction-Market-Tools** — curated collection of 170
    prediction-market tools/resources.
    https://github.com/aarora4/Awesome-Prediction-Market-Tools

Why filed: Money 24's prediction arm doesn't need to be built from zero —
#9 (multi-agent analysis) and #8 (Claude terminal via MCP) map onto the
agent design, #2 is training/backtest data, #3 is the backtest-harness
pattern.
