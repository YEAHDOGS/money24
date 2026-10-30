# Loop 3 — Muse review of Loop 2 drafts (ARCHITECTURE.md + DESIGN.md)

> Agent: Muse (Loop 3 of 5, review sub-agent) · Date: 2026-10-30 · Branch: `claude-10-trading-architecture`
> Inputs: `docs/research/BRIEF.md` (incl. owner Go update), `AGENTS.md`, `docs/ARCHITECTURE.md` (Loop 2 draft, §1–§19), `docs/DESIGN.md` (Loop 2 draft, §1–§8), `docs/research/loop1-muse-platform-payments.md`, `docs/research/loop1-astra-broker-data.md`, `docs/research/loop1-claude-inspiration.md`, `src/styles/theme.css`.
> Output: this file only. No other file edited. No commit.
> Method: Part A is a document review (no new web research; section numbers quoted). Part B is live-web research, 2026-10-30; every price/limit carries a URL; anything not confirmed on an official page is marked **UNVERIFIED**. Blunt per BRIEF.md. No praise section.

---

# Part A — Critical review

## A1. Architecture (§2, §3, §5, §7, §8)

**Wrong**

1. §2 claims the control plane holds a "Durable Object per user (hibernating WS hub, realtime fan-out, per-user state)" while §5 says the runner "pushes summaries, heartbeats and decisions to the user's Durable Object over one WebSocket". A runner-held outbound WebSocket into a DO prevents hibernation (Loop 1, `loop1-muse-platform-payments.md` §2.1: hibernation is server-side only; an open outbound socket blocks eviction). The draft never resolves which side initiates, who reconnects, and what happens to in-flight `propose_intent` calls when the DO is evicted. **Change:** rewrite §2/§5/§7 to state one pattern only: runner dials out to a Worker endpoint; DO accepts as server via `acceptWebSocket`; runner reconnects with jittered backoff; all sync messages are idempotent with `client_order_id` + `loop_run` id deduplication in DO SQLite. Delete any wording implying the DO dials the runner.
2. §3 estimates the hand-written Go Webull client at "a few hundred to ~1,500 lines" including "request signing, REST, MQTT quote stream, gRPC trade events (compile Webull's protos if published; else reverse from the Java SDK)". That estimate excludes: signing edge cases per endpoint/version, MQTT session/subscription state machine, reconnect + retained-state rebuild (1-minute retained window, Loop 1 Astra §4), gRPC streaming reconnect/replay, sandbox-vs-production host switching, and acceptance tests. "Reverse from the Java SDK" for protos is not a plan; if protos are unpublished this is a multi-week reverse-engineering project with breakage on every Webull deploy. **Change:** replace the 1,500-line claim with a Phase 0 spike gate (already in §18) that must answer "are protos published?" before any Go commitment; budget REST+MQTT first, gRPC last, REST order-status polling as the shipping fallback. State the fallback in §3, not only in Loop 1.
3. §8 says validators "are not a duplicated schema; the migrations remain the schema" while also requiring Go structs in the engine mirroring the same `docs` kinds. Two validators (JS + Go) over one JSON shape *is* a duplicated schema in practice; they will drift. **Change:** pick one source: generate Go structs and JS JSDoc typedefs from one `docs/kinds.json` (or equivalent) checked into the repo, or admit §8 duplicates the schema and add a contract-test requirement. Do not claim both "no duplication" and dual validators.
4. §5 "Journal: local SQLite (pure-Go driver)" — the pure-Go SQLite driver (`modernc.org/sqlite`) is a transpiled C codebase, large and slower than the `mattn` cgo driver; cgo breaks the "one static binary per OS" claim in §3 (needs a C toolchain per target, complicates Tauri sidecar cross-builds). The draft picks neither and hides the trade-off. **Change:** §3/§5 must name the driver, state cgo on/off per OS target, and show the sidecar build matrix still produces one file per OS.

**Missing**

5. No deployment/update story for `m24d`: version skew between Tauri shell, sidecar binary, Worker API and DO SQLite schema; no auto-update, no rollback, no minimum-version gate. A stale sidecar placing orders against a changed API contract is a safety bug. **Change:** add to §5: sidecar reports `engine_version` + `journal_schema_version` on connect; Worker rejects order-intent sync from versions below a floor; Tauri updater ships shell+sidecar atomically.
6. No time discipline: nothing on clock sync, exchange timestamps vs runner wall-clock, candle-boundary alignment, or the delayed-sandbox clock problem that §14 itself flags ("A paper strategy must use the same clock as its fills"). **Change:** §5 Snapshot builder must carry `feed_ts`, `receive_ts`, `decision_ts`; journal stores all three; paper mode forbids mixing live external prices with delayed sandbox fills (enforce, don't warn).
7. No single-writer rule stated for the farm: two runners (desktop + VPS) logged into the same account will double-submit. §5 lists three run modes with no fencing. **Change:** add a farm lease: one active runner per `farm` doc, heartbeat-renewed in the DO, second runner starts read-only. This is a §5 blocker, not a later optimisation.
8. §7 "One Cron/Queue poller fetches snapshots for the union of watched symbols" contradicts §7's own redistribution warning and Loop 1 Astra §4 (sharing across users needs licence permission). The draft keeps both the shared poller and the warning. **Change:** delete the shared-poller fan-out for broker-entitled data until redistribution is confirmed; shared poller may only carry delayed/IEX/public crypto data labelled as such.

**Over-engineered**

9. Four planes, DO-per-user, D1, R2, KV, Queues, Cron, remote MCP, local MCP, hash-chained journal, envelope encryption — for a paper-trading MVP with zero users. §18 Phase 1 needs: one Go runner, one garden, local journal, one Svelte monitor. **Change:** §18 must name what is *deleted* from Phases 0–1 (remote MCP, KV, Queues, R2 Parquet compaction, envelope encryption) instead of implying all of §2 ships early.
10. §4 ports 5–8 TradingAgents roles per garden (30 agents ≈ 4 gardens). The debate (Bull/Bear/Trader/Risk/PM) multiplies every escalated decision by 3–5 LLM calls; §6 cost math counts one call per loop (see B2 — this understates the smart preset by ~5×). **Change:** v1 garden = Gate + one Thinker + deterministic Risk. Debate is a Phase 2 experiment behind a budget flag, off by default.

**Risky**

11. §5 "Works offline and catches up" for order intents is dangerous: an intent queued offline and synced after a price move must never become an order. **Change:** offline queue may carry only journals/heartbeats; intents expire (TTL in seconds) and are revalidated against a fresh snapshot + Reflex checks on reconnect, or discarded.
12. §3 Go decision is recorded as "needs owner sign-off" but §5, §18 and DESIGN.md §6 already assume the Go sidecar everywhere. The drafts spend the decision before it is approved. **Change:** mark every Go-dependent section conditional until the AGENTS.md amendment ("JavaScript only, except the runner engine in `engine/`, which is Go") is accepted; keep the JS-runner fallback (Loop 1 topology) named in §5.

## A2. Pros/cons (§12)

**Wrong / missing**

1. §12 lists 7 pros / 7 cons as if balanced. It omits the three cons that can kill the product: (a) MoR payment rejection for trading software (§15's Creem recommendation is contradicted by Creem's own prohibited list — see B3; this is not a "con", it invalidates the launch plan), (b) Webull Connect partner approval is a binary gate with unknown criteria/timeline (buried as "gates the SaaS Log in with Webull"), (c) consumer AI subscription terms may forbid the exact 24/7 unattended use §6 depends on (§6 admits **UNVERIFIED**; §12 does not list it at all). **Change:** rewrite §12 cons to rank by kill-risk: payments acceptance, Connect approval, subscription terms, Go client proto availability, data redistribution. Delete symmetric padding rows ("JSON queries are weaker") or move them to §14.
2. Pro "Webull holds money truth; our DB stays tiny" is overstated: Loop 1 Astra §5 shows no historical P&L, no historical position snapshots, Connect/Broker history parity **UNVERIFIED**. Strategy-level P&L, equity curves and season score cards (DESIGN.md §4) are ours to compute and store forever. **Change:** pro becomes "Webull holds balances/positions/current orders; Money 24 still owns all history, P&L and audit storage" and §8 R2 retention gets a size estimate.
3. Pro "$0 to build" ignores the $100 Webull API-eligibility account minimum (Loop 1 Astra §2), domain (~$12/yr, Loop 1 Muse §5.7) and the owner's own LLM bills during development. Small, but §12/§16 claim $0 in three places. **Change:** write "~$0 infra; ~$112 first-year cash (domain + Webull account funding, which stays the owner's money)" once and reference it.

