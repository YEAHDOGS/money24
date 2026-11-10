// Market data for the Markets page, read from the m24d runner's /api/market routes: one symbol's
// quote, candles, news and related tickers, plus the reader's own followed list. Everything is
// polled on slow timers; the runner caches, so many open windows cost the provider nothing extra.

import { DEFAULT_RUNNER } from '../trading/trading-client.svelte.js';

export const RANGES = ['1d', '5d', '1mo', '6mo', 'ytd', '1y', '5y', 'max'];
/** Candle sizes each range can be drawn in. The first is the default. Mirrors engine/market. */
export const INTERVALS = {
  '1d': ['1m', '2m', '5m', '15m'],
  '5d': ['5m', '1m', '15m', '30m', '1h'],
  '1mo': ['30m', '5m', '15m', '1h', '1d'],
  '6mo': ['1d', '1h', '1wk'],
  ytd: ['1d', '1h', '1wk'],
  '1y': ['1d', '1h', '1wk'],
  '5y': ['1wk', '1d', '1mo'],
  max: ['1mo', '1d', '1wk'],
};
const DEFAULT_SYMBOL = 'NVDA';
const DEFAULT_FOLLOWING = ['NVDA', 'SPY', 'BTC-USD'];
const DEFAULT_RANGE = '1d';
const LIVE_RANGES = new Set(['1d', '5d']); // ranges whose newest candle is still moving
const QUOTE_POLL_MS = 15000;
const LIST_POLL_MS = 60000;
const SEARCH_DELAY_MS = 220;
const MAX_FOLLOWING = 30;
const STORE_RUNNER = 'm24.trading.runner';
const STORE_FOLLOWING = 'm24.markets.following';
const STORE_SYMBOL = 'm24.markets.symbol';
const NOT_FOUND = 404;

/** Browser storage can be missing or throw (private mode, some WebViews): fall back to defaults. */
function stored(/** @type {string} */ key, /** @type {string} */ fallback) {
  try {
    return localStorage.getItem(key) ?? fallback;
  } catch {
    return fallback;
  }
}

function store(/** @type {string} */ key, /** @type {string} */ value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Not persisted on this device; still used for this visit.
  }
}

export class MarketClient {
  symbol = $state('');
  range = $state(DEFAULT_RANGE);
  interval = $state(INTERVALS[DEFAULT_RANGE][0]);
  /** @type {any} quote, statistics, profile and quarters; null until the first answer */
  summary = $state(null);
  /** @type {any} candles for the current range */
  chart = $state(null);
  /** True when `chart` is a new symbol, range or interval, false when it is a live refresh. */
  chartIsNew = $state(true);
  /** @type {any[]} */
  news = $state([]);
  /** @type {any[]} */
  related = $state([]);
  /** @type {string[]} */
  following = $state([]);
  /** @type {any[]} short quotes for `following` */
  followed = $state([]);
  /** @type {any[]} search matches */
  matches = $state([]);
  loading = $state(true);
  /** 'offline' when the runner cannot be reached, 'missing' for an unknown symbol, else ''. */
  problem = $state('');

  #base = stored(STORE_RUNNER, DEFAULT_RUNNER).replace(/\/+$/, '');
  #timers = [];
  #searchTimer = 0;
  #opened = 0;

  constructor() {
    try {
      this.following = JSON.parse(stored(STORE_FOLLOWING, '')) ?? DEFAULT_FOLLOWING;
    } catch {
      this.following = DEFAULT_FOLLOWING;
    }
  }

