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

## @xmayeth ladder bot (X post 2119813469000306722) — Polymarket 5-minute ladder market-making; real technique, marketing math, wrong game for us (evaluated Nov 9, 2026)

Source: X post he sent Nov 9, 2026 — @xmayeth (may.crypto, 39K followers, founder @xspreadmap), posted Nov 9, 2026. Read in full incl. the attached silent terminal video ("xmayeth-ladder v0.3.2") and the linked Polymarket profile. The post's only link is polymarket.com/@pspspsps5?via=edges (tracking parameter, no affiliate disclosure in the post).

WHAT IT IS: passive ladder market-making on Polymarket 5-minute BTC/ETH/SOL Up/Down markets. Resting BUY limit orders at every cent from 1c to 99c on BOTH sides at once, cancelled and reposted every ~5 seconds, pulled in the last minute; whatever fills is held to settlement — it never sells. No indicator, no model, no directional view. The author's own framing: "think about it like a bookmaker." This is the same family as the @SukantsaS eval (Nov 7): latency/liquidity grinding in 5-minute markets. It is NOT copy trading — there is no trader to follow; the resting ladders ARE the position, and a public fill print is stale the instant it appears.

THE CLAIMS: "$810 a day is the average"; the 90c+ side makes "a small but almost steady 1.5-3%"; cheap fills pay "20-30x" when the market flips; best day $3,350; "280 markets per day"; "a typical trade is $3."

THE PROFILE CHECK (pspspsps5, read live Nov 9, 2026 ~11:50 AM CST): Total PnL +$275.9K, total volume $22.7M (a ~1.2% net margin on volume), 191,220 markets traded, biggest win $1,337.01. Run rates: Past Day +$241.66; Past Week -$3,491.11 (NEGATIVE); Past Month +$12,892.72 (~$430/day). Verdict on the headline: $810/day roughly matches the account's LIFETIME average (~$275.9K over ~11 months) but not its current run rate — the last month runs at about half the claim and the last week lost money. Edge decay as competition arrives is the part the post omits. This is also a whale-scale operation ($22.7M volume); nothing about it transfers to a $100 float.

THE STRUCTURAL PROBLEM (visible in his own video): on Polymarket, an UP share + a DOWN share pay exactly $1 combined. Laddering both sides means your combined cost basis decides everything — above $1 per pair you have locked in a loss, below it a profit, and fills are adversely selected: you get filled hardest on the side that is crashing, by flow that knows more or is panicking out. His demo market #41277 ends DOWN -$10.65, and the mid-market frame shows positions costing $104.65 total (UP $14.85 + DOWN $89.80) against a max payout of $94 or $45 — negative on BOTH branches at that snapshot. The cheap-side "20-30x" fills pay $1 only if price flips back before expiry; otherwise $0. The expensive side at ~95c risks 100% to make ~5%. None of this says the technique can't work — real market makers do run it — but the edge lives in fill quality, rebate/liquidity-reward capture, and cancel/repost SPEED (his loop is ~5 seconds; professional flow is far faster), none of which a post can hand you. Sloppy numbers in the post itself: "165 orders" vs the 198 that 99 levels x 2 sides implies.

FIT FOR MONEY 24: (1) Not copy trading, not the indicator — a third thing (own-bot market making), outside the stated strategy focus. (2) Mechanically un-copyable, same lesson as SukantsaS: by the time a ladder fill is public, the price is gone. (3) Capital/scale mismatch is total: covering ~198 resting levels across 280 markets a day is a bankroll-and-infrastructure game; $100 covers a handful of levels in one market and eats the toxic fills without the speed to dodge them. (4) The funnel: the author sells nothing in the post; his bio funnels to SpreadMap (spreadmap.io), his Polymarket/Kalshi arbitrage-gap tool.

STATUS: evaluated and filed at his order. No signup, no spend, no bot. Full read archived from the Nov 9, 2026 browser report.

### @Raphael1402X — "$2 became $2.2M" CASHCAT top-trader post (evaluated Nov 9, 2026)

Source: https://x.com/Raphael1402X/status/2119866081091240093 — his send with "crypto can work going from dollars to at least hundreds."

