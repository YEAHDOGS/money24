// The pocket's data: every status route the runner and the news service answer, read together every
// ten seconds while the pocket is on screen, and at once each time the tray shows it. Each route is
// asked on its own, so one that is missing or slow (an older runner, a desk turned off) never holds
// up the rest. The pocket only reads; Stop goes through the app (src/tauri/src/supervisor.rs).
import { STORE_RUNNER, demoFirst, stored } from '../lib/demo.js';
import { native } from './native.js';
import { Poller } from '../home/poller.js';
import { aiRows, aiSummary, deskRows, overall, playRows, problems, recorderRows, serviceRows } from './pocket-model.js';

const DEFAULT_RUNNER_API = 'http://127.0.0.1:8025';
const DEFAULT_NEWS_API = 'http://127.0.0.1:8024';
const DEFAULT_RUNNER = import.meta.env.VITE_TRADING_API || DEFAULT_RUNNER_API;
const NEWS_API = (import.meta.env.VITE_NEWS_API || DEFAULT_NEWS_API).replace(/\/+$/, '');
const REFRESH_MS = 10000;
const REQUEST_TIMEOUT_MS = 6000;
const HEALTH_PATH = '/healthz';
const ROUTES = {
  supervisor: '/api/supervisor',
  follow: '/api/follow',
  kalshi: '/api/kalshitape',
  meme: '/api/meme',
  tape: '/api/tape',
  predict: '/api/predict',
  memedesk: '/api/memedesk',
  scout: '/api/plays/scout',
  ai: '/api/ai',
};
// The tray sends this each time it shows the pocket (src/tauri/src/pocket.rs).
export const SHOWN_EVENT = 'pocket-shown';

/** A GET that gives up after a while; null when it fails to answer at all. */
async function request(/** @type {string} */ url) {
  const stop = new AbortController();
  const timer = setTimeout(() => stop.abort(), REQUEST_TIMEOUT_MS);
  try {
    return await fetch(url, { signal: stop.signal, cache: 'no-store' });
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}

/** The body of a GET as JSON, or null for any failure: a missing route reads as "not reported". */
async function read(/** @type {string} */ url) {
  const res = await request(url);
  return res?.ok ? res.json().catch(() => null) : null;
}

export class PocketClient {
  /** @type {import('./pocket-model.js').Snapshot | null} null until the first read */
  snap = $state.raw(null);
  /** When the last read finished, epoch ms: the clock the rows are measured against. */
  now = $state(Date.now());
  /** Bumped each time the tray shows the pocket, so its content plays its entrance again. */
  shown = $state(0);

  problems = $derived(this.snap ? problems(this.snap, this.now) : []);
  services = $derived(this.snap ? serviceRows(this.snap, this.now) : []);
  recorders = $derived(this.snap ? recorderRows(this.snap, this.now) : []);
  desks = $derived(this.snap ? deskRows(this.snap) : []);
  plays = $derived(this.snap ? playRows(this.snap, this.now) : []);
  ai = $derived(this.snap ? aiRows(this.snap) : []);
  aiNotes = $derived(this.snap ? aiSummary(this.snap, this.now) : []);
  overall = $derived(overall(this.snap, this.problems.length));

  #base = stored(STORE_RUNNER, DEFAULT_RUNNER).replace(/\/+$/, '');
  #poller = new Poller(() => this.#load(), () => REFRESH_MS);

  /** True on a public website: the pocket belongs to the desktop app and asks nothing there. */
  publicPage = (false);

  start() {
    // Outside the desktop app and the runner's own pages, never ask 127.0.0.1: a public website that does
    // makes the browser ask the visitor for local-network access.
    if (!native && demoFirst()) {
      this.publicPage = true;
      return;
    }
    window.addEventListener(SHOWN_EVENT, this.#onShown);
    this.#poller.start();
  }

  stop() {
    window.removeEventListener(SHOWN_EVENT, this.#onShown);
    this.#poller.stop();
  }

  #onShown = () => {
    this.shown += 1;
    this.#load();
  };

  async #load() {
    const keys = /** @type {(keyof typeof ROUTES)[]} */ (Object.keys(ROUTES));
    const newsCheck = request(NEWS_API + HEALTH_PATH).then((res) => Boolean(res?.ok));
    const runner = Boolean((await request(this.#base + HEALTH_PATH))?.ok);
    // A runner that does not answer its health check is not asked for anything else until it does.
    const [news, ...bodies] = await Promise.all([newsCheck, ...keys.map((key) => (runner ? read(this.#base + ROUTES[key]) : null))]);
    this.snap = /** @type {import('./pocket-model.js').Snapshot} */ ({ runner, news, ...Object.fromEntries(keys.map((key, i) => [key, bodies[i]])) });
    this.now = Date.now();
  }
}
