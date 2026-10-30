# Money 24 — Architecture

Status: **Loop 3 revision** (2026-10-30). Planning only; no application code exists yet.
Evidence lives in [`docs/research/`](research/): Loop 1 research (`loop1-*`) and Loop 3
reviews (`loop3-*`). **UNVERIFIED** means a spike or written vendor answer is needed
before anything is promised to users.

Section references in the reviews (A§n) point at the Loop 2 draft; this revision
renumbers sections.

---

## 1. Goals, non-goals, and the honest version of "money while I sleep"

### Goals
1. **Paper first, safely.** A bot that paper trades unattended on Webull's API sandbox and
   never loses track of an order, even across crashes, sleep, timeouts and reconnects.
2. **A garden of agents.** Users plant *bots* (agents with a role), group them into
   *gardens* (one strategy over a universe), and run a *farm* (all gardens on one broker
   account). Up to 30 bots per user is a **product capacity**, not an architecture
   requirement: each extra AI role must earn its place (§4.3).
3. **Fast where speed matters, smart where it pays.** Reflexes in Go and broker-resting
   orders; LLMs on minute cadences; humans for anything that raises risk.
4. **Bring your own AI**, via API keys. No AI bill for DOGS.
5. **Open to other agents** through MCP and an HTTP API, paper first.
6. **Every screen.** Svelte web app; Tauri desktop (Windows, macOS; Linux best effort);
   Android as a monitor and remote control.
7. **No platform bill while building**, and a paid tier that a payment processor will
   actually accept (§14).

### Non-goals
- **No profit promise.** The system runs strategies, scores them honestly, and stops them
  when they lose. "Money while I sleep" is the hope, not a property the software has.
- **No HFT.** Webull's quote stream delivers at most 3 pushes/s per connection; LLMs take
  seconds. We compete on discipline, coverage and patience.
- **No consumer AI subscriptions in the automated loop** (§6.2). Account bans are the risk.
- **No live trading of other people's accounts** until a securities lawyer signs off.
- **No memecoins, MEV, pump.fun, cracked software.**
- **No Python requirement.** Python is an optional research sidecar only.

### What it costs to run (honest)
| Who pays | Build phase | One farm running 24/7 |
|---|---|---|
| DOGS (platform) | $0 (Cloudflare Free) | $0–5/mo until real users |
| Owner/user: AI | $0 if paused | **~$1–3/day** on the default Budget preset for one garden; more with more gardens (§6.4) |
| Owner/user: runtime | own computer (electricity) | or a VPS ~$4–6/mo |
| Owner/user: data | $0 (delayed / IEX / crypto public) | real-time stocks + options ~$99/mo, optional |

---

## 2. The big picture

```
 ┌───────────────────── CONTROL PLANE (Cloudflare, JS) — added in Phase 2+ ─────────────────────┐
 │ Pages (Svelte)  Workers API  Remote MCP (paper)  Auth  Billing webhook                       │
 │ Durable Object per user: hibernating WebSocket server for runner + UI; watchdog alarm        │
 │ D1: accounts, config docs, replicated ledger summaries   R2: archives, published ledgers     │
 └───────────────▲──────────────────────────────────────────────────────────────────────────────┘
                 │ runner dials out (one WS); idempotent sync keyed by IDs
 ┌───────────────┴──────────── RUNNER PLANE: Go engine "m24d" — v1 lives here ───────────────────┐
 │ Webull adapter (REST signed, MQTT quotes, gRPC trade events)    Data adapters                │
 │ Snapshot builder (staleness, clocks)   Reflex + risk (Go)   Strategy rules (Go)               │
 │ Supervisor (loops, budgets, queues)    Ledger (SQLite, hash chain, sole source of decisions) │
 │ Local MCP (stdio)   Keys in OS keychain   Executor lease                                     │
 │ Runs as Tauri desktop sidecar, or headless on a VPS — never both on one account at once      │
 └───────────────▲─────────────────────────────────────────────────▲────────────────────────────┘
                 │ structured JSON in/out, no tools                │ orders, fills, quotes
 ┌───────────────┴──── BRAIN (API keys) ───┐       ┌───────────────┴──── BROKER ────────────────┐
 │ Anthropic, OpenAI, Google, xAI,         │       │ Webull US sandbox → live (own account)     │
 │ OpenRouter, Jev; optional local model   │       │ later: Alpaca, Kalshi, Coinbase, IBKR      │
 └─────────────────────────────────────────┘       └────────────────────────────────────────────┘
```

