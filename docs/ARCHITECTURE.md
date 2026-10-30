# Money 24 — Architecture

Status: **Loop 2 draft** (2026-10-30). Planning only; no application code exists yet.
Sources: [`docs/research/`](research/) (Loop 1 reports, cited there). Anything marked
**UNVERIFIED** must be proven by a spike before it is promised to users.

---

## 1. Goals and non-goals

### Goals
1. **Paper first.** A multi-asset paper-trading bot (stocks, ETFs, options, futures, crypto,
   event contracts) on Webull's API sandbox, running 24/7 without the owner awake.
2. **A garden of agents.** Users plant *bots* (agents with a mandate), group them into
   *gardens* (a strategy over a universe), and run a *farm* (all gardens on one account).
   Up to ~30 agents per user, each on its own loop cadence, supervised and restartable.
3. **Fast where speed matters, smart where it pays.** Second-level reactions come from
   deterministic code and broker-native orders. LLMs think on minute cadences.
4. **Bring your own brain.** Claude, GPT/Codex, Grok, Gemini, Jev, OpenRouter, or a local
   model, connected by the user. No AI bill for DOGS during the build phase.
5. **Open to other agents.** An MCP server and HTTP API so any external agent can paper
   trade (and, later, live trade under the same risk gates).
6. **One codebase, every screen.** Svelte web app plus Tauri shells for Windows, macOS,
   Linux and Android (iOS later).
7. **$0/month to build.** Cloudflare free tier, free broker sandbox, free data feeds.
8. **Paid tiers later** with fees that do not eat a $19/month plan.

### Non-goals (and why)
- **No promise of profit.** "Making money while I sleep" is the hope, not a requirement
  the system can meet. The system's job is to run strategies safely, honestly score them,
  and stop them when they lose.
- **No high-frequency trading.** Webull's market stream caps at 3 pushes/s per connection
  and LLMs take seconds. We compete on discipline and coverage, not microseconds.
- **No live trading for other people's money** until a securities lawyer signs off
  (adviser / broker-dealer / CTA risk, see §11).
- **No memecoin sniping, MEV, pump.fun, cracked software.** Rejected in Loop 1.
- **No Python requirement.** Python is allowed as an optional research sidecar only.

---

## 2. The big picture: four planes

```
                ┌──────────────────────── CONTROL PLANE (Cloudflare, JS) ────────────────────────┐
  Web / Tauri   │ Pages (Svelte app)   Workers API   Remote MCP server   Auth   Billing webhooks │
  UI  ◄────────►│ Durable Object per user (hibernating WS hub, realtime fan-out, per-user state) │
                │ D1 (docs + audit)    R2 (candles, ledgers, archives)    KV (config cache)      │
                └───────────────▲───────────────────────────────────────────▲────────────────────┘
                                │ sync: summaries, decisions, heartbeats    │ shared market snapshots
                ┌───────────────┴──────── RUNNER PLANE (Go engine "m24d") ──┴────────────────────┐
                │ Broker adapters (Webull REST + MQTT quotes + gRPC trade events, Alpaca, ...)   │
                │ Reflex layer (stops, brackets, risk limits)   Gate layer (rules / Jev)          │
                │ Agent supervisor (30 loops, heartbeats, budgets)   Local MCP   Local journal    │
                │ Runs as: Tauri sidecar on the user's machine  |  or a $4–6/mo VPS               │
                └───────────────▲───────────────────────────────────────────▲────────────────────┘
                                │ prompts / tool calls                      │ orders, fills, quotes
                ┌───────────────┴───── BRAIN PLANE ──────┐   ┌──────────────┴──── BROKER PLANE ────┐
                │ BYO API keys: Anthropic, OpenAI, xAI,  │   │ Webull (sandbox → live), Alpaca,    │
                │ Google, OpenRouter, Jev                │   │ Kalshi, Coinbase; IBKR for global   │
                │ Local CLIs: claude -p, codex exec,     │   │ Webull = source of truth for        │
                │ gemini (user's own install)            │   │ balances, positions, orders, fills  │
                │ Local models: Ollama / OpenCode (opt.) │   └─────────────────────────────────────┘
                └────────────────────────────────────────┘
```

