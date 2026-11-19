// Display helpers for stories: colors per angle and reading, country names, the attention
// sparkline and the numbers beside an instrument.
import { DEMO_PARAM } from '../lib/demo.js';

const REGION_NAMES = new Intl.DisplayNames(['en'], { type: 'region' });
const PERCENT_DECIMALS = 2;
const VOLUME_DECIMALS = 1;
const MILLION = 1e6;
const THOUSAND = 1e3;
const SCORE_MAX = 100;
const MARKETS_PAGE = 'markets.html';
const SYMBOL_PARAM = 's';

/** Full class strings so Tailwind can see them. One entry per angle in stories.json. */
export const ANGLE_CLASSES = {
  bullish: { text: 'text-ok', border: 'border-ok/40', bg: 'bg-ok' },
  bearish: { text: 'text-bad', border: 'border-bad/40', bg: 'bg-bad' },
  mixed: { text: 'text-warn', border: 'border-warn/40', bg: 'bg-warn' },
  unclear: { text: 'text-muted', border: 'border-line', bg: 'bg-muted' },
};
/** One entry per reading in stories.json. Early is the one worth a look, so it is the loud one. */
export const READING_CLASSES = {
  early: 'border-neon-red bg-neon-red text-app',
  confirmed: 'border-ok/50 text-ok',
  exhausted: 'border-warn/50 text-warn',
  quiet: 'border-line text-muted',
  none: 'border-line text-muted',
};
export const TREND_CLASSES = { rising: 'text-ok', steady: 'text-muted', fading: 'text-bad' };

/** A country's English name from its two-letter id; the id itself when the browser has none. */
export function placeName(/** @type {string} */ id) {
  try {
    return REGION_NAMES.of(id) ?? id;
  } catch {
    return id;
  }
}

/** "+2.10%" or "-0.54%". */
export function signedPct(/** @type {number} */ pct) {
  return `${pct > 0 ? '+' : ''}${pct.toFixed(PERCENT_DECIMALS)}%`;
}

/** Volume against the usual pace: "1.8×". */
export function pace(/** @type {number} */ rel) {
  return `${rel.toFixed(VOLUME_DECIMALS)}×`;
}

/** Dollars traded on a prediction market: "$1.2M", "$48K", "$310". */
export function shortDollars(/** @type {number} */ usd) {
  if (usd >= MILLION) return `$${(usd / MILLION).toFixed(1)}M`;
  if (usd >= THOUSAND) return `$${Math.round(usd / THOUSAND)}K`;
  return `$${Math.round(usd)}`;
}

/** A link to the markets page for one instrument, staying in the demo when this page is in it. */
export function marketHref(/** @type {string} */ symbol) {
  const link = new URLSearchParams({ [SYMBOL_PARAM]: symbol });
  if (new URLSearchParams(location.search).has(DEMO_PARAM)) link.set(DEMO_PARAM, '');
  return `${MARKETS_PAGE}?${link}`;
}

/**
 * An SVG path through attention scores (0 to 100), oldest left. One reading draws a flat line.
 * @param {number[]} scores @param {number} width @param {number} height
 */
export function sparkPath(scores, width, height) {
  if (!scores.length) return '';
  const points = scores.length === 1 ? [scores[0], scores[0]] : scores;
  const step = width / (points.length - 1);
  return points.map((score, i) => `${i ? 'L' : 'M'}${(i * step).toFixed(1)} ${(height - (Math.min(score, SCORE_MAX) / SCORE_MAX) * height).toFixed(1)}`).join('');
}
