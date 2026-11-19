// The stories board and the world map behind it: what newsd grouped, read and scored. The feed
// client owns the live stream and calls refresh() when the engine says stories changed; this
// class only loads while one of its pages is on screen, so a hidden tab costs nothing.
// On the demo it reads the recorded boards (public/demo/news.json) and one file per story.
import { loadDemo, stored } from '../lib/demo.js';

const BOARD_LIMIT = 120;
const MIN_REFRESH_MS = 20000; // stories change every half minute at most; one reload per burst
const DEMO_STORY_DIR = 'stories/';
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
  /** @type {{tradable: Board, all: Board} | null} the recorded boards, on the demo */
  #demo = null;
  #loadedAt = 0;
  #stale = false;

  /** The live service to read from. @param {string} api */
  connect(api) {
    this.#api = api;
  }

  /** The recorded boards and map. @param {any} snap public/demo/news.json */
  fromDemo(snap) {
    this.outdated = !snap.stories;
    if (this.outdated) return;
    this.#demo = { tradable: snap.stories, all: snap.stories_all ?? snap.stories };
    this.board = this.tradable ? this.#demo.tradable : this.#demo.all;
    this.world = snap.world ?? null;
  }

  /** Shows only tradable stories, or all of them, and remembers the choice. @param {boolean} on */
  setTradable(on) {
    this.tradable = on;
    try {
      localStorage.setItem(STORE_TRADABLE, String(on));
    } catch { /* storage is optional */ }
    if (this.#demo) {
      this.board = on ? this.#demo.tradable : this.#demo.all;
      return;
    }
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
    if (this.#demo || !this.#api) return;
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
