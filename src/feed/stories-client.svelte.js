// The stories board and the world map behind it: what newsd grouped, read and scored. The feed
// client owns the live stream and calls refresh() when the engine says stories changed; this
// class only loads while one of its pages is on screen, so a hidden tab costs nothing.
// On the demo it reads the recorded board (public/demo/stories.json), the full board only when the
// Tradable switch is turned off (stories-all.json), and one file per story when it is opened.
import { loadDemo, stored } from '../lib/demo.js';

const BOARD_LIMIT = 120;
const MIN_REFRESH_MS = 20000; // stories change every half minute at most; one reload per burst
const DEMO_STORY_DIR = 'stories/';
const DEMO_STORIES = 'stories.json';
const DEMO_STORIES_ALL = 'stories-all.json';
const HTTP_NOT_FOUND = 404;
const STORE_TRADABLE = 'm24.news.tradable';

/** @typedef {{id:string, n:number}} Place */
/** @typedef {{symbol:string, relation:string, sign:number, why:string, angle:string, quoted:boolean, price:number, move_pct:number, rel_volume:number}} StoryLink */
/** @typedef {{id:number, first_seen:string, last_seen:string, title:string, category:string,
 *   angle:{label:string, score:number, bullish:number, bearish:number, terms:{term:string, side:string, n:number}[]},
 *   tags:string[], articles:number, outlets:number,
 *   attention:{score:number, last_hour:number, prev_hour:number, trend:string},
 *   validity:{score:number, independent:number, primary:boolean, known:number},
 *   from:Place[], about:Place[], links:StoryLink[],
 *   markets:{id:string, question:string, url:string, yes:number, volume:number}[],
 *   reading:{label:string, symbol:string}, spark:number[], image?:string, image_by?:string}} Story */
/** @typedef {{updated_at:string, quotes_at:string|null, categories:{id:string, stories:number}[], stories:Story[], movers:{symbol:string, move_pct:number, rel_volume:number}[]}} Board */
/** @typedef {{id:string, stories:number, articles:number, attention:number, top:{id:number, title:string, category:string, angle:string, attention:number}[]}} MapPlace */
/** @typedef {{story:Story, events:import('./feed-client.svelte.js').Post[], history:any[]}} StoryDetail */

export class StoryBoard {
  /** @type {Board | null} */
  board = $state(null);
  /** @type {{updated_at:string, from:MapPlace[], about:MapPlace[]} | null} */
  world = $state(null);
  /** @type {Record<number, StoryDetail>} a story's articles and record, loaded when it is opened */
  details = $state({});
  /** True when the news service answered but has no stories route: it is older than this page. */
  outdated = $state(false);
  /** Only stories tied to an instrument on the watchlist. Off shows everything the engine grouped. */
  tradable = $state(stored(STORE_TRADABLE, 'true') !== 'false');
  /** Set by the pages: stories only reload while one of them is on screen. */
  watching = false;

  #api = '';
  #demo = false;
  /** @type {{tradable: Board | null, all: Board | null}} the recorded boards, each read once */
  #recorded = { tradable: null, all: null };
  #loadedAt = 0;
  #stale = false;

  /** The live service to read from. @param {string} api */
  connect(api) {
    this.#api = api;
  }

  /** Read the recorded boards instead of a live service. */
  useDemo() {
    this.#demo = true;
    if (this.watching) this.load();
  }

  /** The recorded board for the Tradable switch as it stands. A missing recording reads as an old service. */
  async #loadDemo() {
    try {
      if (!this.#recorded.tradable) {
        const snap = await loadDemo(DEMO_STORIES);
        this.#recorded.tradable = snap.stories;
        this.world = snap.world ?? null;
      }
      if (!this.tradable && !this.#recorded.all) this.#recorded.all = (await loadDemo(DEMO_STORIES_ALL)).stories;
      this.board = this.tradable ? this.#recorded.tradable : this.#recorded.all;
    } catch {
      this.outdated = !this.board;
    }
  }

  /** Shows only tradable stories, or all of them, and remembers the choice. @param {boolean} on */
  setTradable(on) {
    this.tradable = on;
    try {
      localStorage.setItem(STORE_TRADABLE, String(on));
    } catch { /* storage is optional */ }
    this.#stale = true;
    this.#loadedAt = 0;
    this.load();
  }

  /** The engine said stories changed: reload now if a page is showing, else when one next shows. */
  refresh() {
    this.#stale = true;
    if (this.watching) this.load();
  }

  /** Loads the board and the map, at most once per MIN_REFRESH_MS unless nothing is loaded yet. */
  async load() {
    if (this.#demo) return this.#loadDemo();
    if (!this.#api) return;
    const fresh = Date.now() - this.#loadedAt < MIN_REFRESH_MS;
    if (this.board && (fresh || !this.#stale)) return;
    this.#loadedAt = Date.now();
    this.#stale = false;
    try {
      const [board, world] = await Promise.all([
        fetch(`${this.#api}/api/stories?limit=${BOARD_LIMIT}&tradable=${this.tradable ? 1 : 0}`),
        fetch(`${this.#api}/api/stories/map`),
      ]);
      this.outdated = board.status === HTTP_NOT_FOUND;
      if (!board.ok || !world.ok) return;
      this.board = await board.json();
      this.world = await world.json();
    } catch { /* the feed client reports the engine as offline on its own */ }
  }

  /** A story's articles and recorded numbers. A failed load leaves the story open without them. @param {number} id */
  async open(id) {
    if (this.details[id] && this.#demo) return;
    try {
      const detail = this.#demo
        ? await loadDemo(`${DEMO_STORY_DIR}${id}.json`)
        : await (await fetch(`${this.#api}/api/stories/${id}`)).json();
      if (detail?.story) this.details = { ...this.details, [id]: detail };
    } catch { /* not recorded, or the engine is away */ }
  }
}