## A3. Goals (§1)

**Wrong**

1. Goal 1 promises a "multi-asset paper-trading bot (stocks, ETFs, options, futures, crypto, event contracts)" as the paper-first target. Loop 1 Astra §3: multi-leg, fractional, overnight, exercise/assignment, margin and expiry sandbox parity are all **UNVERIFIED**; a bond API was announced but not verified. Shipping five asset classes in Phase 1 multiplies every unverified behaviour by five. **Change:** Goal 1 v1 = stocks/ETFs paper only; options/futures/crypto/events each enter only after a named sandbox acceptance test (partial fill, cancel, expiry/assignment where relevant) passes. List the tests in §18.
2. Goal 6 "One codebase, every screen… Linux and Android (iOS later)" plus DESIGN.md §6 Android companion: Tauri `externalBin` sidecars are desktop-only; Android cannot ship the Go sidecar as a separate process (see B4). The goal as written implies parity that the platform forbids. **Change:** Goal 6 must state Android = monitor/remote only, no runner, no sidecar; Linux desktop is a real target with its own signing/update story or it is dropped from v1.
3. Goal 4 "No AI bill for DOGS during the build phase" conflicts with §10 Sentry/PostHog free tiers being the only monitoring and with any DOGS-hosted demo/cloud-only mode (§5 Cloud-only) — cloud-only paper loops need *someone's* LLM; if not the user's key, it is DOGS' bill or Workers AI's 10k Neurons/day (exhausted by ~50 calls, see B1). **Change:** state cloud-only free tier is rules-only / BYO-key-only; no DOGS-funded inference at any tier until paid plans exist.

**Missing**

4. No measurable goal: "running 24/7 without the owner awake", "honestly score them" (§1 non-goals) have no numbers. **Change:** add exit metrics to §1 or §18: e.g. 7 days unattended, zero unreconciled orders, 100% of orders traceable to an intent + risk decision, paper P&L computed net of modelled fees on every season card.
5. No goal for data honesty in the UI beyond one clause; DESIGN.md §2 "Honest numbers" exists but §1 never requires delayed-data labelling, paper watermarking, or gross-vs-net fee display as a *system* property. **Change:** promote DESIGN.md §2.3 into ARCHITECTURE.md §1 as a goal enforced in the snapshot/journal (every fill carries `is_paper`, `data_delay_s`, `fees_modelled`).

**Over-engineered**

6. Goal 5 (MCP + HTTP API for external agents, live later) in the first goals list pulls OAuth, API-key scoping, rate limiting and prompt-injection posture (§9) into the critical path of a paper MVP. **Change:** Goal 5 becomes Phase 3 (as §18 already phases it); §1 should not list it beside paper-first as if co-equal.

## A4. Bottlenecks (§13)

**Wrong**

1. §13 ranks "Market data rights and speed" #1 and "LLM latency and cost" #2. For the stated MVP (paper, delayed sandbox data, BYO keys) neither binds first. The binding constraints are: Webull sandbox acceptance behaviour, Connect approval lead time, and the runner staying alive on a sleeping laptop (§13 #3). Ranking data *speed* first repeats the owner's second-interval wish as if it were the product's problem; §4 already (correctly) gives seconds to Reflex/broker-native orders. **Change:** re-rank: (1) approvals (Connect + payments + subscription terms — calendar time, not engineering), (2) always-on runner liveness, (3) LLM cost under debate multiplication, (4) data rights for redistribution, (5) data speed last with the sentence "speed does not bind until live trading with real-time entitlements".
2. §13.6 "Free-tier write caps… Batch writes; journal locally; sync summaries" understates Loop 1 Muse §1.3: Queues break on day 1 at 10k ops/day if every tick is queued; DO duration breaks at 2 always-on users. §13 lists caps without the forbidden patterns. **Change:** copy the three forbidden patterns into §13 verbatim: no Queue per tick, no non-hibernating DO per user, no KV for hot state/entitlements.

**Missing**

3. No bottleneck for *support*: one founder, Windows+macOS+Linux+Android shells, broker edge cases, payment disputes. **Change:** add "founder support load" as a bottleneck with the mitigation already implied elsewhere (desktop-first, one broker, paper-first) stated as scope control.
4. No bottleneck for backtest/paper data volume: R2 `ndjson.gz` per symbol/day plus nightly Parquet (§7/§8) has no size/retention budget. **Change:** §13 or §8: estimated GB/month for the v1 universe at 3 pushes/s, retention (ticks 7–30 days per Loop 1), and who pays past R2's 10 GB free.

## A5. Issues and risks (§11, §14)

**Wrong**

1. §14 "Payment processors may refuse trading software. Apply early" is far too weak given B3: Creem and Polar *publish* prohibitions on trading bots/execution tools, Paddle prohibits trading signals/strategies. This is a known rejection, not a risk. **Change:** §14 + §15 rewritten per B3 verdict (Stripe-first, MoRs assumed hostile until a written approval exists).
2. §11 PDT paragraph says "Read account capabilities, never hardcode" — correct — but §5 Reflex layer hardcodes "max-loss and position-size limits" with no mention that intraday-margin capabilities differ per account during the 2026-06-04 → 2027-10-20 transition (Loop 1 Astra §8). **Change:** Reflex limits read `account_capabilities` (margin regime, options level, futures approval) from the broker adapter at session start; unknown capability = most restrictive limit.
3. §14 overfitting control ("walk-forward verdict before it may go live") has no enforcement point: who blocks live, where is the verdict stored, can the user override? **Change:** verdict is a `garden` doc field written only by the Strategist job; live enable (§9) checks `verdict = validated` + 90-day paper record (§18 Phase 5) in the Worker, not in UI copy.

**Missing**

