// One shape for the Home page's news, whichever way it arrives. The news service's stories
// (GET /api/stories: grouped coverage with an angle, attention and linked tickers) are used when
// they exist; otherwise plain headlines (GET /api/events) stand in, with the post's kind as its
// category. Labels come from the news page's own files, so both pages always say the same words.
import storyStrings from '../feed/stories.json';
import feedStrings from '../feed/strings.json';
import { NEWS_PAGE, STORIES_HASH, pageLink } from './home-links.js';

export const MODE_STORIES = 'stories';
export const MODE_EVENTS = 'events';
export const CATEGORY_ALL = 'all';
const TICKERS_MAX = 4; // tickers shown on a slide or a row

/** @typedef {{ symbol: string, linked: boolean, movePct: number | null }} Ticker linked: a real market symbol, so it can open on Markets */
/**
 * @typedef {{ id: string, title: string, category: string, at: string, href: string, external: boolean,
 *   source: string, tickers: Ticker[], angle: string, outlets: number, attention: number | null,
 *   trend: string }} Headline
 * angle, trend: keys of stories.json "angles" and "trends", empty on plain headlines.
 */
/** @typedef {{ id: string, count: number }} Category */
/** @typedef {{ mode: string, headlines: Headline[], categories: Category[] }} News */

/** @type {News} before anything has loaded */
export const NO_NEWS = { mode: MODE_EVENTS, headlines: [], categories: [] };

/** A category's display name: a story category or, on plain headlines, the post's kind. */
export function categoryLabel(/** @type {string} */ mode, /** @type {string} */ id) {
  const labels = mode === MODE_STORIES ? storyStrings.categories : feedStrings.kinds;
  return labels[id] ?? id;
}

/** @returns {Headline} */
function fromStory(/** @type {any} */ s) {
  const links = (s.links ?? []).map((/** @type {any} */ l) => ({ symbol: l.symbol, linked: true, movePct: Number.isFinite(l.move_pct) ? l.move_pct : null }));
  const tags = (s.tags ?? []).map((/** @type {string} */ symbol) => ({ symbol, linked: false, movePct: null }));
  return {
    id: `story-${s.id}`,
    title: s.title,
    category: s.category ?? '',
    at: s.last_seen ?? s.first_seen ?? '',
    href: pageLink(NEWS_PAGE, {}, STORIES_HASH),
    external: false,
    source: '',
    tickers: (links.length ? links : tags).slice(0, TICKERS_MAX),
    angle: s.angle?.label ?? '',
    outlets: s.outlets ?? 0,
    attention: Number.isFinite(s.attention?.score) ? Math.round(s.attention.score) : null,
    trend: s.attention?.trend ?? '',
  };
}

/** @returns {Headline} */
function fromEvent(/** @type {any} */ p) {
  return {
    id: `post-${p.id}`,
    title: p.title,
    category: p.kind ?? '',
    at: p.published_at ?? p.seen_at ?? '',
    href: p.url,
    external: true,
    source: p.source ?? '',
    tickers: (p.tags ?? []).slice(0, TICKERS_MAX).map((/** @type {string} */ symbol) => ({ symbol, linked: false, movePct: null })),
    angle: '',
    outlets: 0,
    attention: null,
    trend: '',
  };
}

/** How many headlines each category holds, biggest first. @returns {Category[]} */
function countCategories(/** @type {Headline[]} */ headlines) {
  /** @type {Record<string, number>} */
  const counts = {};
  for (const h of headlines) counts[h.category] = (counts[h.category] ?? 0) + 1;
  return Object.entries(counts).map(([id, count]) => ({ id, count })).sort((a, b) => b.count - a.count);
}

/**
 * The answer of GET /api/stories. Stories arrive sorted by attention, highest first, and stay so.
 * @param {any} body
 * @param {boolean} [askable] the service can be asked for a category's own stories, so its counts
 *   of every story on record hold. A recording cannot: its categories count what was recorded.
 * @returns {News}
 */
export function newsFromStories(body, askable = true) {
  const headlines = (body.stories ?? []).map(fromStory);
  const served = (body.categories ?? []).filter((/** @type {any} */ c) => c.stories > 0).map((/** @type {any} */ c) => ({ id: c.id, count: c.stories }));
  return { mode: MODE_STORIES, headlines, categories: askable && served.length ? served : countCategories(headlines) };
}

/**
 * The answer of GET /api/events, newest first by publish time.
 * @param {any[]} posts
 * @returns {News}
 */
export function newsFromEvents(posts) {
  const headlines = posts.map(fromEvent).sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
  return { mode: MODE_EVENTS, headlines, categories: countCategories(headlines) };
}