**Why the loop lives in Go on the user's side and not on Cloudflare:** one always-on
Durable Object burns ~10,800 of 13,000 free GB-s/day; outbound sockets cannot hibernate;
Webull order events are gRPC, which Workers cannot speak
([Astra L1 §4](research/loop1-astra-broker-data.md), [Muse L1 §2](research/loop1-muse-platform-payments.md)).

**Record ownership (one owner per record, no split brain):**
| Record | Owner | Replicas |
|---|---|---|
| Balances, positions, broker orders, fills | Webull | runner cache |
| Intents, risk decisions, client order IDs, submissions, reconciliations, P&L | Runner ledger | D1 summaries, R2 published archive |
| Account, config, gardens, entitlements, API keys | D1 | runner cache (versioned) |
| Live presence, heartbeats | DO memory | none (not persisted) |

---

## 3. Languages (amends AGENTS.md — needs owner sign-off)

| Layer | Language |
|---|---|
| Svelte UI, Workers, remote MCP | JavaScript + JSDoc |
| Runner `m24d` | **Go** |
| Tauri host (tray, keep-awake, autostart, sidecar lifecycle) | Rust (Tauri's own; small custom code) |
| Research | Go first; Python optional sidecar |

Go is chosen for reliability, concurrency and one-file shipping, not raw speed: an order
round trip is tens to hundreds of ms and an LLM call is seconds either way.

**Webull from Go** ([Astra L3 §B1](research/loop3-astra-review.md)):
- Official SDKs: Python and Java. Webull also publishes an **official Go CLI** with signing
  code (internal package, not an importable SDK). A **community Go SDK** exists
  (`github.com/sfreiberg/webull`; licence and maintenance UNVERIFIED).
- Trade-event and quote **`.proto` files are published** in the official SDK repos, so the
  gRPC client can be generated.
- **Signing discrepancy:** docs specify HMAC-SHA1; the current Python SDK uses
  HMAC-SHA256; the Go CLI supports both. gRPC signing differs from REST signing. Pin one
  implementation and build cross-language golden test fixtures.
- Estimate: 2–3 day happy-path spike; **3–5 engineer-weeks** for a narrow, reliable adapter
  including reconciliation and fault tests.

Libraries: `eclipse/paho.mqtt.golang` (MQTT 3.1.1), `google.golang.org/grpc` +
`protobuf`, `modernc.org/sqlite` (CGO-free; pin with its `libc` version). No Go tray
library (it needs CGO); tray and keep-awake belong to the Tauri host.

Proposed AGENTS.md amendment: *"JavaScript only, except the runner engine in `engine/`
(Go) and the Tauri host (Rust). No TypeScript; Python never required."*

---

## 4. Decision tiers and agents

### 4.1 Tiers
| Tier | Cadence | Who | Does |
|---|---|---|---|
| **Broker-resting** | always, even if we are offline | Webull | Every entry carries a resting stop (bracket/OTO/OCO where supported). Positions that cannot carry a resting stop are size-capped. |
| **Reflex** | ms–1 s | Go | Risk limits, exposure reservation, staleness halt, kill states, reconciliation |
| **Rules** | 1–10 s | Go strategy code; optional Jev | Signals and "wake the thinker?" scoring. Never places orders alone in v1. |
| **Thinker** | 5–15 min or on wake | One LLM call per garden | Returns one structured intent (thesis, side, size, stop, target, expiry) or "no action" |
| **Strategist** | daily/weekly | LLM + backtester | Season reviews, walk-forward verdicts, retire bots |
| **Human** | on request | User | Live enable, risk-raising changes, approvals above limits |

**Rule:** an LLM produces an *intent*. Only Go turns an intent into an order, and only
after the Reflex checks pass.

### 4.2 v1 garden
`Rules (Go) → one Thinker call (structured JSON) → Risk check (Go) → order with resting stop`.
An optional "devil's advocate" call only for intents above a size threshold.

### 4.3 Earning more agents
The TradingAgents role set (analysts, bull/bear debate, trader, risk, portfolio manager;
Apache-2.0, credited in NOTICES) is the **menu**, not the default. Each extra role must
win an **ablation**: same garden, same season, with vs. without the role, scored against
a no-LLM rules baseline on the anchored paper record. Portfolio sizing and indicator
"analysts" are arithmetic: Go does them. Multiple roles, when used, are **one call
returning multi-role JSON**, not five calls.

Analyst caches expire on time **and** on events (price move > k·ATR, new headline,
session change).

---

## 5. Runner (`m24d`)

### 5.1 Components
- **Broker adapter contract:** capabilities (per region, account, environment, asset,
  order type, each with a *tested-on* date), accounts, instruments, positions/balances,
  preview/submit/cancel/replace, reconcile, events. Native IDs, multipliers, sessions and
  expiries preserved. Money as integer minor units or exact decimal strings, never floats.
- **Data adapters**, separate from execution. Webull MQTT: max 5 connections per app key,
  3 pushes/s each; **only one device may hold LV1/LV2 entitlements at a time**
  (Astra L3 §B2), so desktop and VPS cannot both stream.
- **Snapshot builder:** every value carries source time and receive time. Per-feed max
  age; a silence watchdog; gaps trigger a fresh snapshot and block new intents. Clock
  offset vs. broker time is measured; beyond a threshold the runner halts entries.
  Limit orders collared to the current quote; no market orders from AI intents.
- **Supervisor:** jittered schedules, per-provider queues, single-flight per garden,
  deadlines, hard token and dollar reservations. Throttled or out of budget → no new
  intents; protection and reconciliation continue. Two consecutive failures stop a bot.
- **Ledger:** local SQLite. The **only** hash chain (§8.3). Intent and client order ID
  written in one transaction **before** submission.
- **Local MCP:** stdio by default. Optional localhost HTTP requires a pairing token plus
  strict `Host` and `Origin` checks (DNS-rebinding defence).
- **Secrets:** broker and AI keys in the OS keychain (Windows Credential Manager, macOS
  Keychain, Secret Service on Linux), never in env vars, files, logs or prompts.

### 5.2 Order safety (the part that loses money if wrong)
1. **Deterministic client order ID** = hash(intent ID, attempt). A timeout makes the order
   `UNKNOWN`, never "resubmit with a new ID".
2. `UNKNOWN` **blocks the symbol** until the reconciler resolves it from broker state.
3. **Intents expire** (TTL); an expired intent is never submitted, and orders are never
   queued while offline.
4. **Reconciler** polls broker orders and positions on a fixed cadence and after every
   reconnect; broker state wins. Manual trades made in the Webull app are detected and
   adopted or flagged.
5. **Exposure is reserved atomically** across gardens before submission, so two gardens
   cannot take opposite or doubled positions on one account by accident.
6. **Single executor per account:** a lease with an epoch number. A runner that loses
   its lease stops submitting. **No automatic failover** between desktop and VPS; handoff
   is explicit and starts with reconciliation.
7. **Order state machine** (separate from intent state):
   `intent: proposed → approved | vetoed | expired`
   `order: pending_submit → submitted → acknowledged → partially_filled → filled`
   `       ↘ rejected | cancel_pending → cancelled | expired | unknown → (reconciled state)`
   `position: open → closing → closed`
8. **Asset-specific rules before each asset class ships:** options long-only defined risk,
   closed by T−1, assignment monitoring; futures cash-settled micros with a roll calendar;
   crypto weekend liquidity limits; event contracts marked at the executable side and
   settlement tracked.

### 5.3 Stop controls (replaces "Frost")
| Control | Effect |
|---|---|
| **Pause** | No new intents or entries. Resting stops and exits stay. |
| **Cancel entries** | Cancels working entry orders only; protective exits stay. |
| **Flatten** | Closes positions at collared limits. Explicit confirmation. |
Every control shows *requested → runner acknowledged → broker confirmed / failed*.

**When the runner is offline**, nothing on Cloudflare can touch the broker (keys stay on
the user's side). Protection then comes from broker-resting stops. Each garden declares
an **offline policy** (e.g. "no new positions within 30 minutes of close", "max overnight
exposure"). The phone shows *last confirmed state* and how to act in the Webull app.

### 5.4 Where it runs
| Mode | Notes |
|---|---|
| **Desktop sidecar** (default) | Tauri `externalBin`, one Go build per target triple, sidecar signed before bundling (Authenticode; macOS hardened runtime + notarization). Keep-awake (opt-in; Windows `SetThreadExecutionState`, macOS IOKit assertion) and autostart in the Tauri host. Autostart opens to tray **unarmed**; arming requires a fresh reconcile. Updates never restart an armed engine silently. Single-instance lock. |
| **VPS** | `m24d --headless`. Always on. Keys in an encrypted file unlocked at start (no OS keychain on servers). |
| **Android** | Monitor and remote control only. Tauri mobile cannot run sidecars; Android kills background work. Separate Android Tauri config with no `externalBin`. |
| **Cloud-only** | Deferred. Later: paper-only, minute cadence, one batched Cron tick for all such users. |

---

## 6. AI connections

### 6.1 Allowed
1. **API keys** (Anthropic, OpenAI, Google, xAI, OpenRouter, Jev). Default and only path
   for automated loops.
2. **Local models** (Ollama / LM Studio / OpenCode), optional. Gate and summaries; not
   trusted for intents until benchmarked against the rules baseline.
3. **Owner-started strategist runs** through a CLI the user already uses (low frequency,
   human-initiated). Not part of the 5-minute loop.

### 6.2 Not allowed in the automated loop (per provider)
| Provider | Finding | Decision |
|---|---|---|
| Anthropic (Claude Pro/Max via `claude -p`) | Consumer terms bar automated access except via API key; plan limits assume "ordinary, individual usage"; third-party subscription use blocked 2026-04-04 ([Claude L3 §B1](research/loop3-claude-redteam.md)) | **No.** API key only. |
| OpenAI (Codex / ChatGPT) | `codex exec` in scripts is documented; "Sign in with ChatGPT" (SIWC) exists for selected partners and open-source apps, with terms: local token storage, consent for background use, **no paywalling the integration**; Plus limits are far below 30 bots × 288 loops/day ([Astra L3 §B3](research/loop3-astra-review.md)) | **Not in v1.** Possible later as a free, local, opt-in, low-frequency feature after applying for SIWC. |
| Google (Gemini CLI / Antigravity / AI Pro) | Terms forbid third-party use of its sign-in (Claude L3 §B2, partly UNVERIFIED) | **No.** API key only. |
| xAI (SuperGrok) | Subscription-linked API accounting exists; third-party path UNVERIFIED | **No** until xAI confirms. API key. |

Requiring OpenCode at install: **no**. It is an optional local-model integration.

### 6.3 Prompt-injection containment
- Thinkers get **no built-in tools** (no shell, files, web). Inputs arrive as quoted JSON
  fields; outputs must match a JSON schema.
- News and web text are untrusted data. The universe whitelist, size, price collar and
  rate limits are enforced in Go regardless of what the model says.
- Approvals bind to an immutable order payload, snapshot version and expiry.

### 6.4 Cost presets (per user, per garden-equivalent of 30 loops every 5 min)
From [Muse L3 §B2](research/loop3-muse-review.md), 3k input / 500 output tokens per
Thinker call. Prices drift and some figures conflict between reviewers (e.g. Grok "Fast"
models were retired 2026-05-15 per Astra); re-verify before showing users.

| Preset | Gate → Thinker | Escalation 10% | 20% |
|---|---|---|---|
| **Budget** (default) | Rules → Gemini Flash-Lite class | ~$1.30/day (~$39/mo) | ~$2.60/day (~$78/mo) |
| Balanced | Rules or Jev → mini/Haiku class | ~$0.50–1.50/day | ~$1–3/day |
| Smart | Jev → Sonnet class | ~$9.50/day (~$285/mo) | ~$19/day (~$570/mo) |
| Local-only | Local model | $0 + electricity | $0 |
| *No gate at all, Sonnet* | — | *~$95/day (~$2,850/mo)* | — |

Defaults: Budget preset, escalation target ≤10%, **per-farm cap $2/day with hard pause**.
The v1 single garden on a smaller universe targets ~200 Thinker calls/day (~$0.30–1/day).
Debate roles multiply cost 3–5×, which is another reason they are off by default.

Jev (TypeSafe, `typesafe/jev-1.14` on OpenRouter): a decision model, not an LLM; 70–500 ms
vendor claim; $0.042/M input, output free. Fits the Rules tier as an optional gate
(~$0.36/day if called every loop). Reported overconfidence on policy-style labels must be
tested on our own labelled data first.

---

## 7. Control plane (Cloudflare, Phase 2+)

| Piece | Job |
|---|---|
| Pages | Svelte app, static fallback, landing page |
| Workers API | Auth, config, entitlements, ledger summary sync, HTTP API |
| Durable Object per user | **Server only**: the runner dials in over one WebSocket; UIs connect too; hibernates between messages. A DO **alarm** runs the offline watchdog (no traffic ≠ code runs). Heartbeats are WS frames held in memory, never persisted. |
| D1 | Accounts, config docs, entitlements, hourly ledger aggregates, intents/orders summaries |
| R2 | Published ledgers, archives, candles written **once per symbol per day** |
| Remote MCP | `createMcpHandler` (Agents SDK; `McpAgent` is deprecated) + `@cloudflare/workers-oauth-provider`. Paper tools only. Workers Paid before inviting outside agents. |

KV is not used in v1 (OAuth provider tokens excepted, with a write budget).

### Free-tier write discipline ([Claude L3 §A8](research/loop3-claude-redteam.md))
- Never one D1 row per loop: `loop_run` stays in the runner ledger; D1 gets hourly
  aggregates plus intents, orders and errors only.
- Heartbeats never persisted. R2 never rewritten per minute.
- External agents: per-key daily counters in the DO, edge-cached quotes, WebSocket
  resources instead of polling, long-lived access tokens with refresh rotation.
- Sentry/analytics client-side rate limits so a crash loop cannot flood free quotas.

### Market data rights
An individual's OpenAPI subscription is for **non-display** use by that user. Showing
quotes to other users needs Webull's **Hosted Display Solution** (partner route, terms
UNVERIFIED) or another licensed vendor. Until then: each runner uses its owner's
entitlements; anything shown across users is delayed or venue data clearly labelled.

---

## 8. Data model

### 8.1 Config: documents, no migrations per field
D1 and runner SQLite share a `docs` table: `kind`, `id`, `owner_id`, `version`, `json`,
plus a few generated columns (`status`, `symbol`, `created_at`, `updated_at`) for
indexing. New config fields are new JSON keys with no migration. Kinds: `connection`,
`farm`, `garden`, `bot`, `api_key`, `mcp_client`, `entitlement`, `season`.
Identity tables belong to the auth library.

### 8.2 Money: explicit tables
Orders, fills, cash movements, fees, exposure reservations and inbox/outbox IDs get real
columns and constraints, because duplicates and float rounding lose money. This is where
the few migrations go. Honest summary for the owner: **config stays schemaless; money
does not.** Expect a handful of additive migrations a year, not hundreds.

JSDoc validators (JS) and Go structs both read one JSON Schema file per kind, with
contract tests, so the shape is not maintained twice by hand.

### 8.3 The ledger and a publishable track record
- Runner ledger is the only chain; D1 and R2 are replicas.
- Entries are canonical JSON (RFC 8785); `hash = SHA-256(prev_hash ‖ entry)`; fills record
  the quote used (bid, ask, source, source time).
- Each runner has an Ed25519 device key and signs daily chain heads.
- **Pre-registration:** a season starts by committing to the hash of its strategy, prompts,
  models, universe, limits and fill model before the first trade.
- Daily heads anchored for free with **OpenTimestamps** and **Sigstore Rekor**; full
  entries published in R2 with a verify script. **All seasons are published**, losers too.
- Without anchoring it is called *integrity checking*, not *tamper-proof*.

### 8.4 Retention
Webull keeps order history (individual API back to 2018-05-21); positions are current-
only; historical P&L is ours. Candles live in R2. Local ledger backups are encrypted;
disk-full and corruption have defined recovery steps.

---

## 9. MCP and public API

- **Local MCP** (stdio) for agents on the same machine, the fast path.
- **Remote MCP** (Phase 4): OAuth 2.1 with scopes `read`, `paper_trade`. **No live-trade
  scope exists** until the legal gate passes. Tools modelled on alpaca-mcp-server (MIT):
  `get_account`, `get_positions`, `get_quote`, `get_bars`, `list_gardens`,
  `propose_intent`, `place_paper_order`, `cancel_order`, `get_ledger`, `get_loop_status`.
  Ships a `skill.md` onboarding file.
- HTTP API mirrors the tools; keys scoped per garden; per-key rate limits.
- CORS: exact allow-list of the web origin and each shipped Tauri origin; bearer tokens,
  no cookies cross-origin; Tauri pinned at or above the CVE-2026-42184 fix (Claude L3).

---

## 10. Monitoring

- Runner ledger holds every loop run (model, tokens, cost, gate score, outcome, summary).
- D1 gets hourly aggregates; the UI streams live runs from the runner through the DO.
- Offline watchdog: DO alarm; alerts deduplicated; one opt-in channel first (Web Push or
  Telegram).
- Budgets: per bot and per farm daily caps; hitting a cap pauses new intents only.
- Errors: one sink (Sentry free) with redaction of keys, account numbers, balances and
  prompts. Analytics later, opt-in, no financial payloads.

---

## 11. Legal and policy gates (not legal advice)

- **Own account, paper:** still subject to broker terms, data licences and privacy.
- **Selling software** the user configures and runs is the safest framing; it does not by
  itself settle adviser or broker-dealer questions.
- **Customer live trading, performance fees, copy-trading, signals** can trigger
  investment adviser, broker-dealer or CTA/CPO (futures) registration. Lawyer first.
- **Anthropic Usage Policy (effective 2026-11-12):** personalised buy/sell recommendations
  are high-risk and need qualified human review. Customer live mode therefore defaults to
  **approve each intent**.
- **PDT:** FINRA's intraday-margin replacement took effect 2026-06-04, transition to
  2027-10-20; read Webull account capabilities, never hardcode.
- **Prediction markets:** state conflicts exist (e.g. Nevada vs. Kalshi); enforce broker
  eligibility; never route around geoblocks.
- **Marketing a paper record** to sell a product needs the lawyer too (hypothetical
  performance and earnings-claim rules).

---

## 12. Pros and cons

| Pros | Cons |
|---|---|
| $0 platform cost until real users | Always-on depends on the user's machine or a VPS |
| Broker keys and AI keys never leave the user's side | No remote control when the runner is offline (resting stops cover it) |
| Second-level reflexes without second-level LLM bills | Two runtimes (JS + Go) plus a small Rust host to sign and ship |
| Webull holds money truth; our database stays small | Webull Go client and reconciliation are real work (3–5 weeks) |
| Adapters allow Alpaca, Kalshi, IBKR later | Data licences block sharing quotes between users |
| Publishable, anchored paper record | Honest records will often show "no edge" |
| Any agent can paper trade via MCP | More surface to secure and rate-limit |

---

## 13. Bottlenecks and risks (ranked)

1. **Approvals:** Webull API (needs ≥$100 account for live API; sandbox rules UNVERIFIED),
   Webull Connect partner status for "Log in with Webull", payment processor acceptance,
   OpenAI SIWC eligibility.
2. **Order correctness:** unknown submissions, partial fills, reconnect duplicates,
   manual trades (§5.2). Solved by design and fault-injection tests, not speed.
3. **Runner liveness:** laptops sleep; one device per data entitlement.
4. **AI cost** per decision (§6.4); unmeasured until a 7-day workload runs.
5. **Strategy edge:** most strategies have none after costs. The walk-forward verdict
   (*validated / trade-off / no edge*) and the no-LLM baseline say so early.
6. **Data:** delayed sandbox data (15 min by default) makes paper results meaningless unless
   fills use the same clock as decisions; real-time costs money; redistribution needs a licence.
7. **Free-tier writes** (§7) once Cloudflare sync exists.
8. **Docs drift:** Webull docs conflict (options market orders, UK options, stream quotas,
   signing algorithm). Test; don't trust.

---

## 14. Payments

**Correction from Loop 2:** the cheapest merchants-of-record **prohibit this product**.
Creem bans "trading and investment tools … that enable or facilitate trade execution or
automation"; Polar bans "trading bots"; Paddle bans "trading signals and strategies";
Lemon Squeezy and Dodo are unclear ([Muse L3 §B3](research/loop3-muse-review.md)).

And Stripe is not the expensive option: card processing is ~2.9% + $0.30 nearly
everywhere. The large cuts come from MoR markups, Gumroad (~13%) and app stores (15–30%).

Plan:
1. **Stripe first** (financial software is reviewed case by case, not category-banned).
   Apply describing exactly: *paper-trading and automation software the user controls; no
   custody, no advice, no signals*. Budget sales tax (Stripe Tax +0.5% or a filing service).
2. In parallel, ask **Dodo** for written pre-clearance as an MoR alternative.
3. **Launch paid tiers paper-only.** Live features are not sold until both the processor
   and a lawyer approve.
4. Web checkout only; Android/desktop unlock by account. App-store distribution is a
   separate decision per store.
5. Price at **$19+/mo**, annual by default (fixed fees ruin $10 plans).
6. Tiers sell **software capacity** (gardens, bots, MCP clients, history), never signals,
   picks or performance. A future SIWC integration must stay free.
7. Webhooks: signature check, unique event IDs, replay protection, grace states. A lapsed
   subscription blocks new strategies but **never** disables protection, reconciliation,
   position view or export.
8. Crypto rail (USDC, 1%) and x402 per-call MCP billing: later, optional.

---

## 15. Side services and subscriptions

| Need | Build phase | Launch |
|---|---|---|
| Hosting | Cloudflare Free | Workers Paid $5/mo |
| Broker | Webull sandbox | Webull live (owner); Connect partner (users, terms UNVERIFIED) |
| Data | Webull delayed, Alpaca IEX free, crypto public WS | Alpaca Algo Trader Plus $99/mo (owner's runner) or Webull OpenAPI entitlements (prices UNVERIFIED; ask Webull for a written quote) |
| AI | Owner's API keys, Budget preset | Users bring their own keys |
| Auth | Passkeys + recovery codes (Better Auth on D1, verified in a spike) | + transactional email sender |
| Errors | Sentry free (redacted) | same |
| Alerts | one opt-in channel | Web Push + Telegram |
| Payments | none | Stripe (+ Stripe Tax) |
| Signing | unsigned dev builds | Apple Developer $99/yr; Windows signing (Azure Trusted Signing or OV ~$215/yr); Play $25 once |
| Runtime | owner's PC | optional VPS ~$4–6/mo |
| Legal | none | securities/fintech lawyer before selling anything live |

Totals are worked out in a launch worksheet in Loop 4, with unknowns left blank rather than
priced at zero.

---

## 16. Global markets

- Webull US API: US stocks/ETFs (24/5 overnight for eligible names), options, futures,
  spot crypto, event contracts. Not foreign exchanges.
- Webull HK: HK stocks, China A-shares, US stocks/options, HK/US futures. Webull JP: US/JP
  stocks. Each needs a regional account.
- Real global coverage (LSE, Euronext, TSX, ASX, FX): an **IBKR adapter** later; its local
  gateway can be hosted by the Go runner.

---

## 17. Roadmap

| Phase | Scope | Exit test |
|---|---|---|
| **0 Spikes** | Go: Webull signing (golden fixtures), sandbox order, MQTT quotes, gRPC events; keychain; Tauri sidecar build+sign on Windows/macOS | Each spike runs 24 h; fault tests below pass |
| **1 Paper core** | One Webull US sandbox account, **stocks/ETFs only**, one garden: rules → one thinker → Go risk; ledger; local Svelte monitor in Tauri | 7 days unattended, zero unreconciled orders, all fault tests pass |
| **2 Farm + sync** | More gardens, budgets, decision board, DO sync, phone monitor, published anchored ledger | 30 days unattended; measured cost/day |
| **3 More assets** | Each of options, futures, crypto, events behind its own acceptance test | Per-asset exit tests |
| **4 Open** | Remote MCP (paper scope), API keys, `skill.md` | External agent paper round trip |
| **5 Pay** | Stripe, entitlements, signed desktop builds | First paying user, paper-only |
| **6 Live (gated)** | Owner's own account first; users only after lawyer + processor approval | Written sign-offs |

**Fault-injection exit tests** (Phase 0–1): drop the response after the broker accepted;
crash between ledger write and send; duplicate and reorder events; disconnect while
cancelling; expire tokens; fill the disk; suspend/resume the laptop; start two runners on
one account; feed a stale quote; skew the clock.

---

## 18. Open questions for Loop 4–5

1. Business model given BYO AI costs (~$39+/mo Budget for a full farm): who is the paying
   customer, and what do they pay DOGS for?
2. Launch cost worksheet; Webull data quote; Connect partner terms.
3. Exact sync protocol (runner ↔ DO messages) and API/MCP schemas.
4. Licence check of `sfreiberg/webull` and which signing algorithm the sandbox accepts.
5. Spike plan with owners and order for Phase 0.
