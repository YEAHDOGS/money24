// The desk's activity for the Home page: the desk's journal turned into rows a person can read.
// What was bought, what was sold and for how much, what was refused and why, newest first; and
// how long until a moment (the next scan, the next open). Pure functions: Desk.svelte and
// DayStrip.svelte only draw.
import strings from './strings.json';
import { fill, price, units, usd } from '../plays/play-format.js';

const ACTION_FILL = 'fill';
const ACTION_CLOSE = 'close';
const ACTION_VOID = 'void';
const KIND_ENTRY = 'entry';
const PLAYS_SOURCE = 'plays'; // a play's journal lines carry the play's id as their post id
const EXPIRY_PARTS = /^\d{4}-(\d{2})-(\d{2})$/;
const TIMES = '×';
const MS_PER_MINUTE = 60000;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;
/** What a row is, for its mark: money going in, a gain, a loss, or nothing traded. */
export const TONE = { buy: 'buy', win: 'win', loss: 'loss', skip: 'skip' };

/**
 * @typedef {{ id: number, at: string, tone: string, title: string, detail: string, pnl: number | null, playId: string }} Activity
 * pnl: dollars made or lost on this line, null when the line moved no money out.
 */

/** "BTC-USD", or "DAL 80P 11/27" for an option. */
export function contractName(/** @type {any} */ signal) {
  const symbol = signal?.symbol ?? '';
  if (!signal?.right) return symbol;
  const date = EXPIRY_PARTS.exec(signal.expiry ?? '');
  return `${symbol} ${signal.strike}${signal.right}${date ? ` ${Number(date[1])}/${Number(date[2])}` : ''}`;
}

/** "0.00274 × $91,240.00" */
function sized(/** @type {number} */ qty, /** @type {number} */ at) {
  return `${units(qty)} ${TIMES} $${price(at)}`;
}

/**
 * One journal line as a row, or null when it is not something the desk did (noise, a source's
 * own exit post).
 * @param {any} line
 * @returns {Activity | null}
 */
function activityRow(line) {
  const name = contractName(line.signal);
  const base = { id: line.id, at: line.at, pnl: null, playId: line.signal?.source_id === PLAYS_SOURCE ? line.signal.post_id ?? '' : '' };
  const reason = strings.desk.reasons[line.result?.reason] ?? line.result?.reason ?? '';
  if (line.plan) {
    return { ...base, tone: TONE.buy, title: fill(strings.desk.bought, { name }), detail: `${sized(line.plan.qty, line.plan.entry)} · ${usd(line.plan.cost_usd)}` };
  }
  if (line.action === ACTION_VOID) return { ...base, tone: TONE.skip, title: fill(strings.desk.voided, { name }), detail: line.note ?? '' };
  if ((line.action === ACTION_FILL || line.action === ACTION_CLOSE) && line.result) {
    const won = line.result.pnl_usd >= 0;
    const title = fill(line.action === ACTION_FILL ? strings.desk.sold : strings.desk.closed, { name, reason });
    return { ...base, tone: won ? TONE.win : TONE.loss, title, detail: sized(line.result.qty, line.result.price), pnl: line.result.pnl_usd };
  }
  if (line.rejected && line.signal?.kind === KIND_ENTRY) {
    return { ...base, tone: TONE.skip, title: fill(strings.desk.refused, { name }), detail: line.rejected };
  }
  return null;
}

/**
 * The newest things the desk did.
 * @param {any[]} journal journal lines, oldest first  @param {number} limit the most rows
 * @returns {Activity[]}
 */
export function activity(journal, limit) {
  /** @type {Activity[]} */
  const rows = [];
  for (let i = journal.length - 1; i >= 0 && rows.length < limit; i--) {
    const row = activityRow(journal[i]);
    if (row) rows.push(row);
  }
  return rows;
}

/**
 * How long until a moment: "12m", "3h 12m", "1d 17h". Empty when it is not ahead or not a time.
 * @param {string | undefined} iso  @param {number} now epoch ms
 */
export function until(iso, now) {
  const minutes = Math.ceil((Date.parse(iso ?? '') - now) / MS_PER_MINUTE);
  if (!Number.isFinite(minutes) || minutes <= 0) return '';
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  if (hours < HOURS_PER_DAY) return `${hours}h ${minutes % MINUTES_PER_HOUR}m`;
  return `${Math.floor(hours / HOURS_PER_DAY)}d ${hours % HOURS_PER_DAY}h`;
}