4. No risk entry for the local CLI bridge (§6.2): `m24d` spawning `claude -p` / `codex exec` with an MCP that can propose intents means a compromised prompt/news item can drive the user's *own* subscription CLI with the user's filesystem access. §9's prompt-injection posture covers news text but not CLI tool scope. **Change:** §6/§9: CLI bridge runs with an allowlisted tool set (local MCP paper tools only), working directory sandbox, no shell tool, and every CLI-proposed intent passes the same Go Reflex checks; state that residual CLI risk is the user's machine risk and must be disclosed in Connections UI (DESIGN.md §4).
5. No risk for Webull sandbox behavioural drift: sandbox fills (15-min delayed by default, Loop 1 Astra §3) can make a losing strategy look profitable. §14 mentions the clock once. **Change:** season score cards must stamp `data_delay_s` and the Strategist verdict must refuse validation on delayed-data seasons.
6. §11 legal gates omit app-store financial-app declarations and the Android/iOS IAP trap (Loop 1 Muse §4.4) from the *architecture* risks; DESIGN.md §6 ships Android before this is decided. **Change:** §11 adds: mobile store distribution is gated on store financial-services review + IAP policy check; web-checkout entitlement unlocking must be re-verified per storefront at submission time (**UNVERIFIED** per storefront in Loop 1).

## A6. Payments (§15)

**Wrong — this section must be rewritten (see B3 for evidence)**

1. §15 recommends Creem at launch ("lowest MoR rate found (3.9% + $0.40)") and lists "Paddle / Lemon Squeezy / Polar Starter" as fallback "if Creem rejects trading software". Published policies invert this: Creem prohibits trading tools that enable execution/automation; Polar prohibits "Financial trading, trading bots, brokerage, or investment advisory services (including insights platforms)"; Paddle prohibits "Trading and financial services/advice… including trading signals and strategies". All three are expected rejections for Money 24's live product, not fallbacks. **Change:** replace the §15 table's recommendation order: (1) apply to Stripe as processor and budget tax handling (Stripe Tax / filing service) or accept US-only launch scope; (2) treat every MoR as unavailable until a written, product-specific approval is in hand; (3) keep the annual-billing + $19+ pricing advice, which is correct and survives the rewrite.
2. §15 fee column "Effective fee at $19/mo" (~6% Creem, ~7.6% Paddle-class, ~5.2% Stripe) omits Stripe Tax (+0.5%), international card (+1.5%) and FX (+1%) uplifts documented in Loop 1 Muse §4.1–§4.3, and omits that MoR percentages are charged on the gross including tax in Paddle's case. At realistic non-US mix the MoR-vs-Stripe gap narrows or flips. **Change:** show domestic and international-mix columns, or delete the single-number column and cite Loop 1 §4.3's $10/$30/$100 table.
3. §15 tiers: "Farmer (~$39–49/mo: live mode when legal…)" sells live mode as a tier feature before §11's lawyer gate. If live is legally unavailable, Farmer is un sellable; if payments reject trading software, no tier is sellable. **Change:** tiers ship paper-only at launch; live is removed from pricing copy until §11 sign-off + a payment approval both exist.

**Missing**

4. Chargeback/refund abuse: trading-adjacent products attract "it lost my money" disputes even in paper mode (users conflating paper P&L marketing with advice). No dunning, dispute-evidence (journal export), or refund policy in §15. **Change:** entitlement Worker stores MoR/processor dispute webhooks; journal export (§9 `get_journal`) doubles as dispute evidence; add a plain-English refund/cancel policy requirement to §15.
5. §15 "Entitlements come from the payment webhook into D1, never from the client" — correct, but §8 puts `entitlement` in the same `docs` table as user-generated kinds with no write-path restriction stated. **Change:** §8/§15: only the billing Worker (service binding, not user-authenticated routes) may write `kind = entitlement`; DO mirrors are read-only for that kind.

## A7. Side services (§16)

**Wrong**

1. §16 "Email: Cloudflare Email Routing (inbound)" + "Transactional sender (free tier)" — Email Routing does not send; the transactional sender is unnamed, so passwordless/passkey recovery, fill alerts and payment receipts have no owner. Loop 1 Muse §5.1 names the pattern (Email Routing inbound + Resend/SES-class sender). **Change:** name the sender class, its free cap (**UNVERIFIED** at draft time in Loop 1), and the DMARC `p=reject` precondition before launch.
2. §16 "Auth: Better Auth on D1 (passkeys, TOTP)" conflicts silently with §8's `docs` `user` kind: Loop 1 Muse §5.3 already resolved this (Better Auth owns `user`/`session`/`account` tables; `docs.owner_id` references the Better Auth id). §8 lists `user` as a docs kind anyway. **Change:** delete `user` from §8 kinds; cite the Better Auth tables as migration 0001b.
3. §16 launch total "~$35–65/mo" reuses Loop 1's range but §16's own rows omit Workers Paid overage variance (Loop 1: $10–60/mo Cloudflare depending on DO discipline). The range's top end assumes the forbidden patterns never happen. **Change:** §16 states the assumption ("hibernating DOs, desktop runners; always-on DOs break this budget — see §13") next to the total.

**Missing**

4. No status/uptime, no backup/restore drill for D1 (7-day Time Travel on free, Loop 1 Muse §1.1), no R2 lifecycle rules. **Change:** §16 adds: D1 export schedule, DO SQLite backup expectation (none — state that hot state is rebuildable from journal + broker reconcile), R2 lifecycle for ticks.
5. No mention of Turnstile, rate limiting, or abuse cost for the public MCP (§9) — an open paper-trading MCP is a free-compute magnet. **Change:** §16 lists Turnstile + per-key rate limits (already in AGENTS.md) as launch blockers for Phase 3, with Workers Paid request overage ($0.30/M) as the cost line.

## A8. Subscriptions needed (§6, §16)

**Wrong**

1. §6.2 local CLI bridge is the plan's load-bearing AI source and its terms are admitted **UNVERIFIED** — yet DESIGN.md §4 onboarding step 4 offers "Connect one AI (key, local CLI, or local model)" as an equal choice. Loop 1 Claude red flag 7: 30 agents every 5–10 min on a consumer subscription "will hit limits within hours"; agent teams cannot even spawn in headless `-p` sessions. **Change:** onboarding and §6 rank the options: API key (supported path) > local model (Gate only) > local CLI (experimental, personal-use, may throttle or violate terms — label exactly that in UI). Remove any copy implying subscription limits can serve 30 agents.
2. §16 "Market data: Webull delayed, IEX via Alpaca free" as build phase, "Alpaca Algo Trader Plus $99/mo" at launch — but §16 launch fixed total ($35–65/mo) excludes the $99 (it is presumably passed through or user-paid; unstated). Who pays for consolidated data — DOGS (redistribution licence needed) or each user (their own Alpaca/Webull entitlement)? **Change:** §16 splits subscriptions into DOGS-paid vs user-paid columns; data entitlements are user-paid/user-entitled until a redistribution licence exists (§7 already hints this; make §16 match).
3. §6 "Jev (70–500 ms, vendor claim) ~$0.04 per M tokens" — Jev is a decision/classifier model, not a chat LLM (Loop 1 Claude post 5: "it can't write you a sentence"). §4 correctly uses it at the Gate, but §6.1 lists Jev beside Anthropic/OpenAI as a general "Brain plane" connection and §6 routing calls it a model users "connect". Users cannot substitute Jev for a Thinker. **Change:** §6 lists Jev only under Gate providers with its input shape (options + probabilities out), context limits and the overconfidence caution (B1).

