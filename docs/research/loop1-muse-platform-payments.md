# Loop 1 — Cloudflare Platform, Data Model, Payments, Side Services

> Agent: Muse (Loop 1) · Date: 2026-10-30 · Scope: Cloudflare platform limits, data model, payments, side services, legal basics for Money 24 by DOGS.
> Method: live web research, official docs preferred (`developers.cloudflare.com`). Prices/limits verified 2026-10-30 unless noted. Anything not verified is marked **UNVERIFIED**. This is planning research only — no application code.
> Companion brief: [BRIEF.md](BRIEF.md). Repo rules: [AGENTS.md](../../AGENTS.md) — SQL migrations are the schema source of truth, Workers + D1 + R2, free tier while building, JS + JSDoc only, no ORM.

**Blunt summary up front:** Cloudflare free tier is excellent for *building* Money 24 and for a handful of paper-trading users. It cannot run a 24/7, second-interval, always-on WebSocket trading fleet for many users for free — the Durable Object (DO) duration meter and the 100k requests/day cap kill that first. The winning shape is: Cloudflare for API/UI/auth/billing/control-plane + the user's own machine (Tauri) or a cheap VPS/container as the always-on runner. Do not try to hold 100 broker WebSockets open inside DOs on free tier. Details and math below.

---

## 1. Cloudflare free-tier limits (late 2026) + Paid $5/mo + request budget model

All figures below are from Cloudflare's official pricing/limits pages fetched 2026-10-30. Free limits reset daily at 00:00 UTC and **hard-fail** (error, not overage billing) when exceeded. Paid has no daily cap; you pay overages.

### 1.1 Master limit table — Free vs Workers Paid ($5/mo)

