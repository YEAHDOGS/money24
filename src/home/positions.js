// Open positions for the Home page: the plays running on the desk right now, one row each, read from
// the play as the runner reports it. The order list (src/plays/orders.js) already knows the entry,
// the stop in force and which exit is next, so this only picks from it. Pure functions:
// Positions.svelte only draws.
import { orders, STATUS_WORKING } from '../plays/orders.js';

export const STATUS_OPEN = 'armed'; // the runner's word for a play running on the desk
const KIND_ENTRY = 'entry';
const KIND_EXIT = 'exit';
const KIND_STOP = 'stop';
const MARKET_SYMBOL = /^[A-Z0-9^][A-Z0-9.=^-]{0,19}$/; // what the runner's quote route accepts (engine/market)

/** @typedef {{ pnl: number, price: number, at: number, modelled: boolean }} Mark the last known value of an open play */
/**
 * @typedef {{ id: string, title: string, symbol: string, entry: number, qty: number, cost: number,
 *   stop: number, next: { price: number, qty: number } | null, exitBy: string, banked: number,
 *   sold: boolean, mark: Mark | null }} Row
 */

/**
 * The last point of a play's curve (GET /api/plays/{id}/curve): what is banked plus what is still
 * open at the last price. Null when the curve has no prices yet.
 * @param {any} curve
 * @returns {Mark | null}
 */
export function markFrom(curve) {
  const last = curve?.points?.[curve.points.length - 1];
  if (!last || !Number.isFinite(last.pnl_usd)) return null;
  return { pnl: last.pnl_usd, price: last.price, at: last.t, modelled: Boolean(curve.modelled) };
}

/**
 * One open play as a row.
 * @param {{ play: any, report: any }} item  @param {Mark | null | undefined} mark
 * @returns {Row}
 */
export function positionRow(item, mark) {
  const { play, report } = item;
  const list = orders(play, report?.record?.stops ?? []);
  const entry = list.find((o) => o.kind === KIND_ENTRY);
  const next = list.find((o) => o.kind === KIND_EXIT && o.status === STATUS_WORKING);
  const fills = play.fills ?? [];
  const soldQty = fills.reduce((/** @type {number} */ n, /** @type {any} */ f) => n + (f.qty ?? 0), 0);
  return {
    id: play.id,
    title: play.title,
    symbol: play.contract?.symbol ?? '',
    entry: entry?.price ?? play.entry,
    qty: Math.max(0, (entry?.qty ?? play.qty) - soldQty),
    cost: report?.record?.actual_amount_usd ?? report?.payoff?.cost_usd ?? NaN,
    stop: list.find((o) => o.kind === KIND_STOP)?.price ?? play.stop,
    next: next ? { price: next.price ?? NaN, qty: next.qty ?? 0 } : null,
    exitBy: play.failsafes?.exit_by ?? '',
    banked: fills.reduce((/** @type {number} */ sum, /** @type {any} */ f) => sum + (f.pnl_usd ?? 0), 0),
    sold: fills.length > 0,
    mark: mark ?? null,
  };
}

/**
 * What the block's header adds up: what the open plays cost, and their open result where a price
 * is known. marked counts the rows that have one.
 * @param {Row[]} rows
 */
export function totals(rows) {
  const marked = rows.filter((r) => r.mark);
  return {
    cost: rows.reduce((sum, r) => sum + (Number.isFinite(r.cost) ? r.cost : 0), 0),
    open: marked.reduce((sum, r) => sum + (r.mark?.pnl ?? 0), 0),
    marked: marked.length,
  };
}

/** The symbols of open plays the ribbon can quote, without repeats. @returns {string[]} */
export function positionSymbols(/** @type {{ play: any }[]} */ items) {
  const symbols = items.filter((x) => x.play.status === STATUS_OPEN).map((x) => String(x.play.contract?.symbol ?? '').toUpperCase());
  return [...new Set(symbols)].filter((s) => MARKET_SYMBOL.test(s));
}