**The post:** 9-second Dexscreener screen recording of CASHCAT/WETH (Uniswap, Robinhood chain), ~$198M market cap. The Top Traders row #1: BOUGHT $2 (16.3M tokens, 5 txns) / SOLD $2.2M (13.8M tokens, 80 txns) / PNL $2.2M / UNREALIZED -$408.4K / BALANCE 2.4M tokens. Wallet address truncated in the video ("8aa..."), never verified. Author: 57.9K followers; the post itself drew 12 likes / 1,336 views. His pinned post is the actual business: a "$50 SOL to $100,000 challenge" funneling to Telegram, with "first 2500 to drop wallets gets $50" engagement bait.

**Verdict: the row is plausibly real Dexscreener data; the implied strategy does not exist.** A $2 cost basis for 16.3M tokens means the wallet was there at the very first blocks (or received tokens at ~zero basis) — the insider/first-buyer seat, decided before any chart existed. Nobody can choose to become that row after the fact. Survivorship does the rest: this table only exists because CASHCAT ran to ~$198M; the thousands of same-day launches that went to zero have no top-trader screenshots. Note also the row's own fine print: the wallet still holds 2.4M tokens at -$408K unrealized, and its $2.2M of sells across 80 transactions WAS the exit liquidity later buyers provided.

**On his thesis (dollars -> hundreds):** that narrower claim is the defensible one, and this post is not its evidence. Dollars-to-millions here is a 1,100,000x lottery/insider outcome. Dollars-to-hundreds is a 10-100x — a different, repeatable-in-principle game (early entry + disciplined exits), consistent with the wallet-level data already on file: a minority of memecoin wallets profit (~15-20% depending on the study), the median buyer loses, and the edge sits in entry timing and exit discipline, not in screenshots of winners. If anything in this post is copyable, it is the seller's exit pattern (80 staged sells into strength), not the $2 entry.

**Filed:** Wordsmith money24 note; this entry; money-makers README source URLs. No action, no signup, no Telegram join.

### @savipww — "Grok Bot" $36.40 → $3,320.19 overnight in the fomo app (evaluated Nov 11, 2026)

Source: https://x.com/savipww/status/2120543076137521434 — posted Nov 11, 2026, 9:44 AM. Author savip. (@savipww, 17.1K followers, "deep into AI & Quant since 2022"); bio link celebex.notpeople.ai; the post pushes his Nov 1 article "My Six Figure Grok Bot Trencher Setup (Free Prompt)". Read in full via browser (post text, attached ~32s video frames, visible replies).

**The claim:** a 5-agent "Grok Bot" (Scanner, Analyst, Brain, Executor, Risk) trading his account in the fomo app (fomo.family) overnight: cut losers at -6%, take profit in parts, roll every win into the next buy. $36.40 became $3,320.19 (91.21x). The key trade: a cat coin bought at a $1.54M market cap at 12:16am because Ansem had put that cat on his profile picture, sold at $10.76M at 1:32am. The token (from his own first-reply referral link): 4CxjZyMNoF1p6eXHXHv1TWpS2X47CuUmLwtGe18spump.

**The proof offered:** a montage of in-app screens — a $36.40 deposit, terminal balances stepping up ($366.80 at 1:32am, $994.49 at 5:13am), and a "Withdrew $3,320.19 USDC" screen. NO wallet address appears anywhere — not in the post text, not in the video frames. The only address published is the token's, via a fomo link carrying his referral parameter (r=savipww): he gains when readers join through it.

**Inconsistencies inside his own materials:** the start is $36.40, yet the cat-coin buy is "$52.50 — every cent I had," with the increase unexplained; the text says 8 trades while his own terminal frame shows 12; his in-app profile stats (2.2K followers, joined Aug 2026, 70 trades) differ from his X profile.

**Verdict: NOT LEGIT as evidence — same class as @Hrundel75 (Nov 7).** The "Grok Bot inside fomo" genre proves P&L with screenshots of an app's own UI. An app screen can display any numbers, the withdrawal screen lives inside the same app, and with no wallet address there is nothing on-chain to audit: the token's price move is checkable, but it proves the token moved, not that his bot bought or sold it. The monetization is visible (article funnel + referral link + "dm open" bio). No download link or key request appears in this post, so the risk class is belief, not theft.