### Why four planes
- **Cloudflare cannot hold the trading loop cheaply.** One always-on, non-hibernating
  Durable Object uses ~10,800 of the 13,000 free GB-s/day; 100 users would cost ~$400/mo
  ([Muse §2](research/loop1-muse-platform-payments.md)). Outbound WebSockets cannot
  hibernate, and Webull's order events are **gRPC**, which Workers cannot speak
  (`node:http2` is a stub) ([Astra §4](research/loop1-astra-broker-data.md)).
- **The runner must live somewhere that can hold sockets forever.** That is the user's
  computer (free, private, keys stay local) or a tiny VPS (always on, ~$5/mo).
- **The brain is the user's.** Their keys, their subscription, their machine, their bill.
- **The broker is the database of record** for money. We only keep what Webull cannot
  tell us: *why* an order happened.

---

## 3. Language decision (amends AGENTS.md — needs owner sign-off)

| Layer | Language | Why |
|---|---|---|
| Svelte UI, Workers, MCP server, Pages Functions | **JavaScript + JSDoc** | Workers run JS natively; existing template. |
| Runner engine `m24d` | **Go** | Owner preference; one static binary per OS; goroutines suit dozens of streams; first-class gRPC (`google.golang.org/grpc`) and MQTT (`eclipse/paho.mqtt.golang`) clients, which are exactly Webull's streaming protocols; ~20–50 MB RAM; runs as a Tauri sidecar. |
| Research / backtests | Go first; Python **optional** sidecar | Many forecasting libs (Kronos, TimesFM, qlib) are Python-only. Never required. |

Speed reality: an order round trip to Webull is tens to hundreds of ms and an LLM call is
seconds. Go vs Python changes neither. Go wins on **reliability, concurrency, memory,
and shipping one file**, not raw speed.

