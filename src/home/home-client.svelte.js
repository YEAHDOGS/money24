// The Home page's data. From the m24d runner: plays (the open ones are the positions), the
// journal and starting capital (the account's history), a mark for each open play,
// quotes for the ribbon, the cash account, the trading calendar and each desk's last pass. From the newsd service: stories, or plain headlines when it has none.
// Everything is read on a slow timer and only while the page is on screen (poller.js); the runner
// caches market data, so many open windows cost the provider nothing extra.
// With no runner or news service (the public site, or ?demo) it shows the recorded snapshot
// instead (src/lib/demo.js), read once. Home only reads: nothing here changes a play or an order.
import { DEMO_PARAM, DEMO_STATUS, STORE_RUNNER, demoFirst, loadDemo, stored } from '../lib/demo.js';
import ribbon from './ribbon.json';
import { Poller } from './poller.js';
import { STATUS_OPEN, markFrom, positionRow, positionSymbols } from './positions.js';
import { CATEGORY_ALL, MODE_STORIES, NO_NEWS, newsFromEvents, newsFromStories } from './news-model.js';

const DEFAULT_RUNNER_API = 'http://127.0.0.1:8025';
const DEFAULT_NEWS_API = 'http://127.0.0.1:8024';
const DEFAULT_RUNNER = import.meta.env.VITE_TRADING_API || DEFAULT_RUNNER_API;
const NEWS_API = (import.meta.env.VITE_NEWS_API || DEFAULT_NEWS_API).replace(/\/+$/, '');
const DEFAULT_CAPITAL = 1000; // the runner's go-live capital in the default sources
const JOURNAL_LIMIT = 1000; // every line the runner keeps in memory
const STORIES_LIMIT = 60;
const EVENTS_LIMIT = 200;
const QUOTES_MAX = 30; // the most symbols the runner quotes in one request (engine/market)
const DAYS_AHEAD = 8; // today and the week after it, for the day strip
const MARKS_MAX = 8; // open plays that get a mark; the rulebook caps plays running at once below this
const SLIDES_MAX = 6; // stories in the banner
const HEADLINES_MAX = 60; // headlines in the list; the news page has the rest
const POLL_MS = 60000;
const RETRY_MS = 10000; // while a service is offline
const STATUS = { connecting: 'connecting', live: 'live', offline: 'offline', demo: DEMO_STATUS };
const DEMO_PLAYS = 'plays.json';
const DEMO_TRADING = 'trading.json';
const DEMO_NEWS = 'news.json';
const DEMO_STORIES = 'stories.json';
const DEMO_HOME = 'home.json';
const DEMO_MARKETS = 'markets/index.json';

/** @typedef {{ symbol: string, name: string, price: number, change_pct: number, currency: string }} Tick */

