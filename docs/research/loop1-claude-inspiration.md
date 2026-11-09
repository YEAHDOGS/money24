# Loop 1: the nine inspiration X posts, and what is behind them

Researched 2026-10-30. Post data was fetched as JSON through `api.fxtwitter.com/<user>/status/<id>`
(direct x.com is blocked). Repo licences, languages and stars come from the GitHub REST API
(`gh api repos/<owner>/<repo>`) on the same day. Anything marked **UNVERIFIED** could not be
confirmed from a primary source.

## TL;DR

- **Only one of the nine posts links to real open-source trading code**: @carm1nee's "10 repos"
  list. Everything else is a paid SaaS funnel (QuantGPT, Volrix, TradingKit/StrategyFactory, a
  Telegram copy-trade referral), an education bot pack, a coding-agent setup tip, a simulated demo,
  or software piracy.
- **No post shows a verifiable live track record.** The headline numbers are a single 22-hour
  Bybit run (unverifiable), a backtest with a 53% drawdown, a 200-day moving-average backtest gross
  of costs, and a pump.fun demo whose own footer says "numbers simulated".
- **"Jev" is real, and the owner almost certainly got the name from @mirku21's post.** Jev is
  TypeSafe AI's *non-generative decision model*: it picks from options you define and returns
  calibrated probabilities. It is not a fast chat LLM. It does suit the "decide every few seconds"
  wish as a gate in front of slower LLMs. Details are in post 5.
- **Reusable code that fits our licence needs** (MIT or Apache-2.0): TradingAgents,
  ai-hedge-fund, qlib, RD-Agent, Kronos, OpenBB, ccxt, hummingbot, alpaca-mcp-server,
  lightweight-charts, timesfm, chronos, gs-quant. **Do not copy:** freqtrade (GPL-3.0),
  nautilus_trader (LGPL-3.0; usable only as a separate process), vectorbt (Apache-2.0 plus
  Commons Clause, so it cannot be part of anything we sell).
- **Nearly all of it is Python.** None of it runs inside Cloudflare Workers. In our JS-only stack
  the realistic paths are porting ideas and small algorithms to JS, an optional local Python sidecar
  for heavy research, or the few JS-native pieces (ccxt JS, lightweight-charts).
