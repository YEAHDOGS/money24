// The news archive behind the calendar's day view: how many articles each New York day holds, and
// one open day's articles grouped by outlet or story. A group's articles load when it is opened,
// a page at a time, because a busy day holds thousands. Nothing loads until the calendar asks.
import { stored } from '../lib/demo.js';

const GROUP_BY = ['source', 'story'];
const STORE_GROUP_BY = 'm24.calendar.newsBy';
const PAGE_LIMIT = 60;
const SEARCH_DEBOUNCE_MS = 300;
const DAYS_REFRESH_MS = 60000; // the browser keeps a minute's copy too, so this costs nothing in between
const HTTP_NOT_FOUND = 404;
const DAY_KEY_LENGTH = 'YYYY-MM-DD'.length;
const MIDDAY_UTC = 'T12:00:00Z'; // a day's middle, so stepping a day at a time never skips or repeats one

/** @typedef {{id:number, seen_at:string, at:string, published_at:string|null, feed:string, feed_url?:string,
 *   source:string, outlet_url?:string, kind:string, title:string, url:string, author?:string, excerpt?:string,
 *   lang?:string, country?:string, tags:string[], image?:string, story?:{id:number, title:string, articles:number}}} Article */
/** @typedef {{key:string, name:string, n:number, latest:string, title:string}} Group */
/** @typedef {{name:string, n:number}} Facet */
/** @typedef {{articles:Article[], next:string, loading:boolean}} OpenGroup */

export class ArchiveDesk {
  /** @type {Record<string, number>} articles per day key, for the days asked for so far */
  counts = $state({});
  /** The first day the archive covers (it began then); empty until the service answers. */
  since = $state('');
  /** The day open in the day view, or empty. */
  day = $state('');
  by = $state(GROUP_BY.includes(stored(STORE_GROUP_BY, '')) ? stored(STORE_GROUP_BY, '') : GROUP_BY[0]);
  query = $state('');
  source = $state('');
  kind = $state('');
  tag = $state('');
  /** @type {{total:number, groups:Group[], more:number} | null} the open day's groups under the filters */
  groups = $state(null);
  /** @type {{sources:Facet[], kinds:Facet[], tags:Facet[]} | null} what the open day can be filtered by */
  facets = $state(null);
  /** @type {Record<string, OpenGroup>} the groups opened, by key */
  open = $state({});
  loading = $state(false);
  /** True on the demo, or when the news service answered but is older than the archive. */
  unavailable = $state(false);

  #api = '';
  /** Bumped by every new question, so an answer to an old one is dropped. */
  #ask = 0;
  #searchTimer = 0;
  /** @type {Record<string, number>} when each week's counts were loaded */
  #daysAt = {};

  /** @param {string} api the news service base URL */
  connect(api) {
    this.#api = api;
  }

  useDemo() {
    this.unavailable = true;
  }

