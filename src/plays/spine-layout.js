// Places a play's levels on one vertical price scale for the spine: highest stock price at the
// top. Nodes too close to read are pushed apart, then the whole set is squeezed back into range.

const PAD = 0.06;       // scale margin above the top price and below the bottom one
const MIN_GAP = 11;     // percent of the spine between two labels
const FULL = 100;
const LEVEL_STOP = 'stop';
export const NODE_SPOT = 'spot';
export const NODE_DECAY = 'decay';

/**
 * @typedef {{ key: string, name: string, stock: number, level?: any, y: number }} SpineNode
 * @param {any} report  @param {any} play
 * @returns {SpineNode[]} sorted top to bottom, y in 0..100
 */
export function spineLayout(report, play) {
  const spot = play.market?.spot ?? 0;
  /** @type {SpineNode[]} */
  const nodes = [];
  if (spot > 0) nodes.push({ key: NODE_SPOT, name: NODE_SPOT, stock: spot, y: 0 });
  for (const l of report?.levels ?? []) {
    if (!l.reachable) continue;
    // The stop triggers on today's price; where decay alone takes it by the exit is a second marker.
    const stock = l.name === LEVEL_STOP ? l.spot_now : l.spot_by_exit;
    if (!(stock > 0)) continue;
    nodes.push({ key: l.name, name: l.name, stock, level: l, y: 0 });
    if (l.name === LEVEL_STOP && l.already_past && l.spot_by_exit > 0) {
      nodes.push({ key: NODE_DECAY, name: NODE_DECAY, stock: l.spot_by_exit, level: l, y: 0 });
    }
  }
  if (nodes.length === 0) return nodes;

  const prices = nodes.map((n) => n.stock);
  const hi = Math.max(...prices), lo = Math.min(...prices);
  const span = hi - lo || hi * PAD || 1;
  const top = hi + span * PAD, bottom = lo - span * PAD;
  nodes.sort((a, b) => b.stock - a.stock);
  for (const n of nodes) n.y = ((top - n.stock) / (top - bottom)) * FULL;

  for (let i = 1; i < nodes.length; i++) {
    nodes[i].y = Math.max(nodes[i].y, nodes[i - 1].y + MIN_GAP);
  }
  const last = nodes[nodes.length - 1].y;
  if (last > FULL) {
    const first = nodes[0].y;
    for (const n of nodes) n.y = first + ((n.y - first) / (last - first)) * (FULL - first);
  }
  return nodes;
}