/** @returns {Promise<any>} */
async function getJSON(/** @type {string} */ url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/** The news demo, unless a hosted news service was built in: then only ?demo asks for it. Same rule as the news page. */
function newsDemo() {
  if (new URLSearchParams(location.search).has(DEMO_PARAM)) return true;
  return !import.meta.env.VITE_NEWS_API && demoFirst();
}

export class HomeClient {
  /** The runner's connection, for the header: connecting, live, offline or demo. */
  status = $state(STATUS.connecting);
  /** The news service's connection. */
  newsStatus = $state(STATUS.connecting);
  /** @type {{ play: any, report: any }[]} every play, as the runner reports them */
  items = $state.raw([]);
  /** @type {any[]} the journal, oldest first */
  journal = $state.raw([]);
  /** Starting capital in dollars. */
  capital = $state(DEFAULT_CAPITAL);
  /** False until the journal has been read once: until then the account is not known. */
  walletReady = $state(false);
  /** @type {Record<string, import('./positions.js').Mark | null>} by play id */
  marks = $state.raw({});
  /** @type {Record<string, Tick>} by symbol */
  quotes = $state.raw({});
  /** @type {import('./news-model.js').News} */
  news = $state.raw(NO_NEWS);
  /** The news category picked, CATEGORY_ALL for every one. */
  category = $state(CATEGORY_ALL);
  /** @type {Record<string, import('./news-model.js').Headline[]>} a category's own list, once the service answered for it */
  fetched = $state.raw({});
  /** The runner's cash account (engine/signals/account.go); null from a runner or a recording that has none. */
  account = $state.raw(null);
  /** @type {any[]} today and the days ahead on the trading calendar (engine/calendar); empty when unknown */
  days = $state.raw([]);
  /** The crypto desk's last pass and the stock finder's (engine/plays); null when unknown. */
  crypto = $state.raw(null);
  finder = $state.raw(null);
  /** The options flow desk's last pass (engine/plays/flow.go); null when unknown. */
  flow = $state.raw(null);
  /** When the demo snapshot was recorded (ISO time), empty off the demo */
  demoTakenAt = $state('');

  /** The plays running on the desk now, one row each. */
  rows = $derived(this.items.filter((x) => x.play.status === STATUS_OPEN).map((x) => positionRow(x, this.marks[x.play.id])));
  /** @type {Tick[]} the ribbon, in its set order, then anything else that was quoted */
  ribbon = $derived.by(() => {
    const wanted = this.#symbols();
    const rest = Object.keys(this.quotes).filter((s) => !wanted.includes(s));
    return [...wanted, ...rest].map((s) => this.quotes[s]).filter(Boolean);
  });
  /** The banner: the top stories, or the newest headlines. */
  slides = $derived(this.news.headlines.slice(0, SLIDES_MAX));
  /** The list under the category chips. */
  headlines = $derived.by(() => {
    if (this.category === CATEGORY_ALL) return this.news.headlines.slice(0, HEADLINES_MAX);
    const own = this.fetched[this.category] ?? this.news.headlines.filter((h) => h.category === this.category);
    return own.slice(0, HEADLINES_MAX);
  });

  #base = stored(STORE_RUNNER, DEFAULT_RUNNER).replace(/\/+$/, '');
  #runnerLoop = new Poller(() => this.#loadRunner(), () => (this.status === STATUS.demo ? 0 : this.status === STATUS.live ? POLL_MS : RETRY_MS));
  #newsLoop = new Poller(() => this.#loadNews(), () => (this.newsStatus === STATUS.demo ? 0 : this.newsStatus === STATUS.live ? POLL_MS : RETRY_MS));

  start() {
    this.#runnerLoop.start();
    this.#newsLoop.start();
  }

  stop() {
    this.#runnerLoop.stop();
    this.#newsLoop.stop();
  }

  /** Picks a news category. A live stories service is asked for that category's own top stories. */
  pick(/** @type {string} */ id) {
    this.category = id;
    this.#loadCategory();
  }

  /** The ribbon's symbols: the set list, then the symbols of open plays. @returns {string[]} */
  #symbols() {
    return [...new Set([...ribbon.symbols, ...positionSymbols(this.items)])].slice(0, QUOTES_MAX);
  }

  async #loadRunner() {
    if (demoFirst()) {
      await this.#demoRunner();
      return;
    }
    try {
      const fresh = this.status !== STATUS.live;
      const after = this.journal[this.journal.length - 1]?.id ?? 0;
      const [items, lines, catalog] = await Promise.all([
        getJSON(`${this.#base}/api/plays`),
        getJSON(`${this.#base}/api/journal?limit=${JOURNAL_LIMIT}&after=${after}`),
        fresh ? getJSON(`${this.#base}/api/sources`) : null,
      ]);
      this.items = items;
      if (lines.length) this.journal = [...this.journal, ...lines];
      if (catalog) this.capital = catalog.go_live?.paper_capital_usd || DEFAULT_CAPITAL;
      this.walletReady = true;
      this.status = STATUS.live;
    } catch {
      this.status = STATUS.offline;
      return;
    }
    await Promise.all([this.#loadQuotes(), this.#loadMarks(), this.#loadDesk()]);
  }

  /**
   * The cash account, the calendar and each desk's last pass. A runner from before any of them
   * answers 404 for it: that block shows it has nothing, and the rest of the page is unaffected.
   */
  async #loadDesk() {
    const read = (/** @type {string} */ path) => getJSON(this.#base + path).catch(() => null);
    const [account, calendar, crypto, finder, flow] = await Promise.all([
      read('/api/account'), read(`/api/day?days=${DAYS_AHEAD}`), read('/api/plays/crypto'), read('/api/plays/finder'), read('/api/plays/flow'),
    ]);
    this.flow = flow;
    this.account = account;
    this.days = calendar?.days ?? [];
    this.crypto = crypto;
    this.finder = finder;
  }

  /** Short quotes for the ribbon. A failed round keeps the last ones. */
  async #loadQuotes() {
    try {
      /** @type {Tick[]} */
      const ticks = await getJSON(`${this.#base}/api/market/quotes?symbols=${encodeURIComponent(this.#symbols().join(','))}`);
      this.quotes = Object.fromEntries(ticks.map((t) => [t.symbol, t]));
    } catch {
      // The provider is having a moment: the ribbon keeps what it had.
    }
  }

  /** Each open play's last known value, from its curve. A play with no prices yet has none. */
  async #loadMarks() {
    const open = this.items.filter((x) => x.play.status === STATUS_OPEN).slice(0, MARKS_MAX);
    const curves = await Promise.all(open.map((x) => getJSON(`${this.#base}/api/plays/${encodeURIComponent(x.play.id)}/curve`).catch(() => null)));
    this.marks = Object.fromEntries(open.map((x, i) => [x.play.id, markFrom(curves[i])]));
  }

  /**
   * The recorded plays, journal and ribbon. The ribbon is home.json plus the markets page's
   * recorded quotes, which win where both have a symbol, so the ribbon and the page it opens agree.
   * A demo recorded before Home existed has no home.json, and shows the markets quotes alone.
   * Missing snapshot: offline.
   */
  async #demoRunner() {
    try {
      const [plays, trading] = await Promise.all([loadDemo(DEMO_PLAYS), loadDemo(DEMO_TRADING)]);
      this.items = plays.items ?? [];
      this.journal = trading.journal ?? [];
      this.marks = Object.fromEntries(Object.entries(plays.curves ?? {}).map(([id, curve]) => [id, markFrom(curve?.live)]));
      this.demoTakenAt = plays.taken_at;
      this.walletReady = true;
      this.status = STATUS.demo;
    } catch {
      this.status = STATUS.offline;
      return;
    }
    const [home, markets] = await Promise.all([loadDemo(DEMO_HOME).catch(() => null), loadDemo(DEMO_MARKETS).catch(() => null)]);
    this.capital = home?.paper_capital_usd || DEFAULT_CAPITAL;
    this.account = home?.account ?? null;
    this.days = home?.days ?? [];
    this.crypto = home?.crypto ?? null;
    this.finder = home?.finder ?? null;
    this.flow = home?.flow ?? null;
    /** @type {Tick[]} */
    const ticks = [...(home?.quotes ?? []), ...Object.values(markets?.quotes ?? {})];
    this.quotes = Object.fromEntries(ticks.map((t) => [t.symbol, t]));
  }

  async #loadNews() {
    try {
      this.news = newsDemo() ? await this.#demoNews() : await this.#liveNews();
      this.newsStatus = newsDemo() ? STATUS.demo : STATUS.live;
    } catch {
      this.newsStatus = STATUS.offline;
      return;
    }
    if (!this.news.categories.some((c) => c.id === this.category)) this.category = CATEGORY_ALL;
    await this.#loadCategory();
  }

  /** Stories when the service has them; a service from before stories answers 404, and plain headlines stand in. */
  async #liveNews() {
    const res = await fetch(`${NEWS_API}/api/stories?tradable=1&limit=${STORIES_LIMIT}`).catch(() => null);
    if (res?.ok) return newsFromStories(await res.json());
    return newsFromEvents(await getJSON(`${NEWS_API}/api/events?limit=${EVENTS_LIMIT}`));
  }

  /**
   * The recorded news: the tradable stories board, which is a small file of its own, and only when
   * that was never recorded the headlines from the much larger news snapshot.
   */
  async #demoNews() {
    const board = (await loadDemo(DEMO_STORIES).catch(() => null))?.stories;
    if (Array.isArray(board?.stories)) return newsFromStories(board, false);
    return newsFromEvents((await loadDemo(DEMO_NEWS)).events ?? []);
  }

  /**
   * The picked category's own top stories. Until they arrive (and on the demo, and on plain
   * headlines) the list is the loaded stories filtered to the category.
   */
  async #loadCategory() {
    const id = this.category;
    if (id === CATEGORY_ALL || this.news.mode !== MODE_STORIES || this.newsStatus !== STATUS.live) return;
    try {
      const body = await getJSON(`${NEWS_API}/api/stories?tradable=1&limit=${STORIES_LIMIT}&category=${encodeURIComponent(id)}`);
      this.fetched = { [id]: newsFromStories(body).headlines };
    } catch {
      // The filtered list stays.
    }
  }
}
