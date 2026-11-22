# Solana meme coins: who makes money, who loses, what a paper bot could test

Researched **Nov 21, 2026**. Sources are linked inline. Every number says where it
came from and how much to trust it. Nothing here is financial advice and no
performance claim is made about Money 24. This note is research only: no code,
no live money, no service touched.

**Short version.** Meme-coin trading on pump.fun is a fee-and-extraction
machine. Roughly 1 in 100 launches graduates, most tokens are dead within an
hour, and the fastest, best-informed actors (deployer-funded snipers, wallet
rings, sandwich bots) take value from slower buyers. The best public study of
*following* the early wallets found that a follower loses money in every
configuration tested and does no better than buying random tokens. A
retail-speed bot has no speed edge. Anything it tests must be judged on
recorded data with realistic fills and costs, and the prior for each strategy
below is **negative**.

## How the sources were checked

- Read directly: pump.fun's fee page (updated Nov 16, 2026) and Terms of Use,
  PumpPortal's docs and fee page, the SEC staff statement (Wayback capture),
  CoinGecko's pump.fun study (Wayback capture, includes the full monthly
  table), PumpFunData's April 2026 analysis, Pine Analytics, Bitquery, arXiv
  abstracts and full text where noted, RugCheck's OpenAPI file and a live
  call, DEX Screener's API docs and a live call, Jupiter, Jito, Birdeye and
  Bubblemaps docs.
- Could not load, so second-hand only: the CourtListener docket (403), the
  Solana fee docs (429), Solscan's API page (403), TechFlow (403). Where a
  claim rests on these it is marked **second-hand**.
- Dropped as lore: Medium and DEV posts, vendor blogs with no method, and
  anything I could only find as a headline.
- The dates in several newer sources (2026) are what the pages say. I did not
  cross-check them against a second outlet unless stated.

---

## 1. Base rates

### Graduation (bonding curve finished, token moves to a pool)

