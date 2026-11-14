// The plays page's connection to the m24d runner: plays with their reports, the link history, the
// rulebook and the AI providers. Plays live in the runner; this only reads and asks. The runner URL
// and secret are the trading desk's, so a phone pointed at a VPS runner works here too.

const DEFAULT_API = 'http://127.0.0.1:8025';
export const STORE_RUNNER = 'm24.trading.runner';
export const STORE_SECRET = 'm24.trading.secret';
export const DEFAULT_RUNNER = import.meta.env.VITE_TRADING_API || DEFAULT_API;
const POLL_MS = 5000;
const POLL_FAST_MS = 2000; // while an AI read is running
const STATUS = { connecting: 'connecting', live: 'live', offline: 'offline' };
const JSON_HEADERS = { 'Content-Type': 'application/json' };

/** Browser storage can be missing or throw (private mode, some WebViews): fall back to defaults. */
export function stored(/** @type {string} */ key, /** @type {string} */ fallback) {
  try {
    return localStorage.getItem(key) ?? fallback;
  } catch {
    return fallback;
  }
}

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

  #base;
  #secret;
  #timer = 0;
  #stopped = false;

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

  /** Pastes a link: the runner resolves it, plans it and (by default) paper trades it at once. */
  async link(/** @type {{url: string, provider?: string, analyze?: boolean, theme?: string}} */ input) {
    try {
      return this.#upsert(await this.#req('/api/plays/link', { method: 'POST', body: input }));
    } finally {
      this.refreshLinks().catch(() => {});
    }
  }

  async save(/** @type {any} */ play) {
    return this.#upsert(await this.#req('/api/plays', { method: 'POST', body: play }));
  }

  async remove(/** @type {string} */ id) {
    await this.#req(`/api/plays/${encodeURIComponent(id)}`, { method: 'DELETE' });
    this.items = this.items.filter((x) => x.play.id !== id);
  }

  async refresh(/** @type {string} */ id) {
    return this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/refresh`, { method: 'POST' }));
  }

  async paper(/** @type {string} */ id) {
    return this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/paper`, { method: 'POST' }));
  }

  async analyze(/** @type {string} */ id, /** @type {string} */ provider) {
    const item = this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/analyze`, { method: 'POST', body: { provider } }));
    clearTimeout(this.#timer);
    this.#schedule();
    return item;
  }

  /** The play resized to units, not saved: the stake dial's live view. @returns {Promise<Item>} */
  preview(/** @type {string} */ id, /** @type {number} */ units, /** @type {AbortSignal} */ signal) {
    return this.#req(`/api/plays/${encodeURIComponent(id)}/preview?units=${units}`, { signal });
  }

  async size(/** @type {string} */ id, /** @type {number} */ units) {
    return this.#upsert(await this.#req(`/api/plays/${encodeURIComponent(id)}/size`, { method: 'POST', body: { units } }));
  }

  async saveRules(/** @type {any} */ rules) {
    this.rules = await this.#req('/api/plays/rules', { method: 'PUT', body: rules });
    await this.refreshPlays();
    return this.rules;
  }

  async resetRules() {
    this.rules = await this.#req('/api/plays/rules/reset', { method: 'POST' });
    await this.refreshPlays();
    return this.rules;
  }
}
