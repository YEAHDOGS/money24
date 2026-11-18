// A play's orders, in the order they happen: when it was made, its entry, each exit, the stop, the
// time exit, and how it ended. Each has a status and, once something happened, an exact time.
// Pure arithmetic over the play as the runner reports it, so PlayOrders.svelte only draws.

export const STATUS_MADE = 'made';
export const STATUS_NOT_PLACED = 'not_placed';
export const STATUS_WORKING = 'working';
export const STATUS_FILLED = 'filled';
export const STATUS_TRIGGERED = 'triggered';
export const STATUS_SCHEDULED = 'scheduled';
export const STATUS_CANCELLED = 'cancelled';
const PLAY_DRAFT = 'draft';
const PLAY_ARMED = 'armed';
const REASON_TARGET = 'target';
const REASON_STOP = 'stop';
const TIME_REASONS = ['time', 'time stop'];

/** @typedef {{ kind: string, n?: number, status: string, at?: string, price?: number, qty?: number, pnl?: number }} Order */

/**
 * @param {any} play
 * @param {number[]} stops stop-loss 1, 2 and 3 from the play's record
 * @returns {Order[]}
 */
export function orders(play, stops) {
  const fills = play.fills ?? [];
  /** What an order that has not happened yet is doing: nothing on a draft, waiting on a live play, pulled on a finished one. */
  const idle = (/** @type {string} */ live) => (play.status === PLAY_DRAFT ? STATUS_NOT_PLACED : play.status === PLAY_ARMED ? live : STATUS_CANCELLED);
  const filled = (/** @type {any} */ f, /** @type {string} */ status) => ({ status, at: f.at, price: f.price, qty: f.qty, pnl: f.pnl_usd });

  /** @type {Order[]} */
  const out = [{ kind: 'made', status: STATUS_MADE, at: play.created_at }];
  out.push(play.status === PLAY_DRAFT
    ? { kind: 'entry', status: STATUS_NOT_PLACED, price: play.entry, qty: play.qty }
    : { kind: 'entry', status: STATUS_FILLED, at: play.filled?.at ?? play.armed_at, price: play.filled?.price ?? play.entry, qty: play.filled?.qty ?? play.qty });

  const targets = [...(play.ladder ?? [])];
  if (play.runner?.qty > 0 && play.runner?.target > 0) targets.push({ qty: play.runner.qty, price: play.runner.target });
  const targetFills = fills.filter((/** @type {any} */ f) => f.reason === REASON_TARGET);
  targets.forEach((t, i) => {
    out.push(targetFills[i] ? { kind: 'exit', n: i + 1, ...filled(targetFills[i], STATUS_FILLED) } : { kind: 'exit', n: i + 1, status: idle(STATUS_WORKING), price: t.price, qty: t.qty });
  });

  // The stop in force is the one for however many exits have filled.
  const stopNow = stops[Math.min(targetFills.length, stops.length - 1)] || play.stop;
  const stopFill = fills.find((/** @type {any} */ f) => f.reason === REASON_STOP);
  out.push(stopFill ? { kind: 'stop', ...filled(stopFill, STATUS_TRIGGERED) } : { kind: 'stop', status: idle(STATUS_WORKING), price: stopNow });

  const timeFill = fills.find((/** @type {any} */ f) => TIME_REASONS.includes(f.reason));
  if (timeFill) out.push({ kind: 'time', ...filled(timeFill, STATUS_TRIGGERED) });
  else if (play.failsafes?.exit_by) out.push({ kind: 'time', status: idle(STATUS_SCHEDULED), at: play.failsafes.exit_by });

  if (play.result) out.push({ kind: 'result', status: play.result.reason, at: play.result.at, pnl: play.result.pnl_usd });
  return out;
}
