// The demo's trial room: what the plays page does with no runner behind it. Every action works,
// on this page only: nothing is sent anywhere and a reload brings the recording back.
// Pure functions over the recorded items, so plays-client.svelte.js only decides when to call them.

const STATUS_ARMED = 'armed';
const CHECK_FAIL = 'fail';
const EXAMPLES_MAX = 4; // as many examples as fit under the inbox on a phone without becoming a list
const SPACES = /\s+/g;

/** How a pasted line is compared: upper case, no spaces. */
const squash = (/** @type {string} */ text) => text.toUpperCase().replace(SPACES, '');

/** "AAL 13P 11/20": a contract the way people type it. */
function typed(/** @type {any} */ c) {
  if (!c?.symbol) return '';
  if (!c.right) return c.symbol;
  const [, mm, dd] = (c.expiry ?? '').split('-');
  return `${c.symbol} ${c.strike}${c.right} ${Number(mm)}/${Number(dd)}`;
}

/**
 * A draft put on paper: active from now, entered at its planned price, with the blocking rules it
 * went through on record, as the runner does for a paper trade.
 * @param {import('./plays-client.svelte.js').Item} item
 * @returns {import('./plays-client.svelte.js').Item}
 */
export function demoPaper(item) {
  const at = new Date().toISOString();
  const broke = item.report.checks.filter((/** @type {any} */ c) => c.status === CHECK_FAIL).map((/** @type {any} */ c) => c.id);
  const play = { ...item.play, status: STATUS_ARMED, armed_at: at, updated_at: at };
  if (broke.length) play.broke_rules = broke;
  const record = { ...item.report.record, actual_entry: item.report.record.expected_entry, actual_amount_usd: item.report.record.expected_amount_usd };
  return { play, report: { ...item.report, record } };
}

/**
 * The recorded play a pasted line names: its title, its contract as typed, or a link it came from.
 * @param {import('./plays-client.svelte.js').Item[]} items
 * @param {string} text
 */
export function demoFind(items, text) {
  const wanted = squash(text);
  return items.find(({ play }) => [play.title, typed(play.contract), ...(play.links ?? [])].some((name) => squash(name) === wanted));
}

/** A few lines the demo's inbox can read, from the recorded plays, newest first. @returns {string[]} */
export function demoExamples(/** @type {import('./plays-client.svelte.js').Item[]} */ items) {
  return [...new Set(items.map(({ play }) => typed(play.contract)).filter(Boolean))].slice(0, EXAMPLES_MAX);
}
