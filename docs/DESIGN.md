# Money 24 — Product and Interface Design

Status: **Loop 2 draft** (2026-10-30). Companion to [ARCHITECTURE.md](ARCHITECTURE.md).
Follows AGENTS.md (layouts per screen class, tokens in `src/styles/theme.css`, motion with
`transform`/`opacity`, Potato Target browsers) and the existing Money 24 landing page
(neon-red on near-black, scroll-snap panels).

---

## 1. The product in one sentence

A farm of AI trading bots you can watch think: plant bots, give them a mandate, let them
paper trade around the clock, and see every decision, every dollar and every veto.

### Metaphor (used everywhere, consistently)
| Word | Meaning |
|---|---|
| **Bot** | One agent with a role, a model and a cadence |
| **Garden** | A strategy: a universe of symbols, a mandate, a set of bots (analysts, trader, risk) |
| **Farm** | All gardens on one broker account, with farm-wide risk limits |
| **Season** | A dated run of a garden (paper or live) with its own score card |
| **Harvest** | Realised P&L, shown only after fees |
| **Frost** | Kill switch: freezes every bot and cancels open orders |

---

## 2. Principles

1. **Show the thinking.** Every order traces back to a card showing who proposed it, why,
   which risk checks it passed, and what it cost in tokens.
2. **Paper is the default, live is a ceremony.** Live mode needs step-up 2FA, a typed
   confirmation, per-garden limits, and a visible LIVE badge on every screen.
3. **Honest numbers.** P&L after fees; paper results labelled *paper* and *delayed data*
   where true; backtests carry a verdict (*validated / trade-off / no edge*).
4. **Frost is always one tap away.** Fixed position, every screen, every size, keyboard
   shortcut, remote button on TV.
5. **Calm by default.** A bot farm that runs 24/7 must not demand attention. Alerts are
   rare, grouped, and actionable.

---

## 3. Visual system

### Brand vs. money colour (important)
The brand is neon red. In trading UIs red means *loss*. Rule: **brand red is chrome only**
(logo, focus rings, active tab, Frost button). Numbers never use brand red.
New tokens to add to `src/styles/theme.css` (values chosen in Loop 3, WCAG AA on `--color-app`):

| Token | Use |
|---|---|
| `--color-gain` | Positive P&L, up candles, filled buys |
| `--color-loss` | Negative P&L, down candles. Distinct hue from `--color-neon-red` |
| `--color-paper` | Paper-mode badge and borders |
| `--color-live` | Live-mode badge (high-contrast, pulsing outline) |
| `--color-warn` | Stale data, budget near cap, runner offline |
| `--color-grid` | 1px spine and grid boundaries |

Gain/loss also carry shape (▲ ▼, +/−), never colour alone.

### Type
Display: the landing page's existing face. Numbers: a tabular-figure monospace (Google
Fonts, e.g. JetBrains Mono or IBM Plex Mono, picked in Loop 3) so prices don't jitter.
Fluid sizes with `clamp()`.

### The spine
A 1px vertical line runs down the centre-left of every screen (the "spine"). Sections
attach to it; the decision board's cards hang off it like vertebrae; on desktop, panels
butt edge-to-edge separated by 1px `--color-grid` boundaries. On mobile, panels go
full-bleed with 0px side margins.

### Motion
150 ms spring on every interactive element. New decision cards slide along the spine;
filled orders pulse once; numbers roll digit by digit (opacity + translate only).
`prefers-reduced-motion` swaps all of it for fades.

---

## 4. Screens

| Screen | Purpose | Key components |
|---|---|---|
| **Farm** (home) | Is everything OK? | Equity curve (paper/live split), today's harvest, runner status, bots thinking now, Frost |
| **Garden** | One strategy | Mandate, universe, bots with status lights, season score card, walk-forward verdict |
| **Board** | Watch decisions live | Kanban: Watching → Thinking → Proposed → Approved/Vetoed → Filled → Closed |
| **Terminal** | TradingView-like chart | `lightweight-charts` (Apache-2.0, attribution kept), indicators, bot markers on candles (entries, stops, thesis tooltips), order ticket (paper) |
| **Ledger** | Every intent, order and fill | Hash-chain check mark, filters, export CSV |
| **Loops** | Monitor the AI | Per-bot heartbeat, last run, tokens and cost today, error streaks, budget bars, restart/pause |
| **Connections** | Broker and AI | Webull (sandbox/live), Alpaca, Kalshi; AI keys, local CLIs detected, local models; runner pairing |
| **Agents API** | External agents | MCP URL, OAuth clients, API keys per garden, `skill.md` download, usage |
| **Settings / Plan** | Account, billing | Plan, entitlements, alerts (push, Telegram, Discord), data sources |

### Onboarding (first 5 minutes)
1. Sign in (passkey). 2. Choose: *try in the cloud* (paper, minute cadence) or *install the
desktop runner*. 3. Connect Webull sandbox (or skip to built-in paper with delayed data).
4. Connect one AI (key, local CLI, or local model). 5. Plant a starter garden from a
template (e.g. "SPY trend + risk manager"). The board lights up within one loop.

---

## 5. Layouts per screen class (AGENTS.md)

| Class | Farm / Board / Terminal |
|---|---|
| Mobile portrait | One column; bottom tab bar (Farm, Board, Terminal, Loops, More) in thumb reach; Frost as a floating button bottom-right; Board becomes a vertical swipe list per column with column chips |
| Mobile landscape | Two compact columns (chart + positions); board columns as a sideways swipe list with arrow controls; no stacked headers |
| Tablet portrait | Persistent left rail (icons + labels); 3-column grid (chart spans 2) |
| Tablet landscape | Rail + 3–4 columns; Board shows 4 columns visible, swipe for rest |
| Desktop landscape | Rail + full dashboard capped at `max-w-7xl`; Terminal can go full width |
| Desktop portrait | Content stacked into rows: chart, then board, then ledger |
| TV | Monitor mode: large type, big equity number, board with 3 columns, D-pad focus, Frost on a remote key |
| Ultra-wide | Panels spread: chart, board and loops side by side |

Fit on one screen; lists scroll internally; no horizontal scrollbar; 320×320 must work
(Farm collapses to status + Frost + harvest).

---

## 6. Native shells (Tauri 2)

- **Desktop** (Windows, macOS, Linux): bundles `m24d` as a sidecar. Tray icon shows runner
  state (green running, amber degraded, red stopped, blue paper). Keeps the machine awake
  while a farm is armed (opt-in, with a visible indicator). Starts at login (opt-in).
- **Android**: monitor and remote control for a desktop/VPS runner; push alerts; Frost.
  No 24/7 runner on phones. iOS later, after store review risk is understood.
- Web app works on its own with cloud-only paper mode; native APIs are feature-detected.

---

## 7. Accessibility

WCAG 2.2 AA contrast on all tokens; tabular numbers; gain/loss with shape cues; all board
actions keyboard reachable; live regions announce fills politely, not every tick; focus
visible on every control; TV D-pad navigation without traps.

---

## 8. Open design questions for Loop 3+

1. Exact gain/loss/paper/live token values with AA contrast checks.
2. Monospace numeric font choice.
3. Should the mobile app be allowed to approve live orders (step-up 2FA on device)?
4. How much of the agents' raw reasoning to show vs. a summary (cost, noise, leakage of
   prompts).
5. Templates for starter gardens (which strategies are honest enough to ship?).
