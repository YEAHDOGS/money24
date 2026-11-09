# Trading service — copy-trading signals (design, Nov 9, 2026)

The second always-on service, next to the news service. It watches callout
sources, turns posts into signals, and paper-trades them within about a
minute. **Status: design only.** No code yet, and the paper rule still
applies.

## The rules it runs on

- **A callout is only an entry signal.** We set our own exits, ignore the
  caller's targets, and place the ladder and stop together with the entry.
- **No exits, no trade.** If the source's defaults and the signal's
  overrides add up to no stop, the trade is rejected.
- **Speed first.** The target from callout to order is 60 seconds or less.
- **Our analysis outranks the callout.** It runs in two tiers so it never
  costs the 60 seconds:
  - **Fast gate**, under 5 s, before entry. It uses cached context per
    ticker: halted, already held, earnings or a Fed event soon, hard news
    against the trade.
  - **Deep research**, after entry and asynchronous. It covers what the
    company is, why it's moving today, and matching headlines from the news
    service, then gives an AI verdict. It may tighten the stop or exit early,
    but it never delays the fill.
- **Paper only.** Webull paper first. Nothing goes live without funded
  accounts and the owner's explicit approval.

## Pipeline

```
adapters → posts → classifier → signal → fast gate → executor → position
                                                      ↘ deep research (async)
                                 exit watcher (same thread / wallet) → close
```

1. **Adapters**: one per source type, all producing the same post shape
   `{source_id, post_id, thread_id, text, images, ts}`. Wallet adapters skip
   straight to a signal.
2. **Classifier**: a regex prefilter, then an LLM with a strict JSON schema:
   `{kind: entry|exit|trim|add|noise, symbol, right, strike, expiry, price, confidence}`.
3. **Fast gate**: rejects on low confidence, on a chase past `max_chase_pct`,
   on a size cap, or on a hard conflict in the news.
4. **Executor**: resolves the contract, then sends the entry, ladder sells
   and stop in one go.
5. **Exit watcher**: acts on the source's "out", "stopped" or "trimming"
   posts, or on the wallet closing, according to the source's settings.
6. **Tracker**: records fills and slippage (called price vs filled price),
   keeps P&L per source, and auto-disables sources that keep losing.

## Source types

| Type | How we read it | Parse effort | Notes |
|---|---|---|---|
| X profile | One private X List of every tracked account, read by headless Playwright on a burner account. An Android emulator is the fallback. | LLM | Avoids the X API's cost and rate limits. Scraping breaks X's terms, so expect bans and use a burner. |
| Telegram channel | User-account API (MTProto) reading any channel the account has joined | LLM | No admin invite needed. Many callout groups live here. |
| Hyperliquid wallet | Public info API: positions, entries, fills | None | The cleanest copy data there is. Leaderboards show the top wallets. |
| Polymarket wallet | Public positions by address (on-chain) | None | Covers the prediction sleeve, including bots' wallets. |
| Solana / EVM wallet | RPC websocket | None | Full of bots, bait wallets and MEV. |
| Webhook | `POST /signals` with a shared secret | None | TradingView alerts, bots, our own scripts. |
| SEC Form 4 | EDGAR feed | None | Insider buys. Filed up to 2 business days late, so a slow signal. |
| Discord | Bot invited by a server admin, or a self-bot | LLM | Paid groups won't invite our bot, and self-bots break Discord's rules. Last on the list. |

Skip congressional trades (up to 45 days late) and 13Fs (quarterly).

### Wallet catches

- You only see one leg. A long on Hyperliquid may be hedged on Binance.
- Wallets with great records attract copiers, and some owners dump on them.
  Score wallets on months of history.
- Serious traders split across many wallets.
- Copy direction and size as a percentage of our account, never the dollar
  amount.

### Forex sources

Forex callouts are mostly scams on Telegram. The usable sources are:
- **Myfxbook / MQL5 Signals**: public accounts with broker-verified track
  records. Read their open trades and treat them like wallets.