| Product | Free tier (per account unless noted) | Workers Paid $5/mo included | Overage (Paid) | Source |
|---|---|---|---|---|
| **Workers** requests | 100,000 / day | 10M / month | $0.30 / M requests | [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/) |
| **Workers** CPU | 10 ms / invocation | 30M CPU-ms / month; max 5 min/invocation (default 30s); 15 min for Cron/Queue consumer | $0.02 / M CPU-ms | [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/), [Workers limits](https://developers.cloudflare.com/workers/platform/limits/) |
| **Workers** memory / size | 128 MB; 64 MiB script; 100 Workers; 50 subrequests/req | 128 MB; 64 MiB; 500 Workers; 10,000 subrequests/req | — | [Workers limits](https://developers.cloudflare.com/workers/platform/limits/) |
| **Durable Objects** requests | 100,000 / day (HTTP + RPC sessions + WS msgs at 20:1 + alarms) | 1M / month | $0.15 / M | [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) |
| **DO duration** | 13,000 GB-s / day | 400,000 GB-s / month | $12.50 / M GB-s | [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) |
| **DO SQLite** rows read | 5M / day | 25B / month incl. | $0.001 / M rows | [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) |
| **DO SQLite** rows written | 100,000 / day | 50M / month incl. | $1.00 / M rows | [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) |
| **DO SQLite** stored | 5 GB total; 10 GB max per DO; 100 DO classes | 5 GB-mo incl.; unlimited total | $0.20 / GB-mo | [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/), [DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/) |
| **D1** rows read | 5M / day | 25B / month incl. | $0.001 / M rows | [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/) |
| **D1** rows written | 100,000 / day | 50M / month incl. | $1.00 / M rows | [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/) |
| **D1** storage / DBs | 5 GB total; 500 MB max per DB; 10 DBs; 7-day Time Travel | 5 GB incl.; 10 GB max per DB; 1 TB acct; 100k DBs incl. | $0.75 / GB-mo; $0.10/DB/mo beyond 100k | [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/), [D1 limits](https://developers.cloudflare.com/d1/platform/limits/) |
| **R2** (Standard) | 10 GB-mo/mo; 1M Class A (writes/lists)/mo; 10M Class B (reads)/mo; egress free | Same free tier applies; then pay | $0.015/GB-mo; $4.50/M Class A; $0.36/M Class B; egress $0 | [R2 pricing](https://developers.cloudflare.com/r2/pricing/) |
| **KV** keys read | 100,000 / day | 10M / month | $0.50 / M | [KV pricing](https://developers.cloudflare.com/kv/platform/pricing/) |
| **KV** keys written/deleted/list | 1,000 each / day; 1 GB stored; 1,000 namespaces | 1M each / month; 1 GB incl. | $5.00 / M; $0.50/GB-mo | [KV pricing](https://developers.cloudflare.com/kv/platform/pricing/) |
| **Queues** ops | 10,000 ops / day; 24h retention (fixed) | 1M ops / month; 4d default → 14d max retention | $0.40 / M ops (3 ops per delivered msg: write+read+delete) | [Queues pricing](https://developers.cloudflare.com/queues/platform/pricing/) |
| **Cron Triggers** | 5 per account; min interval 1 min; 10 ms CPU (as Worker) | 250 per account; min 1 min; 30s CPU (<1h interval) / 15 min (≥1h) | Counts as Worker requests/CPU | [Workers limits](https://developers.cloudflare.com/workers/platform/limits/), [Cron docs](https://developers.cloudflare.com/workers/configuration/cron-triggers/) |
| **Workflows** | 100k exec/day (shared w/ Workers); 100 concurrent; 1,024 steps; 100MB state/instance; 3d retention | Unlimited exec; 50,000 concurrent; 10k steps (→25k); 1 GB state; 30d retention | Billed as Workers requests/CPU per step | [Workflows limits](https://developers.cloudflare.com/workflows/reference/limits/) |
| **Workers AI** | 10,000 Neurons / day | 10,000 Neurons/day free, then pay | $0.011 / 1,000 Neurons (≈ per-model token rates) | [Workers AI pricing](https://developers.cloudflare.com/workers-ai/platform/pricing/) |
| **AI Gateway** | Core (analytics, caching, rate-limit) free; 10 gateways; 100k logs total (legacy) | Unlimited gateways (20 incl.); 10M logs/gateway (legacy) | Unified Billing: +5% on credit purchase; inference pass-through, no markup | [AI Gateway pricing](https://developers.cloudflare.com/ai-gateway/reference/pricing/) |
| **Pages** | 500 builds/mo; 1 concurrent; 20k files/site; 25 MiB/file; Functions billed as Workers | Via zone plan (Pro: 5k builds, 5 concurrent, 100k files) | Functions = Workers pricing | [Pages limits](https://developers.cloudflare.com/pages/platform/limits/) |
| **Hyperdrive** | 100,000 queries / day | Unlimited queries (pooling/caching incl.) | Included in Workers Paid; external DB billed separately | [Hyperdrive pricing](https://developers.cloudflare.com/hyperdrive/platform/pricing/) |
| **Containers** | **Not available** (N/A) | 25 GiB-hrs/mo + 375 vCPU-min/mo + 200 GB-hrs disk incl. | $0.0000025/GiB-s mem; $0.000020/vCPU-s; egress $0.025/GB NA/EU (1 TB incl.) | [Containers pricing](https://developers.cloudflare.com/containers/platform/pricing/) |
| **Sandbox SDK** | Not available (needs Containers/Paid) | Billed as Containers + Workers + DO + Logs | Same as Containers | [Sandbox pricing](https://developers.cloudflare.com/sandbox/sdk/platform/pricing/) |

Notes that matter for Money 24:

- **Free = SQLite-backed DOs only.** KV-backed DO namespaces are Paid-only (legacy). New DO classes must use SQLite storage. ([DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/))
- **Workers AI free 10k Neurons/day is tiny for trading agents.** Example: Llama 3.1 8B ≈ 4,119 Neurons/M input tokens + 34,868/M output. 10k Neurons ≈ ~150k input tokens *or* ~15k output tokens per day for the whole account. One busy agent loop eats it. Money 24's plan to use the *user's own* LLM subscription/key (via AI Gateway BYOK / direct provider call) is therefore not optional — it is required. AI Gateway itself is free for routing/analytics; the LLM tokens are billed by the user's provider, not Cloudflare. ([Workers AI pricing](https://developers.cloudflare.com/workers-ai/platform/pricing/), [AI Gateway pricing](https://developers.cloudflare.com/ai-gateway/reference/pricing/))
- **Pages Functions = Workers.** Every chart API call, auth check, etc. through Pages Functions burns the same 100k/day. Hosting the SPA as static assets is free/unlimited; dynamic calls are not. ([Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/))
- **Cron minimum is 1 minute.** There is no sub-minute Cron. "Decisions at second intervals" cannot be driven by Cron Triggers. Seconds-level loops need a long-lived process (DO alarm loop, container, VPS, or local Tauri runner) or client-side polling — see §2. ([Cron docs](https://developers.cloudflare.com/workers/configuration/cron-triggers/))
- **Containers/Sandbox require Paid.** No free always-on Linux process on Cloudflare. A single `basic` (¼ vCPU, 1 GiB) container running 24/7 ≈ 730h × 1 GiB ≈ 730 GiB-hrs → (730−25) × $0.0000025×3600 ≈ **~$6.35/mo memory** + CPU-active + DO/Worker overhead. A `lite` (256 MiB) 24/7 ≈ **~$0.90/mo** memory overage. Cheap, but not $0, and still per-instance — 100 users × 24/7 container each is a real bill (see §1.3). **UNVERIFIED:** exact sleep/wake behaviour for idle containers in late 2026 — verify `sleepAfter` defaults before costing. ([Containers pricing](https://developers.cloudflare.com/containers/platform/pricing/))

### 1.2 The $5/mo Paid plan — what it actually buys

$5/mo is an account minimum covering Workers + Pages Functions + KV + Hyperdrive + DOs. Included monthly: 10M Worker requests, 30M CPU-ms, 1M DO requests, 400k DO GB-s, D1 25B reads/50M writes, KV 10M reads/1M writes, Queues 1M ops, Hyperdrive unlimited queries. ([Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/))

In practice for Money 24, $5/mo moves you from "hard wall at 100k/day" to "pay pennies per million." Worker requests at $0.30/M are negligible; **DO duration at $12.50/M GB-s is the only meter that can surprise you** (see §2). Everything else (D1, KV, Queues, R2) stays in included/free at Money 24's early scale. Budget $5–15/mo total Cloudflare at launch for ≤100 light users *if* you do not hold always-on DOs/containers per user (see model below).

### 1.3 Request-budget model — 1 / 10 / 100 users × ~30 agent loops every 5–10 min + live charts

**Assumptions (state yours differently and rescale linearly):**
- 30 agent loops per user, tick every 7.5 min avg (midpoint of 5–10) = 8 ticks/hr/agent = **192 ticks/day/agent** = **5,760 agent ticks/day/user**.
- Each tick = 1 Worker invocation (orchestrator) + 1 DO RPC/alarm invocation + ~3 D1 rows written (decision log, order intent, heartbeat) + ~5 D1 rows read (portfolio, rules, recent candles index) + 0.2 LLM calls avg (most ticks are rule checks; 1 in 5 calls the LLM). LLM tokens billed to user's own key, not Workers AI.
- Live charts: user has dashboard open 4 hrs/day, 1 WebSocket per user (hibernatable, DO as server) receiving 1 quote update / 5s fanned out from a shared feed = 2,880 inbound WS msgs/day/user → billed as 2,880/20 = **144 DO requests/day/user**. If you instead poll REST every 5s: 2,880 Worker requests/day/user — *never poll; use WS.*
- Market-data ingest is **shared**, not per-user: 1 poller (Cron 1-min or 1 DO alarm loop) fetching ~50 symbols = ~72k upstream fetches/day total, fanned out. Counted once, not × users. If you poll per-user, multiply everything by users and you die — don't.
- Cron: 1 trigger/minute account-wide for housekeeping = 1,440 invocations/day.

| Meter (daily) | 1 user | 10 users | 100 users | Free limit/day | Paid incl./mo (≈/day) | Verdict |
|---|---|---|---|---|---|---|
| Agent ticks | 5,760 | 57,600 | 576,000 | — | — | driver of everything |
| Worker requests (ticks + charts WS upgrade + API) | ~6,000 | ~58,000 | ~580,000 | 100,000 | 10M/mo (~333k/day) | Free OK to ~15 users; Paid overage at 100 users ≈ (17.4M−10M)×$0.30 ≈ **$2.20/mo** — trivial |
| DO requests (ticks + alarms + WS 20:1) | ~5,900 | ~59,000 | ~590,000 | 100,000 | 1M/mo (~33k/day) | **Free breaks ~17 users. Paid overage at 100 users ≈ (17.7M−1M)×$0.15 ≈ $2.50/mo** — trivial |
| D1 rows written (3/tick) | 17,280 | 172,800 | 1,728,000 | 100,000 | 50M/mo (~1.6M/day) | **Free breaks at ~6 users on writes alone.** Paid fine to ~95 users, then $1/M — still cheap |
| D1 rows read (5/tick) | 28,800 | 288,000 | 2,880,000 | 5,000,000 | 25B/mo | Free OK to ~170 users; Paid effectively unlimited here |
| KV writes (session/config churn) | ~50 | ~500 | ~5,000 | 1,000 | 1M/mo | **Free breaks ~20 users** if you write KV per tick — so don't; KV is config/session only, DO/D1 is hot state |
| Queues ops (3× msgs, 1 msg/tick) | 17,280 | 172,800 | 1,728,000 | 10,000 | 1M/mo | **Free breaks on day 1 for even 1 user** if every tick goes through a Queue. Use Queues only for order execution / retries, not every tick |
| DO duration — hibernating design (DO wakes per tick/alarm, sleeps between) | ~1,500 GB-s | ~15,000 GB-s | ~150,000 GB-s | 13,000 | 400k/mo (~13k/day) | Free OK for 1 user only; Paid overage at 100 users ≈ (4.5M−400k)×$12.50/M ≈ **$51/mo** — the real cost driver. Optimise tick CPU/wake time |
| DO duration — always-on non-hibernating (1 DO/user held open) | 10,800 GB-s | 108,000 GB-s | 1,080,000 GB-s | 13,000 | 400k/mo | **Free: 1 user eats 83% of daily budget; 2 users impossible. Paid 100 users ≈ (32.4M−400k)×$12.50/M ≈ $400/mo.** Never do this — see §2 |
| Workers AI Neurons (if using CF models, 2k tok/tick LLM call × 20% ticks) | ~40k Neurons | ~400k | ~4M | 10,000 | 10k/day free then $0.011/1k | **Free dead immediately.** Must use user's own LLM key |

**Which limits hit first (in order):**
1. **Queues 10k ops/day** — if you queue every tick (don't).
2. **DO duration 13,000 GB-s/day** — if any DO stays awake (the silent killer; 1 always-on DO ≈ 10,800 GB-s/day).
3. **D1 writes 100k/day** — ~6 users at 3 writes/tick.
4. **KV writes 1,000/day** — if misused for hot state.
5. **Workers + DO requests 100k/day** — ~15–17 users.
6. Workers AI Neurons — instant, if you use CF-hosted models for agents.

**Takeaway:** Free tier comfortably supports **1–5 real users paper-trading** with a disciplined hibernating design (alarms + hibernatable WS, shared market feed, Queues only for orders). At 10 users you need Paid ($5) mainly to lift D1-write and request caps. At 100 users Cloudflare infra is still only ~$10–60/mo *provided* DOs hibernate between ticks and the runner is not 100 always-on DOs/containers. The architecture decision in §2 decides whether 100 users cost $15/mo or $400+/mo.

> Sensitivity: if ticks go to every 5 min (12/hr) multiply agent-driven rows by 1.5×; every 10 min multiply by 0.75×. If "second intervals" means 1 tick/sec/agent, multiply by ~11,000× — no Cloudflare free/paid design survives that server-side; seconds-level decisions must happen locally (Tauri/VPS) with only summaries synced to Cloudflare. See §2.4.

---

## 2. Can a Durable Object hold a long-lived outbound WebSocket to a broker/exchange 24/7?

**Short answer: technically yes on Paid, practically no on Free, and it's the most expensive way to do it. Don't make it the default.**

### 2.1 How DO WebSockets actually work (the hibernation trap)

Cloudflare has two WebSocket modes inside a DO ([DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/), [DO lifecycle](https://developers.cloudflare.com/durable-objects/concepts/durable-object-lifecycle/)):

- **Hibernation API (`ctx.acceptWebSocket`) — DO as *server*.** Browser/app connects *in* to the DO. Between messages the DO is evicted from memory; the socket stays open at the edge. **Duration billing stops while hibernated.** Inbound messages billed 20:1. This is cheap and is the right pattern for *fanning quotes out to user dashboards*.
- **Standard `accept()` or outbound client socket — DO as *client* to Webull/exchange.** Hibernation is **unavailable for outgoing WebSocket use cases** — "Hibernation is only supported when the Durable Object acts as a server." An open outbound socket (plus any heartbeat timer) blocks hibernation, so the DO stays resident and **duration accrues for the entire connection lifetime**. ([DO WebSocket docs via cloudflare-docs `websocket-hibernation-server.mdx`](https://github.com/cloudflare/cloudflare-docs/blob/main/src/content/docs/durable-objects/examples/websocket-hibernation-server.mdx); [DO lifecycle](https://developers.cloudflare.com/durable-objects/concepts/durable-object-lifecycle/): hibernation requires "no outbound connection remains open.")

Additional friction: opening an outbound WS with custom headers (`Authorization: Bearer …` for broker APIs) from Workers/DO has historically required workarounds (fetch-upgrade pattern) and has open workerd issues — verify against current [WebSocket client example](https://developers.cloudflare.com/workers/examples/websockets/) before committing. **UNVERIFIED** for Webull's exact auth handshake — test with a spike.

Alarms: a DO alarm is billed as 1 DO request + 1 SQLite row written (`setAlarm`). Alarms are the correct "wake up every N minutes, do a tick, hibernate again" mechanism. Minimum practical alarm cadence is seconds, but each wake costs duration for the wake window only. ([DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/))

### 2.2 Duration-billing math for a 24/7 outbound socket

DO memory is billed at a fixed 128 MB = 0.125 GB regardless of actual use. ([DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/))

- 1 DO held awake 24/7: 86,400 s/day × 0.125 = **10,800 GB-s/day**.
- Free allowance: 13,000 GB-s/day → **one always-on broker socket consumes 83% of the entire account's daily free duration.** Two users = impossible; the third concurrent thing that wakes a DO pushes you over and DO operations hard-fail until 00:00 UTC.
- Paid: 400,000 GB-s/month included. 1 DO 24/7 = 324,000 GB-s/month → fits inside inclusion (~$0 overage). 10 DOs = 3.24M GB-s → (3.24M−400k)×$12.50/M ≈ **$35.50/mo overage**. 100 DOs = 32.4M GB-s → **~$400/mo overage** on top of $5. And that's before requests/storage.
- Hibernating alternative (alarm every 7.5 min, ~1s awake per tick incl. REST poll to Webull): 192 wakes/day × 1s × 0.125 ≈ **24 GB-s/day/user** — ~450× cheaper. Poll REST on an alarm instead of holding a socket, unless you truly need tick-level push.

**Verdict:** A DO *can* hold a broker WS open 24/7 on Paid for a few users. On Free it is effectively a single-user demo that starves the account. At 100 users it is a $400/mo duration bill for the privilege of doing what a $5 VPS does flat-rate. There is also eviction risk: DOs can be evicted/restarted by the platform; your socket *will* drop and need reconnect + state rebuild from SQLite storage. Design for reconnect regardless of host.

### 2.3 Alternatives compared

| Runner option | 24/7 WS to broker? | Cost | Pros | Cons / risks |
|---|---|---|---|---|
| **DO + outbound WS (non-hibernating)** | Yes, Paid only practically | $5 + ~$3.55/DO/mo duration overage at scale (see above) | No servers; per-user isolation; SQLite state co-located | Hibernation impossible; evictions drop socket; header/auth friction; cost scales per user |
| **DO alarms + REST polling (hibernating)** ⭐ | No socket; poll every 1–7 min | Free for 1 user; ~$0.50/user/mo Paid at scale | Cheapest Cloudflare-native; survives free tier; simple retry | 1-min+ latency; not "second intervals"; Webull rate limits on polling — respect them |
| **User's own desktop via Tauri runner** ⭐ | Yes, on user's machine/IP | $0 infra to DOGS | Zero duration bill; user's own broker credentials never leave device (best security/compliance story, §6); true second-level loops; scales free with users | User must keep app open; NAT/sleep/laptop-close gaps; support matrix (Win/macOS); code-signing cost (§5) |
| **$4–5/mo VPS (Hetzner etc.)** | Yes | ~€3.29–4.50/mo CX22-class (2 vCPU/2–4 GB) flat — **verify current** ([Hetzner pricing via search](https://www.hetzner.com/cloud/)) | Flat rate, full Linux, run 10s of sockets on one box; WebSocket + cron + SQLite all trivial | You operate it (patching, uptime); one box per region; shared-IP rate-limit / broker geo issues. **UNVERIFIED** late-2026 Hetzner price — check before budgeting |
| **Fly.io Machines** | Yes | No free tier for new users (legacy Hobby grandfathered only); ~$2–7/mo per small Machine + egress — pay-as-you-go ([Fly pricing](https://fly.io/docs/about/pricing/)) | Edge regions; autostop/start; good WS support | Metered — an always-on Machine is not free; per-user Machines multiply cost like DOs |
| **Railway** | Yes | ~$5/mo Hobby (incl. $5 usage credit), then usage (~$5–10/mo small svc) ([Railway pricing](https://railway.com/pricing)) | Easy deploys; persistent volumes | Free tier is a 30-day $5 trial then $1/mo credit — not a durable free host; usage billing can drift |
| **Cloudflare Containers** | Yes (Paid) | $5 + ~$1–6/mo per 24/7 small container (see §1.1) | Stays in Cloudflare; scale-to-zero; per-user sandbox possible | Paid only; newer product — verify cold-start + max duration limits for month-long processes before betting the runner on it. **UNVERIFIED** for 30-day uninterrupted uptime |

⭐ = recommended combination (see Recommendations).

### 2.4 Recommended topology for Money 24

- **Control plane on Cloudflare (free → $5):** Pages/Workers API, auth, billing webhook, agent config, paper-trade ledger (D1), R2 candle archive, hibernatable DO per user *as server* fanning quotes/status to that user's dashboard WS.
- **Data plane wherever the user's runner lives:** Tauri desktop (default) holds the outbound Webull WS + runs the 30 loops locally at whatever cadence (including seconds) the user's machine/LLM can sustain; it syncs decisions/orders/fills back to Cloudflare via HTTPS (batched, e.g. every tick or every 30s) and receives config/kill-switch via the hibernatable DO WS or long-poll.
- **Server-side runner as paid add-on only:** offer a Hetzner/Fly/Container runner per *paying* user who wants true 24/7 without their laptop. Price it to cover ~$4–7/mo wholesale. Never offer it free — the DO-duration math above is why.
- **Shared market-data ingest:** one poller (Cron 1-min → Queue → Worker, or one alarm-loop DO) for common symbols, writing to R2/D1 and fanning out. Per-user private symbols/options chains are fetched by the user's runner, not multiplied server-side.

This keeps free tier viable through build + first users, keeps broker keys off DOGS servers by default (huge §6 win), and makes "second intervals" a local-compute feature instead of a Cloudflare billing event.

---

## 3. Data model — "hates migrations, wants NoSQL" vs AGENTS.md

### 3.1 The constraint, stated plainly

[AGENTS.md](../../AGENTS.md) is non-negotiable: *"Database changes require versioned SQL migrations. SQL migrations are the source of truth; do not duplicate database schemas in application code"* and *"Use Cloudflare Workers bindings and D1's native APIs … no ORM."* The owner wish is: schemaless, add fields without migrations.

These reconcile cleanly: **write 3–5 migrations once, ever, for a document-shaped schema. After that, new fields live inside JSON and never need a migration.** You get NoSQL ergonomics on top of the mandated SQL source of truth. Pure NoSQL options on Cloudflare (KV, R2 objects, DO SQLite misused as blob store) each fail a real Money 24 requirement (see comparison), so the answer is document-style D1/DO-SQLite, not a different database.

Also honour the brief wish *"store as much as possible at Webull to minimise our database"*: Webull remains system of record for balances, positions, live orders, fills. Money 24 stores only what Webull doesn't keep for you: agent configs/loops, decision audit trail, paper-ledger shadow, candle history, billing/entitlement, encrypted credential envelopes. That keeps D1 small and the privacy surface minimal.

### 3.2 Options compared

| Option | Migration pain | Query power | Free-tier fit | Consistency / fit for trading | Verdict |
|---|---|---|---|---|---|
| **D1 — classic relational (a table per entity)** | High — every new field = migration. Exactly what owner hates | Excellent (joins, indexes) | Excellent (5M reads/100k writes/day free) | Strong; single-writer serialised writes suit ledgers | Correct but miserable for fast-evolving agent payloads |
| **D1 — document-style (`docs` + generated columns)** ⭐ | **~Zero after bootstrap** — new fields are just JSON keys | Good — `json_extract` + generated-column indexes on hot paths | Excellent (same D1 limits) | Same D1 guarantees; JSON ≤2MB/row cap is fine | **Recommended system of record** |
| **DO SQLite per user (document-style inside DO)** | Zero after bootstrap; per-user DB created lazily | Good inside the DO; **no cross-user queries** | Excellent; 10 GB/DO, 5 GB acct free | Perfect isolation + serialised per-user writes; DO is also your realtime hub | **Recommended for per-user hot state** (see hybrid below) |
| **KV** | Zero (key→value) | Key lookup only; eventually consistent (~60s global propagation) | Poor for writes (1,000/day free!) | **Dangerous for orders/entitlements** — stale reads can double-spend or mis-gate | Config/flags/session cache only. Never ledger, never entitlement source of truth |
| **R2 objects (NDJSON/Parquet per symbol/day)** | Zero (files) | None natively — scan via Worker/Athena-style external engine | Outstanding (10 GB + 10M reads free, $0 egress) | Immutable append-only history — ideal for candles/ticks/backtests | **Recommended for market history**, not for state |
| **External NoSQL (Mongo Atlas, Firestore, Supabase…)** | Zero | Varies | Free tiers exist (**UNVERIFIED** current terms) | Extra vendor, extra secrets, Hyperdrive/egress questions; violates "all Cloudflare" wish | Reject — no benefit over document-style D1 |

### 3.3 Recommended design (hybrid, ~4 migrations total, forever)

**Principle:** D1 = shared system of record + billing + audit. DO SQLite per user = hot, isolated, realtime state. R2 = immutable market history. KV = disposable cache only. Webull = broker truth.

**Migration 0001 — D1 core (`docs` pattern).** One table holds almost everything:

```sql
-- 0001_core.sql (illustrative sketch — planning only, not application code)
CREATE TABLE docs (
  kind       TEXT NOT NULL,          -- 'user' | 'agent' | 'loop_run' | 'paper_order' | 'paper_fill' | 'entitlement' | 'credential' | 'audit'
  id         TEXT NOT NULL,          -- ULID/UUID; prefixed by kind for debuggability, e.g. 'agt_01J…'
  owner_id   TEXT,                   -- user id; NULL for global docs (market calendar, fee schedules)
  json       TEXT NOT NULL,          -- the document. New fields = new keys. No migration.
  -- Promoted hot columns (generated) — the ONLY columns you ever index/filter on:
  status     TEXT GENERATED ALWAYS AS (json_extract(json, '$.status')) VIRTUAL,
  symbol     TEXT GENERATED ALWAYS AS (json_extract(json, '$.symbol')) VIRTUAL,
  created_at TEXT GENERATED ALWAYS AS (json_extract(json, '$.createdAt')) VIRTUAL,
  updated_at TEXT GENERATED ALWAYS AS (json_extract(json, '$.updatedAt')) VIRTUAL,
  PRIMARY KEY (kind, id)
) STRICT;
CREATE INDEX idx_docs_owner   ON docs (kind, owner_id, created_at);
CREATE INDEX idx_docs_symbol  ON docs (kind, symbol, created_at);
CREATE INDEX idx_docs_status  ON docs (kind, owner_id, status);
```

Rules that make this work: (a) only `status`/`symbol`/`created_at`-style fields get promoted; everything else stays in `json`. (b) Adding a *new promoted column* is the only thing that ever needs migration 0005+ — and you do it lazily, only when a query is hot, as an additive `ALTER TABLE … ADD COLUMN … GENERATED` + index. Additive-only keeps staging/prod safe per AGENTS.md. (c) Validate document shape in JS with JSDoc-typed validators at write time (no ORM, no duplicated schema tables — validators are code, migrations remain schema truth). (d) Cap: D1 row max 2 MB, 100 columns/table — JSON docs stay well under both. ([D1 limits](https://developers.cloudflare.com/d1/platform/limits/))

Append-only friends (still migration 0001): `audit_log(id, owner_id, kind, ref_id, json, created_at)` for the immutable decision/order audit trail (insert-only by app convention; D1 has no per-table ACLs — enforce in Worker code and say so). Do **not** abuse `docs` for candles (millions of rows) — that's R2's job.

**Migration 0002 — DO SQLite per user (created by DO class migration, same `docs` shape, smaller).** Each user's DO gets: `docs` (agent state, loop cursors, last决策 snapshot, paper positions cache), `outbox` (pending sync to D1/Webull with retry), `kv` (`key TEXT PRIMARY KEY, value TEXT, expires_at`). Per-user isolation means a runaway loop corrupts one user's 10 GB sandbox, not the shared DB. Cross-user admin queries go through D1 (DO mirrors summaries up on each sync). DO SQLite billing mirrors D1 (5M reads/100k writes/day free). ([DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/))

**Migration 0003 — R2 layout (no SQL; documented as bucket convention).** `candles/{exchange}/{symbol}/{yyyy}/{mm}/{dd}.ndjson.gz` for raw ingest, compacted nightly (Cron → Worker) to `candles-parquet/{symbol}/{yyyy}-{mm}.parquet` for backtests. One object per symbol-day keeps Class A writes tiny (~50 symbols × 30 days = 1,500 writes/mo — deep inside 1M free). Reads via Worker streaming + client-side chart lib; never load Parquet into D1. Ticks (if ever stored) are R2-only and expire via lifecycle rule after 7–30 days — tick storage in D1 would burn the 100k writes/day free cap in minutes.

**Migration 0004 — KV namespaces (config, not data).** `CONFIG` (fee schedules, market calendar, feature flags — write rarely, read often), `SESSION` (Better Auth session cache if needed — source of truth stays D1, §5). Respect 1,000 writes/day free: flags change by deploy, not by tick.

**What you deliberately do NOT build:** no per-entity tables, no ORM, no external NoSQL, no candle rows in D1, no entitlements in KV, no broker secrets in plaintext anywhere (§5.4). Total expected migrations over the product's life: ~4–6, all additive. That is the honest answer to "I hate migrations": you cannot have zero (AGENTS.md forbids it, and ledgers need *some* contract), but you can have *almost* zero.

---

## 4. Payments — low fees, merchant-of-record (MoR), and the Tauri mobile trap

### 4.1 The two families (this choice matters more than the %)

- **Processor (you are merchant):** Stripe (+ Billing/Tax). Cheapest headline rate. **You** register for sales tax/VAT/GST (US states via economic nexus, EU VAT MOSS/OSS, UK VAT, etc.), file, remit, handle chargebacks/fraud tooling. Fine if you sell mainly in one country; a trap for a broke solo founder selling worldwide from day one.
- **Merchant of Record (they are merchant):** Paddle, Lemon Squeezy, Polar, Creem, Dodo, Gumroad. They collect + remit global taxes, handle invoices, VAT IDs, refunds mechanics. You pay ~1–2% extra for never touching a tax return. For a solo founder selling a $10–30/mo SaaS worldwide, MoR is almost always worth it until ~$10k MRR, when re-pricing Stripe becomes worthwhile.

Stripe Billing adds **0.7%** on top of processing; Stripe Tax adds **0.5%** where you collect. So "Stripe is 2.9%" is really **3.6% + $0.30** (with Billing) or **4.1% + $0.30** (Billing + Tax) before international-card (+1.5%) and FX (+1%) uplifts. ([Stripe pricing](https://stripe.com/pricing), [Stripe Billing pricing](https://stripe.com/billing/pricing) — 2.9%+$0.30 US cards, Billing 0.7%, verified via search 2026-10.)

### 4.2 Fee table (headline, US domestic card, before intl/FX uplifts)

| Provider | Model | Headline fee | MoR tax handling? | Notes (verify before signup) |
|---|---|---|---|---|
| **Stripe + Billing** | Processor | 2.9% + $0.30, +0.7% Billing (+0.5% Tax) | No (Tax product calculates; you register/file) | Best APIs/webhooks; Radar extra; you are liable. Intl +1.5%, FX +1% |
| **Paddle** | MoR | 5% + $0.50 (custom < $10 / invoicing) | Yes — VAT/sales tax/GST globally | Veteran SaaS MoR; charged on gross incl. tax; FX 2–3% in MSA can push real take to ~7–8% non-USD. ([Paddle pricing](https://www.paddle.com/pricing)) |
| **Lemon Squeezy** | MoR (Stripe-owned) | 5% + $0.50; +1.5% intl, +1.5% PayPal, +0.5% subscriptions in some cases | Yes | Indie-friendly; extras stack — read the fine print per payout country. ([Lemon Squeezy pricing](https://www.lemonsqueezy.com/pricing)) |
| **Polar** | MoR | Starter 5% + $0.50; Pro $20/mo → 3.8% + $0.40; Growth/Scale down to 3.4% + $0.30; +1.5% non-US cards (orgs from 2026-05-27) | Yes | Dev/OSS favourite; usage-metering strong for AI credits. Tier maths only wins above ~$1k/mo volume. ([Polar pricing](https://polar.sh/pricing) via search 2026-10) |
| **Creem** | MoR | **3.9% + $0.40** | Yes (50+ countries) | Lowest flat MoR rate found; younger vendor — check payout hold (24–72h) + $50 min payout. ([Creem pricing](https://www.creem.io/pricing) via search 2026-10) |
| **Dodo Payments** | MoR | 4% + $0.40 US (+1.5% intl, +0.5% subs in some listings — **contract reportedly says 5%+$0.50; verify**) | Yes (190+ countries claimed) | Built for global/IN founders; confirm the signed terms, not the blog rate. ([Dodo pricing](https://dodopayments.com/pricing) via search 2026-10) **UNVERIFIED** — headline vs contract discrepancy flagged in sources |
| **Gumroad** | MoR (since 2025-01-01) | 10% + $0.50 direct **plus** ~2.9%+$0.30 processing ≈ 12.9% + $0.80; 30% flat via Discover (incl. processing) | Yes | Creator storefront, not SaaS billing. Weak subscriptions/dunning. Avoid for Money 24 core subs. ([Gumroad fees](https://gumroad.com/pricing) via search 2026-10) |
| **Apple IAP** | Store | 15% (Small Business / subs after yr 1) / 30% standard | Apple collects/remits in most regions | **Mandatory** for digital goods in iOS app (see §4.4). ([Apple](https://developer.apple.com/app-store/small-business-program/)) |
| **Google Play Billing** | Store | 15% (subs + first $1M/yr enrolled) / 30% standard — verify current tier | Google collects/remits in most regions | **Mandatory** for digital goods in Android Play app. ([Google Play](https://support.google.com/googleplay/android-developer/answer/112622) — **UNVERIFIED** late-2026 rate, confirm in Play Console) |
| **Coinbase Commerce** | Crypto processor | 1% (Commerce); Coinbase-to-Coinbase free in some flows | **No** — you handle tax/accounting | USDC on Base etc. No chargebacks; refunds manual. ([Coinbase Commerce](https://commerce.coinbase.com/) via search) |
| **BTCPay Server (+ Lightning)** | Self-hosted crypto | 0% processor fee (hosting + on-chain/Lightning fees only) | No | Fully non-custodial; you run the server (that VPS again). Best margin, most ops. ([BTCPay](https://btcpayserver.org/)) |
| **x402 (Coinbase facilitator)** | Agent micropayments (USDC/Base) | ~$0.001/tx facilitator from 2026-01-01, first 1,000/mo free (**UNVERIFIED** — new) | No | Interesting for *agent-to-agent / MCP pay-per-call* later; not a subscription rail today. ([x402](https://www.coinbase.com/developer-platform/products/x402) via search) |

### 4.3 Fee math — what you actually keep (US domestic card, single monthly charge)

Net = price − fee. MoR rows exclude VAT the MoR collects on top (customer pays price + tax; your net is on the ex-tax price in most MoR setups — confirm per provider). Stripe row includes Billing 0.7% (add 0.5% if Stripe Tax).

| Provider (effective) | $10/mo → fee / you keep (take%) | $30/mo → fee / keep | $100/mo → fee / keep |
|---|---|---|---|
| Stripe+Billing 3.6%+$0.30 | $0.66 / **$9.34** (6.6%) | $1.38 / **$28.62** (4.6%) | $3.90 / **$96.10** (3.9%) |
| Creem 3.9%+$0.40 | $0.79 / **$9.21** (7.9%) | $1.57 / **$28.43** (5.2%) | $4.30 / **$95.70** (4.3%) |
| Dodo 4%+$0.40* | $0.80 / **$9.20** (8.0%) | $1.60 / **$28.40** (5.3%) | $4.40 / **$95.60** (4.4%) |
| Polar Pro 3.8%+$0.40 (+$20/mo flat) | $0.78+$20/sub / worse <$25 subs | $1.54+$20/sub | $4.20+$20/sub — wins only at volume |
| Paddle / Lemon / Polar Starter 5%+$0.50 | $1.00 / **$9.00** (10%) | $2.00 / **$28.00** (6.7%) | $5.50 / **$94.50** (5.5%) |
| Gumroad ~12.9%+$0.80 | $2.09 / **$7.91** (20.9%) | $4.67 / **$25.33** (15.6%) | $13.70 / **$86.30** (13.7%) |
| Apple/Google 15% | $1.50 / **$8.50** | $4.50 / **$25.50** | $15.00 / **$85.00** |
| Apple/Google 30% | $3.00 / **$7.00** | $9.00 / **$21.00** | $30.00 / **$70.00** |
| Coinbase Commerce 1% | $0.10 / **$9.90** | $0.30 / **$29.70** | $1.00 / **$99.00** |
| BTCPay self-hosted ~0% | ~$0.01 / **~$9.99** (+ VPS share) | ~$0.01 / **~$29.99** | ~$0.01 / **~$99.99** |

*Dodo rate UNVERIFIED (see table). Fixed $0.30–0.50 hurts $10 pricing disproportionately — at $10, every MoR's fixed fee is 4–5% alone. **Price at $19–30/mo, not $10**, or annual-bill ($99–290/yr in one charge) to amortise the fixed fee. Annual billing is the single cheapest "fee optimisation" available.

### 4.4 The Tauri mobile trap (read before shipping Android/iOS)

Tauri desktop (Windows/macOS direct download) can use any web checkout (Stripe/MoR) — no store cut. **The moment you ship through Apple App Store / Google Play, digital-goods rules bite:** subscriptions, credits, and feature unlocks consumed in-app must use Apple IAP / Google Play Billing (15/30%), with narrow exceptions (reader rules, external-link entitlements that vary by jurisdiction post-litigation — **get advice, this moved repeatedly in 2025–26 and is UNVERIFIED here per-storefront**). Sideloaded Android APK (direct download) can use web checkout; Play-distributed cannot dodge it. Practical play: sell the subscription on the **web** (MoR checkout), let the mobile app be a free companion that unlocks via account entitlement ("Subscribe on web, sign in here"). This is common but must be implemented to each store's current external-purchase/reader guidelines to avoid rejection — budget review time and have a fallback (IAP via RevenueCat-style abstraction) if a reviewer insists. **Do not build the business plan assuming 0% store fees on iOS.**

Crypto: offer USDC-on-Base via Coinbase Commerce (1%) and/or BTCPay Lightning (0%) as a *secondary* rail for the crypto-native segment and for MCP/agent micropayments (x402 later). Expect <5% take-up, manual refunds, price-volatility handling (price in USD, settle in USDC), and tax accounting you still owe. Never make crypto the only rail.

### 4.5 Recommended payment path for a broke solo founder

1. **Launch: Creem (3.9%+$0.40) or Dodo (verify contract) as MoR** — lowest flat MoR take, global tax handled, no monthly fee, no tax registrations. Keep Paddle as the fallback if Creem/Dodo reject a trading-adjacent product (see Open Risks — MoRs can be squeamish about trading/finance; apply early and describe the product accurately as *paper-trading software / user-controlled automation*, never as a fund or advisory).
2. **Web checkout only at launch.** Desktop Tauri = web entitlement. Android APK direct = web entitlement. Defer App Store/Play until revenue justifies IAP + review risk.
3. **Price to the fee math:** e.g. Paper Pro $19/mo or $190/yr; Live $39/mo or $390/yr (illustrative). Annual default in UI. Fixed-fee drag at $19 (≈5.5% Creem) is tolerable; at $10 (≈8%) it is not.
4. **Add Coinbase Commerce USDC** as an optional annual-pay rail (keep ~99%); advertise "save ~5% pay annual in USDC" only if accounting can handle it.
5. **Migrate to Stripe + Billing (+ Tax or a filing service) only** when MRR ≳ $5–10k and your sales are concentrated enough that tax registrations are few — the ~1% saving then funds the bookkeeping. Keep the MoR price IDs abstracted in `docs` (kind=`entitlement`) so migration is a Worker change, not an app release.

Entitlement source of truth: MoR webhook → Worker → D1 `docs` (kind=`entitlement`, promoted `status` column). Apps read entitlement from D1 via API. Never trust client-side "isPro" flags, KV, or store receipts alone without server validation.

---

## 5. Side services & subscriptions — what you need, what it costs

### 5.1 Domain, email, DNS

- Domain via Cloudflare Registrar (at-cost, WHOIS privacy included): ~$9–15/yr for `.com`-class. DNS + CDN + WAF free tier is ample. ([Cloudflare Registrar](https://www.cloudflare.com/products/registrar/))
- Email: Cloudflare Email Routing (free) forwards `support@` to a personal inbox; sending via Cloudflare Email Service / Resend / Amazon SES free-ish tiers. Transactional (receipts, magic links) volume at launch is <1k/mo — stay free. Budget $0 build, $0–20/mo launch (Resend Pro $20 if you outgrow free 3k/mo — **UNVERIFIED** current free cap, check).
- Do not self-host email. Deliverability (SPF/DKIM/DMARC) via the sender provider's wizard; set DMARC `p=reject` before launch.

### 5.2 Error monitoring, analytics, uptime

| Need | Pick | Free tier (verified via search 2026-10) | Launch cost trigger |
|---|---|---|---|
| Errors + perf | **Sentry** (Developer free) | ~5k errors/mo class (sources vary 5k–50k by plan vintage — check [sentry.io/pricing](https://sentry.io/pricing/) at signup; treat as "enough to launch") | ~$26/mo Team when errors/users grow |
| Product analytics + flags + replay | **PostHog Cloud** free | 1M events/mo, 5k replays/mo, 1 project | Usage-based after; cap events in code (sample chart interactions!) |
| Marketing-site analytics (privacy) | Cloudflare Web Analytics (free) **or** Plausible $9/mo | CF free; Plausible has **no free cloud tier** ($9/mo / self-host free) | $9/mo if you want cookieless simplicity |
| Uptime + status | Better Stack / UptimeRobot free tier + Cloudflare health checks | Free tiers exist (**UNVERIFIED** exact probe counts) | $0–10/mo |
| Logs | Workers Logs (included) + Tail Workers if needed | Included allotments vary — enable only on Paid with retention discipline | Watch log volume from agent loops; sample, don't log every tick |

Rule: instrument the Worker/DO with Sentry + structured Workers Logs, PostHog only in the *product* (funnel: signup → connect broker (paper) → first loop → subscribe), CF Web Analytics on the landing page. Agent-loop ticks must not emit per-tick analytics events (see §1.3 — you'd burn 1M events in 2 days at 10 users). Batch to hourly rollups.

### 5.3 Auth — Access vs Better Auth vs roll-your-own WebAuthn

| Option | Fit for Money 24 end-users? | Cost | Notes |
|---|---|---|---|
| Cloudflare Access (Zero Trust) | **No for public users.** Yes for *DOGS admin* (protect `/admin`, staging, D1 console links) | Free ≤50 users | IdP-fronted gateway; not a customer auth system. Use it day one for internal surfaces. ([Access](https://developers.cloudflare.com/cloudflare-one/access-controls/)) |
| **Better Auth (self-hosted, D1)** ⭐ | **Yes** | $0 library + D1 usage (inside free) | D1 is a first-class adapter; email+password, OTP, social (Google/Apple/GitHub), **passkey/WebAuthn plugin**, TOTP 2FA, orgs. Sessions in D1 (source of truth) + optional KV cache. You own the user table (`docs` kind=`user` or Better Auth's own tables via one migration — reconcile: let Better Auth own `user`/`session`/`account` tables in migration 0001b; `docs` references `owner_id` = Better Auth user id). ([Better Auth](https://www.better-auth.com/docs)) |
| Clerk / Auth0 / WorkOS | Yes, fastest | Free tiers then per-MAU (**UNVERIFIED** current MAU pricing) | Vendor lock + MAU bill as you grow; data leaves Cloudflare. Only if Better Auth blocks shipping |
| Roll-your-own WebAuthn | No | $0 + weeks of risk | WebAuthn ceremonies, challenge storage, attestation, recovery — do not hand-roll for a product holding broker credentials. Better Auth's plugin *is* the roll-your-own, audited |

Enable passkeys + TOTP, require 2FA step-up for "enable live trading" + "reveal/export keys", and issue scoped API keys (+ MCP tokens) as hashed, revocable `docs` (kind=`credential`, scope field promoted). Rate-limit auth endpoints in the Worker (Access + Turnstile on signup/password-reset; Turnstile free).

### 5.4 Secret storage for user broker keys (the highest-stakes §)

Threat model: DOGS servers holding live Webull keys for 100 users is a honeypot and a liability magnet (§6). Design in layers:

1. **Default (desktop runner): keys never leave the user's device.** Store in OS keychain (Windows Credential Manager / macOS Keychain via Tauri `keyring`-class plugin), encrypted at rest. Cloudflare stores only a public identifier + "runner last seen". This is the marketing line *and* the security line: "Your keys stay on your machine."
2. **Server runner (paid add-on only): envelope encryption.** Per-user Data Encryption Key (DEK, AES-256-GCM) encrypts the broker secret; DEK wrapped by a Key Encryption Key (KEK) held as a **Cloudflare Worker secret** (`wrangler secret put KEK_…`, never in `wrangler.jsonc`, never in Git, separate per environment local/staging/prod per AGENTS.md). Store only `{ciphertext, iv, wrappedDek, keyVersion}` in D1 `docs` (kind=`credential`). Decrypt only inside the runner Worker/Container at order time; never return plaintext to the browser; never log it; redact in Sentry breadcrumbs. Rotate KEK by re-wrapping DEKs (no data migration). ([Workers secrets](https://developers.cloudflare.com/workers/configuration/secrets/))
3. **Platform secrets (DOGS' own):** Worker secrets + GitHub Actions secrets (least-privilege Cloudflare API token, preview ≠ prod secrets per AGENTS.md). Cloudflare Secrets Store if you need sharing across Workers. Never `VITE_*`, never in Tauri bundles.
4. **Paper keys** still get envelope treatment in dev (build the muscle before live keys exist). Add break-glass: user can instantly "revoke all server runners" (delete wrapped DEKs) from any signed-in device.

### 5.5 Code signing & store fees (one-time / annual — plan cash for these)

| Item | Cost | When needed | Notes |
|---|---|---|---|
| Apple Developer Program | **$99/yr** | Before any signed macOS/iOS distribution | Developer ID signing + notarization; without it Gatekeeper blocks you. ([Apple](https://developer.apple.com/programs/)) |
| Windows code signing — OV cert | ~$150–350/yr | Before Windows launch (to reduce SmartScreen friction) | Reputation accrues over downloads. |
| Windows — EV cert | ~$280–500/yr + hardware token | Optional | **Note:** sources report EV lost its instant-SmartScreen advantage ~Aug 2024 (**UNVERIFIED** — confirm with CA). Cheaper path: Microsoft Store distribution or Azure Trusted/Artifact Signing ~$9.99/mo (US/CA eligibility limits — verify). SignPath Foundation free for qualifying OSS (Money 24 is commercial — likely ineligible). Budget OV ~$215/yr and test SmartScreen empirically. |
| Google Play developer account | **$25 one-time** | Before Play distribution | Plus Play Billing if in-app digital goods (§4.4). |
| Apple App Store | Included in $99/yr | iOS distribution | + IAP cut if applicable. |
| Android APK direct (sideload) | $0 | Alternative to Play | No store cut; user must enable install; update mechanism is yours (Tauri updater + signed releases on R2/GitHub). |

Signing pipeline lives in the manually-triggered GitHub Actions native workflow (per AGENTS.md); certs as Actions secrets, never in repo. Unsigned "developer builds" for testing are free — signing spend starts at public launch, not during build.

### 5.6 Notifications (push / Telegram / Discord / email)

- **Telegram Bot API** ($0) + **Discord webhooks** ($0): highest signal/cost for trading alerts ("loop placed paper order", "kill-switch tripped"). User pastes a bot token / webhook URL; store as `credential` docs; send from Worker/DO alarm. Respect Telegram ~30 msg/s global bot limits and per-chat throttles — batch fills.
- **Email alerts:** via §5.1 sender (fill confirmations, security alerts — required, not marketing).
- **Mobile/web push:** Web Push (VAPID) is free (browser push services); APNs/FCM via a thin provider (OneSignal free tier / self via Worker) — **UNVERIFIED** free caps. Defer native push until mobile ships; Telegram/Discord/email cover launch.
- Every alert path needs a per-user rate cap + digest mode, or one volatile session spams 30 loops × alerts and users churn/mute.

### 5.7 Monthly cost table — $0 build phase vs launch phase

Assumes: Cloudflare-centric, Tauri runner default, Creem MoR, ≤100 users launch, no always-on per-user DOs/containers.

| Line | Build phase (target $0) | Launch phase (≤100 users) | Notes |
|---|---|---|---|
| Cloudflare Workers/Pages/DO/D1/R2/KV/Queues | $0 (Free) | **$5** (Paid) + $0–55 overages (DO duration discipline decides) | See §1.3; hibernating ≈ $10 total |
| Domain (amortised) | ~$1/mo ($12/yr) | ~$1/mo | Registrar at-cost |
| Email sending (Resend/SES/CF) | $0 | $0–20 | Free tier → $20 Pro if needed |
| Sentry | $0 | $0 → $26 | Free until error volume forces Team |
| PostHog + CF Web Analytics | $0 | $0 | 1M events free; sample aggressively |
| Auth (Better Auth + Turnstile + Access) | $0 | $0 | Self-hosted; Access free ≤50 (admin only) |
| Notifications (Telegram/Discord/Web Push) | $0 | $0 | User-supplied tokens/webhooks |
| Optional shared VPS (market ingest / BTCPay) | $0 (Cron instead) | $0–5 | Only if Cron polling insufficient |
| Server-runner add-on (per subscribed user) | — | Passed through in price (+$5–10/mo tier uplift) | Never subsidise from base tier |
| **Infra subtotal** | **~$1/mo** | **~$6–35/mo typical** | Excludes payment % (§4.3) |
| Apple Developer (amortised) | $0 (defer) | $8.25/mo ($99/yr) | From macOS/iOS launch |
| Windows OV cert (amortised) | $0 (defer) | ~$18/mo (~$215/yr) | From Windows public launch |
| Google Play (one-time $25) | $0 | ~$2/mo amortised yr-1 | If Play ships |
| Payment processing | $0 (no sales) | 3.9–5% + $0.40–0.50 per charge (MoR) | §4.3; annual billing minimises drag |
| **All-in fixed (excl. % fees)** | **~$1/mo** | **~$35–65/mo** | Founder time excluded (the real cost) |

Build phase is genuinely ~$0–12/yr (domain only) if signing is deferred — matching the owner's "free while building" constraint. Launch fixed costs stay under ~$65/mo before payment percentages; the business breaks even on fixed infra at ~3–5 paying users at $19/mo.

---

## 6. Legal / compliance basics for selling an auto-trading SaaS (not legal advice)

> **This section is general information, not legal advice. Money 24 needs a licensed attorney (securities/fintech) before (a) taking live-money orders, (b) marketing performance, (c) holding user credentials server-side. Budget for that consult before live trading — it is cheaper than an enforcement letter.** Flags: ⚖️ = lawyer required.

### 6.1 The lines that decide whether you need registration ⚖️

US-centric sketch (other jurisdictions differ — EU MiFID/PSD, UK FCA, etc. — ⚖️ per market you sell into):

- **Investment adviser (SEC/state RIA; Advisers Act §202(a)(11)).** Providing *securities advice for compensation* can make you an investment adviser — fiduciary duty, Form ADV, compliance program. Robo-advisers are RIAs. The narrower you stay to "**software tool the user configures and controls**" (user sets rules/strategies, user can override/kill, no discretionary management of their account by DOGS, no personalised recommendations marketed as advice), the stronger the "tools not advice" position. The moment Money 24 markets "our AI picks winning trades for you", runs discretionary copy-trading, or pools decisions across users, you drift toward adviser/broker analysis. Disclaimers help but **do not override what the product actually does** ("What you do matters more than what you say"). ([SEC robo-adviser guidance](https://www.sec.gov/investment/im-guidance-2017-02.pdf) concept; verify current text ⚖️)
- **Broker-dealer (Exchange Act §15(a)).** Effecting securities transactions for others / receiving transaction-based compensation risks BD registration + FINRA membership. Routing user-directed orders through the *user's own* Webull account via the *user's own* credentials, charging a flat SaaS subscription (not per-trade commissions, not % of profits/assets), is the lowest-risk shape. Taking a cut per trade or of profits changes the analysis — don't, without ⚖️.
- **Futures/options/prediction markets multiply regulators** (CFTC/NFA for futures + commodity interests; options need options-specific disclosures (OCC characteristics/risks); prediction markets / event contracts have evolving CFTC + state gaming analysis). Paper trading avoids most of this; live futures/options is a second legal project. ⚖️
- **Internet Adviser Exemption** (amended 2025) exists for purely algorithmic, interactive-website advice with no human-personnel advice — but still requires SEC registration + Form ADV. It is not a free pass; it is a registration path. ⚖️

### 6.2 Holding user broker keys ⚖️

- Custody of *credentials* isn't custody of *assets*, but a breach that drains user brokerage accounts creates negligence, state privacy-law, and FTC §5 (unfair/deceptive practices) exposure, plus Webull API terms liability. Server-side keys (§5.4 layer 2) therefore need: explicit informed consent screen ("you grant Money 24 authority to place orders in your account…"), scope minimisation (trade-only API keys if Webull offers them; never withdrawal-capable credentials), instant revocation, encryption + access logging, breach-notification plan (state laws: 72h–60d windows vary).
- Prefer the desktop-runner model precisely because "keys never leave your device" shrinks custody, breach blast-radius, and the sales pitch simultaneously.
- Check Webull OpenAPI terms: automated trading, credential storage, third-party SaaS use, rate limits, and whether paper vs live keys differ. Terms violations can get *users'* accounts restricted — disclose that. **UNVERIFIED** — read current Webull developer agreement before building on it. ⚖️/contract review.

### 6.3 Disclaimers, marketing, and product hygiene (do from day one)

- Required-style disclosures (attorney to finalise wording ⚖️): "Money 24 is software, not a broker-dealer, investment adviser, or financial planner. Nothing here is financial advice or a recommendation. Trading involves substantial risk of loss, including loss of principal. Past/hypothetical/paper performance does not predict future results. You are solely responsible for orders placed in your account, including orders placed by automations you configure." On: landing page, pricing page, onboarding, live-trading enable screen, every strategy template.
- **Paper trading must be labelled everywhere** (charts, fills, P&L) and paper results never marketed as live performance. Hypothetical-performance rules (SEC Marketing Rule for advisers; FTC truth-in-advertising for everyone) govern backtests — show methodology, period, costs, and "hypothetical" watermarking. No "AI wins X%", no cherry-picked testimonials without substantiation + disclosures. ⚖️ before any performance claim ships.
- Kill-switch, position/size limits, "require confirmation for live orders > $X", and a prominent live-vs-paper mode indicator are not just UX — they are your best evidence of user control (supports tools-not-advice) and of reasonable care.
- Terms of Service + Privacy Policy + Risk Disclosure as separate, versioned documents (acceptance logged in D1 `audit_log`); arbitration/class-action, limitation of liability, and indemnity clauses need ⚖️ drafting — templates are a starting point, not a shield. Age gate 18+, US-only at launch (geo-fence live trading) simplifies tax/securities scope; maintain OFAC/sanctions screening if you later open globally. ⚖️
- Accessibility + consumer law: auto-renewing subscriptions need clear price/renewal/cancel disclosure (FTC Click-to-Cancel environment + state auto-renewal laws — **UNVERIFIED** current federal rule status post-litigation; MoR handles mechanics, you handle honest copy). Easy cancel in-app; no dark patterns.
- Data protection: even US-only, adopt GDPR-grade basics (export/delete my data, breach notice, subprocessors list: Cloudflare, MoR, Sentry, PostHog). If any EU/UK users: GDPR/UK GDPR + VAT via MoR. ⚖️-lite (checklist review may suffice pre-launch).
- Record-keeping: retain decision/order audit trail (§3) — it defends you ("user configured X; system did X") and helps users dispute broker fills. Define retention (e.g. 3–6 years) with counsel. ⚖️

### 6.4 What stays paper-only until counsel clears it

Paper trading + education + user-configured automation with desktop-held keys is the lowest-risk launch. **Gate behind counsel sign-off:** live-money execution, server-held live keys, options/futures, copy-trading/leaderboards that execute in followers' accounts, profit-based pricing, and any marketing using performance numbers.

---

## Recommendations for Money 24

1. **Stay Cloudflare-native for control plane; do not run the trading loop server-side by default.** Pages/Workers + D1 (`docs`) + per-user DO (hibernating, as server) + R2 candles + KV config. Ship the **Tauri desktop as the always-on runner** holding the Webull WebSocket and the 30 loops locally; sync summaries to Cloudflare. This keeps build at ~$0, makes second-interval decisions possible, and keeps broker keys off DOGS servers.
2. **Adopt the document-style D1 + DO-SQLite + R2 hybrid (§3.3).** Four bootstrap migrations, additive-only thereafter. Webull stays broker system-of-record; Money 24 stores configs, audit trail, entitlements, and candle history only. Reject external NoSQL and per-entity tables.
3. **Budget for Workers Paid ($5/mo) at first real users, not at 100.** Free supports ~1–5 disciplined paper users. The $5 unlock (D1 writes, requests) plus hibernating DO design keeps 100 users at roughly **$10–60/mo** Cloudflare total. The one forbidden pattern is always-on non-hibernating DOs per user (~$400/mo at 100 users + free-tier death at 2 users).
4. **Shared market-data ingest, never per-user polling.** One Cron/Queue poller fans out via hibernatable DO WebSockets to dashboards. Polling dashboards or per-user symbol fetches will burn the 100k/day cap and Webull goodwill simultaneously.
5. **Payments: launch on a low-rate MoR (Creem 3.9%+$0.40; Dodo as verified-contract alternative; Paddle fallback), web checkout only, $19+/mo with annual default.** Add USDC (Coinbase Commerce 1%) as an optional annual rail. Defer Stripe until ~$5–10k MRR and tax scope is narrow. Defer App Store/Play IAP until store revenue justifies 15% + review risk; sell on web, unlock in-app by entitlement.
6. **Auth: Better Auth on D1 (passkeys + TOTP + social), Cloudflare Access for DOGS admin only, Turnstile on abuse surfaces.** Envelope-encrypt any server-held broker secret (Worker-secret KEK, D1-stored wrapped DEKs); default to keys-on-device. Require step-up 2FA for live-trading enable.
7. **Side services at $0:** Sentry + PostHog (sampled) + Cloudflare Web Analytics + Telegram/Discord alerts + Email Routing. Defer all signing spend (Apple $99/yr, Windows OV ~$215/yr, Play $25) until public native launch — unsigned dev builds cost nothing.
8. **Paper-first, counsel-gated live.** Launch paper trading with full risk disclosures, kill-switch, size limits, and audit logging. Engage a fintech/securities attorney before live orders, server-held live keys, options/futures, copy-execution, or any performance marketing. Describe the product everywhere as *user-controlled software*, because legally, what it does is what it is.

## Open risks

- **MoR / processor acceptance of trading-adjacent SaaS.** Paddle/Creem/Dodo/Lemon may classify auto-trading software as restricted/high-risk and reject or later offboard Money 24. Apply early, describe accurately (paper-first, user-controlled, no custody of funds), and keep a Stripe fallback integration path. **UNVERIFIED** per-provider stance — test with a real application before launch depends on it.
- **Webull OpenAPI terms, rate limits, and paper-vs-live parity.** Automation, third-party credential use, WS entitlements, and options/futures permissions are contract questions. If Webull forbids server-side credential storage or caps polling aggressively, the runner topology (§2.4) shifts further toward desktop-only. **UNVERIFIED** — requires reading the current developer agreement + a technical spike.
- **DO outbound-WebSocket auth + eviction behaviour.** Custom-header outbound WS and long-lived socket survival across DO evictions need a hands-on spike; reconnect/state-rebuild must be assumed. If Webull requires headers Workers cannot set, desktop/VPS runner becomes mandatory, not optional.
- **Cloudflare limit/price drift.** All §1 numbers are 2026-10-30 snapshots; DO SQLite storage billing itself only switched on Jan 2026. Re-verify Workers/DO/D1 pricing pages before pricing the product, and alert on usage (DO GB-s especially) from day one — a single non-hibernating code path can 10× the bill overnight.
- **"Second intervals" expectation vs physics/billing.** Server-side second-level loops for many users are not economically or rate-limit viable on Cloudflare (or Webull). If the owner insists seconds matter, the honest product is local execution; Cloudflare cadence stays minutes. LLM latency/cost per tick (user's own subscription terms for programmatic use — **UNVERIFIED**, each provider's automation/ToS stance differs) may bind harder than Cloudflare does.
- **Store policy (Apple/Google) for trading apps.** Financial-services declarations, external-purchase link rules, and IAP scope changed repeatedly through 2025–26. App review may demand IAP, licences, or geo-restrictions. Web-first entitlement mitigates but does not eliminate rejection risk. ⚖️ + pre-submission review advised.
- **Securities/commodities classification creep.** Adding discretionary AI selection, copy-execution, profit-share pricing, futures/options, or performance advertising can each flip Money 24 into adviser/BD/CTA analysis. Any one of those features is a legal project, not a feature flag. ⚖️
- **Key-compromise blast radius.** One KEK leak or Worker compromise with server-held live keys endangers every server-runner user's brokerage account. Desktop-default keys, trade-only scopes, revocation drills, and breach runbook are launch blockers for the server-runner tier, not hardening niceties.
- **Vendor youth (Creem/Dodo/Polar tiers, x402, Containers).** Lowest-fee MoRs and newest Cloudflare primitives carry continuity/pricing-change risk (Dodo headline-vs-contract discrepancy noted in §4.2; Containers/Sandbox long-duration behaviour UNVERIFIED). Keep payment price/entitlement abstraction and runner abstraction so any one vendor can be swapped without an app recall.

---

*End of Loop 1. Files touched: this file only. Next loops should validate: Webull API capabilities/terms (broker loop), LLM-provider programmatic-use terms + "Jev" identity (AI loop), and chart/data-source latency (markets loop) — several UNVERIFIED flags above depend on them.*