- **Subscription login is a terms problem.** Anthropic does not allow third-party products to offer
  claude.ai login or subscription rate limits without prior approval
  ([Agent SDK docs](https://code.claude.com/docs/en/agent-sdk/overview)). "Connect your Claude
  subscription" therefore cannot be a Money 24 feature. Bring-your-own-API-key can. TradingAgents
  v0.7 does ship ChatGPT Plus/Pro login (see post 2). Whether OpenAI permits that in third-party
  products is **UNVERIFIED** and needs checking before we copy it.

---

## Per-post findings

### 1. @seelffff: Solana sandwich-attack explainer plus copy-trade referral

- URL: https://x.com/seelffff/status/2116178087419208072. Posted 2026-10-30 14:39 UTC by
  "self.dll" (about 7.5k followers). 1.2k views, 20 likes.
- **Claim:** on Oct 10 a bot sandwiched eight buys of a Solana token (QUANT) for about 25 SOL
  (~$2.4k). The post cites "146K sandwiches in 30 days" and gives the usual defences: low slippage,
  split orders, MEV protection.
- **Product behind it:** a Telegram copy-trading bot via a **referral link**,
  `t.me/copyfomo_bot?start=ref_imyatacg`. There is no repo.
- **Quoted tweet:** @Mellon0x's long-form X article "Grok Bot: Build a Team That Finishes the Work".
  It is generic agent-ops advice: one narrow task first, a clear finish line, review gates, saved
  routines, cost control on schedules, and a stop at approval points. Mellon0x's GitHub
  (https://github.com/Mellon0x) has no trading code; its only repos are a Blender demo and a
  profile README.
- **Markets:** Solana memecoins. **AI:** none in the post itself. Grok Bot appears only in the
  quoted article.
- **Credibility: low.** It is engagement content that ends in a referral funnel, and the profit
  figures are self-reported.
- **Useful for Money 24:** only the idea. MEV and sandwich protection matter if we ever execute
  on-chain: use private RPC or a protected route, cap slippage, split orders. The quoted article's
  "approval points and finish lines for unattended agents" is a good principle for our loop
  supervisor.

### 2. @DaviddDotTech: "Kimi K3.1 beats 90% of traders" ($10k Bybit, 22 hours)

- URL: https://x.com/DaviddDotTech/status/2115867763088765298. Posted 2026-10-29 by
  "Davidd Tech" (about 11k followers). Bio: builds AI trading bots and sells a Skool community
  (https://www.skool.com/daviddtech/about).
- **Claim:** gave Kimi K3.1 (through Kimi Code) a $10,000 Bybit account and one instruction,
  "make profit". In 22 hours it made 16 trades, held 11 positions at once, and ended on 8 straight
  winners. In a second window, TradingKit "found 151 strategies". The best showed a **6,724%
  backtest return with a 53% drawdown** over 849 trades. The author also says "Kimi takes trades
  Claude refuses" and that two jobs used 26% of his weekly Kimi plan.
- **How the AI is used:** a coding-agent CLI (Kimi Code, from $15/month) acts as the trader. The
  prompt says to verify equity, study the market for an hour, put stop-loss and take-profit on every
  position, and **check in every 15 minutes**. Orders go through a "skill" from trigger.trade.
  Backtests come from TradingKit's MCP server.
- **Products behind it:**
  - https://trigger.trade turns TradingView alerts into exchange orders on Bybit, Blofin, Toobit,
    WEEX and Bitunix. It is "powered by StrategyFactory" and "free for Skool members". It has no
    repo and no licence. The live page still shows a developer note saying sign-in isn't configured
    because a Clerk key is missing, which says a lot about its maturity.
  - https://tradingkit.com, run by StrategyFactory.ai (Dubai). An AI writes Pine Script v6,
    backtests it with walk-forward optimisation, and forward-tests it in an "Incubator". It offers a
    remote MCP server (`app.tradingkit.com/mcp`) with OAuth and costs $0 to $39.99/month. Its nav
    and footer link to the DaviddTech Skool. **The post does not disclose that the author is tied to
    the tools it promotes.**
  - Kimi K3.1 itself exists. Secondary reports put its release around 2026-10-23 and list it in
    Kimi Code ([runtimewire](https://runtimewire.com/article/kimi-code-is-preparing-to-release-kimi-k3-1-as-model-appears-mid-session),
    [vktr](https://www.vktr.com/ai-platforms/kimi-k31-pro-nears-top-proprietary-models-as-moonshot-limits-its-cyber-skills/)).
    I found no primary Moonshot announcement, so the exact date is **UNVERIFIED**.
- **Markets:** crypto perpetuals on Bybit. **Bybit does not serve US residents.** If the owner is in
  the US this setup is unusable, and Webull is the right broker direction anyway.
- **Credibility: low.** It is one run, there is no account statement, and "8 straight winners in 22
  hours" is meaningless as a sample. A 6,724% backtest with a 53% drawdown is the classic signature
  of an overfit strategy. "Kimi takes trades Claude refuses" presents weaker guardrails as a
  selling point. It is a paid-community funnel.
- **Useful for Money 24:**
  - The ideas are sound: no withdrawal permission on API keys, mandatory SL/TP on every position,
    a fixed check-in cadence, and a separate backtest agent from the trading agent.
  - TradingKit's walk-forward "Incubator" (re-running strategies daily on new candles) is a good
    pattern to copy, as an idea only.

### 3. @carm1nee: "10 repos that replace a hedge fund" plus "Most Dangerous Repos (Full Course)"

- URL: https://x.com/carm1nee/status/2115936358003052667. Posted 2026-10-29 (about 17.5k
  followers, 23k views, 524 bookmarks). It quotes the author's own Oct 22 article
  (https://x.com/i/article/2113219739837243392, 406k views).
- **Claim:** ten free GitHub repos add up to a "full AI hedge fund", covering a research desk, an
  investment committee and an execution desk. Each repo comes with a prompt to paste into Claude
  Code. The article ends with a combined `fund.py` that runs daily. It pulls data, filters by
  Kronos path forecasts, gets a rating from the TradingAgents committee, sizes each trade to risk 1%
  of equity, places **bracket orders on an Alpaca paper account**, and sends a Telegram summary. It
  adds a 30-day paper plan and a 10% drawdown stop.
- **Credibility: medium-high for the repo list, low for the framing.** The repos are real and
  popular. Star totals are inconsistent (329k in the tweet, 575k in the article, and per-repo
  numbers differ). "Replaces a hedge fund" and "free" are hype: LLM tokens and premium data cost
  money. The article's own caveats are honest: backtests mislead, LLM outputs vary between runs, and
  some repos are not ready for real capital. This post is the most valuable of the nine.
- **Repo table** (GitHub API, 2026-10-30):

| Repo | Licence | Lang | Stars | What it is | Reuse verdict |
|---|---|---|---|---|---|
| [TauricResearch/TradingAgents](https://github.com/TauricResearch/TradingAgents) | Apache-2.0 | Python | 118.7k | LangGraph multi-agent "firm": analysts, bull and bear researchers, trader, risk team, portfolio manager. Point-in-time backtests, many providers (incl. Kimi, Ollama, OpenRouter). v0.7 adds a ChatGPT Plus/Pro subscription login. Research only, per its README. | **Port the role graph and prompts to JS** (with attribution). Best single blueprint for our agent roles. |
| [virattt/ai-hedge-fund](https://github.com/virattt/ai-hedge-fund) | MIT | Python | 64.4k | Investor-persona agents (Buffett, Munger, etc.). Paper funds with a **hash-chained ledger**, an approval step before execution, a kill switch and a per-fund cadence. Providers include Anthropic, OpenAI, xAI, Kimi and **TypeSafe (Jev)**. Educational only. | **Borrow the paper-ledger, approval and kill-switch design.** Port the persona prompts if wanted. |
| [ZhuLinsen/daily_stock_analysis](https://github.com/ZhuLinsen/daily_stock_analysis) | MIT | Python | 67.3k | Scheduled daily LLM stock report pushed to Telegram, runs on GitHub Actions. The README carries **sponsored and affiliate links to discounted API resellers** (e.g. "Claude at 0.03x"). Avoid those: gray-market keys. | Idea only: a scheduled digest. Our Cron Triggers already cover it. |
| [shiyu-coder/Kronos](https://github.com/shiyu-coder/Kronos) | MIT | Python | 44.1k | Candlestick foundation model. Sample around 100 future paths and count how many go up. Last push 2026-04. | **Local sidecar only** (needs PyTorch). Optional filter, not core. |
| [microsoft/qlib](https://github.com/microsoft/qlib) | MIT | Python | 49.8k | Quant research platform: data, factors, models, backtest. | Sidecar or idea only. Too heavy to port. |
| [microsoft/RD-Agent](https://github.com/microsoft/RD-Agent) | MIT | Python | 15.1k | An LLM agent that proposes, codes and tests factors. The "2x return, 70% fewer factors" figure is a README backtest claim, not live. | Borrow the loop idea (hypothesis, code, test, keep or discard) for our strategy-research agent. |
| [OpenBB](https://github.com/openbq-org/OpenBB) (API redirects `OpenBB-finance/OpenBB` here) | **Apache-2.0** (the LICENSE now says so; older versions were AGPL, so pin a version and check) | Python | 74.9k | Data platform with providers for equities, options and macro, plus an MCP server. | **Local sidecar or MCP**, or read its provider list to choose data APIs. Do not bundle it. |
| [ccxt/ccxt](https://github.com/ccxt/ccxt) | MIT | JS/TS/Py/… | 44.6k | Unified API for 100+ crypto exchanges. | **Import the JS build directly** for crypto, if we add crypto venues. Check its bundle size against the Worker size limit, or import single exchanges. |
| [freqtrade/freqtrade](https://github.com/freqtrade/freqtrade) | **GPL-3.0** | Python | 56.2k | Crypto bot with backtesting, hyperopt and lookahead-bias checks. | **Do not copy code.** Ideas only, e.g. lookahead-analysis and dry-run mode. |
| [hummingbot/hummingbot](https://github.com/hummingbot/hummingbot) | Apache-2.0 | Python | 20.8k | Market-making bot with paper connectors. | Idea only. Market making is out of scope. |
| [nautechsystems/nautilus_trader](https://github.com/nautechsystems/nautilus_trader) | **LGPL-3.0** | Rust/Py | 31.1k | Event-driven engine; backtest code matches live code. | Separate-process sidecar only. Never vendor it into our code. Idea: one engine for backtest, paper and live. |
| [alpacahq/alpaca-mcp-server](https://github.com/alpacahq/alpaca-mcp-server) | MIT | Python | 1.1k | Official broker MCP: stocks, ETFs, crypto, options. | **Reference design for our own Webull MCP tool surface** (tool names, order schemas). Port to JS. |
| [tradingview/lightweight-charts](https://github.com/tradingview/lightweight-charts) | Apache-2.0 | TS | 17.7k | The "TradingView-like charts" the owner wants. Its NOTICE requires TradingView attribution; keep the default attribution logo/link. | **Import as a dependency.** Our chart answer. |
| [google-research/timesfm](https://github.com/google-research/timesfm) | Apache-2.0 | Python | 34.8k | Time-series foundation model. | Sidecar or skip. |
| [amazon-science/chronos-forecasting](https://github.com/amazon-science/chronos-forecasting) | Apache-2.0 | Python | 6.2k | Time-series foundation model. | Sidecar or skip. |
| [goldmansachs/gs-quant](https://github.com/goldmansachs/gs-quant) | Apache-2.0 | Python | 13.8k | Mostly a client for GS Marquee (needs a GS account). | Skip. |
| [lballabio/QuantLib](https://github.com/lballabio/QuantLib) | BSD-style (modified) | C++ | 7.7k | Derivatives pricing. | Skip; Black-Scholes Greeks fit in about 40 lines of JS. |
| [skfolio/skfolio](https://github.com/skfolio/skfolio) | BSD-3-Clause | Python | 2.6k | Portfolio optimisation. | Idea only. |
| [polakowo/vectorbt](https://github.com/polakowo/vectorbt) | **Apache-2.0 + Commons Clause** | Python | 9.6k | Vectorised backtesting. | **Cannot be part of a paid product.** Avoid. |

- **AI usage:** the posts suggest Claude Code as the operator that installs and wires the repos.
  The trading itself runs from a **daily** script. There is no seconds-level loop anywhere.
- The article names "Claude Opus 5.5 / Sonnet 5.5". Those model names are consistent with the
  current Claude lineup, but checking them was outside this research.

### 4. @hooeem: "18 bots that teach trading from beginner to expert"

- URL: https://x.com/hooeem/status/2116177486765556082. Posted 2026-10-30 by "hoeem" (about
  168k followers). Bio: "turning screen time into cash flow" and a founder of a newsletter
  (https://sevenc.substack.com).
- **Claim:** 18 Grok Bot tutors, built on "crypto cred's" technical-analysis series, covering
  candlesticks, risk management, market structure, S/R, Ichimoku, RSI and more. There is a
  placement chat and progress is saved.
- **Product behind it:** shared bots on xAI's Grok Bot platform (e.g.
  https://x.ai/bot/q-yUP6MiyxCh9fGf_fjbu, "Risk Management Tutor": seven lessons, quizzes,
  explicitly "not financial advice, no buy/sell calls"). No price is shown. There is no code and no
  repo.
- **Markets:** generic, crypto-flavoured TA. **Credibility:** harmless education content with no
  performance claims. Whether the creator of the TA series consented is **UNVERIFIED**.
- **Useful for Money 24:** a product idea for a later paid tier: an onboarding "academy" that
  explains each agent's decision in plain language and teaches risk rules before the user can switch
  from paper to live. It also hints that Grok users expect bot-sharing UX.

### 5. @mirku21: Claude Code agent teams with Opus/Sonnet/Haiku/Fable, routed through "JEV"

- URL: https://x.com/mirku21/status/2115911644501147972. Posted 2026-10-29 21:00 UTC (about 3k
  followers, 28k views). It quotes the author's 2.27M-view article "Harness Engineering: How to
  Build AI Workflows That Never Fall Apart" (https://x.com/i/article/2098825381931311117).
- **Claim:** "Anthropic revealed" an agent-team setup.
  - Opus 5.5 is the lead and splits work into a shared task list.
  - Sonnet 5.5 teammates build in parallel.
  - Haiku 5.5 scouts audit the code, at "340 tok/s".
  - Fable 5.1 is an "adversary", consulted before contracts lock, after a test fails twice, and
    before sign-off.
  - **JEV** routes simple deterministic decisions so the big models only get hard work.
- **Verified:** agent teams are real and experimental. They are enabled with
  `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`, use a shared task list with file-locked claiming, and
  give each agent a JSON mailbox under `~/.claude/teams/` with tasks under `~/.claude/tasks/`
  ([docs](https://code.claude.com/docs/en/agent-teams)).
  - **Teammates cannot spawn in non-interactive `-p` or Agent SDK sessions**, so agent teams cannot
    be the engine of a headless 24/7 trading loop.
  - Token cost scales linearly with the number of teammates. The docs recommend 3 to 5.
  - The "340 tok/s" figure for Haiku is **UNVERIFIED**.
- **This is a coding-workflow post, not a trading one.** Its relevance is that **"JEV" is where the
  owner's "Jev" comes from.** It is TypeSafe AI's Jev:
  - **What it is:** a "System One" *decision model*, not an LLM: "Jev isn't a large language model,
    and it can't write you a sentence." You define the options and it returns a typed answer with
    calibrated probabilities. Answer types are yes/no, choice from a list, and score on a scale, and
    one call can answer several questions at once
    ([launch post](https://typesafe.ai/blog/introducing-system-one-models-and-jev),
    [Pulumi write-up](https://www.pulumi.com/blog/route-every-claude-code-message-to-the-right-model-with-jev/)).
  - **Speed and price (vendor figures, UNVERIFIED independently):** 70 to 500 ms end to end, and
    $0.042 per million input tokens with output free.
  - **Access:** early access with a waitlist. It is listed on OpenRouter as `typesafe/jev-1.14`
    ([OpenRouter](https://openrouter.ai/typesafe/jev-1.14)) and supported as a provider in
    virattt/ai-hedge-fund.
  - **Caution:** one calibration study reportedly found Jev "overconfident when a label encodes your
    own policy". That is exactly what a trade/no-trade threshold is, so it needs to be tested on our
    own labelled data.
  - Whether its terms of use restrict financial decisions is **UNVERIFIED**; its AUP was not
    readable.
  - The open-source router [dirien/jev-router](https://github.com/dirien/jev-router)
    (Apache-2.0, per the Pulumi post) is a Claude Code model router, not trading code.
- **The harness article is worth adopting as principles:**
  - bounded contracts;
  - a map, not an encyclopedia;
  - externalised memory in files;
  - objective sensors before autonomy;
  - plain-language rules paired with mechanical gates;
  - writer and reviewer kept separate;
  - "same test fails twice: stop and escalate".
  Its "cuts hallucinations in half" figure is unsourced.

### 6. @issatheblows: "Grok built a pump.fun terminal" plus "How to Trade Pump.fun With Claude"

- URL: https://x.com/issatheblows/status/2115882378271055879. Posted 2026-10-29 (about 2k
  followers, 55k views). It quotes the author's article (https://x.com/i/article/2115496326771113984).
- **Claim:** a one-board terminal with CHECKING, HOLDING and DONE columns where Grok passes or
  skips each new coin. The post claims $1,500 became $6,185 in 17 seconds, with 8 wins and 1 loss.
  Grok wrote itself rules: skip dev holdings above 10% and bundles above 25%, skip repeat-rug devs,
  exit when a bundle moves, close everything at the goal. **The post itself ends "illustrative
  build, numbers simulated."**
- **Article:** "Claude analyzes. You click." It suggests a Claude Project with `rules.md`,
  `trades.csv` and `wallets.txt`, plus prompts for a 60-second token autopsy, bonding-curve zones,
  narrative scans and exits, a tilt check, and a trade journal. It also describes a **read-only**
  Python scanner (Helius RPC, holder and dev filters, Telegram alerts, a CSV decision log, "no
  wallet, no keys, no signing", tests first).
- **Tools:** Axiom, Photon, GMGN, BullX, Solscan, Bubblemaps, RugCheck. There is no repo.
- **Credibility:** the headline numbers are **fake by the author's own admission**, which makes the
  P&L engagement bait. The article itself is reasonably responsible: human in the loop, paper or
  tiny size first, and "most pump.fun tokens go to zero".
- **Useful for Money 24:**
  - **The "one board, every decision visible" UI is exactly what our AI-loop monitor should be**: a
    kanban of candidates moving from checking to holding to done, each card carrying the rule that
    passed or failed it.
  - Rules the agent writes for itself should be stored as data and shown in the UI.
  - The read-only scanner with a decision log is a good shape for a Cloudflare Cron Worker.
  - Pump.fun or memecoin trading should **not** be a Money 24 market; it is not in the brief and is
    a near-certain money trap.

### 7. @RohOnChain: "free open library of profitable, fully backtested strategies" (QuantGPT)

- URL: https://x.com/RohOnChain/status/2115871785250111615. Posted 2026-10-29 (about 83k
  followers, 81k views, 2k bookmarks). It quotes the author's Oct 12 article "How to Build the Most
  Powerful AI Trading System (Hedge Fund Infrastructure)" (2.0M views), which promotes
  **QuantGPT** (https://quantgpt.co).
- **Claim in the tweet:** a "FREE" library of "PROFITABLE", "FULLY BACKTESTED" strategies with a
  results dashboard (return, CAGR, Sharpe, drawdown, win rate, profit factor) and Python code. It
  covers stocks, ETFs, FX and futures, from daily down to 1-minute bars, and says strategies can be
  deployed as 24/7 bots on Alpaca, Interactive Brokers or OANDA with risk limits and a kill switch.
  It also says "Most DANGEROUS thing on internet today. Steal this before everyone starts using it."
- **What I could verify (QuantGPT site):**
  - **There is no permanent free tier**, only a 3-day trial that needs a card. Plans are $49/month
    and $149/month, and Business is $1,499/month or more.
  - The MCP server is at `terminal.quantgpt.co/mcp`.
  - Data: a US daily warehouse with delisted companies from 1998, fundamentals by filing date,
    crypto and FRED macro.
  - The library has 175 documented strategies, about 55 to 63 of them runnable.
  - **I found no Python code, no GitHub repo, no FX strategies, no 1-minute data, and no IBKR or
    OANDA support** ([library](https://quantgpt.co/library/)). The article itself says live trading
    is **Alpaca, US stocks and ETFs only** and that results are **gross of costs**.
  - The tweet's photo may show a different product. I found no product matching every claim, so
    the tweet's feature list is **UNVERIFIED and contradicted** by the product it promotes.
- **Headline result:** the 200-day moving-average rule on the S&P 500 from 1999 to 2026 had the
  same CAGR as buy-and-hold (6.8% vs 6.8%). The drawdown was smaller, the Sharpe was 0.61 vs 0.44,
  and it made 42 trades gross of costs. That is an honest but very ordinary result, and it shows
  nothing about "profitable".
- **Credibility: low. This is a paid-SaaS funnel with urgency bait.** The article's
  infrastructure points are still correct and useful:
  - survivorship bias (delisted stocks);
  - look-ahead bias (fundamentals dated by filing date, not quarter end);
  - out-of-sample splits;
  - "a strategy that runs only while your terminal is open is a hobby".
- **Useful for Money 24:**
  - Use the three-verdict output (DATA VALIDATED, TRADE-OFF, NO EDGE) for every backtest the agents
    run, with a fixed out-of-sample split.
  - Use a journal that re-checks old backtests as new data arrives.
  - MCP is the integration surface every one of these products converged on. That supports the
    owner's wish for a Money 24 MCP server.

### 8. @niraj_munot: "Open Library of Trading Strategies" (Volrix)

- URL: https://x.com/niraj_munot/status/2115712164375957669. Posted 2026-10-29 by a quant
  developer building @volrixai (about 2.4k followers, 45k views, 1.4k bookmarks).
- **Claim:** 70+ strategies collected from YouTube, Reddit, X, blogs and papers. Each has its
  original source credited, **backtests updated daily**, and the exact code and logic so anyone can
  reproduce it. The post asks for submissions and includes "No investment advice". The
  "link below" was in a reply I could not fetch. It is probably the Volrix "Open strategies" page
  (https://app.volrix.ai/strategies), but that is **UNVERIFIED**.
- **Product:** https://volrix.ai. You describe a strategy to your own AI agent, which writes the
  code. Volrix backtests it with costs, slippage and trade logs, and offers a public ranked strategy
  library.
  - It runs remote MCP over Streamable HTTP with OAuth, and ships a skill file
    (https://volrix.ai/skill.md) for Claude Code, Codex, Cursor and ChatGPT.
  - **Markets:** Indian index derivatives, MCX commodities, and some crypto perpetuals.
  - **Pricing:** free tier (500 tool calls, last 6 months of data), Pro ₹750 to ₹999/month,
    Max ₹1,500 to ₹1,999/month.
  - The GitHub org (https://github.com/volrixqt) has no strategy code that I could find, so the
    "exact code" lives inside the app, not an open repo with a licence. **Cannot copy.**
- **Credibility: medium.** The framing is more honest than most (sources credited, costs modelled,
  disclaimers), but it is still a funnel to a paid app, and it is India-market focused, which is
  irrelevant to Webull US.
- **Useful for Money 24:**
  - Store each strategy as a record holding its source URL, rule text, generated code, last
    backtest and a daily re-run.
  - Publish a **skill.md plus remote MCP with OAuth** so any user's agent can connect. This is the
    cleanest model for our "external agents can paper trade" feature.

### 9. @cryptotrice: "TradingView is officially cooked" (TradingView Premium crack)

- URL: https://x.com/cryptotrice/status/2115453869883047937. Posted 2026-10-28 (about 5k
  followers, 19k views).
- **Claim:** "Claude Fable 5.1" built an activator that generates licence keys and unlocks
  TradingView Premium via "reverse-engineered TradingView Premium apps for Windows and Mac". The
  repo "recently went private", and a "full guide" was promised.
- **This is software piracy, and almost certainly malware bait.** "Cracked desktop app plus private
  repo plus guide coming" is the standard pattern for spreading info-stealers that target crypto
  wallets. **Do not download anything, do not reuse anything, do not link to it from our repo.**
  Using it would breach TradingView's terms and probably the law.
- **Legitimate route to what the owner actually wants:** TradingView's own
  **lightweight-charts** (Apache-2.0, see post 3) gives TradingView-style charts legally, with an
  attribution notice. TradingView's full Advanced Charts library is licensed separately and is
  outside this research.

---

## Capability matrix

Legend for the posts: S=seelffff, D=DaviddDotTech, C=carm1nee (incl. its repos), H=hooeem,
M=mirku21, I=issatheblows, R=RohOnChain, N=niraj_munot, T=cryptotrice.

| Capability | Posts that have it | Money 24 priority | Notes |
|---|---|---|---|
| Paper trading before live | C (Alpaca paper, ai-hedge-fund ledger), R (Alpaca), I (paper or tiny size), D (no) | **Must** | Brief is paper-first. Webull paper is the target broker; see the Webull research. |
| Hash-chained or append-only trade ledger | C (ai-hedge-fund) | **Must** | Makes the paper track record tamper-evident. It fits D1 or R2 append-only. |
| Kill switch and drawdown stop | C (1% risk, 10% DD stop, ai-hedge-fund `h`/`r`), R (claims) | **Must** | Global and per-agent switches, enforced on the server, not in the prompt. |
| Mandatory SL/TP, bracket orders | D, C (bracket orders) | **Must** | Use the broker's native bracket or OCO orders where Webull supports them. |
| API keys with no withdrawal permission | D | **Must** | Also: keys never leave the Worker and are never sent to the LLM. |
| Human approval step before execution | C (ai-hedge-fund), I ("you click"), S/Mellon (approval points) | **Must** for live, optional for paper | |
| Multi-agent role graph (analysts, bull/bear, trader, risk, PM) | C (TradingAgents, ai-hedge-fund) | **Must** (as design) | Port the roles; we don't need 30 agents. 5 to 7 roles × N symbols is enough. |
| Visible decision board / loop monitor | I (kanban board), M (task list) | **Must** | Owner wants loop monitoring; post I's board is the right UX. |
| Fast decision gate in front of LLMs | M (Jev routing), D (15-minute check-ins) | **Nice → Must** if "seconds" stays a goal | Jev or deterministic rules every few seconds; LLM every 5 to 15 minutes. |
| Bring-your-own model, many providers | C (TradingAgents, ai-hedge-fund: Anthropic, OpenAI, xAI, Kimi, Jev, Ollama), N (any MCP client) | **Must** | Via API keys. Subscription login: see red flags. |
| MCP server for external agents | R (QuantGPT MCP), N (Volrix MCP + skill.md), D (TradingKit MCP), C (alpaca-mcp-server, OpenBB MCP) | **Must** | Every serious product here converged on remote MCP with OAuth. |
| Backtesting with point-in-time data | C (TradingAgents v0.5, qlib, freqtrade lookahead checks), R, N, D | **Must** (simple), Nice (heavy) | Start with simple daily backtests in JS; heavy research goes to a sidecar. |
| Walk-forward / out-of-sample verdicts | D (TradingKit), R (3 verdicts, 2015 split) | **Must** | Cheap to implement, and protects against the 6,724% kind of result. |
| Daily re-run of strategy library | N, D (Incubator), R (journal) | Nice | Cron Trigger plus D1 rows. |
| Strategy library with source credit | N, R | Nice | Later; also feeds marketing. |
| TradingView-like charts | T (illegitimately), C (lightweight-charts) | **Must** | lightweight-charts, Apache-2.0. |
| Price-path forecasting models (Kronos, TimesFM, Chronos) | C | Nice, sidecar only | No evidence they add edge after costs. |
| Telegram / push summaries | C (daily_stock_analysis, fund.py), I, S | Nice | Web Push or Telegram bot from a Worker; free. |
| Education / tutor bots | H | Nice (paid-tier idea) | |
| MEV protection | S | Only if on-chain execution is added | |
| Memecoin / pump.fun trading | I, S | **Reject** | Money trap; not in brief. |
| Cracked paid software | T | **Reject** | Illegal and a malware vector. |

---

## What to reuse and how

### Import as a dependency (JS, permissive, small enough)
1. **`lightweight-charts`** (Apache-2.0). This is the chart engine. Keep TradingView's attribution
   as the NOTICE requires.
2. **`ccxt`** (MIT), JS build, only if and when crypto venues beyond Webull are added. Measure the
   bundle first and import per exchange. For Webull-only v1, skip it.
3. The Anthropic, OpenAI, and other provider SDKs, or plain `fetch`, are not from these posts.
   Per AGENTS.md, plain `fetch` is probably enough.

### Port to JS (read the code, rewrite it in our style, credit it in a NOTICES file)
1. **TradingAgents role graph** (Apache-2.0): the analyst, bull/bear debate, trader, risk and
   portfolio-manager sequence, its structured output schemas, and its five-tier rating scale. Keep
   the attribution and record any modifications, as Apache-2.0 requires.
2. **ai-hedge-fund paper-fund design** (MIT): a fund "mandate" (strategies, capital, cadence,
   universe) kept apart from tickers; an append-only hash-chained session ledger; an approval step;
   a kill switch. This maps cleanly onto D1 rows (or R2 JSONL) plus one Cron Worker.
3. **alpaca-mcp-server tool surface** (MIT): a template for our Webull MCP tools (`get_account`,
   `get_positions`, `place_order` with bracket legs, `cancel_order`, `get_bars`, and so on), ported
   to a JS MCP server on Workers.
4. **Risk maths:** 1% risk-per-trade sizing, a max-drawdown stop, walk-forward split logic, and
   Sharpe, CAGR and max-DD metrics. These are about 20 lines each, so write them ourselves. No
   dependency is needed.

### Run as an optional local sidecar (Python, never in Workers, never required for the web app)
- **Kronos, TimesFM, Chronos** (forecast filters), **qlib and RD-Agent** (factor research),
  **OpenBB** (data and MCP), **TradingAgents itself** (if a user wants the full Python version). Run
  them on the user's machine next to OpenCode or Tauri, talking to our API over HTTP or MCP.
  Strictly opt-in, because the brief wants the free, serverless path to work without them.
- **nautilus_trader** (LGPL-3.0) is only acceptable as an unmodified separate process. Probably not
  worth it.

### Borrow the idea only (no code)
- The decision kanban board (I).
- The three-verdict backtest result and a re-checking journal (R).
- A strategy record with source credit and daily re-runs, plus `skill.md` and OAuth MCP onboarding
  (N).
- The harness rules: bounded contract, sensors before autonomy, two failures means stop and
  escalate, writer and reviewer kept separate (M).
- Approval points and finish lines for unattended agents (S/Mellon).
- No-withdrawal keys and mandatory SL/TP (D).
- Lookahead-bias analysis and dry-run mode (freqtrade, GPL: idea only).
- The Jev decision-gate pattern (M): a cheap classifier decides "escalate to LLM or not" every tick.
  For v1 that classifier should be **deterministic JS rules**, with Jev as an optional
  bring-your-own-key provider.

### Do not use
- freqtrade code (GPL-3.0), vectorbt (Commons Clause), Volrix and QuantGPT and TradingKit content
  (proprietary), the copyfomo referral, the TradingView activator, and the discounted API resellers
  advertised in the daily_stock_analysis README.

---

## Red flags

1. **Unverifiable or fake P&L.** D (one 22-hour run, no statement), I ("numbers simulated", stated
   by the author), S (self-reported sandwich profits). Treat all of it as marketing.
2. **Backtest worship.** 6,724% with a 53% drawdown (D) is overfitting. QuantGPT's showcase only
   matches buy-and-hold CAGR (R). RD-Agent's "2x" is a README backtest (C). Every Money 24 backtest
   must report out-of-sample results, costs and drawdown first.
3. **Paid funnels presented as "free" or "open".** R (QuantGPT has no free tier, and the tweet's
   claims don't match the product), N (Volrix, paid tiers, code not in a public repo), D (Skool,
   TradingKit and trigger.trade, with no disclosure of the affiliation), S (Telegram referral).
4. **Urgency bait.** "Most DANGEROUS thing on the internet", "Steal this before everyone…" (R, C),
   "officially cooked" (T).
5. **Weaker guardrails sold as a feature.** "Kimi takes trades Claude refuses" (D). An agent
   ignoring risk refusals is a bug, not edge.
6. **Piracy and malware vector.** T. Avoid it entirely.
7. **Subscription and terms traps relevant to the owner's wishes:**
   - Anthropic forbids third-party products from offering claude.ai login or subscription rate
     limits without approval ([Agent SDK docs](https://code.claude.com/docs/en/agent-sdk/overview)).
     Money 24 must use the user's **API key**, or run on the user's own machine inside *their* Claude
     Code or OpenCode install as their own tool. Even then, an always-on unattended trading loop on a
     consumer plan is risky under usage limits.
   - TradingAgents v0.7 ships ChatGPT Plus/Pro login. Whether OpenAI permits that in third-party
     products is **UNVERIFIED**.
   - D notes that two jobs used 26% of a weekly plan. **30 agents every 5 to 10 minutes on a
     consumer subscription will hit limits within hours.**
8. **Agent teams are not a runtime.** M's setup is interactive-only and experimental. Teammates do
   not spawn in headless `-p` or Agent SDK sessions ([docs](https://code.claude.com/docs/en/agent-teams)).
   Do not plan the 24/7 loop around it.
9. **Gray-market API resellers.** The daily_stock_analysis README advertises "Claude at 0.03x"
   resellers. These are almost certainly terms violations or stolen capacity. Never recommend them.
10. **Jurisdiction.** D's setup uses Bybit, which does not serve US residents. Pump.fun and memecoin
    flows (I, S) are near-certain losses for retail. Neither belongs in Money 24.
11. **Model-name hype.** "Opus 5.5 / Sonnet 5.5 / Haiku 5.5 / Fable 5.1", "Kimi K3.1 beats 90% of
    traders", "Haiku at 340 tok/s". Model capability claims in these posts are vibes; the 340 tok/s
    figure and Kimi's release date are **UNVERIFIED**.

## What the nine posts actually tell us about cadence (input for the architecture loop)

- No post runs LLM decisions at second intervals.
  - D checks in every **15 minutes**.
  - C's fund script runs **daily**.
  - ai-hedge-fund runs **per session**.
  - I's "17 seconds" is simulated.
- The only seconds-level component suggested anywhere is a **non-generative decision model** (Jev,
  70 to 500 ms per vendor) or plain rules.
- Recommended pattern, consistent with all of them:
  - **Ticks or seconds:** deterministic JS rules (stops, limits, triggers) in a Worker, Durable
    Object or the broker's native orders, so nothing depends on an LLM to be fast.
  - **Minutes (5 to 15):** LLM agent loop per symbol group, escalated only when the rules gate or
    Jev flags something.
  - **Daily:** research and backtest jobs, strategy-library re-runs, digest.
