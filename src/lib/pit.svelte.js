// The Pit's watcher: asks the runner for the plays and turns every new play and every change of phase into an
// event, kept on this device (a log the header's bell shows, and an unread count). It runs on every page that
// wears the header, but only against a real runner: the demo has nothing that moves. No data leaves the device.
import { Poller } from '../home/poller.js';
import { DEFAULT_RUNNER, STORE_RUNNER, demoFirst, stored } from './demo.js';
import strings from './pit.json';

const STORE_LOG = 'm24.pit.log';
const STORE_SEEN = 'm24.pit.seen';
const STORE_READ = 'm24.pit.read';
const MAX_EVENTS = 100;
const POLL_MS = 5000;
const NEW_PLAY = 'new';
const PLAYS_PATH = '/api/plays';

function load(/** @type {string} */ key, /** @type {any} */ fallback) {
  try {
    return JSON.parse(localStorage.getItem(key) ?? 'null') ?? fallback;
  } catch {
    return fallback;
  }
}
function save(/** @type {string} */ key, /** @type {any} */ value) {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Not remembered on this device.
  }
}

/** @typedef {{ id: string, at: number, play: string, title: string, from: string, to: string }} PitEvent */

class Pit {
  /** @type {PitEvent[]} newest first */
  events = $state(load(STORE_LOG, []));
  #readAt = $state(Number(load(STORE_READ, 0)));
  /** Whether a runner answers: the demo and an offline desk both say no. */
  live = $state(false);
  /** The plays seen so far and the phase each was in, so only a change is news. @type {Record<string, string> | null} */
  #seen = load(STORE_SEEN, null);
  #poller = new Poller(() => this.#poll(), () => POLL_MS);
  #started = false;

  get unread() {
    return this.events.filter((e) => e.at > this.#readAt).length;
  }

  start() {
    if (this.#started || demoFirst()) return;
    this.#started = true;
    this.#poller.start();
  }

  markRead() {
    this.#readAt = Date.now();
    save(STORE_READ, this.#readAt);
  }

  async #poll() {
    const base = stored(STORE_RUNNER, DEFAULT_RUNNER).replace(/\/+$/, '');
    try {
      const res = await fetch(base + PLAYS_PATH);
      if (!res.ok) throw new Error(String(res.status));
      this.#take(await res.json());
      this.live = true;
    } catch {
      this.live = false;
    }
  }

  /** The first sight of the desk is a baseline, not news: only what happens after it is announced. */
  #take(/** @type {{ play: any }[]} */ items) {
    const first = this.#seen === null;
    const seen = this.#seen ?? {};
    const fresh = [];
    for (const { play } of items) {
      const before = seen[play.id];
      seen[play.id] = play.status;
      if (first || before === play.status) continue;
      fresh.push({ id: `${play.id}:${play.status}`, at: Date.now(), play: play.id, title: play.title, from: before ?? NEW_PLAY, to: play.status });
    }
    this.#seen = seen;
    save(STORE_SEEN, seen);
    if (!fresh.length) return;
    this.events = [...fresh, ...this.events].slice(0, MAX_EVENTS);
    save(STORE_LOG, this.events);
    fresh.forEach((e) => this.#notify(e));
  }

  /** A system notification, only when the page cannot be seen and the visitor asked for it. */
  #notify(/** @type {PitEvent} */ e) {
    if (!document.hidden || typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
    new Notification(e.title, { body: describe(e), tag: e.id });
  }
}

/** "New play" or "Armed to Open", in the Pit's own words. */
export function describe(/** @type {PitEvent} */ e) {
  const name = (/** @type {string} */ phase) => /** @type {Record<string, string>} */ (strings.phases)[phase] ?? phase;
  if (e.from === NEW_PLAY) return `${strings.new}: ${name(e.to)}`;
  return strings.moved.replace('{from}', name(e.from)).replace('{to}', name(e.to));
}

export const pit = new Pit();
