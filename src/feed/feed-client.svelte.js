// Live connection to the newsd engine: loads recent posts, then holds one Server-Sent Events stream
// that delivers new posts, network-meter changes and settings. The browser's EventSource reconnects
// on its own and sends Last-Event-ID, so the engine backfills anything missed while asleep or offline.
// With no engine (the public site with no news service set, or ?demo) it shows a recorded feed,
// calendar and network meter instead (src/lib/demo.js).
import { withMarketDays } from './market-days.js';
import { DEMO_PARAM, DEMO_STATUS, demoFirst, loadDemo } from '../lib/demo.js';
import { StoryBoard } from './stories-client.svelte.js';
import { ArchiveDesk } from './archive-client.svelte.js';

const DEFAULT_API = 'http://127.0.0.1:8024';
const API_BASE = import.meta.env.VITE_NEWS_API || DEFAULT_API;
const DEMO_SNAPSHOT = 'news.json';
const INITIAL_LIMIT = 200;
const MAX_ITEMS = 400;
const MAX_PENDING = 400;
const FRESH_PER_BATCH = 10; // only the first few of a burst animate, so a flood stays smooth
const RETRY_MS = 3000;
const HISTORY_POINTS = 100;
const SETTINGS_DEBOUNCE_MS = 250;
const MAX_CYCLES = 60;
const CALENDAR_DEBOUNCE_MS = 800; // one reload per burst of calendar changes
const STATUS = { connecting: 'connecting', live: 'live', reconnecting: 'reconnecting', offline: 'offline', demo: DEMO_STATUS };

/** The demo, unless a hosted news service was built in: then only ?demo asks for it. */
function newsDemo() {
  if (new URLSearchParams(location.search).has(DEMO_PARAM)) return true;
  return !import.meta.env.VITE_NEWS_API && demoFirst();
}

/** Newest first by publish time, falling back to when the engine saw it. */
function byPublished(/** @type {Post} */ a, /** @type {Post} */ b) {
  return Date.parse(b.published_at ?? b.seen_at) - Date.parse(a.published_at ?? a.seen_at);
}

/** @typedef {{id:number, seen_at:string, published_at:string|null, feed:string, source:string, kind:string,
 *   title:string, url:string, summary?:string, lang?:string, country?:string, tags:string[], image?:string,
 *   fresh?:boolean, order?:number}} Post */

export class FeedClient {
  /** @type {Post[]} newest first */
  items = $state([]);
  /** @type {Post[]} arrived while the reader was scrolled down */
  pending = $state([]);
  status = $state(STATUS.connecting);
  /** @type {{poll_every_ms:number, min_ms:number, max_ms:number} | null} */
  settings = $state(null);
  /** @type {Record<string, any>} live meter per source id */
  sources = $state({});
  /** @type {any} the latest meter totals (without per-source rows) */
  totals = $state(null);
  /** @type {{inRate:number, outRate:number, servedRate:number}[]} bytes per second, oldest first */
  history = $state([]);
  /** @type {{last_at:string, next_at:string, every_ms:number, offsetMs:number} | null} engine clock; offsetMs = engine time - local time */
  plan = $state(null);
  /** @type {any} the latest round report */
  report = $state(null);
  /** @type {any[]} recent round reports, newest first */
  cycles = $state([]);
  /** @type {import('./calendar-format.js').CalEvent[]} scheduled events, soonest first */
  calendar = $state([]);
  calendarLoaded = $state(false);
  /** While true, new posts wait in `pending` instead of shifting what the reader is looking at. */
  holding = false;
  /** When the demo feed was recorded (ISO time), empty off the demo */
  demoTakenAt = $state('');
  /** The stories board and world map: the same articles, grouped and scored by the engine. */
  stories = new StoryBoard();
  /** The news archive behind the calendar's day view. */
  archive = new ArchiveDesk();

  #es = null;
  #lastId = 0;
  #seen = new Set();
  #queue = [];
  #frame = 0;
  #retry = 0;
  #prevTotals = null;
  #settingsTimer = 0;
  #calendarTimer = 0;

