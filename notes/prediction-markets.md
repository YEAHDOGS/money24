# Prediction markets

## Venues

- **Polymarket** — the desk already reads it: public Gamma API,
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

**Money 24 takeaway:** prediction stays in the pot. The honest desk-test
candidate is a maker-side quoting / sub-$1 arb scanner on 5-minute crypto
markets **with the real fee formula modeled** — entry rule pure, per the
standing testing rule. Nothing built; the engine stop order and the standing rule
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

## The prediction desk (paper, live): `engine/predict`, the Odds tab

Built 2026-11-21. A paper book of Kalshi event contracts at Kalshi's real, live prices, running
inside m24d around the clock. It places no order anywhere.

- **Prices:** Kalshi's public market data. No key needed.
- **Fills:** against the real order book (Kalshi lists bids only; a NO bid at p is a YES offer at
  1 − p), whole contracts, with Kalshi's taker fee `ceil(0.07 × C × P × (1 − P))` on every fill.
  A few series charge less; the desk charges the standard rate everywhere.
- **Settlement:** at the real result, $1 a contract to the winning side. Positions are held to
  settlement: a fee going in, none coming out.
- **The honest record:** every market a strategy's rules take is followed to its result, traded or
  not, as if one contract were bought at that moment's price with the fee. Per strategy: win rate,
  the win rate that breaks even, what a dollar made, and (for strategies that give chances) the
  average chance they gave, to check those chances can be trusted.
- **Files:** `predict-journal.jsonl` and `predict-strategies.json` in m24d's data folder.
  `GET /api/predict` serves the desk.
- **Live check:** `M24D_LIVE=1 go test ./predict -run Live -v` runs every default strategy once
  into a throwaway book.
- **Known gap:** markets in different events can still move together (bitcoin, ether and solana
  closing the same hour; New York, Newark and Trenton weather). One position per event does not
  catch that yet.
- **Next:** Kalshi demo orders with the keys in `.env` (`KALSHI_KEY_ID`, `KALSHI_PRIVATE_KEY_PATH`,
  `KALSHI_ENV=demo`), once a strategy's record says it is worth it.

### Strategies are plug-ins

- **The rulebook** is `predict-strategies.json` in m24d's data folder, written with the defaults
  on first start. It holds the paper capital, the daily loss limit across the desk, the desk's
  **mode** (low, medium or high: strategies riskier than the mode keep their record but buy
  nothing), and every strategy: id, kind, name, on/off, risk, how often it looks, its entry rules
  (price range, spread, volume, time left, least edge, least chance, most edge), its limits (stake
  per market, open positions, new per pass, stake per day), and the kind's own settings.
- **Edit the file** to add, remove, retune or switch off a strategy. The desk re-reads it within a
  minute. A broken file keeps the last good one running and shows the mistake on the Odds tab.
  `PUT /api/predict/strategies` replaces it too (local desk or webhook secret only).
- **One kind, many strategies:** the two weather strategies are the same code with different
  settings. Copy an entry, change its id and settings, and both run side by side with records of
  their own.
- **A new kind** is one Go file in `engine/predict` that calls `Register` from `init` with a
  `New(params)` returning a `Strategy`: `Scope()` (which series, or every market closing within N
  hours), `Prepare()` (fetch its data once a pass) and `Evaluate(market)` (the side to buy, and the
  chance it gives YES when it has one). The desk does the rest: entry rules, the record, fills,
  limits, settlement and the page.

### The strategies running now

| Strategy | Kind | Risk | What it does |
|---|---|---|---|
| Favorites near the close | `favorites` | medium | Any market closing within 6 hours: buys the side priced 85–95¢. Record kept by price band. |
| Weather: what's already recorded | `weather` | low | Only ranges the station's record has already decided, with a degree to spare: a high already past a range, a low already below one, a finished day well inside one. |
| Weather: forecast edge | `weather` | medium | Today and tomorrow: buys where its chance beats the price by 8¢ after the fee, and skips gaps over 25¢ as more likely its own mistake. |

**The weather model:** 24 US stations, the ones Kalshi's rules name (Central Park for New York,
Midway for Chicago). A day's value is the larger (for a high) or smaller (for a low) of what the
station has recorded so far, from the National Weather Service, and each of 82 forecast runs
(GEFS and ECMWF ensembles via Open-Meteo) for the hours left, each with a spread for rounding and
for the time still to come. Kalshi's weather markets settle on The Weather Company's number, which
can differ from the station's readings by a degree; the known margin is there for that.

**What the first live passes showed (2026-11-21, 10pm):** decided ranges were already priced at
98–99¢, above the known strategy's 97¢ cap: the market already prices what is known. The forecast
strategy found about 35 candidates a pass. Next for weather: learn each station's own bias against
the forecasts from the recorded results.

### The strategy menu: where else the same approach works

Same shape as weather: public data, a number Kalshi settles on, and a model that turns the data
into a chance for each range.

| Family | Kalshi markets | Data | Known side | Forecast side |
|---|---|---|---|---|
| Prices at a set time | Bitcoin, ether, solana hourly and daily ranges; S&P 500 and Nasdaq daily ranges | Coinbase live prices; option-implied volatility (the tape already records SPX chains) | Minutes before the close, price far from a range | Volatility math: the chance price ends in each range |
| Economic releases | CPI, jobs report, weekly jobless claims, GDP | Cleveland Fed inflation nowcast, Atlanta Fed GDPNow, claims history | None before the release | The nowcast against the range prices |
| Fed decisions | Rate decision, number of cuts | Fed funds futures (market-implied odds) | None | Kalshi against the futures' own odds |
| Daily counts | TSA checkpoint numbers, gas and diesel prices | TSA daily numbers, AAA daily averages, EIA weekly | Most of the week already published | Weekly patterns and holidays |
| Counted events | Posts by a public figure, box office, chart positions | Live counters, early numbers | The count already past a range | Pace models |
| Structural | Any | Kalshi itself | Results already out while the market still trades | Favorite–long-shot bias (running); Kalshi against Polymarket |

Sports and corporate-deal markets stay off the low and medium levels: too much that cannot be known.
