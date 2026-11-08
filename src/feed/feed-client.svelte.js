// Live connection to the newsd engine: loads recent posts, then holds one Server-Sent Events stream
// that delivers new posts, network-meter changes and settings. The browser's EventSource reconnects
// on its own and sends Last-Event-ID, so the engine backfills anything missed while asleep or offline.

const DEFAULT_API = 'http://127.0.0.1:8024';
const API_BASE = import.meta.env.VITE_NEWS_API || DEFAULT_API;
const INITIAL_LIMIT = 200;
const MAX_ITEMS = 400;
const MAX_PENDING = 400;
const FRESH_PER_BATCH = 10; // only the first few of a burst animate, so a flood stays smooth
const RETRY_MS = 3000;
const HISTORY_POINTS = 100;
const SETTINGS_DEBOUNCE_MS = 250;
const STATUS = { connecting: 'connecting', live: 'live', reconnecting: 'reconnecting', offline: 'offline' };

/** @typedef {{id:number, seen_at:string, published_at:string|null, feed:string, source:string, kind:string,
 *   title:string, url:string, summary?:string, lang?:string, country?:string, tags:string[],
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
  /** While true, new posts wait in `pending` instead of shifting what the reader is looking at. */
  holding = false;

  #es = null;
  #lastId = 0;
  #seen = new Set();
  #queue = [];
  #frame = 0;
  #retry = 0;
  #prevTotals = null;
  #settingsTimer = 0;

  async start() {
    try {
      const res = await fetch(`${API_BASE}/api/events?limit=${INITIAL_LIMIT}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      /** @type {Post[]} */
      const posts = await res.json();
      for (const p of posts) this.#seen.add(p.id);
      this.#lastId = posts[0]?.id ?? 0; // newest arrival: the stream resumes after it
      // The backlog reads best by publish time; live posts then enter at the top as they arrive.
      this.items = [...posts].sort((a, b) =>
        Date.parse(b.published_at ?? b.seen_at) - Date.parse(a.published_at ?? a.seen_at));
      this.#connect();
    } catch {
      this.status = STATUS.offline;
      this.#retry = setTimeout(() => this.start(), RETRY_MS);
    }
  }

  stop() {
    clearTimeout(this.#retry);
    cancelAnimationFrame(this.#frame);
    this.#es?.close();
  }

  #connect() {
    const es = new EventSource(`${API_BASE}/api/stream?after=${this.#lastId}`);
    this.#es = es;
    es.onopen = () => { this.status = STATUS.live; };
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
    es.addEventListener('settings', (m) => { this.settings = JSON.parse(m.data); });
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