Cost of Go: Webull's official SDKs are Python and Java (Go SDK **UNVERIFIED**, assume
none). We write a thin Go client: request signing, REST, MQTT quote stream, gRPC trade
events (compile Webull's protos if published; else reverse from the Java SDK). Estimated
a few hundred to ~1,500 lines.

Proposed AGENTS.md amendment: *"JavaScript only, except the runner engine in `engine/`,
which is Go. No TypeScript, no Python requirement."*

---

## 4. Speed tiers: who decides what, and how fast

The owner wants second-level decisions and 30 agents looping every 5–10 minutes. Both
are possible if each tier does only what it is fast and cheap at.

| Tier | Cadence | Who | Does | Cost |
|---|---|---|---|---|
| **Reflex** | ms–1 s | Go code + broker-native orders | Stops, take-profits, OCO/bracket legs, max-loss and position-size limits, kill switch, stale-data halt | $0 |
| **Gate** | 1–10 s | Go rules; optional **Jev** (70–500 ms, vendor claim) | "Is anything interesting happening?" Score each symbol, decide whether to wake a thinker. Never places orders by itself in v1. | $0 (rules) / ~$0.04 per M tokens (Jev, UNVERIFIED) |
| **Thinker** | 5–15 min, or on gate wake-up | LLM agents (roles below) | Read a compact snapshot, argue, propose an order *intent* with thesis, size, stop, target | User's own model bill |
| **Strategist** | daily / weekly | LLM + backtester | Review P&L and journal, retire losing bots, propose new ones, walk-forward verdicts | User's own model bill |
| **Human** | when asked | Owner / user | Approve live orders above limits, enable live mode, change mandates | — |

**Rule: no LLM output reaches the broker without passing the Reflex risk checks.** An
LLM proposes *intents*; Go turns intents into orders only if limits allow.

### Agent roles (ported from TradingAgents, Apache-2.0, credited in NOTICES)
Per garden: Market analyst, News/sentiment analyst, Fundamentals analyst (stocks only),
Bull and Bear researchers (debate), Trader (writes the intent), Risk manager (can veto),
Portfolio manager (sizes across gardens). That is 5–8 roles; 30 agents ≈ 4 gardens.
Not every role runs every loop: analysts cache their view and refresh on their own cadence.

---

## 5. Runner plane: the Go engine (`m24d`)

### Components
- **Broker adapters** behind one small contract: capabilities, accounts, instruments,
  positions/balances, preview/submit/cancel/replace, reconcile, events. Preserve native
  IDs, multipliers, sessions, expiries. Multi-leg options, futures and event contracts are
  not squashed into a stock model. **No automatic order failover between brokers** (a
  timed-out order has unknown state; reconcile first).
- **Data adapters**, separate from execution: Webull MQTT (max 5 connections per app key,
  3 pushes/s each), Coinbase/Kraken public WebSockets for crypto, Kalshi WS for events,
  Alpaca IEX free / $99 SIP later. One shared feed per symbol, fanned out in memory.
- **Snapshot builder**: turns ticks into the compact JSON an agent reads (last N candles,
  indicators, positions, open orders, recent news headlines). Agents never call data APIs
  themselves; that is how 30 agents stay inside rate limits.
- **Supervisor**: schedules every loop, enforces per-agent token and dollar budgets,
  restarts crashed loops with backoff, emits a heartbeat every loop, and stops an agent
  after two consecutive failures (harness rule from Loop 1).
- **Journal**: local SQLite (pure-Go driver) holding intents, risk decisions, client order
  IDs, submissions, broker responses, reconciliations. Written **before** submission.
  Hash-chained (ai-hedge-fund pattern) so the paper record is tamper-evident.
- **Local MCP server** (stdio + localhost HTTP) so Claude Code / Codex / OpenCode on the
  same machine can drive the engine directly.
- **Sync client**: pushes summaries, heartbeats and decisions to the user's Durable
  Object over one WebSocket; pulls config changes. Works offline and catches up.

### Where it runs
| Mode | Who | Pros | Cons |
|---|---|---|---|
| **Desktop sidecar** (default) | Every Tauri desktop user | $0, keys never leave the machine, second-level reflexes, can call the user's local CLIs | Stops when the laptop sleeps; we must prevent sleep while armed and alert when offline |
| **VPS** (power users / owner) | Same binary, `m24d --headless` | Always on, low latency to broker | ~$4–6/mo; keys on a server the user controls |
| **Cloud-only** (web users, no runner) | Paper users trying it out | Nothing to install | Minute cadence only (Cron + hibernating DO), REST polling, no gRPC events |

Android: the runner does not run 24/7 on a phone (OS kills background work). Mobile is a
**remote control and monitor** for a desktop or VPS runner.

---

## 6. Brain plane: connecting AI

### Allowed connection types
1. **API keys (BYO).** Anthropic, OpenAI, xAI, Google, OpenRouter, Jev. Stored encrypted on
   the runner; never sent to DOGS servers in desktop mode.
2. **Local CLI bridge.** If the user has Claude Code, Codex CLI or Gemini CLI installed and
   logged in on their own machine, `m24d` can call it headless (`claude -p`, `codex exec`,
   `gemini -p`) with our local MCP attached. We never see or store their subscription
   credentials. **Whether each provider's consumer terms allow this for unattended 24/7
   use, and the usage caps that would throttle 30 agents, is UNVERIFIED** and is a Loop 3
   research item. Treat it as "personal use on your own machine", never as a hosted
   feature.
3. **Local models.** Ollama / LM Studio / OpenCode, optional. Good for the Gate tier and
   cheap summaries; not trusted for Trader or Risk roles until benchmarked.

### Not allowed
- **"Log in with your Claude subscription" inside Money 24.** Anthropic's Agent SDK docs
  bar third-party products from offering claude.ai login or subscription limits without
  approval ([Claude §5/§8](research/loop1-claude-inspiration.md)). Same caution for
  ChatGPT login until OpenAI's terms are verified.
- **Requiring OpenCode at install.** It adds a heavy dependency to every install for a
  benefit only some users want. Offer it as a one-click optional integration.

### Model routing (the "Jev pattern")
Cheap, fast model or rules decide *whether* to think; strong model decides *what* to do.
Default routing per role is configurable; the UI shows the cost per loop.

### Cost math (to be completed in Loop 3)
30 agents × every 5 min × 24 h = 8,640 LLM calls/day per user if every agent always runs.
With gating (only 10–20% of loops escalate) and cached analyst views, target
**< 1,500 calls/day**. At ~3k input + 500 output tokens per call, that is ~5M tokens/day:
cents/day on Jev/Flash-class models, dollars/day on Sonnet-class, more on Opus-class.
Loop 3 fills in exact prices.

---

## 7. Control plane: Cloudflare (JS)

| Piece | Job |
|---|---|
| **Pages** | Svelte app (web), static fallback, marketing page |
| **Workers API** (`api/functions/`) | Auth, account, gardens/bots config, journal sync, entitlements, MCP over HTTP |
| **Durable Object per user** | Realtime hub: runner WebSocket in, UI WebSockets out (hibernating, server side only), per-user SQLite for hot state and outbox |
| **D1** | Shared system of record (`docs` pattern, §8), audit log, entitlements |
| **R2** | Candles (`ndjson.gz` per symbol/day, Parquet monthly), journal archives, backtest results |
| **KV** | Config and calendars only (1,000 writes/day free; never ledgers or entitlements) |
| **Cron Triggers** | Cloud-only paper loops (minute cadence), nightly compaction, daily digest |
| **Remote MCP server** | Workers + Agents SDK, OAuth. Paper tools for everyone; live tools gated (§9) |

### Shared market data, never per-user polling
One Cron/Queue poller fetches snapshots for the union of watched symbols and fans them
out through DOs. Redistributing broker data to other users needs **licence permission**;
until confirmed, each user's runner uses their own broker entitlements, and cloud-only
users get delayed/IEX/crypto public data clearly labelled as such.

### Free-tier budget (from Muse §1)
Free tier supports ~1–5 disciplined paper users. First real users → Workers Paid ($5/mo).
100 users with hibernating DOs and desktop runners: ~$10–60/mo Cloudflare. The one
forbidden pattern: always-on non-hibernating DOs per user.

---

## 8. Data model: "almost NoSQL" with ~4 migrations ever

The owner hates migrations; AGENTS.md requires SQL migrations as the schema source. Both
are satisfied by a **document-shaped schema written once**:

- **D1 `docs` table**: `kind`, `id`, `owner_id`, `json`, plus a few *generated* columns
  (`status`, `symbol`, `created_at`, `updated_at`) that read from the JSON for indexing.
  New fields are new JSON keys: **no migration**. A migration is only needed when a new
  field must be indexed, and that is an additive `ALTER TABLE ... ADD COLUMN ... GENERATED`.
- **D1 `audit_log`**: append-only decision and order audit, hash-chained.
- **DO SQLite per user**: same `docs` shape plus `outbox` and `kv`. Hot state lives here.
- **R2**: market history and archives. Never put candles or ticks in D1.
- **Runner SQLite**: the local journal (same `docs` shape; Go embeds the migration).
- **Webull**: balances, positions, orders, fills. Order history back to 2018 on the
  individual API; positions are current-only; historical P&L is ours to compute.

Validation: JSDoc-typed validator functions at write time (JS) and Go structs (engine).
These are not a duplicated schema; the migrations remain the schema.

Kinds (initial): `user`, `connection` (broker/AI, encrypted), `farm`, `garden`, `bot`,
`loop_run`, `intent`, `order_ref`, `paper_fill`, `entitlement`, `api_key`, `mcp_client`.

---

## 9. MCP server and public API

- **Remote MCP** (Cloudflare Workers + Agents SDK + OAuth 2.1). Tools modelled on
  alpaca-mcp-server (MIT): `get_account`, `get_positions`, `get_quote`, `get_bars`,
  `list_gardens`, `propose_intent`, `place_order` (paper), `cancel_order`,
  `get_journal`, `get_loop_status`. Ship a `skill.md` onboarding file (Volrix idea).
- **Local MCP** from `m24d` for agents on the same machine (fast path, no cloud hop).
- **HTTP API** mirrors MCP tools, API keys scoped per garden, rate-limited per key.
- **Live tools** (`place_order` live) are off by default and require: live mode enabled
  with step-up 2FA, per-order risk check, optional human approval above a size limit,
  and the legal gate in §11.
- **Prompt-injection posture**: news and web text are untrusted. Agents get them as
  quoted data; only the Trader role can emit an intent; only Go can place an order.

---

## 10. Monitoring the loops

- Every loop run writes one `loop_run` doc: agent, garden, model, start/end, tokens,
  cost, gate score, outcome (`idle`, `intent`, `vetoed`, `ordered`, `error`), and a short
  reasoning summary.
- Heartbeats: runner → DO every 15 s; DO marks the runner offline after 60 s and alerts
  (Web Push / Telegram / Discord, free).
- UI: a **decision board** (from @issatheblows' kanban idea): columns *Watching → Thinking →
  Proposed → Approved / Vetoed → Filled → Closed*, one card per intent, live.
- Budgets: per-agent and per-farm daily token and dollar caps; hitting a cap pauses the
  agent, never silently drops risk checks.
- Error tracking: Sentry free tier (JS + Go). Product analytics: PostHog sampled, Cloudflare
  Web Analytics.

---

## 11. Legal and safety gates (not legal advice)

- **Phase 0–2: own account, paper only.** No regulatory issue beyond broker terms.
- **Selling software that the user configures and runs** on their own account is the
  safest framing ("user-controlled software"). Still needs disclaimers and a lawyer
  before live trading is sold.
- **Paid autonomous trading of customers' accounts, performance fees, copy-trading,
  signals marketed as advice** can trigger investment adviser, broker-dealer, or CTA/CPO
  (futures) registration. Each is a legal project.
- **PDT**: FINRA's intraday-margin replacement took effect 2026-06-04 with transition to
  2027-10-20; Webull's rollout is **UNVERIFIED**. Read account capabilities, never hardcode.
- **Prediction markets**: state conflicts (e.g. Nevada vs Kalshi). Enforce broker
  eligibility; never route around geoblocks. International Polymarket is close-only for US.
- **Webull Connect** (OAuth "Log in with Webull") requires partner approval. Apply early.

---

## 12. Pros and cons of this architecture

| Pros | Cons |
|---|---|
| $0 to build; Cloudflare only pays at real usage | Desktop runner means "always on" depends on the user's machine (or a VPS) |
| Keys and AI subscriptions stay on the user's machine | Two languages (JS + Go) and a sidecar to ship and sign |
| Second-level reflexes without paying for second-level LLM calls | Webull Go client must be hand-written |
| Webull holds money truth; our DB stays tiny | Partner approval (Connect) gates the SaaS "Log in with Webull" |
| Broker and data adapters allow Alpaca, Kalshi, IBKR later | Data licensing limits sharing quotes between users |
| Documents in D1 mean almost no migrations | JSON queries are weaker than real columns for analytics |
| Any agent can paper trade through MCP | Exposing trading to external agents widens the attack surface |

---

## 13. Bottlenecks (ranked)

1. **Market data rights and speed.** Webull: 3 pushes/s/connection, 5 connections/key,
   real-time needs paid entitlements (prices UNVERIFIED). Consolidated stocks + options
   realistically ~$99/mo (Alpaca). Futures depth needs CME licensing.
2. **LLM latency and cost.** Seconds per call; 30 agents × 288 loops/day is expensive on
   strong models. Solved by gating, caching analyst views, and cheap models for cheap jobs.
3. **Always-on runtime.** Laptops sleep. VPS costs money. Cloud DOs cost money if held open.
4. **Webull approvals.** Individual API approval, Connect partner approval, sandbox parity.
5. **Provider terms for subscriptions.** Using consumer AI subscriptions for 24/7 bots may
   be disallowed or rate-capped.
6. **Free-tier write caps.** D1 100k writes/day, KV 1k writes/day, DO 100k requests/day.
   Batch writes; journal locally; sync summaries.

## 14. Known issues and risks

- Webull docs conflict (options market orders; UK options; stream quotas). Test, don't trust.
- Paper fills are not real fills; sandbox data is 15-minute delayed by default. A paper
  strategy must use the same clock as its fills or results are meaningless.
- LLMs hallucinate tickers, sizes and prices. Intents are schema-validated and range-checked.
- Overfitting: every strategy gets a walk-forward verdict (*validated / trade-off / no edge*,
  QuantGPT idea) before it may go live.
- Key compromise: server-held live keys endanger everyone. Default keys-on-device; envelope
  encryption if any server mode exists; trade-only scopes; no withdrawal permission.
- Payment processors may refuse trading software. Apply early and describe it accurately.
- App stores: trading apps face review; IAP rules may apply to mobile subscriptions.

---

## 15. Payments

**Belief to correct:** Stripe is not the expensive option. Card fees are ~2.9% + $0.30 at
nearly every processor. What takes "a huge cut" is merchant-of-record markups (5% +
$0.50), Gumroad (~13%), and app stores (15–30%).

| Path | Effective fee at $19/mo | Handles sales tax/VAT | Notes |
|---|---|---|---|
| **Creem** (MoR) | ~6% | Yes | Recommended at launch; lowest MoR rate found (3.9% + $0.40) |
| Dodo Payments (MoR) | ~6% (UNVERIFIED) | Yes | Alternative |
| Paddle / Lemon Squeezy / Polar Starter | ~7.6% | Yes | Fallback if Creem rejects trading software |
| Stripe + Billing | ~5.2% | No (Stripe Tax +0.5%, you file) | Move here at ~$5–10k MRR |
| Coinbase Commerce (USDC) | 1% | No | Optional annual rail for crypto users; also the path to x402 per-call MCP billing |
| Apple / Google IAP | 15–30% | Yes | Avoid: sell on the web, unlock mobile by account |

Plan: web checkout only; annual billing as the default (one fixed fee per year); price
at $19+ (fixed fees ruin $10 plans). Entitlements come from the payment webhook into D1,
never from the client. Illustrative tiers (Loop 4 refines): **Free** (paper, cloud-only,
3 bots), **Grower** (~$19/mo: desktop runner, 30 bots, MCP), **Farmer** (~$39–49/mo:
live mode when legal, priority data, VPS template).

---

## 16. Side services and subscriptions

| Need | Build phase ($0) | Launch phase |
|---|---|---|
| Hosting, DB, storage | Cloudflare Free | Workers Paid $5/mo |
| Broker | Webull sandbox (free; live account needs ≥$100 for API approval) | Webull Connect partner (terms UNVERIFIED) |
| Market data | Webull delayed, IEX via Alpaca free, crypto public WS | Alpaca Algo Trader Plus $99/mo or Webull paid entitlements |
| AI | User's own keys / CLIs; owner uses existing subscriptions on own machine | Same (BYO). Optional DOGS-hosted cheap model later |
| Auth | Better Auth on D1 (passkeys, TOTP), Turnstile | Same |
| Errors / analytics | Sentry free, PostHog free, CF Web Analytics | Same |
| Alerts | Web Push, Telegram bot, Discord webhook | Same |
| Email | Cloudflare Email Routing (inbound) | Transactional sender (free tier) |
| Payments | none | Creem MoR (% only, no monthly fee) |
| Code signing | unsigned dev builds | Apple Developer $99/yr, Windows OV cert ~$215/yr (or Azure Trusted Signing), Google Play $25 once |
| Legal | none (own account, paper) | Fintech/securities lawyer before selling live trading |
| VPS for owner's 24/7 runner | optional | ~$4–6/mo |

Build phase total: **$0/mo**. Launch: **~$35–65/mo** fixed plus one-off signing fees.

---

## 17. Global markets

- Webull US API: US stocks/ETFs (24/5 overnight for eligible names), options, futures,
  spot crypto (24/7), event contracts. Not foreign exchanges.
- Webull HK API: HK stocks, China A-shares (Connect), US stocks/options, HK/US futures.
  Webull JP: US/JP stocks. Each requires an account in that region.
- For real global coverage (LSE, Euronext, TSX, ASX, FX): add an **IBKR adapter** later
  (needs a local gateway process, which the Go runner can host).

---

## 18. Phased roadmap

| Phase | Outcome | Exit test |
|---|---|---|
| 0 Spikes | Webull sandbox order via Go; MQTT quote stream in Go; gRPC trade events in Go; DO outbound WS test | Each spike runs 24 h without manual help |
| 1 Paper core | `m24d` with one garden, Reflex + Gate + Thinker, local journal, Svelte monitor | 7 days unattended paper trading with zero unreconciled orders |
| 2 Farm | 30 agents, budgets, decision board, sync to Cloudflare, Tauri desktop builds | 30 days unattended; cost per day known |
| 3 Open | Remote MCP, API keys, paper for external agents, cloud-only free tier | External agent completes paper round trip via MCP |
| 4 Pay | Creem checkout, entitlements, signed desktop builds, Android companion | First paying user |
| 5 Live (gated) | Live mode for the owner's own account, then for users after legal review | Lawyer sign-off; 90-day paper record per strategy |

---

## 19. Open questions for Loop 3+

1. Consumer AI subscription terms for local, unattended, 24/7 CLI use (Anthropic, OpenAI,
   xAI, Google) and the usage caps that would hit 30 agents.
2. Exact LLM prices and speeds (Jev, Groq, Cerebras, Gemini Flash, Haiku, GPT mini, Grok
   fast); finish §6 cost math.
3. Webull Go client feasibility: are trade-event protos published?
4. Webull OpenAPI market-data prices and redistribution terms.
5. Does Creem (or any MoR) accept trading software?
6. Tauri sidecar + keep-awake on Windows/macOS; Android background limits.
