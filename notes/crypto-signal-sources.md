# Crypto signal sources worth backtesting

Researched 2026-11-21 (data to about 02:30 UTC on 2026-11-22). Candidates and
red flags, not winners: the backtester scores every source on its own call
history before anything follows it, and every source below starts on probation.
The proposed catalog additions are in
[engine/signals/default_sources.json](../engine/signals/default_sources.json).

## How this was checked

- **Telegram**: each channel's public preview page (`https://t.me/s/<channel>`)
  was fetched and the newest ~20 posts read. Channel searches
  (`?q=`) were used to look for loss, stop and results posts. Nothing is listed
  that did not return posts.
- **X**: x.com does not render without a login from here. Profiles and the
  newest ~20 posts came from the fxtwitter mirror API
  (`api.fxtwitter.com/2/profile/<handle>/statuses`). That is a third-party
  mirror, so treat X data as "exists and posts", not as a complete history.
- **Reddit**: could not be opened from this machine. www.reddit.com and
  old.reddit.com return a login wall or 403, the JSON and RSS routes return 403
  or 404, and the fetch tool refuses reddit.com. **No Reddit source is listed
  and none is proposed.** Re-check from a logged-in browser or the official API.
- **Discord**: not checked. Discord servers need an invite or a bot, so nothing
  here was opened. Listed as a gap, not a source.
- "Full call" means one post carries coin, direction, entry, target(s) and stop.
- Post counts are from the newest ~20 posts the preview page returned, so
  rates are rough.

## Headline findings

1. **Almost nobody posts a full, checkable call in public.** Of about 35
   Telegram channels opened, only five post entry, target and stop in the open
   on a regular basis (Mudrex Crypto Insights, MYC Signals, Crypto Signal
   Farmers, and two lookalike "free signal" funnels). The big channels
   (Binance Killers, Fed Russian Insiders, Crypto VIP Signal, Fat Pig, Raven)
   keep calls behind a VIP paywall and show only market commentary, "VIP
   FLASH SALE" posts and win screenshots in public.
2. **Public results are marketing.** Where a channel posts a monthly or weekly
   tally, the win rate excludes expired or open signals, or sums leveraged
   percentages (see Mudrex, Raven and Binance Killers below).
3. **Referral and upsell money is everywhere.** Exchange invite links (BingX,
   BloFin, Bybit, Binance) sit in the descriptions of Fed Russian Insiders,
   Crypto Inner Circle, Crypto Whale Pumps and Whale|Crypto|Signals. Free
   channels routinely withhold targets and stops and point to a paid group.
4. **Two lookalike funnels fabricate quick wins.** See "Free Crypto Signals
   (Profitable Zone)" below: the call and "completed all targets" arrive 23
   seconds apart.
5. **X is almost all commentary.** The large crypto accounts opened
   (Kaleo, Altcoin Sherpa, Ted Pillows, Capo, Pentoshi) post no entry/target/stop
   calls in their newest ~20 posts.

## Ranked shortlist

Ranked by how checkable the calls are from public posts alone. "Probation"
means it is proposed for `default_sources.json`.

### Tier A: full calls readable in public

