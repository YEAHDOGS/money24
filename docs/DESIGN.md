# Money 24 — Product and Interface Design

Status: **Loop 3 revision** (2026-10-30). Companion to [ARCHITECTURE.md](ARCHITECTURE.md).
Follows AGENTS.md: deliberate layouts for eight screen classes, tokens only in
`src/styles/theme.css`, motion with `transform`/`opacity`, 320×320 floor. App screens
need JS; the `index.html` fallback mirrors marketing copy only and never shows account state.

---

## 1. The product in one sentence

A farm of AI trading bots you can watch think: plant bots, give them a mandate, let them
paper trade around the clock, and see every decision, every dollar and every veto.

### Words
Plain financial terms are the **primary labels**; the farm metaphor is secondary flavour
(headings, empty states, onboarding). Never hide a financial concept behind a metaphor.

| Primary label | Flavour | Meaning |
|---|---|---|
| Bot | — | One AI role with a model and cadence |
| Strategy | Garden | A universe, a mandate, rules and bots |
| Account | Farm | All strategies on one broker account, with account-wide limits |
| Season | Season | A dated, pre-registered run of a strategy, paper or live |
| Realized P&L | Harvest | After fees |
| Pause / Cancel entries / Flatten | — | Stop controls (§2.4) |

---

## 2. Principles

1. **Show the decision trail, not private reasoning.** Every order links to its intent
   card: who proposed it, the inputs and their timestamps, the summary thesis, each risk
   check, the approval, and the cost in tokens. Raw model reasoning is not promised.
2. **Paper is the default; live is a ceremony.** Live needs step-up 2FA, a typed
   confirmation and per-strategy limits, and shows a LIVE badge on every screen.
3. **Honest numbers.** P&L after fees; *paper* and *delayed data* labels where true;
   every season shows its benchmark, its no-AI baseline and its walk-forward verdict
   (*validated / trade-off / no edge*).
4. **Stop controls are always reachable** (§2.4), on every screen and size, from the
   keyboard and from a TV remote.
5. **Calm by default.** A 24/7 farm must not demand attention. Alerts are rare, grouped
   and actionable.
6. **State over reassurance.** When space is tight, show mode, staleness, runner status,
   positions and stop controls before any profit number.

### 2.4 Stop controls
| Control | What it does | Confirmation |
|---|---|---|
| **Pause** | No new entries; resting stops and exits stay | One tap |
| **Cancel entries** | Cancels working entry orders; protective exits stay | One tap |
| **Flatten** | Closes positions at collared limit prices | Hold-to-confirm |

Each shows *requested → runner acknowledged → broker confirmed / failed*. When the runner
is offline the controls are disabled with the reason, the last confirmed state, and
"act in the Webull app" guidance.

---

## 3. Visual system

### 3.1 Colour tokens (add to `src/styles/theme.css` with use comments)
Brand neon red is **chrome only** (logo, active tab, focus ring with a second cue). Money
never uses brand red. Contrast measured on `--color-app` `#050508`
([Muse L3 §A11](research/loop3-muse-review.md)):

| Token | Hex | Contrast | Use |
|---|---|---|---|
| `--color-gain` | `#2EEA7B` | 12.77:1 | Positive P&L, up candles; always with ▲ / + |
| `--color-loss` | `#FF6B4A` | 7.23:1 | Negative P&L, down candles; always with ▼ / − |
| `--color-paper` | `#6EC1FF` | 10.41:1 | PAPER badge and borders |
| `--color-live` | `#E879F9` | 8.27:1 | LIVE badge (with text) |
| `--color-warn` | `#FFB224` | 11.29:1 | Stale data, budget near cap, runner offline (with ⚠ and reason) |
| `--color-grid` | `#2E2E3A` | 1.52:1 | Decorative 1px lines only; structural boundaries use `--color-muted` |

Gain and loss are close for deuteranopes, so shape is load-bearing: arrows and signs on
numbers, filled vs. hollow candles.

### 3.2 Type
- `--font-display`: the landing page's existing face, tokenised.
- `--font-numeric`: **IBM Plex Mono** with `font-variant-numeric: tabular-nums` on every
  price, P&L and quantity, so numbers never change width.
- Fluid sizes with `clamp()`.

### 3.3 The spine
Inside the app, the spine is the **right edge of the side rail**: a 1px line that decision
cards hang from. On mobile (no rail) it is the left edge of the content column. Panels
butt edge to edge with 1px boundaries on desktop; mobile panels are full-bleed with 0px
side margins.

### 3.4 Motion
- 150 ms spring on interactive elements.
- New intent cards slide in along the spine; their space is reserved first.
- Number changes roll inside a fixed-width box (tabular font).
- A fill pulses once **and** gains a "Filled" badge; state never travels by animation alone.
- `prefers-reduced-motion`: animations removed, state changes instant.
- Lists freeze their sort order while the user is hovering or focused inside them.

---

## 4. Screens

