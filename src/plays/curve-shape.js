// Turns a play's replay curve (engine/plays/record.go) into what the chart draws: the price line,
// the stop as it climbs, the entry, exit and stop levels, and a dot for everything the play did.
// Pure arithmetic, so PlayRecord.svelte only draws.

const WIDTH = 600;
const HEIGHT = 180;
const PAD = 12; // room above and below the highest and lowest price, in chart units
const LEVEL_REACH = 2.5; // an exit further than this many times the price range away is left off, or it would flatten the line
const DECIMALS = 1;
const MIN_RANGE_OF_ENTRY = 0.1; // a flat line still gets this much of the entry price as its height

/**
 * @param {any} curve
 * @returns {{ width: number, height: number, top: number, bottom: number, priceLine: string, stopLine: string,
 *   levels: { kind: string, y: number }[], events: { kind: string, x: number, y: number, price: number }[] }}
 */
export function curveShape(curve) {
  // The entry opens the line: the first candle only comes after it.
  const series = [{ t: curve.from, price: curve.entry, stop: curve.stops[0] }, ...curve.points];
  const prices = series.map((p) => p.price);
  const low = Math.min(...prices, curve.stops[0]);
  const high = Math.max(...prices, curve.entry);
  const range = Math.max(high - low, curve.entry * MIN_RANGE_OF_ENTRY);
  const near = (/** @type {number} */ v) => v > 0 && v <= high + range * LEVEL_REACH;
  const top = Math.max(high, ...curve.exits.filter(near));
  const bottom = low;
  const span = top - bottom || 1;
  const x = (/** @type {number} */ i) => (series.length > 1 ? (i / (series.length - 1)) * WIDTH : WIDTH / 2);
  const y = (/** @type {number} */ v) => PAD + (1 - (v - bottom) / span) * (HEIGHT - PAD * 2);
  const point = (/** @type {number} */ px, /** @type {number} */ py) => `${px.toFixed(DECIMALS)},${py.toFixed(DECIMALS)}`;
  /** Where on the line a moment in time falls: the last point at or before it. */
  const indexAt = (/** @type {number} */ t) => series.reduce((at, p, i) => (p.t <= t ? i : at), 0);

  return {
    width: WIDTH,
    height: HEIGHT,
    top,
    bottom,
    priceLine: series.map((p, i) => point(x(i), y(p.price))).join(' '),
    stopLine: series.map((p, i) => point(x(i), y(p.stop))).join(' '),
    levels: [
      { kind: 'entry', y: y(curve.entry) },
      ...curve.exits.filter(near).map((/** @type {number} */ v) => ({ kind: 'exit', y: y(v) })),
    ],
    events: curve.events.map((/** @type {any} */ e) => ({ kind: e.kind, x: x(indexAt(e.t)), y: y(e.price), price: e.price })),
  };
}