- **The news service plus the economic calendar** (CPI, NFP, central bank
  decisions). Forex here is mostly driven by our own analysis.
- **CFTC COT report**: weekly positioning, for bias rather than entries.

Execution goes through OANDA practice, or CME FX futures through Webull.
See [forex.md](forex.md).

## Default sources

Every install ships with [engine/signals/default_sources.json](../engine/signals/default_sources.json):
about 16 callout and flow accounts on X across all six markets, 7 news
squawk accounts (these feed the fast gate and never trade on their own),
daily leaderboard pickers for Hyperliquid, Polymarket, Collective2 and
Myfxbook, insider buys from Form 4, and the webhook. Every default starts
on **probation**. It's scored on paper results and auto-disabled if it
loses. A **blocklist** holds accounts charged with pump-and-dump schemes.

Leaderboard sources choose wallets and strategies each day by their record
(minimum age, maximum drawdown, verified, no martingale), so stale
addresses are never hardcoded.

## Per-source settings (defaults)

| Setting | Default | Meaning |
|---|---|---|
| `enabled` | true | |
| `stop_loss_pct` | 20 | Stop below the entry fill |
| `ladder` | 25% at +30%, 50% at +60% | Limit sells; the rest is the runner |
| `runner_trail_pct` | off | Optional trailing stop on the runner |
| `follow_source_exit` | true | Close what's left when the source calls out |
| `follow_source_trim` | false | Mirror the source's trims |
| `follow_source_add` | false | Ignore adds by default so size stays predictable |
| `max_chase_pct` | 15 | Skip if the price has run past the called price by this much |
| `min_confidence` | 0.8 | Classifier threshold |
| `max_usd_per_trade` / `max_usd_per_day` / `max_open_positions` | set per account | Hard caps |
| `max_signal_age_sec` | 120 | Skip callouts that are already this old when we see them |
| `max_hold_minutes` | 240 | Close whatever is left after this long, win or lose |
| `time_stop` | 45 min, 0% | Not up by this point? Close: dead trades bleed theta |
| `exit_days_before_expiry` | 0 | Options: close this many days before expiry (0 = same day) |
| `exit_minutes_before_close` | 30 | Options: out this long before the expiry-day close |

Every plan carries an `exit_by` time: the earlier of the max hold and the
expiry exit. A callout that arrives after its own expiry exit is skipped.
Day-trade sources want short holds; swing sources want days. Set it per
source.

Every signal can override the defaults of its source.

## Client app

The client manages sources and settings. The chart shows the live price
with the ladder and stop drawn on it, and there's a scoreboard per source.

## Long-term: prop-firm capital

Run proven strategies on funded prop-firm accounts. Copy-trading is banned
there, so only our own strategies qualify. See [prop-firms.md](prop-firms.md).

## Open items

- Verify that the Webull API supports paper **options** with linked exit
  orders. If it doesn't, simulate the exits inside the service.
- Collect 5–10 real callout and exit posts per format as classifier fixtures.
- Decide what happens to callouts after the market closes: queue them for
  the open, or drop them.
- Stack: Go, an `engine/signals/` package run by `m24d`, per
  [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md). Its state goes in the
  runner's SQLite ledger. News context comes from `newsd`'s SSE stream.
  Runs on Windows and Linux natively, and in Docker as the `trading`
  service in the root `docker-compose.yml` (same Dockerfile, `CMD: m24d`).
  The X scraper drives headless Chrome from Go over the DevTools protocol,
  with no Node. The Telegram reader is a Go MTProto client, which needs a
  stated reason as a dependency.
- **Legal gate** (ARCHITECTURE §11): copy-trading for our own account on
  paper is fine. Copy-trading *for customers* can trigger investment-adviser
  or broker-dealer registration, so that needs a lawyer before anyone else
  gets it.