| Screen | Purpose | Key content |
|---|---|---|
| **Account** (home) | Is everything OK? | Mode badge, runner status and data freshness, positions, open orders, stop controls, equity curve, realized P&L |
| **Strategy** | One strategy | Mandate, universe, bots with status, offline policy, current season, verdict, baseline comparison |
| **Board** | Watch decisions live | Intent lane and order lane (§4.1) |
| **Chart** | TradingView-like | `lightweight-charts` (Apache-2.0, attribution kept); intents, entries, stops and exits marked on candles; paper order ticket |
| **Ledger** | Every intent, order and fill | Chain status with a link to the public proof (anchored seasons only); filters; CSV export |
| **Loops** | Monitor the AI | Per-bot last run, outcome, tokens, cost today, error streak, budget bar, pause/resume |
| **Connections** | Broker and AI | Webull sandbox/live, AI keys (stored in OS keychain), local models, runner pairing and executor lease |
| **Agents API** | External agents (Phase 4) | MCP URL, OAuth clients and scopes, API keys per strategy, `skill.md`, usage |
| **Settings / Plan** | Account and billing | Plan, entitlements, alert channel, data sources, cost preset and daily cap |

### 4.1 Board states
Intents and orders are different things and get different lanes:
- **Intent lane:** Watching → Thinking → Proposed → Approved / Vetoed / Expired.
- **Order lane:** Pending → Submitted → Acknowledged → Partially filled → Filled, with
  Rejected, Cancel pending, Cancelled, Expired and **Unknown (reconciling)** as visible states.
- **Positions:** Open → Closing → Closed.
A vetoed intent never shows a fill; a filled order is not a closed position.
Cards update in place. A card changing state never steals or drops keyboard focus.

### 4.2 Onboarding (first 5 minutes)
1. Install the desktop app, sign in with a passkey, save the recovery codes.
2. Connect the Webull sandbox (keys go to the OS keychain).
3. Add one AI API key; pick a cost preset (Budget default) and a daily cap.
4. Plant a starter strategy from a template; its season is pre-registered automatically.
5. The Board shows the first loop within one cadence.

---

## 5. Layouts per screen class

| Class | Nav | Account / Board / Chart | Stop controls | 320×320 |
|---|---|---|---|---|
| Mobile portrait | Bottom tab bar (Account, Board, Chart, Loops, More) | One column; Board lanes as chips with a vertical list | Floating button bottom-right opens the three controls | Mode, runner status, staleness, stop controls |
| Mobile landscape | Compact side rail (icons) | Two columns (chart + positions); Board lanes as a sideways list with swipe, drag, wheel and arrow buttons | In rail | Same as portrait |
| Tablet portrait | Persistent side rail with labels | 3-column grid; chart spans 2 | In rail | — |
| Tablet landscape | Side rail | 4 columns; Board lanes side by side | In rail | — |
| Desktop landscape | Side rail | Dashboard capped at `max-w-7xl`; Chart may go full width | Rail + header | — |
| Desktop portrait | Side rail | Rows: chart, Board, Ledger | Rail + header | — |
| TV | Top focus bar, D-pad | Monitor mode: large mode badge, positions, Board at 3 columns; overscan-safe margins | Dedicated remote key; Back long-press opens controls | — |
| Ultra-wide | Side rail | Beyond `max-w-7xl`: chart, Board and Loops side by side | Rail + header | — |

Fit on one screen; lists scroll internally; no horizontal scrollbar. If browser zoom or
large text forces page scrolling, that is allowed, and stop controls stay pinned.

---

## 6. Native shells

- **Desktop** (Windows, macOS first; Linux best effort): bundles `m24d` as a sidecar. Tray
  icon shows runner state with shape plus colour (running, degraded, stopped, paper).
  Closing the window keeps the runner going (tray); Quit stops it after confirmation if a
  strategy is armed. Keep-awake and start-at-login are opt-in; start-at-login opens to the
  tray **unarmed**. Updates never restart an armed runner without asking.
- **Android:** monitor and stop-control remote for a desktop or VPS runner; push alerts.
  Live approvals on the phone require step-up 2FA on the device. No runner on the phone.
- **Web:** monitor and settings; cloud-only paper mode comes later.

---

## 7. Accessibility

- WCAG 2.2 AA on every token (§3.1); tabular numbers; shape cues for gain/loss.
- Live region announces a **status summary** (fills count, last fill, runner status),
  never every tick.
- All Board actions are keyboard reachable; cards never move DOM focus.
- Focus-visible rings use brand red plus a 2px offset outline, never colour alone.
- TV: D-pad focus order follows reading order; no traps; Back always goes up one level.
- Strings: every screen with more than 10 UI strings keeps them in a `.json` beside the
  component, ready for translation.

---

## 8. Open design questions for Loop 4–5

1. Starter strategy templates that are honest enough to ship (with their baselines).
2. What the public season page looks like (anchored proof, all seasons, losers included).
3. Free vs. paid screens and limits once the business model settles (ARCHITECTURE §18).