**1. Mudrex Crypto Insights** — Telegram — [t.me/s/mudrexinsights](https://t.me/s/mudrexinsights) — probation
- 11.4K subscribers. Newest posts 2026-11-16 to 2026-11-21; about 9 trade alerts in 6 days (roughly 1.5 a day).
- Format has coin, direction, leverage, entry, one TP, one SL, expected holding time and a risk/reward ratio.
  Example (2026-11-20): `TRADE: ANT SHORT / Pair: ANT/USDT / Leverage: 4x / Entry: $91.3 / Take Profit (TP): $88.75 / Stop Loss (SL): $93`.
- **Publishes losses**: weekly "Trade Ideas Performance" posts, and a "both trades hit stop-loss" note on 2025-10-29. Week of 2026-06-22 to 06-28: 35 signals, 17 TP, 10 SL, 8 expired, "Win Rate: 63%". Week of 06-08 to 06-14: 18 signals, 10 TP, 3 SL, 4 expired, 1 active, "76.9%". Both rates equal TP/(TP+SL), so **expired signals are left out of the stated win rate**.
- Channel is run by a crypto exchange (Mudrex). Posts exchange promotions, for example a "24-Hour ANT Leaderboard Challenge" with cash and console prizes on 2026-11-20. Calls sit right beside trading-volume contests.
- Red flags: operator earns from trading volume; many alerts are on small perps (ANT, RIVER); alerts on stock and commodity perps (NVDA, XAU, CL) will not price as crypto in our engine; claims "lowest win rate was 58%" with no raw log.
- Best candidate in the list: complete fields, losses acknowledged, enough volume to reach 100 calls in about 3 months.

**2. MYC Signals** — Telegram — [t.me/s/mycryptopedia](https://t.me/s/mycryptopedia) — probation
- 14.1K subscribers. 20 posts between 2026-10-13 and 2026-10-28, mostly call and result pairs.
- Cornix-style bot format. Example (2026-10-28): `#JTO/USDT / Exchanges: Binance Futures / Signal Type: Breakout (Long) / Leverage: Isolated (50.0X) / Entry Targets: 1) 0.4645 / Take-Profit Targets: 1) 0.4688 - 30% 2) 0.4702 ... / Stop Targets: ...`. Futures with 50x leverage.
- The same feed posts bot results ("#AUCTION/USDT All targets achieved, Profit: 79.7101%, Period: 7 hr 15 min"). Losses are rare in what the channel search returned: one "Stop Target Hit ✅ Profit: 0.0%" (2024-12-18) and vague "we also had losses trading ETH and XRP" lines. Treat the public outcome feed as wins-biased and score from the calls only.
- Red flags: 50x leverage, tight take-profits on small caps, sells VIP groups via a support bot, results posted as profit percent at leverage (925.65% month claim on 2025-10-12).

**3. Crypto Signal Farmers** — Telegram — [t.me/s/CryptoSignalFarmers](https://t.me/s/CryptoSignalFarmers) — probation
- 2.87K subscribers. Posts 2026-09-29 to 2026-10-25; **nothing since 2026-10-25** (27 days silent).
- Compact calls with entry, stop and target. Example: `Long $BTW limit / entry 1.6116 / SL 1.398 / TP 1.81-2.04`. Stops sit 4% to 13% from entry on small coins; one BTC scalp (`Entry 81338 / SL 81000 / TP 1-2R`).
- **The same call is reposted days later** (BTW three times in one day, API3 reposted three days after the first post, the BTC scalp two days later). The first post's timestamp is the only honest one; the scorer must dedupe.
- Description says "over 90% win rate per week" and "No fee required to join our VIP" (the usual exchange-referral gate). No loss posts seen.

**4. Binance Killers** — Telegram — [t.me/s/binancekillers](https://t.me/s/binancekillers) — probation
- 227K subscribers, 10+ posts a day. Public posts are mostly "VIP MARKET UPDATE" analysis with price levels, VIP sales posts ("VIP FLASH SALE", "92%+ Historical Accuracy") and "SIGNAL ID: #2275 [VIP ONLY]" teasers. Signal IDs run 2183 (2026-07-17) to 2275 (2026-11-21), about 90 signals in four months, but the content is paywalled.
- Occasionally a full call is public. Example (2026-07-17): `SIGNAL ID: #2183 / COIN: $BULLA/USDT (2-5x) / Direction: LONG (Binance Futures) / ENTRY: Below 0.0089 / TARGETS: 0.0091 - 0.0093 - ... - 0.017 - OPEN / STOP LOSS: Below 0.0061 / Same pattern as other coins that pumped 10-20x`. Ladders of eight-plus open-ended targets on a micro cap.
- **Inconsistent results claims.** On 2026-05-01: "April is closed. 1035% total profit. Zero losses." Four days later (2026-05-05) the PNL Summary reads "April results: 21 trades, 16 winners, 1 loser, 3 closed at breakeven, 1 still running." Also posts "setups from October and even September are still running", which means trades with no effective stop or time limit.
- Paid VIP and a concierge bot. An independent tracker's figure (77.78% over 22 trades in March 2026 against a claimed 92%) is only a search snippet; its page returned 403 here (see trackers below).
- Red flags: microcap calls, "total profit" sums that mix leverage, claim of 90%+ lifetime accuracy over 2,080+ signals with no raw log, hype language around "the bull run", paid only.

### Tier B: free funnels with partial or suspect calls

**5. Free Crypto Signals (Profitable Zone)** — Telegram — [t.me/s/btctradingclub](https://t.me/s/btctradingclub) — probation, high suspicion
- 5.4K subscribers. Posts 2026-11-14 to 2026-11-20. Mixed: most free posts are entry-only, "Make a LONG/BUY on #HYPE/USDT from 58.90 on Futures ... Contact @primemod to enter the premium group & trade with perfect targets & stop loss levels". Occasionally a full call (`#BTC/USDT SHORT/SELL / Entry: 64900 / Target 1: 64576 ... / Stop Loss: 65387 / Leverage: 10x`).
- **Red flag, observed on 2026-11-19:** the full BTC short call posted at 15:40:08 UTC and "✅✅ #BTC/USDT SHORT has completed all the profit targets with ease" followed at 15:40:31, 23 seconds later. The next post is "$35 for lifetime access of Premium Group", "SPECIAL OFFER valid for 8 hours". The targets sit 0.5% to 1.5% below the 64900 entry, so "completing" them in 23 seconds only works if the call was posted after the fact.
- Same call posted twice back to back (msg 27892 and 27893). Linked results channel: [t.me/s/premiumclubresults](https://t.me/s/premiumclubresults) (297 subscribers, posts to 2026-11-06), which posts the same "#FREESIGNAL" template with a `@futurechief` contact, the same contact the Free Binance Futures channel below uses (this channel names `@primemod`).
- Score it, but expect the timestamps to be untrustworthy. If its "calls" are ever followed live they could lag the posting.

**6. Free Binance Futures / Bybit / OKX / Kucoin / Bitget Signals & Bot** — Telegram — [t.me/s/binancefuturesignal](https://t.me/s/binancefuturesignal) — probation, high suspicion
- 10.1K subscribers. Posts 2026-11-18 to 2026-11-20; about 14 calls in the newest 20 posts.
- Entry only. Example: `#FREESIGNAL Make a LONG/BUY on #EVAA/USDT from 4.1000 on Futures / Leverage - 5x - 10x - 25x - 50x / Contact @futurechief to enter the premium group & trade with perfect targets & stop loss levels`. No target, no stop, a leverage range from 5x to 50x. The scorer has only direction and entry to work with, so it will use the default stop policy.
- Same template as the Profitable Zone channel above, and the same `@futurechief` contact as its "results" channel. Reads as one funnel run under several names.

**7. Whale|Crypto|Signals** — Telegram — [t.me/s/WhaleCryptoSignals](https://t.me/s/WhaleCryptoSignals) — probation
- 253 subscribers; posts 2026-09-27 to 2026-10-07; **quiet since 2026-10-07**.
- Complete calls but dumped in bursts of 5 to 15 within seconds (about 9 views each). Example: `#ORCAUSDT SELL: 2.735-2.7897 / Leverage: Cross (10.00X) / TARGETS 1) 2.71312 ... 5) 2.529875+ / STOPLOSS: 2.84440`. Targets sit about 1% to 8% from entry.
- Description carries a Binance referral link (`binance.com/?ref=...`). No result or loss posts. Tiny audience.

**8. Crypto Inner Circle** — Telegram — [t.me/s/CryptoInnerCircle](https://t.me/s/CryptoInnerCircle) — probation
- 106K subscribers. Posts 2026-09-30 to 2026-10-05; **quiet for 47 days**.
- Terse spot calls with entry only and an after-the-fact "UP 45%" follow-up. Example: `EIGENUSDT SPOT BUY HERE AT 0.194` then `EIGENUSDT SPOT UPDATE- UP 45% 🔥🔥`. No stop, no target, no loss posts seen.
- Description: "Binance Futures Trading Signals", BingX invite link, contact for VIP.

**9. Wolf of Trading** — Telegram — [t.me/s/wolfoftrading](https://t.me/s/wolfoftrading) — probation
- 114K subscribers. Posts 2026-11-05 to 2026-11-19, about 1.4 a day. Posts "ideas" in natural language, with the entry implied by the current price and a stop only sometimes stated.
  Example (2026-11-19): `JUPUSDT Update: ... Filling my spot and holding for 10% - 20% minimum. with a stop loss at around 10% also.`
- Description: "Check pinned message to join FREE VIP", i.e. a funnel. No loss posts seen. Partial calls at best.

### Tier C: results published, calls paywalled

**10. Evening Trader (stats channel)** — Telegram — [t.me/s/evening_trader_stats](https://t.me/s/evening_trader_stats) — not proposed
- 46 subscribers; weekly logs 2026-08-31 to 2026-11-02 for the crypto and gold rooms (VIP and "Whale Room"). The calls themselves are in the paid rooms; the public main channel did not open at `t.me/s/EveningTrader`.
- **Shows losses.** Crypto week 3 of October: 14 signals, several `Long $COOKIE ... SMR: -1.00 R ❌`. Gold week 3: "Winrate: 40.00%, Total SMR: -2.00R". Per-trade lines in R.
- But wins are generous: `Long $TAO — TP1 | MFE: 3.34 R | SMR: 0.00 R ✅` counts as a win at zero R. Self-defined metrics (MFE, SMR). Page text says "We DO NOT guarantee anything" and invites readers to compare before joining.
- Useful as a results-audit template, not a call source.

**11. RAVEN Signals Pro** — Telegram — [t.me/s/RavenSignalsPro](https://t.me/s/RavenSignalsPro) with stats at [t.me/s/ravensignalspro_stats](https://t.me/s/ravensignalspro_stats) — not proposed
- 47.6K subscribers. Public posts are almost all VIP sales copy and result screenshots ("🔥 RAVEN VIP IS HOT RIGHT NOW", "ACCESS RAVEN VIP HERE"). Crypto and forex.
- Claims week of Nov 9–15: "+430.89%", "84.62% TP/SL Accuracy, 11 Winning Trades, 2 Stop Losses", "8 setups reached TP6". Percentages look like summed per-trade gains; the post does not say whether they are leveraged. The channel itself says "Anyone can screenshot TP6" and promotes publishing losses; its stats channel shows a forex week with a -80 pips Monday.
- Calls (entry, targets, stop) are VIP only.

**12. Fed. Russian Insiders** — Telegram — [t.me/s/fedrussianinsiders](https://t.me/s/fedrussianinsiders) — not proposed
- 108K subscribers, about 2 posts a day, since 2017. Technical analysis with conditional levels. Example (2026-11-21): `$BTC/USDT trades at $60,507 ... Reclaim $62,500 and the divergence plays out toward $68,500 with $76,000 above. Lose $56,500 and the setup fails`. Conditional triggers and invalidations are checkable, but there is no entry, size or stop.
- Every post carries a "Join our VIP" footer; description has BingX and BloFin bonus links. Third-party review: [CoinSpot](https://coinspot.io/en/best-crypto-trading-telegram/fed-russian-insiders/) says limited verification and aggressive ROI claims (a review site with its own affiliate links, so low weight).
- **Near-identical text to Bitcoin Bullets** (next entry) within three minutes of each other, same levels (62,500, 68,500, 76,000, 50,500). Looks like one content pipeline across several channel brands. Ownership not confirmed.

**13. Bitcoin Bullets** — Telegram — [t.me/s/BitcoinBullets](https://t.me/s/BitcoinBullets) — not proposed
- 94.2K subscribers; posts to 2026-11-21. Same analysis text as Fed. Russian Insiders (2026-11-21 16:01 vs 16:04 UTC). No calls.

**14. Fat Pig Signals** — Telegram — [t.me/s/fatpigsignals](https://t.me/s/fatpigsignals) — not proposed
- 40.9K subscribers; public posts 2026-06-13 to 2026-10-12 are marketing about a "Portfolio Builder" ("+38% SINCE JUNE", "Securing Over 1,100% in Aggregated Gains"). No call in public. Review sites show mixed Trustpilot scores with scam allegations that are unverified; no regulator action found.

**15. AltSignals.io** — Telegram — [t.me/s/altsignals](https://t.me/s/altsignals) — not proposed
- 26.5K subscribers; posts to 2026-11-20. Discount promos ("BLACKFRIDAY Discount") and thin chart commentary (`USDT.D ... We are expecting to see a rise from here`). Self-reported results pages. No full calls in public.

**16. Crypto VIP Signal** — Telegram — [t.me/s/CryptoVIPsignalTA](https://t.me/s/CryptoVIPsignalTA) — not proposed
- 314K subscribers. Commentary ("Bitcoin is in a continued downtrend ... The last support level is $60,000"). No entries, no stops. Claims "Best crypto Telegram channel" in its description.

**17. Binance Signals** — Telegram — [t.me/s/binancesignals](https://t.me/s/binancesignals) — not proposed
- 40K subscribers; several posts a day. Direction-only: `#IDOL BULLISH x10`, `#PIEVERSE BULLISH`. Posts the same alert twice. No entry, target or stop. Says it is "the one and only official Binance Signals channel", which is not a claim to take at face value.

**18. Universal Crypto Signals Pro** — Telegram — [t.me/s/universalcryptosignals](https://t.me/s/universalcryptosignals) — not proposed
- 19.1K subscribers; posts to 2026-03-25 (silent since). Full calls but on gold: `Swing trade - #Gold / Buy zone - 4420-4230 / Targets - 4750 4950 5310 5940 / Stop loss - 4050`. Reposts the same call. Not crypto in practice.

### Red-flag channels (do not follow, optional for scoring)

**19. Crypto Pump Club** — Telegram — [t.me/s/crypto_pumps_p](https://t.me/s/crypto_pumps_p)
- 186K subscribers. Self-described organised pumps: 2026-09-03 "THE BIGGEST PUMP WE'VE EVER SEEN ... Have your SOL ready", 2026-09-02 a "BUYING GUIDE" for Phantom/Raydium with "Adjust Slippage Settings to 50%", "1 HOUR LEFT ... today's pump", then "THIS INSIDER LAUNCH HAS FINISHED". Victory lap on 2026-09-10 about $BULLA "Profit: 6,550% on 5x". Last post 2026-10-14. **Added to the blocklist.**

**20. Crypto Whale Pumps** — Telegram — [t.me/s/whalepumpgroup23](https://t.me/s/whalepumpgroup23)
- 15.9K subscribers; posts 2025-09-30 to 2026-03-23, dormant since. Bybit referral deal ("15% Lower Fees + $20 Bonus + $500 Airdrop") and a 2026-03-23 pivot to a sports-betting tips group. No calls in the newest posts.

**21. Rocket Wallet Signals** — Telegram — [t.me/s/RocketWallet_Official](https://t.me/s/RocketWallet_Official)
- 22.4K subscribers. 2026-06-12: "Selling the channel. Dm me for more details". A channel for sale means a new owner could change what it does overnight (listicles still call it a free source that "publishes losses"; the sale notice was the newest relevant post).

**22. Margin Whales** — Telegram — [t.me/s/MarginWhales](https://t.me/s/MarginWhales)
- 10.8K subscribers. Last two posts (2026-02-05 and 2026-02-28) are the same "Best time to BUY $ETH under 2000$ ... Target is 2500-3000-5000-10k" text. No entry, no stop.

### X accounts (all verified to exist; none post explicit calls)

Profile data from fxtwitter on 2026-11-21. Newest ~20 posts read for each.

| Account | Followers | What the newest posts are | Notes |
| --- | --- | --- | --- |
| [@CryptoKaleo](https://x.com/CryptoKaleo) | 727K | One-line bullish takes, e.g. `$HMM Looks good. Send it.` | Coin and direction only; token-promotion tone ("GENERATIONAL BOTTOM IS IN FOR THE ROBINHOOD TRENCHES", contract address included). Proposed on probation. |
| [@AltcoinSherpa](https://x.com/AltcoinSherpa) | 263K | Commentary, sports bets | Bio: "Betting with" a gambling-site sponsor, plus a Telegram TA channel link. Proposed on probation. |
| [@TedPillows](https://x.com/TedPillows) | 325K | Chart and flow commentary | No entries or stops. Proposed on probation. |
| [@CryptoCapo_](https://x.com/CryptoCapo_) | 985K | Cryptic one-liners, rare posts (latest 2026-11-19) | Not proposed. |
| [@Pentosh1](https://x.com/Pentosh1) | 941K | Opinion; bio "full time larp"; latest 2026-11-04 | Not proposed. |
| [@IncomeSharks](https://x.com/IncomeSharks) | 745K | Stock and index commentary with an "indicator" promo and paid trade room link | Not proposed. |
| [@ToneVays](https://x.com/ToneVays) | 287K | Posts rarely, mostly non-trading | Already in `default_sources.json`; it opens, but the sample posts were not trading calls. |

Not found by the mirror API (404 or no data, so not listed): CryptoCred, CryptoGodJohn, CryptoBusy, CredibleCrypto, HsakaTrades, MacnBTC, SmartContracter, CryptoTony_, CryptoMichNL, and the exchange-style handles guessed for the Telegram brands (Binance_Killers, CryptoVIPSignal, AltSignalsio). A 404 here can mean a rename, suspension or a lookup miss, so it is "not verified", not "does not exist".

### Discord and Reddit

No Discord server and no subreddit could be opened, so none is listed. Reddit is
worth retrying with the official API (OAuth): r/CryptoMoonShots,
r/SatoshiStreetBets and r/CryptoTechnicalAnalysis are the obvious starting
points, and all three should be expected to be heavy on small-cap promotion.

## Independent trackers and what they found

None of these is truly independent of commercial interest (each sells a
dashboard, directory or referrals), but they replay calls rather than copy
screenshots, which is the useful part.

- **[SignalTrack](https://signaltrack.io/)** (opened): follows Telegram channels,
  captures each call at publication, replays entry, targets, stop and expiry on
  exchange candles, and says edits and deleted calls count against the provider.
  Homepage on 2026-11-22: 259 providers, 32,514 signals, 21,929 closed and
  verified, **47.6% win rate of closed signals**. The top eight providers by
  expectancy show win rates of 33.7% to 61.6% on samples of 26 to 377 signals,
  and are mostly gold and forex. It also shows "Top 3 result -15.8%" against an
  "Average result +264.1%" for the last 90 days, which I could not interpret
  reliably from the page. Per-provider pages for our candidates returned 503
  (rate limit) when tried, so none of our candidates has a SignalTrack record in
  this note.
- **[TraderStat](https://traderstat.com/telegram-signals)** (opened): a directory
  of 582 Telegram providers with outcomes by evidence type. It only ranks
  "eligible closed signals with source links and observed dates" over the same
  90 days, and mixes in owner-supplied "paid aggregate outcomes", which are not
  independent. Its own caveat: "This result is not account profitability."
- **SmartOptions** (page returned 403, so **not opened**; the figures below come
  from a search summary and need a manual check): reported a composite win rate
  of 70.5% over 549 signals in Nov to Dec 2025, 77.78% for Binance Killers over
  22 trades in March 2026 (claimed ~92%), 53.51% for WolfX Signals in Q3 2026
  (provider-reported), and 52.9% over 84 trades for CoinCodeCap (claimed ~80%).
  SmartOptions also sells affiliate links.
- **[CoinSpot reviews](https://coinspot.io/en/reviews/turbo-traders/)** (opened one):
  self-described audits, for example "Turbo Traders ... 23% Win Rate" with a
  "six-month audit" and "backtested" figures. The same page advertises its own
  "referral-based VIP access model", so it is a review site with a stake.
- **Academic and security research**: a UCL study reported by
  [TechRadar](https://www.techradar.com/pro/438-crypto-masterminds-are-responsible-for-the-majority-of-pump-and-dump-crypto-coin-schemes-globally-researchers-find)
  (opened, 2025-04-02) found 438 "masterminds" behind most pump-and-dump
  schemes, with Telegram the main tool. [CertiK](https://www.certik.com/blog/beware-the-honeypot-how-telegram-channels-used-actors-to-promote-scams)
  (opened, 2024-02-07) traced at least five Telegram channels that posted
  ordinary technical analysis for months before promoting honeypot tokens
  (about $3.2 million in losses across 10 contracts).
- **Regulators**: CFTC customer advisory ["Use Caution Responding to Messaging Apps"](https://www.cftc.gov/LearnAndProtect/AdvisoriesAndArticles/MessagingApps.html)
  (2024-10-31, from search results, not opened) warns about pump-and-dump group
  chats on WhatsApp and Telegram promising 100%, 500% or 1,000% profits. A
  September 2026 SEC action against WhatsApp "VIP" groups with AI "signals"
  claiming 98% accuracy was seen only through a secondary summary
  ([Legal 500](https://www.legal500.com/intelligence/united-states/criminal-law/sec-charges-whatsapp-driven-crypto-fraud-schemes-totaling-over-15-million),
  not opened). I found no SEC, CFTC or FCA action naming any channel above.

**What the trackers say overall:** where someone replays calls on exchange
data, closed-signal win rates land around 47% to 70%, well under the 85% to 95%
the channels advertise, and being top-ranked by expectancy does not need a high
win rate. Sample sizes are small, periods are short, and the sources disagree
with each other, so this supports "claims are inflated", not any specific number.

## Blocklist (with sources)

| Handle or name | Platform | Reason | Source |
| --- | --- | --- | --- |
| `crypto_pumps_p` ("Crypto Pump Club") | Telegram | Runs scheduled, announced pumps with buying guides and 50% slippage settings; boasts a 6,550% $BULLA result | The channel's own posts, 2026-09-02 to 2026-09-10 ([t.me/s/crypto_pumps_p](https://t.me/s/crypto_pumps_p)). Added to `default_sources.json`. |
| `MrZackMorris` | X | Charged by the SEC and DOJ in the Dec 2022 Atlas Trading pump-and-dump case | Already on the blocklist |
| AltLex, DON CRYPTON, SZ Trades, Roger's Academy, Insider Lui | Telegram | CertiK links these to honeypot-token promotion (about $3.2 million lost across 10 contracts), with months of ordinary analysis before the scam | [CertiK, 2024-02-07](https://www.certik.com/blog/beware-the-honeypot-how-telegram-channels-used-actors-to-promote-scams). Their `t.me/s/` pages return no posts, so they are not added to the JSON file. |
| Turbo Traders (and similar "turbo trading" gold-scalping channels) | Telegram | Review-site accusation, not a regulator finding: 23% backtested win rate, free signals recycled from a paid feed | [CoinSpot review](https://coinspot.io/en/reviews/turbo-traders/). Watchlist only. |

Not blocklisted, but watch: `btctradingclub`, `binancefuturesignal` and
`premiumclubresults` (fabricated-looking instant wins and a shared template),
and `RocketWallet_Official` (channel for sale).

## What to do with this

1. Start with Mudrex Crypto Insights, MYC Signals and Crypto Signal Farmers.
   They are the only ones with enough public, complete calls to reach a
   hundred scored trades.
2. Dedupe reposted calls and count only the first timestamp. Treat expired and
   still-open signals as results, not omissions, since the channels leave them
   out of their win rates.
3. For free-funnel channels that give entry only, score with the default stop
   policy and expect weak results; they are included so the scorer, not a
   human, rules them out.
4. For the paywalled big channels there is no public call history to backtest.
   The only evidence is their own marketing, which is internally inconsistent
   (Binance Killers: "Zero losses" and "1 loser" for the same month).
5. Re-check Reddit and Discord by another route, and run the tracker lookup
   again later for the candidates above.
