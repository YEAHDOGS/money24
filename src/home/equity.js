// The paper account over time, worked out in the browser from the runner's paper journal: starting
// paper capital plus every realized gain or loss, in the order they happened. Pure arithmetic, so
// WalletChart.svelte only draws. Nothing here is real money, and nothing is invented: with few
// closed trades the line is flat, because the account was.
//
// Journal lines (engine/signals/journal.go, score.go): a "fill" line carries one rung's gain; a
// "close" line carries the whole trade, earlier fills included. So a close adds only what its
// fills had not banked yet, and the curve ends on the same total the runner's scoreboard shows.

const ACTION_FILL = 'fill';
const ACTION_CLOSE = 'close';
const MS_PER_DAY = 86400000;
const PERCENT = 100;
export const RANGE_ALL = 'all';
/** The range buttons, left to right. days 0: everything on record. */
export const RANGES = [
  { id: '1w', days: 7 },
  { id: '1m', days: 30 },
  { id: '6m', days: 182 },
  { id: '1y', days: 365 },
  { id: RANGE_ALL, days: 0 },
];
export const DEFAULT_RANGE = '1m';
const EMPTY_SPAN_DAYS = 30; // "All" with nothing on record still draws a month
const FLAT_PAD = 0.01; // a flat line gets this share of its value above and below, so it sits mid-chart
const SCALE_PAD = 0.15; // room above the highest and below the lowest point, as a share of the spread

/** @typedef {{ t: number, pnl: number, closed: boolean }} Step one realized gain or loss; t is epoch ms */
/** @typedef {{ t: number, v: number }} Point equity v at time t */
/** @typedef {{ points: Point[], start: number, end: number, change: number, changePct: number, drawdownPct: number, trades: number }} Curve */

/**
 * Every realized gain or loss in the journal, oldest first.
 * @param {any[]} journal journal lines, in any order
 * @returns {Step[]}
 */
export function realizedSteps(journal) {
  /** @type {Map<number, number>} plan id -> what its fills have banked so far */
  const banked = new Map();
  /** @type {Step[]} */
  const steps = [];
  for (const line of [...journal].sort((a, b) => a.id - b.id)) {
    const pnl = line.result?.pnl_usd;
    const t = Date.parse(line.at);
    if (!Number.isFinite(pnl) || !Number.isFinite(t)) continue;
    if (line.action === ACTION_FILL) {
      banked.set(line.plan_id, (banked.get(line.plan_id) ?? 0) + pnl);
      steps.push({ t, pnl, closed: false });
      continue;
    }
    if (line.action !== ACTION_CLOSE) continue;
    steps.push({ t, pnl: pnl - (banked.get(line.plan_id) ?? 0), closed: true });
    banked.delete(line.plan_id);
  }
  return steps.sort((a, b) => a.t - b.t);
}

/**
 * Where a range starts. "All" starts at the first thing on record.
 * @param {string} id a RANGES id  @param {number} first epoch ms of the oldest journal line, NaN when there is none
 * @param {number} now epoch ms
 */
export function rangeStart(id, first, now) {
  const days = RANGES.find((r) => r.id === id)?.days ?? 0;
  if (days) return now - days * MS_PER_DAY;
  return Number.isFinite(first) && first < now ? first : now - EMPTY_SPAN_DAYS * MS_PER_DAY;
}

/**
 * The equity curve between two moments, with what the stat row shows. Drawdown is the deepest
 * fall from a peak inside the range.
 * @param {Step[]} steps oldest first  @param {number} capital starting paper capital
 * @param {number} from epoch ms  @param {number} to epoch ms
 * @returns {Curve}
 */
export function equityCurve(steps, capital, from, to) {
  let equity = capital;
  for (const s of steps) if (s.t < from) equity += s.pnl;
  const start = equity;
  /** @type {Point[]} */
  const points = [{ t: from, v: start }];
  let peak = start;
  let drawdownPct = 0;
  let trades = 0;
  for (const s of steps) {
    if (s.t < from || s.t > to) continue;
    equity += s.pnl;
    points.push({ t: s.t, v: equity });
    peak = Math.max(peak, equity);
    if (peak > 0) drawdownPct = Math.max(drawdownPct, ((peak - equity) / peak) * PERCENT);
    if (s.closed) trades += 1;
  }
  points.push({ t: to, v: equity });
  const change = equity - start;
  return { points, start, end: equity, change, changePct: start ? (change / start) * PERCENT : 0, drawdownPct, trades };
}

/** The account's value at a moment: the last point at or before it. */
export function valueAt(/** @type {Point[]} */ points, /** @type {number} */ t) {
  let v = points[0]?.v ?? 0;
  for (const p of points) {
    if (p.t > t) break;
    v = p.v;
  }
  return v;
}

/**
 * The curve as SVG paths inside a box. Realized equity is a step: it holds, then jumps when a
 * trade pays or loses, so the line runs flat and then straight up or down.
 * @param {Point[]} points  @param {number} width  @param {number} height
 * @returns {{ line: string, area: string, lo: number, hi: number, x: (t: number) => number, y: (v: number) => number, t: (x: number) => number }}
 */
export function chartShape(points, width, height) {
  const values = points.map((p) => p.v);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const pad = max > min ? (max - min) * SCALE_PAD : Math.max(Math.abs(max) * FLAT_PAD, FLAT_PAD);
  const lo = min - pad;
  const hi = max + pad;
  const from = points[0].t;
  const span = Math.max(1, points[points.length - 1].t - from);
  const x = (/** @type {number} */ t) => ((t - from) / span) * width;
  const y = (/** @type {number} */ v) => height - ((v - lo) / (hi - lo)) * height;
  const t = (/** @type {number} */ px) => from + (Math.min(Math.max(px, 0), width) / Math.max(1, width)) * span;
  const round = (/** @type {number} */ n) => n.toFixed(1);
  let line = `M${round(x(points[0].t))},${round(y(points[0].v))}`;
  for (const p of points.slice(1)) line += `H${round(x(p.t))}V${round(y(p.v))}`;
  return { line, area: `${line}V${round(height)}H0Z`, lo, hi, x, y, t };
}
