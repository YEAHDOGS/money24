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

// Stretch: how far a symbol has run from where it normally sits. Three gauges, each 0 to 1 at its ceiling.
const RSI_DAYS = 14;
const RSI_NEUTRAL = 50;
const RSI_SPAN = 25; // RSI 75 or 25 is a full gauge
const RUN_DAYS = 20;
const RUN_FULL_PCT = 30; // a 30% move in a month is a full gauge
const EXTENSION_FULL_ATR = 4; // four ATRs from the 20-day average is a full gauge
const STRETCH_ON = 60; // score at which a symbol counts as stretched
const PERCENT = 100;

// Three outcomes (up, flat, down) over a horizon, from a normal distribution of moves. A tilt shifts it:
// the trend lean pushes with the move, a stretched symbol pulls back against it. Starting weights, tuned on paper.
const SIGMA_OF_ATR = 0.8; // one day's close-to-close spread as a share of the average range
const FLAT_BAND_SIGMA = 0.5; // flat is within this many spreads either side of where it is now
const LEAN_TILT = 0.05; // spreads of tilt per point of lean score
const SNAPBACK_TILT = 0.35; // spreads of pull-back at a fully stretched symbol
const HORIZONS = { day: 1, week: 5 };

/** The normal curve's cumulative distribution (Abramowitz and Stegun 7.1.26). */
function phi(/** @type {number} */ x) {
  const t = 1 / (1 + 0.3275911 * Math.abs(x) / Math.SQRT2);
  const poly = t * (0.254829592 + t * (-0.284496736 + t * (1.421413741 + t * (-1.453152027 + t * 1.061405429))));
  const erf = 1 - poly * Math.exp(-(x * x) / 2);
  return 0.5 * (1 + (x < 0 ? -erf : erf));
}

/** Wilder's relative strength index of the closes. */
function rsi(/** @type {Candle[]} */ bars) {
  const closes = bars.map((b) => b.c);
  let gain = 0;
  let loss = 0;
  for (let i = 1; i <= RSI_DAYS; i++) {
    const d = closes[i] - closes[i - 1];
    if (d > 0) gain += d;
    else loss -= d;
  }
  gain /= RSI_DAYS;
  loss /= RSI_DAYS;
  for (let i = RSI_DAYS + 1; i < closes.length; i++) {
    const d = closes[i] - closes[i - 1];
    gain = (gain * (RSI_DAYS - 1) + Math.max(d, 0)) / RSI_DAYS;
    loss = (loss * (RSI_DAYS - 1) + Math.max(-d, 0)) / RSI_DAYS;
  }
  return loss === 0 ? PERCENT : PERCENT - PERCENT / (1 + gain / loss);
}

const unit = (/** @type {number} */ x) => Math.max(0, Math.min(1, x));

/** Up, flat and down shares (each 0 to 1, summing to 1) over `days` sessions, given a tilt in spreads. */
function outcomes(/** @type {number} */ sigma, /** @type {number} */ days, /** @type {number} */ tilt) {
  // Flat stays the same size in price, so it is a smaller share of a longer horizon's wider spread.
  const band = FLAT_BAND_SIGMA / Math.sqrt(days);
  const down = phi(-band - tilt);
  const up = 1 - phi(band - tilt);
  return { up, flat: Math.max(0, 1 - up - down), down, spread: sigma * Math.sqrt(days) };
}

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
 *   bias: Bias, score: number, stretch: {score:number, direction:'up'|'down'|'none', runPct:number, extension:number, rsi:number},
 *   odds: Record<string, {up:number, flat:number, down:number, spread:number}>, reasons: string[], marks: {price:number, title:string, tone:string}[] }}
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

  // Stretch: the run, the distance from the average and RSI, each against its ceiling, in the direction of the run.
  const monthAgo = bars[Math.max(0, bars.length - 1 - RUN_DAYS)].c;
  const runPct = ((price - monthAgo) / monthAgo) * PERCENT;
  const extension = (price - fast) / range;
  const strength = rsi(bars);
  const direction = extension >= 0 ? 'up' : 'down';
  const sign = direction === 'up' ? 1 : -1;
  const gauges = [unit(Math.abs(runPct) / RUN_FULL_PCT), unit(Math.abs(extension) / EXTENSION_FULL_ATR), unit(((strength - RSI_NEUTRAL) * sign) / RSI_SPAN)];
  const stretchScore = Math.round(mean(gauges) * PERCENT);
  const stretched = stretchScore >= STRETCH_ON;
  const stretch = { score: stretchScore, direction: stretched ? direction : 'none', runPct, extension, rsi: strength };

  // The odds: the trend's lean pushes with the move, a stretched symbol pulls back against it.
  const sigma = (SIGMA_OF_ATR * range) / price;
  const tilt = score * LEAN_TILT - (stretched ? sign * (stretchScore / PERCENT) * SNAPBACK_TILT : 0);
  const odds = Object.fromEntries(Object.entries(HORIZONS).map(([name, days]) => [name, outcomes(sigma, days, tilt)]));

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
    stretch,
    odds,
    reasons: checks.map(([up, yes, no]) => (up ? yes : no)),
    marks,
  };
}