  async start() {
    if (newsDemo()) {
      await this.#startDemo();
      return;
    }
    try {
      const res = await fetch(`${API_BASE}/api/events?limit=${INITIAL_LIMIT}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      /** @type {Post[]} */
      const posts = await res.json();
      for (const p of posts) this.#seen.add(p.id);
      this.#lastId = posts[0]?.id ?? 0; // newest arrival: the stream resumes after it
      // The backlog reads best by publish time; live posts then enter at the top as they arrive.
      this.items = [...posts].sort(byPublished);
      this.stories.connect(API_BASE);
      this.archive.connect(API_BASE);
      this.#connect();
    } catch {
      this.status = STATUS.offline;
      this.#retry = setTimeout(() => this.start(), RETRY_MS);
    }
  }

  /**
   * Loads the recorded feed, calendar and meter. No stream: nothing in it changes. The meter's
   * recorded readings are replayed in order, so the network page draws the same rates it drew live.
   * Missing snapshot: offline.
   */
  async #startDemo() {
    try {
      const snap = await loadDemo(DEMO_SNAPSHOT);
      this.items = [...snap.events].sort(byPublished);
      this.calendar = withMarketDays(snap.calendar ?? []);
      this.stories.useDemo();
      this.archive.useDemo();
      this.calendarLoaded = true;
      this.settings = snap.settings ?? null;
      for (const reading of snap.stats ?? []) this.#applyStats(reading);
      this.cycles = snap.cycles ?? [];
      this.report = this.cycles[0] ?? null;
      this.demoTakenAt = snap.taken_at;
      this.status = STATUS.demo;
    } catch {
      this.status = STATUS.offline;
    }
  }

  stop() {
    clearTimeout(this.#retry);
    clearTimeout(this.#calendarTimer);
    cancelAnimationFrame(this.#frame);
    this.#es?.close();
  }

  #connect() {
    const es = new EventSource(`${API_BASE}/api/stream?after=${this.#lastId}`);
    this.#es = es;
    es.onopen = () => {
      this.status = STATUS.live;
      this.#loadCalendar(); // on every (re)connect, so a sleeping laptop wakes up current
      this.stories.refresh();
    };
    es.onerror = () => {
      if (es.readyState !== EventSource.CLOSED) {
        this.status = STATUS.reconnecting;
        return;
      }
      // The browser gave up (engine gone or refused): start over from a fresh snapshot.
      this.status = STATUS.offline;
      this.#retry = setTimeout(() => this.#connect(), RETRY_MS);
    };
    es.onmessage = (m) => this.#enqueue(JSON.parse(m.data));
    es.addEventListener('stats', (m) => this.#applyStats(JSON.parse(m.data)));
    es.addEventListener('calendar', () => {
      clearTimeout(this.#calendarTimer);
      this.#calendarTimer = setTimeout(() => this.#loadCalendar(), CALENDAR_DEBOUNCE_MS);
    });
    es.addEventListener('stories', () => this.stories.refresh());
    es.addEventListener('settings', (m) => { this.settings = JSON.parse(m.data); });
    es.addEventListener('plan', (m) => {
      const p = JSON.parse(m.data);
      this.plan = { ...p, offsetMs: Date.parse(p.now) - Date.now() };
    });
    es.addEventListener('cycle', (m) => {
      const r = JSON.parse(m.data);
      if (this.cycles[0]?.at === r.at) return; // the replay sent on reconnect
      this.report = r;
      this.cycles = [r, ...this.cycles].slice(0, MAX_CYCLES);
    });
  }

  /** The engine's default window: last week to four weeks ahead. A failed load keeps what we have. */
  async #loadCalendar() {
    try {
      const res = await fetch(`${API_BASE}/api/calendar`);
      if (!res.ok) return;
      this.calendar = withMarketDays(await res.json());
      this.calendarLoaded = true;
    } catch { /* the stream reports the engine as offline on its own */ }
  }

  /** @param {Post} post */
  #enqueue(post) {
    if (this.#seen.has(post.id)) return;
    this.#seen.add(post.id);
    this.#lastId = Math.max(this.#lastId, post.id);
    this.#queue.push(post);
    if (!this.#frame) this.#frame = requestAnimationFrame(() => this.#flush());
  }

  /** One DOM update per frame, however many posts arrived in it. */
  #flush() {
    this.#frame = 0;
    const batch = this.#queue.splice(0).sort((a, b) => b.id - a.id);
    batch.forEach((p, i) => {
      p.fresh = i < FRESH_PER_BATCH;
      p.order = i;
    });
    if (this.holding) {
      this.pending = [...batch, ...this.pending].slice(0, MAX_PENDING);
      return;
    }
    this.items = [...batch, ...this.items].slice(0, MAX_ITEMS);
    this.#trimSeen();
  }

  /** Bring held posts into the feed, e.g. when the reader scrolls back to the top. */
  release() {
    if (!this.pending.length) return;
    this.items = [...this.pending, ...this.items].slice(0, MAX_ITEMS);
    this.pending = [];
    this.#trimSeen();
  }

  #trimSeen() {
    if (this.#seen.size < MAX_ITEMS * 4) return;
    this.#seen = new Set([...this.items, ...this.pending].map((p) => p.id));
  }

  #applyStats(snap) {
    const next = { ...this.sources };
    for (const s of snap.sources) next[s.id] = s;
    this.sources = next;
    const { sources: _ignored, ...totals } = snap;
    this.#pushHistory(totals);
    this.totals = totals;
  }

  #pushHistory(totals) {
    const prev = this.#prevTotals;
    this.#prevTotals = totals;
    if (!prev) return;
    const seconds = (Date.parse(totals.now) - Date.parse(prev.now)) / 1000;
    if (seconds <= 0) return;
    const point = {
      inRate: (totals.wire.in - prev.wire.in) / seconds,
      outRate: (totals.wire.out - prev.wire.out) / seconds,
      servedRate: (totals.served.out - prev.served.out) / seconds,
    };
    this.history = [...this.history, point].slice(-HISTORY_POINTS);
  }

  /** Change the global poll interval. Debounced, optimistic; the engine confirms over the stream. */
  setPollEvery(ms) {
    if (this.settings) this.settings = { ...this.settings, poll_every_ms: ms };
    if (this.status === STATUS.demo) return; // the demo's slider moves the numbers on this page only
    clearTimeout(this.#settingsTimer);
    this.#settingsTimer = setTimeout(() => {
      fetch(`${API_BASE}/api/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ poll_every_ms: ms }),
      }).catch(() => { this.status = STATUS.offline; });
    }, SETTINGS_DEBOUNCE_MS);
  }
}