  /** @returns {Promise<any>} */
  async #get(/** @type {string} */ route, /** @type {Record<string, string>} */ params) {
    const res = await fetch(`${this.#base}/api/market/${route}?${new URLSearchParams(params)}`);
    if (!res.ok) throw res.status;
    return res.json();
  }

  start() {
    // A link can name the symbol and the range: markets.html?s=NVDA&r=5d
    const link = new URLSearchParams(location.search);
    const range = link.get('r') ?? '';
    if (RANGES.includes(range)) {
      this.range = range;
      this.interval = INTERVALS[range][0];
    }
    this.open(link.get('s') || stored(STORE_SYMBOL, DEFAULT_SYMBOL));
    this.#timers = [
      setInterval(() => this.#refresh(), QUOTE_POLL_MS),
      setInterval(() => this.#lists(), LIST_POLL_MS),
    ];
  }

  stop() {
    this.#timers.forEach(clearInterval);
    clearTimeout(this.#searchTimer);
  }

  /** Shows another symbol: everything about the old one is dropped first. */
  async open(/** @type {string} */ symbol) {
    this.symbol = symbol.trim().toUpperCase();
    this.matches = [];
    this.summary = null;
    this.chart = null;
    this.news = [];
    this.related = [];
    this.problem = '';
    this.loading = true;
    const opened = ++this.#opened;
    store(STORE_SYMBOL, this.symbol);
    this.#link();
    try {
      const [summary] = await Promise.all([this.#get('summary', { symbol: this.symbol }), this.#candles(true)]);
      if (opened !== this.#opened) return;
      this.summary = summary;
    } catch (status) {
      if (opened !== this.#opened) return;
      this.problem = status === NOT_FOUND ? 'missing' : 'offline';
    }
    this.loading = false;
    this.#lists();
    this.#stories(opened);
    this.#get('related', { symbol: this.symbol }).then((r) => { if (opened === this.#opened) this.related = r; }).catch(() => {});
  }

  /** Stories filed under the ticker; failing that (most coins), under its name without the currency. */
  async #stories(/** @type {number} */ opened) {
    const name = this.summary?.name ?? '';
    const bare = name.endsWith(` ${this.summary?.currency}`) ? name.slice(0, name.lastIndexOf(' ')) : '';
    for (const q of [this.symbol, name, bare]) {
      if (!q) continue;
      const news = await this.#get('search', { q }).then((r) => r.news).catch(() => []);
      if (opened !== this.#opened) return;
      this.news = news;
      if (news.length) return;
    }
  }

  /** Keeps the address bar a link to what is on screen. */
  #link() {
    history.replaceState(null, '', `?${new URLSearchParams({ s: this.symbol, r: this.range })}`);
  }

  async setRange(/** @type {string} */ range) {
    this.range = range;
    this.interval = INTERVALS[range][0];
    this.#link();
    await this.#candles(true).catch(() => {});
  }

  async setInterval(/** @type {string} */ interval) {
    this.interval = interval;
    await this.#candles(true).catch(() => {});
  }

  async #candles(/** @type {boolean} */ isNew) {
    const [symbol, range, interval] = [this.symbol, this.range, this.interval];
    const chart = await this.#get('chart', { symbol, range, interval });
    // An answer for a view the reader has already left is thrown away.
    if (symbol !== this.symbol || range !== this.range || interval !== this.interval) return;
    this.chartIsNew = isNew;
    this.chart = chart;
  }

  async #refresh() {
    if (!this.symbol || this.problem === 'missing' || document.hidden) return;
    const symbol = this.symbol;
    try {
      const summary = await this.#get('summary', { symbol });
      if (symbol !== this.symbol) return;
      this.summary = summary;
      this.problem = '';
      if (LIVE_RANGES.has(this.range) || !this.chart) await this.#candles(!this.chart);
    } catch {
      this.problem = this.summary ? '' : 'offline';
    }
  }

  async #lists() {
    if (!this.following.length) {
      this.followed = [];
      return;
    }
    this.followed = await this.#get('quotes', { symbols: this.following.join(',') }).catch(() => this.followed);
  }

  /** Looks up tickers as the reader types, once they pause. An empty query clears the list. */
  search(/** @type {string} */ query) {
    clearTimeout(this.#searchTimer);
    const q = query.trim();
    if (!q) {
      this.matches = [];
      return;
    }
    this.#searchTimer = setTimeout(async () => {
      this.matches = await this.#get('search', { q }).then((r) => r.matches).catch(() => []);
    }, SEARCH_DELAY_MS);
  }

  toggleFollow(/** @type {string} */ symbol) {
    const has = this.following.includes(symbol);
    this.following = has ? this.following.filter((s) => s !== symbol) : [symbol, ...this.following].slice(0, MAX_FOLLOWING);
    store(STORE_FOLLOWING, JSON.stringify(this.following));
    this.#lists();
  }
}
