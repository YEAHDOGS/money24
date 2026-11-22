import strings from './strings.json';
import { stored, STORE_RUNNER } from '../lib/demo.js';

const DEFAULT_RUNNER = import.meta.env.VITE_TRADING_API || 'http://127.0.0.1:8025';
const API_PATH = '/api/scoreboard/all';
const SORT_DEFAULT = 'desk';
const SORT_VERDICT = 'verdict';
const SORT_RETURN = 'mean_return';
const SORT_LATEST = 'latest_at';
const UNIT_R = 'r';
const VERDICT_RANK = { passes: 0, promising: 1, losing: 2, 'not enough data': 3 };
const VERDICT_TONE = { passes: 'text-ok', promising: 'text-paper', losing: 'text-bad', 'not enough data': 'text-warn' };
const DATE = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeZone: 'UTC' });
const PERCENT = new Intl.NumberFormat(undefined, { style: 'percent', maximumFractionDigits: 2 });
const SIGNED_PERCENT = new Intl.NumberFormat(undefined, { style: 'percent', maximumFractionDigits: 2, signDisplay: 'exceptZero' });
const SIGNED_NUMBER = new Intl.NumberFormat(undefined, { maximumFractionDigits: 3, signDisplay: 'exceptZero' });

export { SORT_DEFAULT, VERDICT_TONE };

/** Reads only the aggregate endpoint from the configured runner. */
export async function readScoreboard(/** @type {AbortSignal} */ signal) {
  const base = stored(STORE_RUNNER, DEFAULT_RUNNER).replace(/\/+$/, '');
  const response = await fetch(base + API_PATH, { signal });
  if (!response.ok) throw new Error(String(response.status));
  const rows = await response.json();
  if (!Array.isArray(rows)) throw new Error(strings.offline);
  return rows;
}

/** Sorts without changing the snapshot. Returns in dollars and R keep their own groups. */
export function sortRows(/** @type {any[]} */ rows, /** @type {string} */ sort, /** @type {string} */ desk) {
  const filtered = rows.filter(row => !desk || row.desk === desk);
  return filtered.sort((a, b) => {
    const byName = strings.desks[a.desk]?.localeCompare(strings.desks[b.desk]) || a.name.localeCompare(b.name);
    if (sort === SORT_DEFAULT) return byName;
    if (sort === SORT_VERDICT) return VERDICT_RANK[a.verdict] - VERDICT_RANK[b.verdict] || b.settled - a.settled || byName;
    if (sort === SORT_LATEST) return (b.latest_at ?? '').localeCompare(a.latest_at ?? '') || byName;
    if (sort === SORT_RETURN && a[sort] != null && b[sort] != null && a.unit !== b.unit) {
      return a.unit.localeCompare(b.unit);
    }
    return (b[sort] ?? -Infinity) - (a[sort] ?? -Infinity) || byName;
  });
}

/** Formats a comparable numeric measure in its own unit. */
export function rate(/** @type {?number} */ value, /** @type {string} */ unit = '') {
  if (value == null) return strings.missing;
  if (unit === UNIT_R) return `${SIGNED_NUMBER.format(value)} R`;
  return (unit ? SIGNED_PERCENT : PERCENT).format(value);
}

/** Formats settlement dates consistently with the runner’s UTC results. */
export function date(/** @type {?string} */ at) {
  const time = Date.parse(at);
  if (!Number.isFinite(time)) return strings.missing;
  return DATE.format(time);
}