**Missing**

4. No subscription inventory for the *owner's own* build: Claude/Codex/Gemini plans the owner already pays for are treated as infinite. Development + 7-day unattended paper test (§18) on consumer plans will consume quota. **Change:** §16 build phase adds a line: owner AI usage budgeted in hours/day of plan quota, with API-key overflow cost estimated from B2's budget preset.
5. No data subscription for options (OPRA), futures (CME non-display classification — Loop 1 Astra §6: display $1.55/exchange vs non-display $609/month/exchange categories) or event contracts. §13 mentions CME once. **Change:** §16 launch phase: options/futures leave the free tier only with per-user entitlement checks; do not budget display prices for automated (non-display) use without vendor confirmation (**UNVERIFIED**).

## A9. Systems (§2, §5, §7, §9, §10)

**Wrong**

1. §10 heartbeat "runner → DO every 15 s; DO marks offline after 60 s" over the runner WebSocket: if the socket is the transport, a dead socket is detected by close, not by missed heartbeats; if heartbeats also traverse Workers requests they burn the 100k/day free cap (Loop 1 Muse §1.3). Transport unspecified. **Change:** heartbeats ride the existing WebSocket as protocol pings/frames (DO WS messages bill 20:1); offline detection = socket close + last-seen timestamp; no HTTP heartbeat endpoint.
2. §9 remote MCP `place_order` (paper) for "everyone" + §9 live tools gated by "the legal gate in §11" — but §11's gate is a lawyer sign-off with no technical artefact. There is no flag, doc kind, or Worker check named. **Change:** live tools check `entitlement.live_enabled` (writable only via an admin path after legal sign-off is recorded as a `docs` kind, e.g. `legal_gate` with date/reference) plus per-user step-up state; absent doc = tool not listed at all (not listed-but-erroring).
3. §10 decision board columns (Watching → Thinking → Proposed → Approved/Vetoed → Filled → Closed) have no state machine owner: which component may move a card (Gate? Thinker? Reflex? broker event?), and what happens on partial fills, cancels, replaced orders. **Change:** define the intent lifecycle state machine in §5 (Go owns transitions; broker events only advance Filled/Closed; veto only from Risk/Reflex; expiry from TTL) and let DESIGN.md render it.

**Missing**

4. §9 has no MCP tool versioning, no per-tool rate limits, no idempotency keys on `place_order`/`propose_intent` (retried MCP calls double-order). **Change:** every mutating tool requires `idempotency_key`; journal deduplicates on it (§5 already has client order IDs — wire them together explicitly).
5. §10 monitoring has no loop-run retention: 30 agents × 288 runs/day = 8,640 `loop_run` docs/day/user synced to Cloudflare would burn D1 writes (100k/day free ≈ 11 users at 1 write/run, before anything else). **Change:** `loop_run` full docs stay in the local journal; Cloudflare sync carries hourly rollups + intent/outcome events only. State the sync budget in §10 with numbers from Loop 1 §1.3.
6. No kill-switch propagation path timed: Frost (DESIGN.md §2.4) must reach a VPS/desktop runner and cancel open orders. §10 lists alerts, §5 lists sync, neither states Frost latency or offline behaviour. **Change:** Frost is a DO-held farm flag checked by the runner on every Gate cycle and pushed over the runner socket; if the runner is unreachable, Frost state persists and the runner must refuse new orders on reconnect until it has read the flag (fail-closed).

**Over-engineered**

7. §9 ships both a remote MCP (Workers + Agents SDK + OAuth 2.1) and a local MCP (stdio + localhost HTTP) and an HTTP API mirroring both — three surfaces for one tool set before one external user exists. **Change:** Phase 3 ships remote MCP only; HTTP API is the MCP transport's by-product if trivial, else cut; local MCP ships only when the CLI bridge (§6.2) is proven.

## A10. DESIGN.md against AGENTS.md

Reviewed: `docs/DESIGN.md` vs `AGENTS.md` (Theme and styles; Responsive layouts; Layout stability and accessibility; Svelte; Verifying work) and `src/styles/theme.css` (current tokens: `--color-neon-red: #ff3344`, `--color-app: #050508`, `--color-card: #0e0e12`, `--color-card-hover: #14141a`, `--color-muted: #8e8e9f`).

**Violations / gaps**

