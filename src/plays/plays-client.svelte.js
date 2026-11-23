// The plays page's connection to the m24d runner: plays with their reports, the link history, the
// rulebook and the AI apps signed in on the desk machine. Plays live in the runner; this only reads and asks. The runner URL
// and secret are the trading desk's, so a phone pointed at a VPS runner works here too.
// With no runner (the public site, or ?demo) it shows a recorded snapshot instead (src/lib/demo.js).
// The demo is a trial room: every action works on this page only (plays-demo.js) and nothing is sent.
import strings from './strings.json';
import { fill } from './play-format.js';
import { demoExamples, demoFind, demoPaper } from './plays-demo.js';
import { DEMO_STATUS, demoFirst, loadDemo } from '../lib/demo.js';

export { stored, STORE_RUNNER } from '../lib/demo.js';

const DEFAULT_API = 'http://127.0.0.1:8025';
export const STORE_SECRET = 'm24.trading.secret';
export const DEFAULT_RUNNER = import.meta.env.VITE_TRADING_API || DEFAULT_API;
const POLL_MS = 5000;
const POLL_FAST_MS = 2000; // while an AI read is running
const STATUS = { connecting: 'connecting', live: 'live', offline: 'offline', demo: DEMO_STATUS };
const JSON_HEADERS = { 'Content-Type': 'application/json' };
const DEMO_SNAPSHOT = 'plays.json';
const DEMO_PREVIEW_DIR = 'preview/';
const DEMO_BUSY_MS = 320; // a demo action takes this long, so its button shows it was pressed
const ALL_PROVIDERS = 'all'; // the runner sends the play to every signed-in AI app at once

/** @typedef {{ play: any, report: any }} Item */

export class PlaysClient {
  /** @type {Item[]} newest first */
  items = $state([]);
  /** @type {any[]} */
  links = $state([]);
  /** @type {any} */
  rules = $state(null);
  /** @type {any[]} */
  providers = $state([]);
  status = $state(STATUS.connecting);
  /** When the demo snapshot was recorded (ISO time), empty off the demo */
  demoTakenAt = $state('');
  /** The day the demo's recorded backtests enter at the close of (YYYY-MM-DD), empty off the demo */
  demoBacktestFrom = $state('');

  #base;
  #secret;
  #timer = 0;
  #stopped = false;
  /** @type {Map<string, Promise<Record<string, any>>>} the demo's recorded dial sizes, per play */
  #demoSizes = new Map();
  /** @type {Record<string, { live: any, backtest: any }>} the demo's recorded curves, per play */
  #demoCurves = {};
  /** The rulebook as recorded: what the demo's "reset" goes back to. */
  #demoRules = null;

  /** @param {string} base runner URL  @param {string} secret webhook secret, empty on the local desk */
  constructor(base, secret) {
    this.#base = base.replace(/\/+$/, '');
    this.#secret = secret;
  }

  /** Any request. A method other than GET sends a JSON body and the secret; errors carry the runner's text. */
  async #req(/** @type {string} */ path, /** @type {{method?: string, body?: any, signal?: AbortSignal}} */ opts = {}) {
    const method = opts.method ?? 'GET';
    /** @type {Record<string, string>} */
    const headers = method === 'GET' ? {} : { ...JSON_HEADERS };
    if (this.#secret && method !== 'GET') headers.Authorization = `Bearer ${this.#secret}`;
    const body = method === 'GET' || method === 'DELETE' ? undefined : JSON.stringify(opts.body ?? {});
    const res = await fetch(`${this.#base}${path}`, { method, headers, body, signal: opts.signal });
    if (res.status === 204) return null;
    const text = await res.text();
    if (!res.ok) {
      let message = text.trim() || `HTTP ${res.status}`;
      try {
        message = JSON.parse(text).error ?? message;
      } catch {
        // plain-text error: keep it as is
      }
      throw new Error(message);
    }
    return text ? JSON.parse(text) : null;
  }

