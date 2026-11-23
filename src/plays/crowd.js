// Crowd room: how many of Money 24's players one play's contract can take without the crowd becoming its own
// exit liquidity. Impact is per contract, so the unit is the contract's open interest and day volume, never a
// head count. Rules from the capacity design (Wordsmith, Nov 23): a play closes to new entries when the crowd's
// contracts reach ~5% of the contract's open interest or ~1% of its day volume; one contract per player; late
// tranches fill only inside a price collar. Numbers are the design's starting values, to be tuned on paper.

export const CROWD_MAX = 10000; // the hard player cap
const OI_SHARE_MAX = 0.05; // the crowd may hold this share of a contract's open interest
const VOLUME_SHARE_MAX = 0.01; // and this share of what traded in it today
export const COLLAR_PCT = 10; // late tranches fill only within this move of the drafted entry
const FEW_COPIES = 5; // up to this many matching contracts is a basket worth building
const PERCENT = 100;

/** @typedef {'fits' | 'basket' | 'thin' | 'closed'} Room */

/**
 * @param {{ open_interest?: number, volume?: number }} market the play's contract market
 * @param {number} entry the drafted entry price per contract
 * @param {number} [crowd] players to fit
 * @returns {{ players: number, copies: number, room: Room, share: number, oiCap: number, volumeCap: number | null,
 *   collar: { low: number, high: number } }}
 */
export function crowdRoom(market, entry, crowd = CROWD_MAX) {
  const oi = Math.max(0, market?.open_interest ?? 0);
  const volume = Math.max(0, market?.volume ?? 0);
  const oiCap = Math.floor(oi * OI_SHARE_MAX);
  // Volume so far today undercounts the day, so it only ever tightens the cap once it is known.
  const volumeCap = volume > 0 ? Math.floor(volume * VOLUME_SHARE_MAX) : null;
  const players = Math.min(oiCap, volumeCap ?? oiCap);
  const copies = players > 0 ? Math.ceil(crowd / players) : Infinity;
  /** @type {Room} */
  const room = players >= crowd ? 'fits' : players === 0 ? 'closed' : copies <= FEW_COPIES ? 'basket' : 'thin';
  const band = (entry * COLLAR_PCT) / PERCENT;
  return { players, copies, room, share: Math.min(1, players / crowd), oiCap, volumeCap, collar: { low: entry - band, high: entry + band } };
}