**Fit for Money 24:** the pitch is an X-reading narrative-chaser (it "caught" an influencer's profile-picture change), not copy trading and not the indicator — and even taken at face value, one overnight 91x run at full-bankroll sizing is a lottery path, not a repeatable system. There is no wallet to copy and no record to grade.

**Status:** evaluated and filed at his order. No signup, no referral click-through action, no spend.

## 2026-11-13 — @thetawarriorpro NVDA whale spread reel (Instagram)

Source: https://www.instagram.com/reel/DffryJ4Pv__/ — @thetawarriorpro ("Theta Warrior", full-time options trader / flow educator). The reel funnels viewers to comment "FLOW" for a free flow-setup checklist by DM — a lead magnet for his paid pipeline.

The trade shown: a whale's NVDA bull call debit spread — long 105,000 contracts of the Feb 19, 2027 $210 call ($56.6M), short 105,000 contracts of the Feb 19, 2027 $255 call ($11.2M collected), net debit $45.402M. Per-contract math from the reel's own numbers: $432.40 debit per spread, max profit $40.68/share, breakeven $214.32 at expiration, upside capped at $255.

**Verdict: STRUCTURE IS SANE, COPY IS UNVERIFIED, SIZE IS IMPOSSIBLE.** A defined-risk debit spread with a far expiry and ~9.4:1 max risk/reward is the respectable end of options copying, and options-flow copying sits inside the standing strategy family. But: (1) the minimum ticket is one contract at ~$432 — over 4x the entire $100 float, so the trade cannot be taken live at any size; (2) flow shows a print, not intent — a spread this size can be a hedge, a roll, or one leg of a larger book, and the reel's own comment thread debates opening vs closing while mocking the timing after NVDA dropped on gossip headlines; (3) the source monetizes the audience, not the trade.

**Fit for Money 24:** paper-desk candidate only — one paper contract of the identical structure, graded on the scoreboard like any other source. No live trade, no FLOW checklist, no DM funnel.

**Status:** evaluated and filed at his order (he asked "should we do this on NVDA"). No spend, no signup, no trade.

## 2026-11-13 — @julias.algos box spread borrowing reel (Instagram)

Source: https://www.instagram.com/reel/DfF8GmMyfs-/ — @julias.algos ("Wall Street Trading Concepts Made Simple"). Educational reel: sell a box spread on SPX to borrow ~$100K at ~4.6% — cheaper than a mortgage (~7%), roughly 0.4% over the 2-year Treasury (SPX box yield curve, Mar 2025). She includes the correct safety warning herself: European-style options only (SPX), never American (SPY) — early assignment risk.

**Verdict: REAL — the rare reel that is textbook-true.** A short box spread is a synthetic loan: credit received now, fixed obligation of the strike width at expiry, the difference is the interest. SPX is European-style and cash-settled, so the payoff is locked. Implied box rates genuinely print a few tenths over matched Treasuries, and the same structure run long is literally a traded product (the BOXX ETF lends via box spreads).

**The correction:** "$0 cash down" is marketing. Brokers only let you extract cash against account equity you already own — in practice this needs a six-figure portfolio and usually portfolio margin; the borrowed cash is leverage on your own assets (commonly used to get liquidity without selling appreciated stock and triggering capital gains). The loan is also short-dated and callable in effect: if the rest of the account falls, margin clerks can force-close the box at bad prices, and at expiry the loan comes due.

**Fit for Money 24:** none as a strategy — a box spread makes nothing; it borrows cheaply. Worth remembering as the legitimate financing move if a large portfolio ever exists. Not runnable at any current scale (his account is $200). No action, no spend.

**Status:** evaluated and filed at his order (he asked "is this possible?").

## 2026-11-13 — @algo.jan VWAP x EMA9 crossover scalp (Instagram) — paper-source spec

Source: https://www.instagram.com/reel/DfkBJy3vBQd/ — @algo.jan (Jan Hottiger, algo trader) reviewing another trader's day-trading system. The origin trader claims $200K+ in October 2026; the reel's own backtest (3-minute timeframe, 2 years) shows +200.1%, $20,010 net, 174 trades, 55.8% win rate, Sharpe 1.80, max drawdown 22.2%, profit factor 2.17; a prop-firm simulator reports an 85.9% pass probability on a $50K My Funded Futures challenge.

The system as stated: session-anchored VWAP + EMA9. Buy puts when VWAP crosses above EMA9; buy calls when it crosses below; stop when price breaks EMA9 or the position hits -30%. (As narrated the legs may be compressed or inverted vs the origin trader's real spec — treat the rules as a hypothesis to pin down in paper, not gospel.)

**Skeptic's read:** 174 trades over two years is very few for a 3-minute crossover system — heavy filtering or parameter fitting is likely; backtest fills ignore options spreads and slippage, which a -30%-stop scalping system pays on every trade; the $200K month is unverified; the 85.9% prop-pass figure is a simulator run on the backtest's own statistics (circular). Decisive local fact: the advertised max drawdown, 22.2%, already exceeds Money 24's own go-live gate (max DD 20% of the $1,000 paper capital).

**Fit for Money 24:** NOT copy trading — this is an own-signal indicator strategy. It fits only under the Nov 6 "an indicator with good paper trading" shape, as a new SOURCE on the paper desk: id `vwap-ema9-scalp`, starts on probation like every source, scored by the scoreboard, auto-disabled if it loses, same go-live gate (100 trades / 30 trading days / profitable after slippage / max DD 20%). Spec for the builder: instrument MES futures or SPY options (pick one per run, record which); 3-minute bars; session-anchored VWAP; EMA9; entries on cross per the rules above; exits on EMA9 break, -30% stop, or session close; every signal emitted through the existing callout/webhook contract so the journal and scoreboard grade it unchanged. Requires his explicit exception to the copy-trading-only rule (Nov 5) or classification under the indicator family (Nov 6). Muse files the spec; his Claude builds; nothing live.

**Status:** evaluated and filed at his order (he asked "can we try this with money24?"). Paper only. No build by Muse, no live trading, no spend.

## 2026-11-13 — @eliteoptionstrader2 pairs trading / statistical arbitrage (Instagram)

Source: https://www.instagram.com/reel/DeaHN_lJJkB/ — @eliteoptionstrader2 ("Brando Le", options community seller, 15,000+ members). Caption: "Don't trade stocks, trade their relationship." Claim: since 1972, buying Pepsi and shorting Coke every time Pepsi lagged would be up over 46,000%, while the reverse loses 99%. Other pairs shown: Walmart/Target, Home Depot/Lowe's.

**The strategy is real and institutional.** Pairs trading / statistical arbitrage — long the laggard, short the leader in two cointegrated competitors, profit when the spread mean-reverts — is textbook quant, pioneered at Morgan Stanley in the 1980s and documented in the academic literature (Gatev/Goetzmann/Rouwenhorst), with returns that have decayed as the trade got crowded. "Hedge funds have done this for decades" is simply true.

**The 46,000% is hindsight marketing.** A 54-year backtest on a pair chosen because it survived and stayed together is survivorship bias in a suit (his own comment section names it). Pairs break: when one business genuinely diverges — PepsiCo became a snacks company, Coke stayed drinks — the "gap" is information, not mispricing, and the convergence trade bleeds as it widens. LTCM is the permanent monument to convergence trades that didn't converge in time. Retail frictions the reel skips: short borrow fees, dividends owed on the short leg, margin for the short side, and small single-digit spread moves that need real size to matter in dollars.

**Fit for Money 24:** own-signal again (not copy trading — same open classification as the VWAP/EMA9 spec), but structurally the best-mannered candidate yet: signals computable from free price data (no Discord, no flow feed, nobody to copy), slow-moving over days to weeks (the opposite of the seconds-latency game), and market-neutral — it trades a relationship, not a headline, which suits the politics-indicator worldview. Paper-source spec if greenlit: universe KO/PEP, WMT/TGT, HD/LOW, V/MA; spread = log-price ratio, 60-day lookback; enter at |z| >= 2 (long laggard / short leader, equal dollars), exit at |z| <= 0.5, stop at |z| >= 3.5 or 30 days; graded by the scoreboard under the same go-live gate. Not runnable live at his scale: two legs, one of them short, needs margin and size.

**Status:** evaluated and filed at his order — he is collecting all strategies in this book. Paper only. No build by Muse, no live trading, no spend.
