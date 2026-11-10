// Indicator maths for the chart. Every function takes plain arrays and returns arrays of the same
// length, with null where the indicator has no value yet (too few bars behind it).

const PERCENT = 100;
const EMA_SMOOTHING = 2;
const SECONDS_PER_DAY = 86400;
const HLC_PARTS = 3;

/** Simple moving average of the last `n` values. */
export function sma(/** @type {number[]} */ values, /** @type {number} */ n) {
  let sum = 0;
  return values.map((v, i) => {
    sum += v - (i >= n ? values[i - n] : 0);
    return i >= n - 1 ? sum / n : null;
  });
}

/** Exponential moving average, seeded with the simple average of the first `n` values. */
export function ema(/** @type {number[]} */ values, /** @type {number} */ n) {
  const k = EMA_SMOOTHING / (n + 1);
  let prev = 0;
  let seed = 0;
  return values.map((v, i) => {
    if (i < n - 1) {
      seed += v;
      return null;
    }
    prev = i === n - 1 ? (seed + v) / n : v * k + prev * (1 - k);
    return prev;
  });
}

/** Bollinger bands: the `n`-bar average and `width` standard deviations either side of it. */
export function bollinger(/** @type {number[]} */ values, /** @type {number} */ n, /** @type {number} */ width) {
  const mid = sma(values, n);
  const upper = [];
  const lower = [];
  mid.forEach((m, i) => {
    if (m === null) {
      upper.push(null);
      lower.push(null);
      return;
    }
    let squares = 0;
    for (let j = i - n + 1; j <= i; j++) squares += (values[j] - m) ** 2;
    const dev = Math.sqrt(squares / n) * width;
    upper.push(m + dev);
    lower.push(m - dev);
  });
  return { mid, upper, lower };
}

/** Relative strength index (Wilder's smoothing), 0 to 100. */
export function rsi(/** @type {number[]} */ values, /** @type {number} */ n) {
  let gain = 0;
  let loss = 0;
  return values.map((v, i) => {
    if (i === 0) return null;
    const move = v - values[i - 1];
    const up = Math.max(move, 0);
    const down = Math.max(-move, 0);
    if (i <= n) {
      gain += up / n;
      loss += down / n;
      if (i < n) return null;
    } else {
      gain = (gain * (n - 1) + up) / n;
      loss = (loss * (n - 1) + down) / n;
    }
    return loss === 0 ? PERCENT : PERCENT - PERCENT / (1 + gain / loss);
  });
}

/** MACD: fast EMA minus slow EMA, its signal line, and the gap between the two. */
export function macd(/** @type {number[]} */ values, /** @type {number} */ fast, /** @type {number} */ slow, /** @type {number} */ signalBars) {
  const quick = ema(values, fast);
  const line = ema(values, slow).map((s, i) => (s === null || quick[i] === null ? null : quick[i] - s));
  const start = line.findIndex((v) => v !== null);
  const smoothed = start < 0 ? [] : ema(/** @type {number[]} */ (line.slice(start)), signalBars);
  const signal = line.map((_, i) => (start < 0 || i < start ? null : smoothed[i - start]));
  const gap = line.map((v, i) => (v === null || signal[i] === null ? null : v - signal[i]));
  return { line, signal, gap };
}

/**
 * Volume-weighted average price, starting again each day.
 * @param {{time:number, high:number, low:number, close:number, volume:number}[]} bars time in seconds
 */
export function vwap(bars) {
  let day = -1;
  let traded = 0;
  let volume = 0;
  return bars.map((b) => {
    const today = Math.floor(b.time / SECONDS_PER_DAY);
    if (today !== day) {
      day = today;
      traded = 0;
      volume = 0;
    }
    traded += ((b.high + b.low + b.close) / HLC_PARTS) * b.volume;
    volume += b.volume;
    return volume > 0 ? traded / volume : null;
  });
}
