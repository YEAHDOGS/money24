// A symbol's levels, worked out from its daily candles alone: where price has turned before (support and
// resistance), where the last session says the next one pivots, how far a normal day reaches, and which side
// of those lines the symbol stands on. It is a map, not a forecast: each line says what crossing it would mean.
// Levels the news outlets publish are not in here yet (they need the news service); see docs/PLAYS.md.

const ATR_DAYS = 14;
const SWING_BARS = 3; // a swing high is a high above the three bars either side of it
const LOOKBACK_BARS = 126; // about six months of sessions
const CLUSTER_ATR = 0.6; // turns closer than this many ATRs are one level
const PER_SIDE = 3; // levels kept above and below the price
const FAST_DAYS = 20;
const SLOW_DAYS = 50;
const MOMENTUM_DAYS = 5;
const MIN_BARS = 20; // fewer than this and there is nothing to read
const BULL_SCORE = 2;
const BEAR_SCORE = -2;
const MARK_LIMIT = 2; // levels per side drawn on the chart

/** @typedef {{t:number, o:number, h:number, l:number, c:number, v:number}} Candle */
/** @typedef {{ price: number, touches: number, pct: number }} Level */
/** @typedef {'bullish' | 'neutral' | 'bearish'} Bias */

const mean = (/** @type {number[]} */ xs) => xs.reduce((a, b) => a + b, 0) / xs.length;

/** Average true range: how far a normal session reaches, in price. */
function atr(/** @type {Candle[]} */ bars) {
  const ranges = bars.slice(-ATR_DAYS).map((b, i, all) => {
    const prev = i ? all[i - 1].c : (bars[bars.length - all.length - 1]?.c ?? b.o);
    return Math.max(b.h - b.l, Math.abs(b.h - prev), Math.abs(b.l - prev));
  });
  return mean(ranges);
}

/** Swing highs and lows: bars that stand clear of the SWING_BARS either side. */
function swings(/** @type {Candle[]} */ bars) {
  /** @type {number[]} */
  const turns = [];
  for (let i = SWING_BARS; i < bars.length - SWING_BARS; i++) {
    const around = bars.slice(i - SWING_BARS, i + SWING_BARS + 1).filter((_, k) => k !== SWING_BARS);
    if (around.every((b) => b.h < bars[i].h)) turns.push(bars[i].h);
    if (around.every((b) => b.l > bars[i].l)) turns.push(bars[i].l);
  }
  return turns;
}

/** Turns near each other become one level, priced at their average and ranked by how often price turned there. */
function cluster(/** @type {number[]} */ turns, /** @type {number} */ gap) {
  /** @type {number[][]} */
  const groups = [];
  for (const price of [...turns].sort((a, b) => a - b)) {
    const group = groups[groups.length - 1];
    if (group && price - mean(group) <= gap) group.push(price);
    else groups.push([price]);
  }
  return groups.map((g) => ({ price: mean(g), touches: g.length }));
}

const sma = (/** @type {Candle[]} */ bars, /** @type {number} */ n) => mean(bars.slice(-n).map((b) => b.c));

/**
 * @param {Candle[]} candles daily candles, oldest first
 * @param {number} [live] the live price, when newer than the last close
 * @returns {null | { price: number, atr: number, expected: {low:number, high:number}, resistance: Level[], support: Level[],
 *   positive: number | null, negative: number | null, pivots: {p:number, r1:number, s1:number}, yearHigh: number, yearLow: number,
 *   bias: Bias, score: number, reasons: string[], marks: {price:number, title:string, tone:string}[] }}
 */
export function levelsFor(candles, live) {
  const bars = (candles ?? []).filter((b) => b.h >= b.l && b.c > 0).slice(-LOOKBACK_BARS);
  if (bars.length < MIN_BARS) return null;
  const last = bars[bars.length - 1];
  const price = live && live > 0 ? live : last.c;
  const range = atr(bars);
  const found = cluster(swings(bars), range * CLUSTER_ATR).map((l) => ({ ...l, pct: ((l.price - price) / price) * 100 }));
  const resistance = found.filter((l) => l.price > price).sort((a, b) => a.price - b.price).slice(0, PER_SIDE);
  const support = found.filter((l) => l.price <= price).sort((a, b) => b.price - a.price).slice(0, PER_SIDE);

  const p = (last.h + last.l + last.c) / 3;
  const fast = sma(bars, FAST_DAYS);
  const slow = bars.length >= SLOW_DAYS ? sma(bars, SLOW_DAYS) : null;
  const before = bars[Math.max(0, bars.length - 1 - MOMENTUM_DAYS)].c;
  const checks = [
    [price > fast, `Above its ${FAST_DAYS}-day average`, `Below its ${FAST_DAYS}-day average`],
    ...(slow === null ? [] : [[price > slow, `Above its ${SLOW_DAYS}-day average`, `Below its ${SLOW_DAYS}-day average`], [fast > slow, `${FAST_DAYS}-day average above the ${SLOW_DAYS}-day`, `${FAST_DAYS}-day average below the ${SLOW_DAYS}-day`]]),
    [price > before, `Up over the last ${MOMENTUM_DAYS} sessions`, `Down over the last ${MOMENTUM_DAYS} sessions`],
  ];
  // Each check is one point up or down.
  const score = checks.reduce((n, [up]) => n + (up ? 1 : -1), 0);
  /** @type {Bias} */
  const bias = score >= BULL_SCORE ? 'bullish' : score <= BEAR_SCORE ? 'bearish' : 'neutral';
  const pivots = { p, r1: 2 * p - last.l, s1: 2 * p - last.h };
  // The nearest line each side, counting the pivot's: a close past it is the first sign the day turned.
  const above = [...resistance.map((l) => l.price), ...(pivots.r1 > price ? [pivots.r1] : [])];
  const below = [...support.map((l) => l.price), ...(pivots.s1 < price ? [pivots.s1] : [])];

  const marks = [
    ...resistance.slice(0, MARK_LIMIT).map((l, i) => ({ price: l.price, title: `R${i + 1}`, tone: 'down' })),
    ...support.slice(0, MARK_LIMIT).map((l, i) => ({ price: l.price, title: `S${i + 1}`, tone: 'up' })),
  ];
  return {
    price,
    atr: range,
    expected: { low: price - range, high: price + range },
    resistance,
    support,
    positive: above.length ? Math.min(...above) : null,
    negative: below.length ? Math.max(...below) : null,
    pivots,
    yearHigh: Math.max(...bars.map((b) => b.h)),
    yearLow: Math.min(...bars.map((b) => b.l)),
    bias,
    score,
    reasons: checks.map(([up, yes, no]) => (up ? yes : no)),
    marks,
  };
}
