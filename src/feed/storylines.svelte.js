// Storylines: your own grouping of the stories the news service found. A storyline has a title you chose and the
// stories you put under it ("GTA 6 Launch" over the release, the voice actors and the delay). Kept on this device;
// the service's own grouping is never changed, and a story you group is shown under its storyline instead of
// on its own. Stories are named by number, so a storyline survives a reload and a story that has aged off the
// board just drops out of its count.

const STORE_LINES = 'm24.storylines';
const MAX_LINES = 100;
const MAX_MEMBERS = 200;
const MAX_TITLE = 80;

/** @typedef {{ id: string, title: string, members: number[] }} Storyline */

function load() {
  try {
    const saved = JSON.parse(localStorage.getItem(STORE_LINES) ?? 'null');
    if (!Array.isArray(saved)) return [];
    return saved.slice(0, MAX_LINES).map((l) => ({
      id: String(l.id).slice(0, 64),
      title: String(l.title ?? '').slice(0, MAX_TITLE),
      members: (Array.isArray(l.members) ? l.members : []).filter(Number.isInteger).slice(0, MAX_MEMBERS),
    }));
  } catch {
    return [];
  }
}

class Storylines {
  /** @type {Storyline[]} */
  list = $state(load());

  #save() {
    try {
      localStorage.setItem(STORE_LINES, JSON.stringify(this.list));
    } catch {
      // Not remembered on this device.
    }
  }

  /** The storyline a story sits under, if any. */
  of(/** @type {number} */ storyId) {
    return this.list.find((l) => l.members.includes(storyId));
  }

  /** Every story number under some storyline. */
  get grouped() {
    return new Set(this.list.flatMap((l) => l.members));
  }

  /** Starts a storyline with one story in it. A story is in one storyline at a time. */
  create(/** @type {string} */ title, /** @type {number} */ storyId) {
    const name = title.trim().slice(0, MAX_TITLE);
    if (!name) return;
    this.#leave(storyId);
    this.list = [{ id: `s-${crypto.randomUUID()}`, title: name, members: [storyId] }, ...this.list].slice(0, MAX_LINES);
    this.#save();
  }

  add(/** @type {string} */ id, /** @type {number} */ storyId) {
    this.#leave(storyId);
    this.list = this.list.map((l) => (l.id === id ? { ...l, members: [storyId, ...l.members].slice(0, MAX_MEMBERS) } : l));
    this.#save();
  }

  /** Takes a story out of its storyline; an empty storyline goes with it. */
  ungroup(/** @type {number} */ storyId) {
    this.#leave(storyId);
    this.#save();
  }

  rename(/** @type {string} */ id, /** @type {string} */ title) {
    const name = title.trim().slice(0, MAX_TITLE);
    if (!name) return;
    this.list = this.list.map((l) => (l.id === id ? { ...l, title: name } : l));
    this.#save();
  }

  remove(/** @type {string} */ id) {
    this.list = this.list.filter((l) => l.id !== id);
    this.#save();
  }

  #leave(/** @type {number} */ storyId) {
    this.list = this.list.map((l) => ({ ...l, members: l.members.filter((m) => m !== storyId) })).filter((l) => l.members.length);
  }
}

export const storylines = new Storylines();