  /** Article counts for the days from..to (YYYY-MM-DD), at most once a minute per span.
   * @param {string} from @param {string} to */
  async loadDays(from, to) {
    const span = `${from}/${to}`;
    if (!this.#api || this.unavailable || Date.now() - (this.#daysAt[span] ?? 0) < DAYS_REFRESH_MS) return;
    this.#daysAt[span] = Date.now();
    try {
      const res = await fetch(`${this.#api}/api/archive/days?from=${from}&to=${to}`);
      this.unavailable = res.status === HTTP_NOT_FOUND;
      if (!res.ok) return;
      const body = await res.json();
      const next = { ...this.counts };
      for (let d = new Date(from + MIDDAY_UTC); d.toISOString().slice(0, DAY_KEY_LENGTH) <= to; d.setUTCDate(d.getUTCDate() + 1)) {
        const key = d.toISOString().slice(0, DAY_KEY_LENGTH);
        next[key] = body.days[key] ?? 0;
      }
      this.counts = next;
      this.since = body.since ?? '';
    } catch {
      delete this.#daysAt[span]; // try again on the next turn of the week
    }
  }

  /** Opens a day in the day view, with the filters cleared. @param {string} day */
  openDay(day) {
    if (this.day === day) return;
    this.day = day;
    this.query = this.source = this.kind = this.tag = '';
    this.facets = null;
    this.#reload();
    this.#loadFacets();
  }

  closeDay() {
    this.day = '';
    this.#ask++;
    this.groups = null;
    this.open = {};
  }

  /** @param {string} by 'source' or 'story' */
  setBy(by) {
    if (!GROUP_BY.includes(by) || by === this.by) return;
    this.by = by;
    try {
      localStorage.setItem(STORE_GROUP_BY, by);
    } catch { /* storage is optional */ }
    this.#reload();
  }

  /** Sets one filter ('source', 'kind' or 'tag') and reloads. @param {'source'|'kind'|'tag'} key @param {string} value */
  setFilter(key, value) {
    if (this[key] === value) return;
    this[key] = value;
    this.#reload();
  }

  clearFilters() {
    clearTimeout(this.#searchTimer);
    this.query = this.source = this.kind = this.tag = '';
    this.#reload();
  }

  /** The search box changed: reload once typing pauses. @param {string} text */
  search(text) {
    this.query = text;
    clearTimeout(this.#searchTimer);
    this.#searchTimer = setTimeout(() => this.#reload(), SEARCH_DEBOUNCE_MS);
  }

  /** Opens or closes a group; an opened group loads its first page. @param {string} key */
  toggle(key) {
    if (this.open[key]) {
      const { [key]: _closed, ...rest } = this.open;
      this.open = rest;
      return;
    }
    this.open = { ...this.open, [key]: { articles: [], next: '', loading: true } };
    this.#loadGroup(key, '');
  }

  /** The next page of an open group. @param {string} key */
  more(key) {
    const group = this.open[key];
    if (!group || group.loading || !group.next) return;
    this.open[key] = { ...group, loading: true };
    this.#loadGroup(key, group.next);
  }

  /** The address of the open day's articles under the filters, narrowed further by `extra`.
   * @param {string} path @param {Record<string, string>} extra */
  #url(path, extra) {
    const params = new URLSearchParams({ day: this.day });
    for (const [key, value] of Object.entries({ q: this.query.trim(), source: this.source, kind: this.kind, tag: this.tag, ...extra })) {
      if (value) params.set(key, value);
    }
    return `${this.#api}${path}?${params}`;
  }

  /** The filters changed: the groups reload and every open group closes, except the first one reopens. */
  async #reload() {
    if (!this.day || !this.#api) return;
    const ask = ++this.#ask;
    this.loading = true;
    try {
      const res = await fetch(this.#url('/api/archive/groups', { by: this.by }));
      if (ask !== this.#ask) return;
      this.unavailable = res.status === HTTP_NOT_FOUND;
      const body = res.ok ? await res.json() : null;
      if (ask !== this.#ask) return;
      this.groups = body;
      this.open = {};
      if (body?.groups.length) this.toggle(body.groups[0].key);
    } catch { /* the stream reports the engine as offline on its own */ } finally {
      if (ask === this.#ask) this.loading = false;
    }
  }

  /** What the day can be filtered by: the whole day's outlets, kinds and tags, read once per day. */
  async #loadFacets() {
    const day = this.day;
    try {
      const res = await fetch(`${this.#api}/api/archive?day=${day}&limit=1`);
      if (!res.ok || day !== this.day) return;
      this.facets = (await res.json()).facets ?? null;
    } catch { /* filters stay empty */ }
  }

  /** @param {string} key @param {string} before the cursor, or '' for the first page */
  async #loadGroup(key, before) {
    const ask = this.#ask;
    const narrow = this.by === 'story' ? { story: key } : { source: key };
    try {
      const res = await fetch(this.#url('/api/archive', { ...narrow, limit: String(PAGE_LIMIT), before }));
      const page = res.ok ? await res.json() : { articles: [], next: '' };
      const group = this.open[key];
      if (ask !== this.#ask || !group) return;
      this.open[key] = { articles: [...group.articles, ...page.articles], next: page.next ?? '', loading: false };
    } catch {
      if (this.open[key]) this.open[key] = { ...this.open[key], loading: false };
    }
  }
}