1. **Layouts (AGENTS.md: deliberate layout at 8 screen classes; 320×320 floor; no horizontal scrollbar; internal list scroll only).** DESIGN.md §5 gives one table row per class but collapses distinctions AGENTS.md requires: tablet portrait and landscape are split (good) yet both say "3-column grid / 3–4 columns" with no side-rail behaviour change; desktop portrait is one line ("stacked into rows") with no nav treatment; TV is "monitor mode" with no focus-order, overscan or remote-back specification; ultra-wide "panels spread" has no max-width rule (AGENTS.md caps desktop at `max-w-7xl`, ultra-wide spreads — the handoff point is undefined). The 320×320 collapse is asserted for Farm only; Board/Terminal/Loops at 320×320 are unspecified. **Change:** §5 needs, per class: nav pattern, column count, Frost placement, chart:board ratio, and the 320×320 behaviour for *every* screen in §4, not just Farm.
2. **Mobile landscape (AGENTS.md: compact columns or sideways swipe list; no stacked headers).** DESIGN.md §5 mobile landscape gives the chart two columns but the Board "sideways swipe list with arrow controls" — arrow controls alone fail AGENTS.md's "swipe, drag and scroll wheel" requirement for sideways content. **Change:** specify touch swipe + drag + wheel + arrow controls together, with visible scroll affordance.
3. **Tokens (AGENTS.md: all tokens in `src/styles/theme.css` as `@theme` vars; no hex in `.svelte`/`.js`; comment each token).** DESIGN.md §3 defers all six new token values to Loop 3 and never states they land in `theme.css` with per-token comments, nor that `--color-app` (#050508, already in `theme.css`) is the AA reference background. It also introduces "the landing page's existing face" for display type without naming it — AGENTS.md has no font-token home defined. **Change:** adopt B-A11 values below as `--color-*` in `theme.css` with use-comments; add `--font-display` / `--font-numeric` tokens the same way; state components reference tokens only.
4. **Motion (AGENTS.md: reserve space; animate `transform`/`opacity` only; no layout shift).** DESIGN.md §3 "numbers roll digit by digit (opacity + translate only)" is compliant in mechanism but digit-rolling *changes string width* unless the numeric font is monospace/tabular and the container reserves max-width — §3 picks the font in the next paragraph but never connects the two. "Filled orders pulse once" on the Frost/live palette risks implying state change by colour alone (§7 requires shape cues; a pulse is neither). **Change:** rolling numbers require the numeric token (A11) + `font-variant-numeric: tabular-nums` + fixed-width reservation; pulse is paired with an icon/label change (e.g. Filled badge) so state never travels by animation alone.
5. **Reduced motion.** §3 "`prefers-reduced-motion` swaps all of it for fades" — fades are still motion; AGENTS.md requires layouts to stay still. **Change:** reduced-motion = no slide/pulse/roll; instant state change with a fade of ≤1 frame is not a fade animation; write "animations removed, state changes instant".
6. **Accessibility (AGENTS.md + DESIGN.md §7 claims WCAG 2.2 AA).** §7 asserts AA with zero measured values (§3 deferred them) — an unverified compliance claim. Live regions "announce fills politely" but the Board is a live kanban: without `aria-live` scoping, a screen reader gets either silence or tick spam; keyboard reachability is claimed for "all board actions" with no focus model for cards moving columns by themselves (a card changing column under focus is a focus-loss trap). Frost's "remote button on TV" and D-pad navigation "without traps" (§7) have no key mapping. **Change:** §7 must specify: live region = status summary only (fills count + last fill), card motion never moves DOM focus (cards update in place; column is an attribute, not a DOM move, or focus is restored), D-pad map (Frost = long-press Back or a named remote key), focus-visible style using a token (focus ring currently implied to be brand red — #ff3344 on #050508 measures 5.63:1, passes, but must not be the *only* focus cue on red chrome).
7. **Spine vs AGENTS.md.** The 1px spine + edge-to-edge 1px boundaries + mobile 0px side margins come from the AGENTS.md Superpower blueprint section (1px grid boundaries; mobile 0px margins) — compliant as far as it goes — but DESIGN.md never addresses the blueprint's "Hero Wormhole" or how the landing page's scroll-snap panels relate to the app shell; the app inherits "spine" branding without a rule for where the spine sits when the left rail (tablet/desktop) exists. **Change:** define spine position relative to the rail (spine = rail's right edge) or drop the spine inside the app and keep it marketing-only.
8. **Potato Target / fallback.** DESIGN.md header claims it "Follows AGENTS.md (…Potato Target browsers)" but no screen in §4 states its no-JS/legacy behaviour; the trading UI obviously needs JS, so the fallback obligation is the *landing* page's — say so. **Change:** one line: app screens require JS; `index.html` fallback mirrors marketing copy only and never shows live/paper account state.
9. **Strings i18n (AGENTS.md: >10 UI strings → `.json` beside component).** DESIGN.md §4 names 9 screens with dozens of labels and §1 a six-word metaphor glossary; no string-file plan. Minor at draft stage; note it before build.

### A11. Concrete AA tokens on `--color-app: #050508` (computed, WCAG 2.x relative-luminance formula; AA normal text ≥ 4.5:1, AA large text / non-text ≥ 3:1)

| Token | Proposed hex | Contrast on #050508 | Verdict | Use / distinction |
|---|---|---|---|---|
| `--color-gain` | `#2EEA7B` | 12.77:1 | AA + AAA (normal text) | Positive P&L, up candles. Green hue (~146°) — far from brand red (~355°). Always paired with ▲ / `+`. |
| `--color-loss` | `#FF6B4A` | 7.23:1 | AA + AAA (normal text) | Negative P&L, down candles. Orange-red hue (~12°) vs brand `#FF3344` (~355°): distinct under normal vision and at small sizes by lightness+saturation; never used for chrome. Always paired with ▼ / `−`. Brand red stays chrome/focus/Frost only (DESIGN.md §3 rule — keep it). |
| `--color-paper` | `#6EC1FF` | 10.41:1 | AA + AAA | Paper badge/borders. Blue — unambiguous vs gain green and live fuchsia by hue + the literal "PAPER" label. |
| `--color-live` | `#E879F9` | 8.27:1 | AA + AAA | Live badge. Fuchsia — cannot be confused with loss orange-red or brand red at badge sizes; pulsing *outline* (DESIGN.md §3) plus "LIVE" text carries the state, colour is redundant coding. |
| `--color-warn` | `#FFB224` | 11.29:1 | AA + AAA | Stale data / budget / offline. Amber, paired with ⚠ icon + text reason; never used for P&L. |
| `--color-grid` | `#2E2E3A` | 1.52:1 | Decorative only — exempt (WCAG 1.4.11 covers meaningful UI boundaries, not decorative gridlines) | 1px spine/panel boundaries. Deliberately low-contrast; any boundary that *conveys* structure (separating actionable regions) must instead use `--color-muted` (#8E8E9F, 6.32:1, already in `theme.css`) or gain a label. State this split in `theme.css` comments. |

Reference points measured the same way: brand `#FF3344` = 5.63:1 (passes AA normal text — usable for focus rings with a second cue), existing `--color-muted: #8E8E9F` = 6.32:1 (passes AA).

Colour-vision note: gain/loss are red-green-adjacent for deuteranopes despite the hue gap above; DESIGN.md §3's shape rule (▲▼, +/−, position above/below baseline on candles) is therefore load-bearing, not decorative. Candles must also differ in fill (gain = filled, loss = hollow or vice versa) — add that to DESIGN.md §3.

**Monospace numeric font:** **IBM Plex Mono** (Google Fonts; 400/500/600). True tabular figures, slashed-zero distinction from `O`, narrower than JetBrains Mono at dashboard densities, OFL-licensed. Token: `--font-numeric: "IBM Plex Mono", ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, monospace;` with `font-variant-numeric: tabular-nums` on all price/P&L/quantity classes. JetBrains Mono is the acceptable alternative; do not ship both.

---

# Part B — Live-web research (2026-10-30)

## B1. LLM speed and price table, late 2026

Prices per 1M tokens (input/output, USD). Speed = output tokens/s unless stated. "Vendor claim" = provider/secondary source, not independently benchmarked here — treat all tok/s as **UNVERIFIED** for capacity planning until load-tested with Money 24 prompts.

| Model / route | $/M in | $/M out | Speed (tok/s) | TTFT | Free tier | Notes |
|---|---|---|---|---|---|---|
| TypeSafe Jev `typesafe/jev-1.14` (OpenRouter Decisions API, alpha) | $0.042 | $0 (free) | n/a — decision model, returns typed choices + probabilities; 70–500 ms/call vendor claim; medians 214–303 ms in secondary tests | = call latency (no streaming tokens) | **UNVERIFIED** (waitlist/early access per Loop 1) | Not an LLM: cannot write text or theses. Gate/classifier only. 64k default / 262k long-context window. 100k tok/s + 80 req/s rate limits reported. Overconfidence-on-policy-labels caution (Loop 1) applies directly to trade/no-trade thresholds. Sources: https://openrouter.ai/typesafe/jev-1.14 , https://typesafe.ai/blog/introducing-system-one-models-and-jev |
| Groq — gpt-oss-20B | $0.075 | $0.30 | ~1,000 | sub-second class | Free tier exists (rate-limited) | Cheapest fast open-weight route. https://groq.com (via secondary benchmarks) |
| Groq — Llama 3.3 70B / Qwen3 32B | ~$0.29 class | ~$0.59 class | ~297–840 (varies by model/benchmark) | low | Free tier exists | Speed figures conflict across benchmarks — **UNVERIFIED** for planning. |
| Cerebras — Llama 3.3 70B class | ~$0.60 class | (bundled rates vary) | ~1,700–2,500 claims | very low | 1M tokens/day free reported | Fastest claims; per-model pricing $0.10–$6/M range-wide. https://www.cerebras.ai — exact Money 24 model rate **UNVERIFIED** until quoted. |
| SambaNova Cloud | $0–$4.50/M range | — | ~281–700 | low | Free tier reported | https://sambanova.ai — model-specific rate **UNVERIFIED**. |
| Gemini 3.1 Flash-Lite | $0.25 | $1.50 | — | — | Flash/Flash-Lite free tier (quotas, content used for improvement on free) | Budget-preset workhorse candidate. https://ai.google.dev (pricing via secondary reads of Google pricing page, Oct 2026) |
| Gemini 3.8 Flash | $0.75 promo → $1.50 (2027-01-01) | $3.75 promo → $7.50 | — | — | Free tier listed | Promo expiry doubles B2 balanced cost in 2027 — date this in §6. |
| Gemini 3.5 Flash-Lite | $0.30 | $2.50 | — | — | Free tier listed | |
| Claude Haiku 5.5 (≤100k prompts) | $0.10 promo class | $0.50 | fast tier | — | No free API tier | Price jumps outside promo/long-context — **UNVERIFIED** at >100k; Haiku 4.5 reference $1.00/$5.00. https://www.anthropic.com/pricing (via Oct 2026 reads) |
| Claude Sonnet 5.5 | $2.00 | $10.00 | mid | — | No free API tier | Smart-preset default in B2. |
| Claude Opus 5.5 | $4.00 | $20.00 | slower | — | No free API tier | Strategist-only candidate. |
| GPT-5 mini / GPT-5.4 Mini | $0.25 / $0.75 | $2.00 / $4.50 | >80 tok/s (Fast-mode SLA class) | — | No free API tier | https://developers.openai.com/api/docs/pricing |
| GPT-5.6 Sol (flagship) / Terra / Luna | $5.00 / $2.50 / $1.00 | $30.00 / $15.00 / $6.00 | — | — | No | Flagship ceiling for B2 sensitivity. https://openai.com/index/gpt-5-6/ |
| Grok 4.7 (xAI flagship) | $2.00 (>$4.00 ≥200k ctx) | $6.00 (>$12.00) | — | — | No | https://x.ai (via Oct 2026 reads of xAI pricing) |
| Grok 4 Fast (non-reasoning) / grok-code-fast-1 | $0.20 | $0.50 / $1.50 | "fast" tier | — | No | Cheap Thinker candidate; reasoning variant pricing differs — **UNVERIFIED** which variant suits Trader role. |
| Cloudflare Workers AI (10k Neurons/day free) | per-model token rates (e.g. Llama 3.1 8B ≈ $0.045/$0.384) | — | edge inference | low | **10,000 Neurons/day/account, resets 00:00 UTC; overage $0.011/1,000 Neurons on Paid** | Free allowance ≈ ~150k input *or* ~15k output tokens/day on 8B-class models (Loop 1 Muse §1.1) — a demo budget, not a trading budget. Official: https://developers.cloudflare.com/workers-ai/platform/pricing/ |

**Reading for Money 24:** (1) Jev's $0.042/M buys classification, not reasoning — §4's Gate is the only correct home for it. (2) The credible Thinker band is $0.0006–$0.004 per 3k/500 call (Haiku 5.5 promo → Gemini Flash promo); Sonnet-class is $0.011/call and flagship $0.022–$0.030/call (B2). (3) Every consumer-subscription "free" path remains terms/cap-bound (Loop 1); no row above makes 30 agents × 288 loops/day free. (4) All tok/s numbers are benchmark-vendor numbers on short prompts; Money 24's 3k-token snapshots will see lower effective throughput — **UNVERIFIED** until spiked.

## B2. ARCHITECTURE.md §6 cost math — completed

**Stated inputs (§6 + task):** 30 agents × every 5 min × 24 h = **8,640 loops/day/user**. Gate escalation 10–20% → **864 / 1,296 / 1,728 Thinker calls/day** (10% / 15% / 20%). Per escalated call ≈ **3,000 in / 500 out tokens**. Debate/roles multiplier excluded first, then applied (see warning).

Escalated tokens/day: 10% → 2.592M in / 0.432M out · 15% → 3.888M / 0.648M · 20% → 5.184M / 0.864M.

Per-call cost at 3k/500 (from B1): Jev-class gate decision $0.000126 (1k-token gate call ≈ $0.000042) · Gemini 3.1 Flash-Lite $0.00150 · Claude Haiku 5.5 (promo) $0.00055 · GPT-5 mini $0.00175 · Grok 4 Fast $0.00110 · Gemini 3.8 Flash (promo) $0.00413 · Claude Sonnet 5.5 $0.01100 · Claude Opus 5.5 $0.02200 · GPT-5.6 Sol $0.03000.

**Cost per user (escalated Thinker calls only; gate = deterministic rules at $0):**

| Escalation | Flash-Lite $0.0015 | Haiku 5.5 $0.00055 | GPT-5 mini $0.00175 | Sonnet 5.5 $0.011 | Opus 5.5 $0.022 |
|---|---|---|---|---|---|
| 10% (864 calls/day) | $1.30/day · **$38.88/mo** | $0.48/day · $14.26/mo | $1.51/day · $45.36/mo | $9.50/day · $285.12/mo | $19.01/day · $570.24/mo |
| 15% (1,296/day) | $1.94/day · **$58.32/mo** | $0.71/day · $21.38/mo | $2.27/day · $68.04/mo | $14.26/day · $427.68/mo | $28.51/day · $855.36/mo |
| 20% (1,728/day) | $2.59/day · **$77.76/mo** | $0.95/day · $28.51/mo | $3.02/day · $90.72/mo | $19.01/day · $570.24/mo | $38.02/day · $1,140.48/mo |

(Month = 30 days. Gemini Flash promo at 15% = $5.35/day · $160.38/mo, doubling to ~$320/mo after 2027-01-01. Grok 4 Fast at 15% = $1.43/day · $42.77/mo.)

**The four routing presets §6 needs (per user):**

| Preset | Routing | Per user per day | Per user per month |
|---|---|---|---|
| **Budget** | Rules gate ($0) → Gemini 3.1 Flash-Lite Thinker; Strategist weekly on Flash-Lite | $1.30 (10%) – $2.59 (20%) | **$39–$78** |
| **Balanced** | Rules/Jev gate → GPT-5 mini Thinker (Haiku 5.5-promo variant: $0.48–$0.95/day) | $1.51 – $3.02 | **$45–$91** (Haiku variant $14–$29, promo-dependent) |
| **Smart** | Jev gate → Sonnet 5.5 Thinker; Opus 5.5 Strategist daily adds ~$0.02–$0.10/day | $9.50 – $19.01 | **$285–$570** |
| **Local-only** | Ollama/local model for Gate + Thinker on user hardware | $0 tokens | **$0** + user electricity/hardware; quality/latency **UNVERIFIED** for Trader role (§6.3 already distrusts it — keep that) |

**Add-ons §6 currently hides:**

- **Jev gate on every loop** (if rules are replaced by Jev): 8,640 calls/day × ~1,000 input tokens × $0.042/M ≈ **$0.36/day · $10.89/mo per user**. Cheap, not free; and Jev output tokens are free, which is why this is the only sane always-on paid gate.
- **No gating at all** (every loop thinks, Sonnet): 8,640 × $0.011 = **$95.04/day · $2,851.20/mo per user**. This is the number §6's "< 1,500 calls/day" target exists to avoid; print it in §6.
- **Debate multiplier (§4):** Bull+Bear+Trader+Risk+PM ≈ 3–5 Thinker calls per intent. Smart preset with full debate ≈ **$855–$2,850/mo per user**. §6's draft math ("cents/day on Jev/Flash-class, dollars/day on Sonnet-class") is wrong by this factor until roles are cut (A1.10).
- **Analyst caching** (§4 "analysts cache their view") is asserted, not modelled: if analysts run on their own cadences regardless of gate, add their calls on top — §6 must give each role a cadence and a calls/day line, not one blended "< 1,500".
- Workers AI free allowance covers ~15k output tokens/day/account — under 2% of one user's 15%-escalation output. It cannot subsidise the Balanced preset; delete any implication otherwise.

**Verdict for §6:** at BYO-key the user pays these numbers, which decides the market: Budget $39–78/mo of AI spend on top of a $19/mo subscription is already a hard sell; Smart is a $300+/mo hobby. The product consequence §6 must state: default preset = Budget, debate off, escalation target ≤10%, per-farm daily dollar cap defaulting to ~$2/day with hard pause (DESIGN.md Loops budget bars already imply this — wire them to these numbers).

## B3. Payments: do Creem, Dodo, Paddle, Lemon Squeezy, Polar accept trading/investing software?

Checked against each provider's published prohibited/acceptable-use material, 2026-10-30. "Accept" below means the published list does not prohibit the category; only a written approval for Money 24's exact product is proof.

| Provider | Published position on trading/investing software | Verdict for Money 24 |
|---|---|---|
| **Creem** | Prohibited: "Trading and investment tools and services that enable or facilitate trade execution or automation". Restricted (review): "Trading and investment communities, courses, and educational products that do not enable trade execution or automation". Sources: https://docs.creem.io/merchant-of-record/account-reviews/account-reviews , https://www.creem.io/SKILL.md | **Prohibited for the core product** (execution/automation is the product). Paper-only *education* framing might fit "restricted" — but the moment the runner places even paper orders as a sold feature, expect rejection/offboarding. ARCHITECTURE.md §15's "Recommended at launch" is inverted. |
| **Polar** | Prohibited: "Financial services, e.g. facilitating transactions, investments or balances for customers" and "Financial trading, trading bots, brokerage, or investment advisory services (including insights platforms)". Source: https://polar.sh/legal/acceptable-use-policy (effective 2026-03-25) | **Prohibited** — "trading bots" and "insights platforms" name Money 24 twice. Not a fallback. |
| **Paddle** | Prohibited category 10: "Trading and financial services/advice — any product or service that is a regulated financial product/service; business or investment opportunities; investment or financial advice, including trading signals and strategies". Source: https://www.paddle.com/help/start/intro-to-paddle/what-am-i-not-allowed-to-sell-on-paddle | **Prohibited as described.** A "user-controlled software, no advice/signals" argument exists on paper, but Paddle's wording covers strategies; approval **UNVERIFIED** — requires pre-application with exact copy. Also note Paddle's 2025 FTC settlement context increases screening strictness (Loop 1 Muse §4 context). |
| **Lemon Squeezy** (Stripe-owned) | Prohibited list bans "regulated services such as… banking/financing, currency exchange…" and services generally; trading-automation software is not named verbatim in the excerpt read. Anecdotal store suspensions for financial services exist (2025). Source: https://docs.lemonsqueezy.com/help/getting-started/prohibited-products | **UNVERIFIED / high-risk.** Absence of the words "trading bots" is not acceptance; Stripe-owned risk review will classify auto-trading software as regulated financial services until told otherwise in writing. |
| **Dodo Payments** | Published merchant-acceptance material prohibits illegal/physical/human-delivered categories; trading software not named in the excerpts read. Source: https://dodopayments.com (Merchant Acceptance Policy, via docs) | **UNVERIFIED.** "Not listed" ≠ accepted. Dodo actively test-purchases and offboards undisclosed categories (their own Trustpilot responses). Requires written pre-clearance describing paper trading + automation honestly. |

**Which is safest?** None of the five MoRs is safe on published terms for a product whose headline feature is automated trade execution. Ranking by published hostility: Creem (explicit execution ban) ≈ Polar (explicit bot ban) > Paddle (explicit signals/strategies ban) > Lemon Squeezy (regulated-services ban, trading unnamed) ≈ Dodo (unnamed, **UNVERIFIED**). The safest launch rail is therefore the one §15 treats as the later option: **Stripe as processor** (financial software is reviewable rather than category-banned; Loop 1 Muse §4.5 migration logic reversed — start there, or launch US-scoped with tax handling budgeted), with MoRs pursued only as written-approval experiments. Practical sequence: (1) apply to Stripe describing "paper-trading simulation software; user-controlled automation; no custody, no advice"; (2) in parallel, request written pre-clearance from Dodo (least explicitly hostile) with the same description; (3) sell paper-only at launch — every provider's risk drops when no live money moves, and Creem's own restricted tier ("communities/courses/education, no execution") shows the only framing MoRs contemplate; (4) never let checkout copy say "AI picks winning trades" — that sentence alone triggers advice/signals bans and §11 adviser risk simultaneously.

## B4. Tauri 2: Go sidecar (`externalBin`), signing, keep-awake, autostart, Android

Sources: Tauri 2 sidecar docs https://v2.tauri.app/develop/sidecar/ plus platform behaviour below; items marked **UNVERIFIED** need a build spike on real hardware.

| Topic | Finding | Consequence for Money 24 |
|---|---|---|
| Shipping a Go binary via `bundle.externalBin` | Supported on desktop (Windows/macOS/Linux): binary named with target triple (e.g. `m24d-x86_64-pc-windows-msvc.exe`, `m24d-aarch64-apple-darwin`), declared in `tauri.conf.json` `bundle.externalBin`, spawned via the shell plugin / `Command::sidecar`. Go's static binaries fit this well; cgo (see A1.4) breaks per-triple purity. | Desktop plan in ARCHITECTURE.md §5 / DESIGN.md §6 is viable. Build matrix = one Go build per Tauri target triple; CI must produce shell+sidecar together (A1.5 version gate). |
| macOS signing + notarization of sidecars | Every nested binary must be code-signed with Hardened Runtime *before* the app bundle is signed and notarized, inside-out. Tauri signs bundle contents during `tauri build` when signing identity env vars are set; sidecars with entitlements/cgo quirks are a known notarization failure class (nested-code signing issues reported against `externalBin`). | Budget the Apple Developer $99/yr (§16) plus a notarization spike: sign `m24d` explicitly in CI, verify with `codesign --verify` + notarization log on the .app, on both arm64 and x86_64 (universal binary doubles sidecar size or ships two builds). Do not promise signed macOS builds in §18 Phase 2 until this spike passes. |
| Windows signing of sidecars | The .exe sidecar inside an NSIS/MSI installer should itself be Authenticode-signed (SmartScreen reputation attaches per-binary); Tauri's Windows signing covers the installer/app — sidecar signing must be wired explicitly (signtool step before bundling, or Azure Trusted Signing). | §16's OV cert line (~$215/yr class) covers this; add "sign sidecar .exe before `tauri build`" to the release checklist. Unsigned sidecars are also the first thing endpoint AV flags (a Go binary spawning broker connections) — unsigned Windows launch will generate false-positive reports; plan the AV-vendor allowlist submissions. **UNVERIFIED** per-vendor. |
| Linux | No signing regime; sidecar ships in AppImage/deb/rpm as a bundled resource. Distro WebView (webkit2gtk) variance is the real cost. | Keep Linux in §18 only if CI builds it per release; otherwise desktop v1 = Windows + macOS and Linux is "best effort" — say which in DESIGN.md §6. |
| Keep-awake while armed | No first-party Tauri "keep awake" plugin covers all desktops. Implementations are OS APIs called from Rust: Windows `SetThreadExecutionState`, macOS `IOPMAssertionCreateWithName` (via `core-foundation`/`io-kit-sys` or a small crate), Linux `systemd-inhibit` / DBus ScreenSaver inhibit (portal variance). Community crates exist but are a dependency decision under AGENTS.md's dependency rules. | DESIGN.md §6 "Keeps the machine awake while a farm is armed (opt-in, with a visible indicator)" is implementable but is custom Rust, not a config flag — §5/§6 must budget it, default OFF, release the assertion on Frost/disarm/crash, and show the indicator whenever asserted (a laptop dying with lid closed is still a dead runner — keep-awake ≠ lid-closed operation on most Windows/macOS power policies; state that limit in the UI copy). |
| Autostart at login | `tauri-plugin-autostart` (official plugin org) covers Windows (registry/Startup), macOS (LaunchAgent / SMAppService class behaviour), Linux (.desktop autostart). Opt-in per DESIGN.md §6 is the correct default. | Add to §5: autostart launches the *shell minimised to tray*; the runner must not auto-arm or auto-trade after reboot without a fresh broker reconcile + capability read (A5.2). "Starts at login" + "resumes armed" are different features; ship only the first at launch. |
| Android sidecar feasibility | **Not feasible as designed.** Tauri mobile does not support `externalBin` sidecar processes; Android builds must exclude the sidecar (builds fail or the binary is unusable when forced). Android also kills background processes aggressively; a 24/7 runner would need a foreground service + native (Kotlin/Rust-as-library) implementation, battery-exemption prompts, and Play background-policy compliance — a different product, not a port. iOS is stricter still. | ARCHITECTURE.md §5 ("Android: the runner does not run 24/7 on a phone. Mobile is a remote control") and DESIGN.md §6 already conclude this — make it unconditional: Android target ships *without* `externalBin` in its Tauri config (separate `tauri.android.conf.json` override), no Go process on device, all trading state from the desktop/VPS runner via the DO. Any future on-device runner is a JNI/.so library project — out of scope until separately approved. |

**Net:** desktop sidecar = sound, with notarization/AV spikes as Phase 2 exit criteria; keep-awake/autostart = small custom Rust surface, feasible; Android sidecar = impossible in Tauri's model — the drafts' monitor-only conclusion stands and must not soften.

---

# Prioritised edits to ARCHITECTURE.md / DESIGN.md (top 10)

1. **§15 + §14 (payments): invert the recommendation.** Creem/Polar/Paddle publish prohibitions covering trading bots/execution/signals (B3). Stripe-first; MoRs only with written pre-clearance; paper-only launch copy; live mode removed from tiers until a processor approval + §11 legal sign-off both exist.
2. **§6 (Brain plane): demote the local CLI bridge and price the presets.** API keys are the supported path; CLI bridge labelled experimental/terms-**UNVERIFIED** with throttling warning; insert B2's preset table (Budget $39–78/mo, Balanced $45–91/mo, Smart $285–570/mo per user at 3k/500, 10–20% escalation) plus the ungated $2,851/mo and debate-multiplier warnings; default = Budget, ≤10% escalation, ~$2/day farm cap.
3. **§4 + §6 (roles): cut the TradingAgents debate from v1.** One Thinker + deterministic Risk per garden; Bull/Bear/PM debate becomes a Phase 2, budget-flagged experiment. §6 cost lines must be per-role with cadences, including analyst caching as numbers.
4. **§2/§5/§7 (sync): specify the runner↔DO pattern.** Runner-dialled WebSocket, DO as hibernating server, idempotent sync on `client_order_id`, expiring intents (never offline-queued orders), farm lease (one active runner per account), heartbeats as WS frames only (§10 fix included).
5. **§3 + §18 (Go client): gate Go on the proto spike.** REST+MQTT first, gRPC last with REST-polling fallback named in §3; name the SQLite driver/cgo decision and the per-triple sidecar build matrix; keep all Go sections conditional on the AGENTS.md amendment being signed off.
6. **§1 + §18 (scope): cut Phase 1 to stocks/ETFs paper, one garden, local journal + monitor.** Options/futures/crypto/events each gated on a named sandbox acceptance test; remote MCP/Queues/R2-Parquet/envelope encryption explicitly deferred to their phases; add measurable exit metrics (7 days unattended, zero unreconciled orders).
7. **§11/§9 (live gate as code):** live MCP tools exist only when a recorded `legal_gate` doc + `entitlement.live_enabled` + step-up state all check out in the Worker; Reflex limits read broker `account_capabilities` (PDT/intraday-margin transition, options/futures approvals); walk-forward verdict enforced server-side before live enable.
8. **§8 (data): remove `user` from docs kinds (Better Auth owns identity tables), resolve the JS/Go validator duplication** (single generated source or admitted duplication + contract tests), add `loop_run` rollup-only sync budget and R2 size/retention estimates; entitlement writes restricted to the billing Worker path.
9. **§13 (bottlenecks): re-rank** — approvals (Connect, payments, subscription terms) and runner liveness first, data speed last; copy in the three forbidden Cloudflare patterns (no per-tick Queues, no non-hibernating per-user DOs, no KV hot state) and the support-load bottleneck.
10. **DESIGN.md §3/§5/§7: adopt the measured token set** (`--color-gain #2EEA7B` 12.77:1, `--color-loss #FF6B4A` 7.23:1, `--color-paper #6EC1FF` 10.41:1, `--color-live #E879F9` 8.27:1, `--color-warn #FFB224` 11.29:1 on `#050508`; `--color-grid #2E2E3A` decorative-only) + IBM Plex Mono `--font-numeric`, fill-pattern candle distinction, reduced-motion = motion removed, per-screen 320×320 + focus/live-region rules for all 8 layout classes.

---

*End of Loop 3 (Muse). Files touched: `docs/research/loop3-muse-review.md` only. All Part B prices/limits are 2026-10-30 snapshots from the cited pages and drift; re-verify before pricing or promising anything. Items marked UNVERIFIED require a spike or written vendor confirmation.*
