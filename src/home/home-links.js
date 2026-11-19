// Links from Home to the other app pages. A page opened with ?demo keeps its links in the demo.
import { DEMO_PARAM } from '../lib/demo.js';

export const PLAYS_PAGE = 'plays.html';
export const MARKETS_PAGE = 'markets.html';
export const NEWS_PAGE = 'feed.html';
export const WALLET_PAGE = 'wallet.html';
export const PLAY_PARAM = 'play'; // plays.html?play=<id> opens that play
export const SYMBOL_PARAM = 's'; // markets.html?s=<symbol> opens that symbol
export const STORIES_HASH = '#stories'; // the stories tab of the news page

/**
 * A link to an app page with its query and hash.
 * @param {string} page  @param {Record<string, string>} [params]  @param {string} [hash] with its "#"
 */
export function pageLink(page, params = {}, hash = '') {
  const query = new URLSearchParams(params);
  if (new URLSearchParams(location.search).has(DEMO_PARAM)) query.set(DEMO_PARAM, '');
  const text = query.toString();
  return `${page}${text ? `?${text}` : ''}${hash}`;
}
