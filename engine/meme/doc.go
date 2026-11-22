// Package meme collects public Pump.fun observations for local paper backtests.
//
// Sources verified with unauthenticated requests in November 2026:
//
//   - PumpPortal's /api/data WebSocket accepted subscribeNewToken and
//     subscribeMigration without a key and delivered both event types. The feed
//     runs at processed commitment, has no replay, and can omit name, symbol
//     and destination pool address. Launch creator is the feed's traderPublicKey.
//     Times are receipt times. Use one socket, at most 200 subscription messages
//     per second and 5,000 addresses per message. We send only two subscriptions
//     per connection. The trade streams require a funded API key and cost
//     0.01 SOL per 10,000 trades. Terms impose eligibility and non-circumvention
//     requirements and restrict copying or sharing the service and its contents.
//     https://pumpportal.fun/data-api/real-time/
//     https://pumpportal.fun/FAQ/
//     https://pumpportal.fun/legal/
//
//   - DEX Screener's /tokens/v1/solana/{mints} returns up to 30 requested mints
//     per call and allows 300 requests/minute. It covers indexed curves and
//     pools and reports price, market cap, liquidity, rolling USD volume and
//     buy/sell counts. The public latest profiles and boosts feeds also returned
//     HTTP 200 and allow 60/minute each. They track metadata and promotions, so
//     the recorder uses the token endpoint at up to 30 requests/minute. API terms allow
//     commercial and noncommercial use subject to competition, resale and
//     third-party access restrictions.
//     https://docs.dexscreener.com/api/reference
//     https://docs.dexscreener.com/api/api-terms-and-conditions
//
//   - RugCheck's single-token report and summary both returned HTTP 200.
//     The report adds holder count and concentration, authorities, per-market
//     LP lock metrics, risk flags and optional creator token history. Its LP
//     fields do not establish how much liquidity was independently burned.
//     FluxRPC documents 1 request/second without authentication. This recorder
//     sends at most one per five seconds and respects Retry-After. Bulk reports
//     need an API key. FluxRPC publishes general service terms. No separate
//     public RugCheck API data license was found.
//     https://api.rugcheck.xyz/swagger/doc.json
//     https://fluxrpc.com/docs/rugcheck/getting-started
//     https://fluxrpc.com/terms-conditions
//
//   - Pump.fun's official developer API responded API_KEY_REQUIRED. Approved
//     keys default to 2 requests/second and 50,000 per UTC day. Its terms limit
//     API caching to 24 hours. The frontend coin routes tested were
//     unavailable and Pump's site terms restrict automated access. Neither is
//     used as a source.
//     https://docs.pump.fun/reference/
//     https://docs.pump.fun/limits-and-headers/
//     https://docs.pump.fun/terms/
//     https://pump.fun/docs/terms-and-conditions
//
// Data lives in DIR/YYYY-MM-DD/records-*.jsonl (UTC), with one observation per
// line. Users can join by mint, use requested_at and at for lookup timing, and
// deduplicate events by mint and kind or by signature where present. Market
// snapshots use the deepest Solana pool with the mint as its base token. A
// successful lookup with no such pool writes a snapshot without market fields.
// A safety observation gives holders as of that check, if reported. The feed
// can omit events during outages and may report events before finality.
//
// Each observed launch and migration goes to disk regardless of capacity.
// Market sampling starts after five seconds, then runs about every minute to
// 30 minutes, every five minutes to six hours and hourly until 48 hours.
// Migrations trigger a fresh lookup and short retries while awaiting indexed
// liquidity. If a launch was missed or no longer tracked, its migration
// starts a new tracking window. At the 600-token cap,
// new launches can displace the longest-quiet ungraduated token after 30
// minutes of inactivity. Migrations can displace any ungraduated token. A
// successful empty lookup can retire an unlisted, inactive mint after 15
// minutes. The dataset marks admissions, evictions and capacity skips.
// Safety checks are attempted during the first 30 minutes as capacity allows.
// The recorder recovers open tracking windows by replaying recent files.
package meme