| Figure | Source | Window | Trust |
|---|---|---|---|
| 1.02% of 15.2M coins graduated | [Meme Coin Factories, arXiv 2609.10246](https://arxiv.org/abs/2609.10246) | Jan 2024 to Jan 2026, all coins | Peer-reviewed venue (CCS 2026 per its listing); metadata for all coins, transactions sampled |
| 0.84% (6,379 of 763,350) | [PumpFunData](https://pumpfundata.com/blog/pumpfun-graduation-rate-analysis) | April 2026, daily range 0.7 to 1.4% | Data vendor selling the same data; method is clear, not independently audited |
| under 2%; Dune puts the all-time rate near 1.4% | [Solana Compass](https://solanacompass.com/news/pumpfun-launched-42000-tokens-in-one-day-fewer-than-2-will-ever-reach-a-dex), citing The Block and a Dune dashboard | to Jun 10, 2026 | Second-hand |
| under 2% of tokens reached a major DEX | [The Memecoin Phenomenon, arXiv 2512.11850](https://arxiv.org/abs/2512.11850) | Q4 2024 | Academic |
| 0.198% (only secondhand) | a DEV Community post citing an arXiv paper | n/a | Not verified; ignore |

Read this as **about 1% per launch, stable over two years**. Two details that
matter for a bot:

- Graduation is fast. Of PumpFunData's April 2026 graduates, 62% of the
  "organic" ones bonded within 10 minutes and 85% within an hour; the median
  was 9.8 minutes. If a token has not graduated in an hour the odds fall
  sharply.
- A large share of graduates are **engineered**. 1,513 of April's 6,379 bonded
  in under 60 seconds and 735 in the same block they were created (the
  creator filled the curve alone). Those had a median of 6 unique buyers
  against 312 for organic graduates; PumpFunData treats under 20 buyers as a
  bot launch. That is about 24% of graduations that are not real demand.
  The Factories paper separately finds wash trading can raise a coin's
  graduation odds up to 10 times.
- The same PumpFunData post says standard launches graduate at the highest
  rate, "cashback" tokens at 0.65% and "mayhem mode" tokens at 0.37%.

### How fast tokens die

- [Catching the Rug, arXiv 2608.20271](https://arxiv.org/abs/2608.20271): 6.4M
  tokens over 7 months; a "vast majority" show rug-pull characteristics within
  **one hour** of launch. Detection from only the first 5 minutes of trading
  worked with gradient boosting. (Abstract-level; I did not audit the labels.)
- [From Hype to Collapse, arXiv 2603.24625](https://arxiv.org/abs/2603.24625):
  100,063 tokens on Orca, Raydium and Meteora in H1 2025; **76,469 labeled rug
  pulls** by a behavior-based pipeline (audited false-positive rate 0.26% on
  382 samples). Lifecycles "extremely short". Note this is non-pump.fun pools.
- [Solidus Labs](https://www.soliduslabs.com/reports/solana-rug-pulls-pump-dumps-crypto-compliance):
  of 7M+ pump.fun tokens with at least five trades (Jan 2024 to Mar 2025),
  only about 97,000 kept more than $1,000 of liquidity; it calls 98.6% "pump
  and dump". **Treat the "98.6% fraud" label as a vendor headline**: the
  measure is "liquidity fell under $1,000", which is not the same as fraud.
  CoinDesk reported pump.fun disputed it.
- [Solana Compass](https://solanacompass.com/news/pumpfun-launched-42000-tokens-in-one-day-fewer-than-2-will-ever-reach-a-dex),
  citing Dune (second-hand): only 96 pump.fun tokens ever reached $1M market
  cap and only 18 reached $10M.

### What share of traders profit

The best series is [CoinGecko's study](https://www.coingecko.com/research/publications/pump-fun-traders-are-making-a-comeback)
(updated May 7, 2026; read via [Wayback capture](https://web.archive.org/web/20260910141325/https://www.coingecko.com/research/publications/pump-fun-traders-are-making-a-comeback)).
It uses **realized PnL only**, nets all trades per wallet per month, does not
filter bots or wash trading, and USD prices on illiquid tokens can be wrong.

| Month | Active wallets | % profitable |
|---|---|---|
| Nov 2024 | 2.70M | 38.8% |
| Mar 2025 | 3.11M | 34.1% |
| **Jun 2025 (low)** | 5.22M | **30.1%** |
| Dec 2025 | 1.80M | 44.0% |
| Feb 2026 | 2.64M | 56.8% |
| Mar 2026 | 3.35M | 70.0% |
| **Apr 2026** | 3.14M | **73.3%** |

Distribution in April 2026 (the same study): 65.1% of wallets made between $1
and $500; 2.8% made $500 to $1,000; **5.4% made more than $1,000**. On the
loss side, 25.2% lost $1 to $500 and 0.8% lost more than $1,000. Monthly
active wallets fell from a 5.2M peak (May 2025) to 1.8M (Dec 2025), which
CoinGecko reads as the unprofitable crowd leaving.

Cautions that decide how to read this:

- Realized-only means a wallet that never sells a dead token shows no loss.
  CoinGecko says so itself. A 73% "profitable" month and a platform where most
  tokens go to zero can both be true.
- Un-filtered bot and wash volume can inflate the profitable count. The
  Factories paper (below) finds wash trading in at least 17% of sampled
  transactions.
- An older, different cut: "3% of users have ever earned more than $1,000"
  ([Solana Compass](https://solanacompass.com/news/pumpfun-launched-42000-tokens-in-one-day-fewer-than-2-will-ever-reach-a-dex),
  citing Dune; second-hand, lifetime not monthly). The two are not comparable.
- I did **not** find a clean, sourced "top 1% of wallets took X% of profit"
  figure. The closest hard concentration number is on the creator side: in the
  Factories paper, the **top 1% of creator groups made 58.6% of all coins**.

### Who is on the other side (late buyers)

[Early-Buyer Groups and Fast-Wallet Teams, arXiv 2610.39458](https://arxiv.org/abs/2610.39458)
(Oct 29, 2026; complete record of 200,443 launches, 15.9M curve trades over
88.3 hours): known early-buyer group members buy earlier and **extract more
cash than a matched placebo set, while late buyers' net cash flow is
negative**. Small window and one dataset, but it is the cleanest direct
measurement found.

---

## 2. Who wins, and can a slower bot take part

| Actor | How it works | What it needs | Can a slow retail bot join? |
|---|---|---|---|
| **Same-block snipers, deployer-funded** | Deployer funds wallets that buy in the creation block, then sell into the first real buyers | Pre-signed transactions, off-chain coordination, insider control of the token | **No.** This is the insider. |
| **Bundled launches** | Many wallets buy in the first block; supply looks distributed but is one party's | Same as above | **No.** A bot's job is to *detect* these and stay out. |
| **Dev / creator wallets and serial creators** | Create, promote, sell | Many wallets, funding hygiene, social reach | **No.** Detect and avoid. |
| **KOL call groups and paid promotion** | Caller holds early supply, posts, sells into the follow-on buyers | An audience | **No**, and following the call is the exit-liquidity trade. |
| **Copy-trading "smart money"** | Mirror a profitable wallet's buys | Low latency, a wallet whose edge persists | **Barely.** See evidence below. |
| **Volume / wash bots** | Self-trade to create fake volume and trending status | Capital, automation, direct on-chain access | **No**, and platform terms bar it. |
| **MEV / sandwich bots** | Buy before and sell after a victim's trade | Block-engine access, bundle tips, low-latency infra | **No.** A slow bot is the victim. |

Evidence behind the table:

- **Snipers and insiders.** [Pine Analytics](https://pineanalytics.substack.com/p/exit-liquidity-machines)
  (spring 2025; the post's exact dates were not stated in the text I read): over **50% of pump.fun tokens were sniped in the creation block**
  (mostly bots). On a high-confidence subset with a provable deployer-to-sniper
  funding link, more than 15,000 SOL of realized profit in a month across
  15,000+ launches, 4,600+ sniper wallets and 10,400+ deployers, with 87% of
  snipes profitable. That window is old; the pattern is the point.
- **Wallet rings and teams.** The 2610.39458 paper found 23,710 coordinated
  wallet groups. A few pools of fast wallets are associated with a 2 to 4
  times higher chance of the curve reaching 20 SOL, with little excess at 60
  SOL and **none for graduation**. Many signals arrive when the curve already
  holds 20 SOL, and the signal adds nothing beyond early curve state.
- **Manipulation at scale.** [Meme Coin Factories (arXiv 2609.10246)](https://arxiv.org/abs/2609.10246),
  15.2M coins over two years:
  - wash trading in at least 17% of sampled transactions;
  - the top 1% of creator groups made 58.6% of coins;
  - at least 1.5M coins (over 10%) copy the name, symbol, description and
    image of an original, and the originals do much better;
  - 23.5% of coins are created after a Twitter or Truth Social post, and 31
    such posts each helped their coin creators earn at least $1M;
  - "market-manipulation-as-a-service" tools exist, and the actors bypass the
    web interface and trade directly on-chain at low latency.
  - A companion study, [arXiv 2507.01963](https://arxiv.org/abs/2507.01963)
    (34,988 tokens, four chains), found 82.8% of tokens with returns over
    100% showed signs of artificial growth.
- **Serial exits.** [arXiv 2610.41597](https://arxiv.org/abs/2610.41597) finds
  repeated team-associated selling around collapses, mostly by a single
  creator wallet, but says the sign of the teams' profit is **fragile**
  (changes with the screen, group definition and about 2% per-side costs) and
  it cannot identify fraud prevalence. A useful caution against over-claiming.
- **Copy trading.** In the 2610.39458 paper, a follower entering at the
  earliest group signal lost money **out of sample in all 32 configurations**,
  between about −4% and −11% mean net return per trade, with 24 to 35% winning
  trades (take profit 1.3x, stop 0.7x, max hold 60 s, latency 1 to 3 s), and
  did **no better than random entry** (−7.8% vs −7.9% for random under one
  cost convention). The paper also documents evaluation mistakes (ignoring
  your own position, label leaks) that make the same data look profitable.
  A separate paper, [arXiv 2601.08641](https://arxiv.org/abs/2601.08641)
  (Jan 2026), says copy trading is the dominant meme-coin entry method and is
  exploited by manipulative bots that front-run copiers; its LLM multi-agent
  defense reaches about +3% average copier return per trade, which is the
  authors' own method on their own test, not an independent result.
- **Sandwich bots.** [Bitquery](https://bitquery.io/investigations/solana-sandwich-attacks)
  (checked Oct 11, 2026): 6.85M sandwiches between Oct 30, 2025 and Oct 7,
  2026 by 2,545 bot wallets against 2.15M wallets, **$29.7M taken before
  costs, 96% on pump.fun coins**; in a checked sample bots kept 68% after fees
  and tips; 39% of the take came from other bots. On pump.fun coins, Jupiter
  swaps allowing **20 to 50% slippage were sandwiched 14 times as often as
  those under 3%**. One worked example: a $240 buy filled 6% worse. Bitquery
  is a data vendor; the transaction-level method is shown. The academic view
  is [Quantifying the Threat of Sandwiching MEV on Jito (IMC 2026)](https://dl.acm.org/doi/10.1145/3730567.3764493).

**Bottom line for a slower bot:** there is no seat at the winners' table. The
only role it can play is a selective buyer that avoids the traps and sells
before the dump, and even that role has to beat roughly 3.5% round-trip costs
(section 5).

---

## 3. Scam patterns a bot can detect before buying

| Check | What it catches | Notes on pump.fun |
|---|---|---|
| **Mint authority and freeze authority** | Creator can print more supply or freeze your tokens | [Helius](https://www.helius.dev/docs/orb/explore-authorities) says pump.fun sets mint authority to null. So on pump.fun this check is mostly a no-op; it matters for tokens launched elsewhere. The Solana rug study ([2603.24625](https://arxiv.org/abs/2603.24625)) names freeze-authority abuse as one of three on-chain rug patterns. Verify on-chain per token, do not assume. |
| **Liquidity not locked or burned, or removable** | Liquidity withdrawal rug | On the bonding curve the liquidity is the curve; the risk is on non-pump.fun pools and after migration. I did not verify who holds LP tokens for PumpSwap canonical pools: **check on-chain before relying on it**. RugCheck exposes `lockers` for LP vaults. |
| **Top-holder and creator concentration** | Insiders hold the supply | Exclude the bonding-curve and pool accounts when counting. Bubblemaps-style cluster maps show linked wallets. |
| **First-block / bundled buys** | Deployer-funded snipers (Pine: over 50% of tokens have a same-block buyer, so a raw "any sniper" flag is too noisy; use *funded by the deployer* and *share of supply*) | Solana Tracker's [first-buyers API](https://www.solanatracker.io/resources/pumpfun-first-buyers-sniper-api) labels snipers, insiders and bundlers; RugCheck has an insiders graph. |
| **Creator wallet history** | Serial creators and prior rugs | Top 1% of creator groups make 58.6% of coins (Factories). Group creators by funding source, not by address. |
| **Copy-paste names and images** | Copycat coins that lure buyers of a real trend | At least 10% of coins copy an original; originals win. Compare name, symbol, image and description to recent launches. |
| **Sudden dev sells** | The exit itself | Watch the creator wallet and its funded wallets; exit on first sell. Most serial exits are one creator wallet (2610.41597). |
| **Too-few buyers / same-block graduation** | Engineered graduation | PumpFunData: under 20 unique buyers, or bonded in the creation block, is almost always a bot launch. |
| **Wash-trading signature** | Fake volume that leads the trending list | Buy/sell pairs by the same wallet inside seconds (Factories heuristics; the share flagged depends heavily on the time window used, from 8% up to 88%, so calibrate on your own data). |

### What the free tools expose

Tested on Nov 21, 2026 where stated.

- **RugCheck**: public API at `api.rugcheck.xyz`. The OpenAPI file declares no
  auth scheme and `GET /v1/tokens/{id}/report/summary` answered with no key.
  Endpoints include `report`, `report/summary`, `insiders/graph`,
  `insiders/networks`, `lockers`, `stats/new_tokens`, `stats/rugs/ticker`.
  Free to call; rate limits are not published in the spec, so cache and be
  polite. Its score is a heuristic, not ground truth.
- **DEX Screener**: no key. Official API docs list **60 requests/min** for
  profile, boost and meta endpoints and **300 requests/min** for pair and token
  lookups. Live call returned 30 recent profiles, 27 Solana. It gives
  liquidity, volume, trade counts, price change and pair age. It does **not**
  give holder, authority or insider data.
- **Birdeye**: free "Standard" tier is 30,000 compute units at 1 request/s
  with limited data; Lite is $39/month for 2.5M CUs at 15 rps
  ([pricing](https://docs.birdeye.so/docs/pricing)). The free tier is
  marked "limited" data access; I did not check which endpoints it covers.
- **Bubblemaps**: the data API needs a key with monthly credits from their Pro
  platform ([auth docs](https://docs.bubblemaps.io/data/api/authentication)).
- **Solana Tracker**: has risk-score, sniper and insider data; signup is
  required for a key (free keys are advertised). I did not confirm free-tier
  limits.
- **Solscan**: its API page would not load here; treat a Solscan Pro API as
  needing a key. Free explorer pages are fine for manual checks.
- **Helius Orb** and any RPC: the authority and holder data is on-chain, so the
  strongest free check is to read it yourself from an RPC.

**Recommendation:** do not depend on a score from someone else. Read on-chain
facts yourself (authorities, creator, first-block buyers, funding sources,
holder shares), use RugCheck and DEX Screener as a cross-check, and **record
every check result with a timestamp** so each decision can be replayed.

---

## 4. Candidate strategies for a paper system

For every one: record first, then replay with an **exact-fill model** (the
bonding curve and PumpSwap pool are deterministic constant-product pools, so
fills, price impact and the fee schedule in section 5 can be computed exactly),
add 1 to 5 seconds of latency, and apply fees on both legs. The 2610.39458
paper is the template for the pitfalls: it shows how ignoring your own
position and label leaks turn the same data into apparent profit.

**S1. Buy organic graduates that pass safety checks.** Buy shortly *after*
migration to PumpSwap if the token has at least 20 unique buyers, did not bond
in the creation block, passes the authority, concentration and funding checks,
and the creator has not sold. Needs: migration event stream, buyer and
holder data at decision time, pool state. Backtest: replay recorded graduations
with fills at pool prices plus latency. Failure modes: edge gets arbitraged
before a slow bot arrives; fees of 1.2% per side on fresh graduates; late
buyers are net negative in the data above; the safe-looking graduate that is
still a pump.

**S2. Avoid-everything-that-fails and measure the avoided losses (shadow
book).** Not a trade: for every launch that a filter rejects, keep a shadow
position and mark it to market under the same exit rules. The result is how
many dollars each rule saved (and how much it cost in missed winners).
Needs: only recorded data. Failure modes: label leakage (a filter that looks
at information that only existed after the dump), and survivorship (tokens
whose data is missing). This is the cheapest and most honest test and builds
the dataset the others need.

**S3. Momentum with hard time stops.** Enter on a defined, pre-registered
momentum trigger on an organic graduate and exit on the first of: target, stop,
or a short time limit (minutes, not hours, given the lifecycle data). Needs:
second-level price and volume snapshots. Failure modes: the paper's own
take-profit 1.3x / stop 0.7x / 60 s policy lost money even with random entries
under realistic costs, so costs plus latency are the whole game. Choose the
rule on an earlier slice and test on a later one; do not tune on the test
window.

**S4. Follow wallets with verified long records.** Needs: a wallet list with a
multi-month verified record of *realized* profit across many tokens, a live
feed of their buys. Backtest: replay their historical buys with your latency.
Failure modes: latency (the follower enters after the early buyers pushed the
price), persistence of the edge, wallets that are insiders cashing out on
followers, and the 2610.39458 result (negative in 32 of 32 configurations).
The 32-of-32 result followed coordinated early-buyer *groups*, not hand-picked
wallets with long records, so it is close evidence, not identical evidence.
Worth running **to confirm or refute the negative result with our own data**,
not because the prior is good.

**S5. "Fade the first spike."** On spot meme coins there is no way to short.
Pump.fun's perpetuals run through a third party and the Terms say they are
**not available to persons in the United States**; the same goes for its
prediction-markets feature. So "fading" can only mean *not buying or selling
a position you already hold*. That is S2. Do not build a short.

Things that would make any of these look better than they are: testing only on
tokens a collector happened to see (a [cross-pipeline study, arXiv
2609.18975](https://arxiv.org/abs/2609.18975) found two pump.fun collectors
overlapped on well under 1% of mints), ignoring your own price impact, using a
price series built from trade amounts (also flagged in 2610.41597), and
treating fills at the "price you saw" as achievable.

---

## 5. Costs and mechanics

### Fees (verified on [pump.fun/docs/fees](https://www.pump.fun/docs/fees), last updated Nov 16, 2026)

- Create a coin: no platform fee. Graduation to PumpSwap: 0.015 SOL per coin
  (taken from the curve for SOL pairs, not from the buyer unless their trade
  triggers migration).
- **Bonding curve:** 1.25% total per trade (0.30% creator, 0.95% protocol).
- **PumpSwap canonical pool**, SOL pairs, by market cap in SOL:
  0 to 420 SOL: 1.25%; 420 to 1,470 SOL: 1.20%; then falling 0.05 point per
  step to 0.30% at 98,240 SOL and up (creator fee shrinks, protocol 0.05%, LP
  0.20% fixed from 420 SOL). Non-canonical pools: 0.30%.
- Using today's SOL price (about **$69**, CoinGecko API, Nov 21, 2026) and the
  $69k to $100k graduation range quoted by Solana Compass, a fresh graduate
  would sit around 1,000 to 1,450 SOL of market cap, so roughly the **1.20%**
  tier. (Derived by me, not stated by pump.fun; check live.)
- Older sources saying PumpSwap is 0.25% total are out of date.

### Third-party API fees

- [PumpPortal](https://pumpportal.fun/fees): **0.5%** per Local trade, **1%**
  per Lightning trade, both before slippage, excluding network and pump.fun
  fees. Since May 1, 2026, `subscribeNewToken` and `subscribeMigration` are free
  and trade streams are metered at 0.01 SOL per 10,000 trades.
- [Jupiter Swap API v2](https://dev.jup.ag/docs/swap/): `/order` returns an
  assembled transaction, `/build` raw instructions; keyless use is limited to
  0.5 requests/s, a key is needed for production. A referral fee is optional.
- Jito: a block-engine tip is paid per bundle. The live tip-floor endpoint
  (Nov 21, 2026) showed a landed median of about 0.000002 SOL (about $0.0001)
  and a 99th percentile under 0.00001 SOL. That is the all-bundle median;
  contested launch trades pay far more.

### What a trade costs at $10, $50, $100

Assumes a bonding-curve or fresh-graduate trade through PumpPortal Local, SOL
at $69:

| Size | pump.fun 1.25% | PumpPortal 0.5% | One-way | Round trip |
|---|---|---|---|---|
| $10 | $0.13 | $0.05 | $0.18 (1.75%) | $0.35 (3.5%) |
| $50 | $0.63 | $0.25 | $0.88 | $1.75 |
| $100 | $1.25 | $0.50 | $1.75 | $3.50 |

Add: the network fee (base fee is per signature and tiny; the Solana fee docs
returned 429 so I could not re-verify the exact lamport figure), a priority fee
(PumpPortal's own examples use 0.00001 to 0.005 SOL, which is $0.0007 to
$0.35), and about 0.002 SOL (about $0.14) to open a token account, refunded
when it is closed. At $10 the fixed costs alone are material. **A strategy
needs to clear about +3.5% gross before it breaks even**, plus price impact
and any sandwich loss (6% on Bitquery's worked example). The 2610.39458 paper
uses about 2% per side as a conservative cost convention, which matches.

### Slippage

Set it tight. The only hard number found is Bitquery's: 14 times more
sandwiches on 20 to 50% slippage settings than under 3% on pump.fun coins. A
tight limit means failed trades instead of bad fills, and failed trades are a
cost a paper model must count.

### How a bot would trade for real later

1. Get an **unsigned** transaction: PumpPortal `POST /api/trade-local` (fields
   `publicKey`, `action`, `mint`, `amount`, `denominatedInSol`, `slippage`,
   `priorityFee`, optional `pool` such as `pump`, `pump-amm`, `raydium`, `auto`)
   returns a serialized transaction; or Jupiter `/order` or `/build`.
2. **Inspect and simulate it locally** (programs touched, accounts, amounts,
   the transfer out of the wallet) and reject anything unexpected.
3. **Sign locally** with a dedicated wallet and send through our own RPC.
4. Prefer this over PumpPortal's Lightning API, which sends on your behalf with
   an API key and so is a custody decision. Their own docs sample puts a
   private key inline in code; do not copy that.

### Wallet security rules

- A **dedicated hot wallet** used only by this bot, never the owner's main
  wallet and never a wallet holding anything else.
- Fund it with a **hard cap** (the live-trading size limits the owner sets),
  top up by hand, sweep profits out by hand.
- Private key only in the OS secret store or an untracked file; never in git,
  never in `.env` committed, never in the browser or a client bundle, never
  pasted into chat. (The repo rules already say no credentials in the repo.)
- Allowlist the programs the bot may sign for (pump bonding curve, PumpSwap,
  the token and system programs, the chosen router). Reject every other
  program, any unexpected signer, and any transfer to an unknown address.
- Per-trade, per-token, daily-loss and open-position limits enforced
  *before* signing; a kill switch that stops signing; every signed transaction
  written to the ledger.
- Pin or review the third-party API's behavior: the service that builds your
  transaction can return anything, so the local checks above are the defense.
- Separate staging and production keys. Live money needs the owner's explicit
  per-action approval and the 60 to 90 day paper rule in
  [README.md](README.md).

---

## 6. Legal and platform risk

All of this is research, not legal advice.

**The main case: Aguilar v. Baton Corp d/b/a Pump.fun (S.D.N.Y. 1:25-cv-00880).**
[Docket](https://www.courtlistener.com/docket/69593359/aguilar-v-baton-corporation-ltd-dba-pumpfun/)
(not readable from here). Class action filed Jan 2025; amended complaints added
RICO claims and defendants from the Solana ecosystem. Per
[SolanaFloor](https://solanafloor.com/news/all-claims-against-solana-labs-and-foundation-dismissed)
(**second-hand; SolanaFloor says it is a subsidiary of Jito Network, a former
defendant's affiliate, so read with that in mind**), Judge Colleen McMahon's
79-page ruling on **Aug 31, 2026**:

- dismissed all claims against Solana Labs, the Solana Foundation and the named
  executives;
- dismissed the Securities Act claims for $FRED and $GRIFFAIN (no "common
  enterprise" under Howey) and the other 18 tokens for lack of class standing;
  it did *not* hold memecoins can never be securities;
- held that buying and selling memecoins is not "betting" under New York law;
- **let the RICO and RICO-conspiracy claims against Baton and its three
  founders proceed** (wire-fraud allegations adequately pleaded) into discovery;
- ordered the plaintiffs to explain why 25 unidentified "Lead KOL" defendants
  should not be dismissed (deadline Sept 10).

Plaintiffs' loss estimate is $4B to $5.5B; that is an allegation, not a
finding. Check the docket before citing any of this anywhere.

**SEC.** The Division of Corporation Finance's [staff statement of Feb 27,
2025](https://www.sec.gov/newsroom/speeches-statements/staff-statement-meme-coins)
says the meme coins it describes are akin to collectibles and their offer and
sale does not involve securities, so buyers are **not protected by the federal
securities laws**. It also says the statement is a staff view with no legal
force, and that fraud in connection with meme coins can still be pursued by
other federal and state agencies. I found no SEC enforcement action over
pump.fun itself.

**Promotion and "insider" cases.** The LIBRA case is the clearest example of
promoter risk: an Argentine judge dismissed the criminal case, and the US
class action was dismissed with prejudice by Judge Rochon (S.D.N.Y.), per
[Unchained](https://unchainedcrypto.com/judge-shuts-libra-case-against-javier-milei-ruling-the-memecoin-facts-fit-no-crime/)
and [Global InDepth](https://globalindepth.com/federal-judge-dismisses-milei-endorsed-libra-memecoin-lawsuit-with-prejudice/)
(second-hand; dates not verified against dockets). Wikipedia reports a
principal admitted to "sniping" two tokens; I did not check that against the
record. The lesson for us: dismissal in court does not mean retail got money
back, and courts are slow to treat meme-coin losses as securities or gambling.

**Platform terms (pump.fun [Terms of Use](https://www.pump.fun/docs/terms-and-conditions), updated Nov 16, 2026).**

- The core platform's "Prohibited Jurisdiction" list is Cuba, Iran, North
  Korea, Syria, Russia, Belarus, the occupied Ukrainian regions, any country
  under comprehensive sanctions, and anywhere pump.fun designates. **The
  United States is not on it for the core platform.** I found no US ban on
  buying and selling coins.
- **US persons are barred from** the perpetuals feature (run through
  Hyperliquid), the prediction-markets feature (Polymarket), and the purchase
  of bridged assets that track securities. They are also named on
  Polymarket's restricted list reproduced in the Terms.
- The Terms prohibit market manipulation including wash trading and pump and
  dump, and contain a **class-action and jury waiver (27.4)** with mandatory
  arbitration.
- pump.fun blocked UK users in Dec 2024 after an FCA warning, per
  [Decrypt](https://decrypt.co/295355/solana-pump-fun-bans-uk-users-regulator?amp=1)
  (second-hand). Platform access can change with one terms update.

**What makes US use risky, plainly:**

1. **No federal investor protection.** The SEC staff view says meme-coin
   buyers do not get securities-law protection, and the court dismissed the
   securities claims it reached. Losses from a rug are mostly unrecoverable.
2. **Platform and interface risk.** Terms can change at any time; a feature or
   geography can be cut off; the RICO case against the operator is in
   discovery. This does not stop use today.
3. **Conduct risk for a bot.** Wash trading, coordinated sells, bundling
   with a deployer and front-running are the behaviors the research and the
   complaints call fraud or manipulation. A paper system that only *buys and
   sells its own small positions* is a different thing, but any design that
   creates volume, funds snipers for a creator, or acts on non-public deployer
   information would be on the wrong side. Do not build those.
4. **Promotion.** Posting calls for tokens we hold is the KOL pattern; keep
   the system's signals private to the owner and never promote.
5. **Records and tax.** Every swap is a disposal event and a bot makes
   thousands. Not researched here; the ledger already records fills.
6. **Sanctions and screening.** pump.fun screens wallets against sanctions
   lists; a flagged wallet can be excluded.

---

## What Money 24 should build first

Paper only, Go for the recorder and replay, no code is requested here. In
order:

1. **A recorder, before any strategy.** Capture *every* launch, not a filtered
   subset (the cross-pipeline paper shows the collector decides what you see).
   Use PumpPortal's free `subscribeNewToken` and `subscribeMigration` streams
   or our own RPC. Store: creation event and creator, first-block buyers,
   curve state snapshots at fixed offsets (for example 10 s, 30 s, 1 m, 5 m,
   30 m, 1 h), migration event, post-migration pool snapshots, and **the result
   of every safety check at the time it was run**, with timestamps.
2. **An exact-fill replay model.** Bonding curve and PumpSwap constant-product
   maths, the fee tables in section 5, a latency parameter (test 1, 3, 5 and
   10 s), our own price impact, and a conservative cost case of about 2% per
   side. Report results under both an optimistic and a conservative cost
   convention, as the 2610.39458 paper does.
3. **The shadow book (S2) first.** It needs no trades and answers the most
   useful question: which safety rules save money, and what do they cost in
   missed winners.
4. **Then the three paper strategies worth the effort:**
   - S1, organic-graduate buys that pass the safety rules, with hard stops
     and a time limit;
   - S3, pre-registered momentum with time stops on the same population;
   - S4, wallet-following, run as a *challenge to the negative result*, with
     realistic latency.
   Each gets a written rule before it sees data, a train slice and a later
   test slice, and a rule that it **fails** if the conservative cost case is
   negative.
5. **A decision gate.** Per the repo rule, 60 to 90 days of paper before live
   money is discussed, then the owner's explicit per-action approval, a
   dedicated capped wallet, local signing and the program allowlist from
   section 5. Before any live money, the owner should take legal advice on
   US-person exposure; this note does not give it.

Expectation to hold the team to: if the recorded data say the answer is "no
edge after costs", that is a valid, useful result and the sleeve stays paper.
