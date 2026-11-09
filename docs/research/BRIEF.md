# Money 24 planning brief (shared by every research agent)

Money 24 by DOGS: a cross-platform, 24/7, autonomous, agentic AI trading system
for stocks, options, futures, crypto and prediction markets. Paper trading first.

Owner constraints and wishes (verbatim intent, not yet validated):
- Broker: Webull API as the primary broker (OpenAPI), including its paper trading.
  Wants to store as much data as possible at Webull to minimise our database.
- Infra: all Cloudflare, all serverless: Workers, Pages, D1 (SQLite), R2, plus
  whatever else is free. Must stay on the free tier while building.
  Owner dislikes migrations and structured schemas, leans NoSQL. Unsure.
- Stack: Svelte 5 + Vite + Tailwind v4, JavaScript with JSDoc only (no TypeScript,
  no ORM). Tauri 2 shells for Windows, macOS, Android APK, plus the web app.
- AI: connect user's own Claude subscription, ChatGPT/Codex subscription, Grok,
  Antigravity/Gemini, or an OpenCode local setup. Maybe require OpenCode on install
  for local compute. ~30 agents running loops every 5-10 minutes, restart forever.
  Wants decisions at second intervals. Wants a very fast LLM called "Jev"
  (identity unknown, find out what the owner likely means).
- Product: Webull login, TradingView-like charts, paper trading, an MCP server and
  API so external agents can paper trade and real trade, monitoring of AI loops.
- Business: owner is broke, wants paid tiers later, thinks Stripe takes too much.
- Open questions: global markets? fastest data source? payments? subscriptions needed?
- Inspiration X posts (owner wants to match their capabilities, reuse open-source code):
  https://x.com/seelffff/status/2116178087419208072
  https://x.com/DaviddDotTech/status/2115867763088765298
  https://x.com/carm1nee/status/2115936358003052667
  https://x.com/hooeem/status/2116177486765556082
  https://x.com/mirku21/status/2115911644501147972
  https://x.com/issatheblows/status/2115882378271055879
  https://x.com/RohOnChain/status/2115871785250111615
  https://x.com/niraj_munot/status/2115712164375957669
  https://x.com/cryptotrice/status/2115453869883047937

Rules for research agents:
- Today is 2026-10-30. Verify facts on the live web; cite URLs inline. Mark anything
  you could not verify as UNVERIFIED. Prefer official docs over blogs.
- Be blunt. If a wish is impossible, illegal, against a provider's terms, or a money
  trap, say so and give the best alternative.
- Planning only. Do not write application code. Do not edit files outside
  docs/research/. Do not commit, push, or create branches.
- Never write any personal name of the owner. The company name DOGS is fine.

## Update from the owner (during Loop 1)
- Language for the trading engine: owner dislikes Python and prefers Go, and suspects
  Go is faster. Working position for later loops: Cloudflare Workers and the Svelte UI
  stay JavaScript; the always-on trading engine (broker streams, strategy runner,
  risk checks, agent scheduler) is evaluated as a Go binary that runs as a Tauri
  sidecar on the user's machine and/or on a cheap VPS. Python is allowed only for
  optional offline research/backtesting, never required. This would amend the
  "JavaScript only" rule in AGENTS.md and must be recorded as a decision.
- Later loops must check: Webull SDK availability per language (Go has no official
  SDK? verify), gRPC/MQTT library quality in Go, and whether Go actually matters
  given broker and LLM latency.