  /** Puts one fresh item in place (or in front), so the page doesn't wait for the next poll. */
  #upsert(/** @type {Item} */ item) {
    const i = this.items.findIndex((x) => x.play.id === item.play.id);
    if (i < 0) {
      this.items = [item, ...this.items];
      return item;
    }
    this.items[i] = item;
    return item;
  }

  async refreshPlays() {
    this.items = await this.#req('/api/plays');
  }

  async refreshLinks() {
    this.links = await this.#req('/api/plays/links');
  }

  async start() {
    this.#stopped = false;
    if (demoFirst()) {
      await this.#startDemo();
      return;
    }
    try {
      const [items, links, rules, providers] = await Promise.all([
        this.#req('/api/plays'), this.#req('/api/plays/links'), this.#req('/api/plays/rules'), this.#req('/api/plays/ai'),
      ]);
      Object.assign(this, { items, links, rules, providers });
      this.status = STATUS.live;
    } catch {
      this.status = STATUS.offline;
    }
    this.#schedule();
  }

  /** Loads the recorded snapshot. No polling: nothing in it changes. Missing snapshot: offline. */
  async #startDemo() {
    try {
      const snap = await loadDemo(DEMO_SNAPSHOT);
      Object.assign(this, { items: snap.items, links: snap.links, rules: snap.rules, providers: snap.providers });
      this.#demoCurves = snap.curves ?? {};
      this.#demoRules = structuredClone(snap.rules);
      this.demoBacktestFrom = snap.backtest_from ?? '';
      this.demoTakenAt = snap.taken_at;
      this.status = STATUS.demo;
    } catch {
      this.status = STATUS.offline;
    }
  }

  /** True in the trial room: actions change this page only. */
  get demo() {
    return this.status === STATUS.demo;
  }

  /** Lines the demo's inbox can read: the recorded contracts. @returns {string[]} */
  get demoExamples() {
    return this.demo ? demoExamples(this.items) : [];
  }

  /** A recorded play, after the short pause every demo action takes. @returns {Promise<Item>} */
  async #demoItem(/** @type {string} */ id) {
    await new Promise((done) => setTimeout(done, DEMO_BUSY_MS));
    const item = this.items.find((x) => x.play.id === id);
    if (!item) throw new Error(strings.demo.gone);
    return item;
  }

  /** The demo's dial: the recorded size nearest to units, merged onto the play as recorded. @returns {Promise<Item>} */
  async #demoPreview(/** @type {string} */ id, /** @type {number} */ units) {
    if (!this.#demoSizes.has(id)) {
      this.#demoSizes.set(id, loadDemo(`${DEMO_PREVIEW_DIR}${encodeURIComponent(id)}.json`).catch(() => ({})));
    }
    const sizes = await this.#demoSizes.get(id);
    const item = this.items.find((x) => x.play.id === id);
    if (!item) throw new Error(strings.demo.gone);
    const keys = Object.keys(sizes ?? {}).map(Number);
    if (!keys.length) return item;
    const near = keys.reduce((best, k) => (Math.abs(k - units) < Math.abs(best - units) ? k : best), keys[0]);
    const at = sizes[near];
    return { play: { ...item.play, ...at.play }, report: at.report };
  }

  stop() {
    this.#stopped = true;
    clearTimeout(this.#timer);
  }

  #schedule() {
    if (this.#stopped) return;
    const fast = this.items.some((x) => x.play.analyzing);
    this.#timer = setTimeout(() => this.#tick(), fast ? POLL_FAST_MS : POLL_MS);
  }

  async #tick() {
    try {
      if (this.status !== STATUS.live) {
        await this.start();
        return;
      }
      await this.refreshPlays();
    } catch {
      this.status = STATUS.offline;
    }
    this.#schedule();
  }

  /** Pastes a link: the runner resolves it, plans it and (by default) trades it at once. */
  async link(/** @type {{url: string, provider?: string, analyze?: boolean, theme?: string}} */ input) {
    if (this.demo) {
      await new Promise((done) => setTimeout(done, DEMO_BUSY_MS));
      const known = demoFind(this.items, input.url);
      if (!known) throw new Error(fill(strings.demo.unknownLink, { example: this.demoExamples[0] ?? '' }));
      return known;
    }
    try {
      return this.#upsert(await this.#req('/api/plays/link', { method: 'POST', body: input }));
    } finally {
      this.refreshLinks().catch(() => {});
    }
  }

  async save(/** @type {any} */ play) {
    if (this.demo) return this.#upsert({ play: { ...play, updated_at: new Date().toISOString() }, report: (await this.#demoItem(play.id)).report });
    return this.#upsert(await this.#req('/api/plays', { method: 'POST', body: play }));
  }

  async remove(/** @type {string} */ id) {
    if (this.demo) await this.#demoItem(id);
    else await this.#req(`/api/plays/${encodeURIComponent(id)}`, { method: 'DELETE' });
    this.items = this.items.filter((x) => x.play.id !== id);
  }

  async refresh(/** @type {string} */ id) {
    if (this.demo) return this.#demoItem(id); // the recording is the market: nothing newer to read
    return this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/refresh`, { method: 'POST' }));
  }

  async paper(/** @type {string} */ id) {
    if (this.demo) return this.#upsert(demoPaper(await this.#demoItem(id)));
    return this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/paper`, { method: 'POST' }));
  }

  /** The signed-in AI apps a play can be sent to, and all of them at once when there are two or more. */
  get aiChoices() {
    const connected = this.providers.filter((p) => p.connected);
    return connected.length > 1 ? [...connected, { id: ALL_PROVIDERS, name: strings.inbox.allProviders, model: '' }] : connected;
  }

  async analyze(/** @type {string} */ id, /** @type {string} */ provider) {
    const item = this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/analyze`, { method: 'POST', body: { provider } }));
    clearTimeout(this.#timer);
    this.#schedule();
    return item;
  }

  /** The play resized to units, not saved: the stake dial's live view. @returns {Promise<Item>} */
  preview(/** @type {string} */ id, /** @type {number} */ units, /** @type {AbortSignal} */ signal) {
    if (this.demo) return this.#demoPreview(id, units);
    return this.#req(`/api/plays/${encodeURIComponent(id)}/preview?units=${units}`, { signal });
  }

  /** Saves a draft's size. The demo keeps it on this page only. */
  async size(/** @type {string} */ id, /** @type {number} */ units) {
    if (this.demo) return this.#upsert(await this.#demoPreview(id, units));
    return this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/size`, { method: 'POST', body: { units } }));
  }

  /** Files the play under a category ("News play"). It is a label: it can change while a play runs. */
  async category(/** @type {string} */ id, /** @type {string} */ category) {
    if (this.demo) {
      const item = await this.#demoItem(id);
      return this.#upsert({ ...item, play: { ...item.play, category } });
    }
    return this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/category`, { method: 'POST', body: { category } }));
  }

  /**
   * The play replayed over its prices: since its own entry, or with a day (YYYY-MM-DD) backtested
   * from that day's close. The demo holds one of each per play, as recorded. @returns {Promise<any>}
   */
  async curve(/** @type {string} */ id, /** @type {string} */ from = '') {
    if (this.demo) {
      const recorded = this.#demoCurves[id]?.[from ? 'backtest' : 'live'];
      if (!recorded) throw new Error(strings.record.noCurve);
      return recorded;
    }
    return this.#req(`/api/plays/${encodeURIComponent(id)}/curve${from ? `?from=${encodeURIComponent(from)}` : ''}`);
  }

  async saveRules(/** @type {any} */ rules) {
    if (this.demo) {
      await new Promise((done) => setTimeout(done, DEMO_BUSY_MS));
      this.rules = rules;
      return rules;
    }
    this.rules = await this.#req('/api/plays/rules', { method: 'PUT', body: rules });
    await this.refreshPlays();
    return this.rules;
  }

  async resetRules() {
    if (this.demo) {
      await new Promise((done) => setTimeout(done, DEMO_BUSY_MS));
      this.rules = structuredClone(this.#demoRules);
      return this.rules;
    }
    this.rules = await this.#req('/api/plays/rules/reset', { method: 'POST' });
    await this.refreshPlays();
    return this.rules;
  }
}
