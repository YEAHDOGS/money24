// The strategy deck: cards kept on this device (localStorage), seeded with DOGS's starter cards the first time.
// Import and export is plain JSON, so a person or an AI can write cards in from outside. Nothing leaves the device.
import seed from './seed.json';
import strings from './strings.json';

const STORE_DECK = 'm24.deck';
const STORE_SEEDS = 'm24.deck.seeds'; // starter cards already offered, so a removed one stays removed
export const KINDS = Object.keys(strings.kinds);
const DEFAULT_KIND = 'rule';
const MAX_TEXT = 600;
const MAX_CARDS = 500;
const FIRST_SEEDS = 8; // the starter cards a deck saved before STORE_SEEDS existed was offered

/** @typedef {{ id: string, name: string, kind: string, asset: string, when: string, then: string, notes: string, by: string, enabled: boolean }} Card */

const clip = (/** @type {any} */ v, /** @type {number} */ max = MAX_TEXT) => String(v ?? '').slice(0, max);

/** Anything in, a safe card out: unknown kinds fall back, text is bounded, nothing else is kept. @returns {Card} */
export function cleanCard(/** @type {any} */ raw) {
  return {
    id: clip(raw?.id, 64) || `c-${crypto.randomUUID()}`,
    name: clip(raw?.name, 80) || strings.defaults.name,
    kind: KINDS.includes(raw?.kind) ? raw.kind : DEFAULT_KIND,
    asset: clip(raw?.asset, 24).toUpperCase() || strings.defaults.asset,
    when: clip(raw?.when),
    then: clip(raw?.then),
    notes: clip(raw?.notes),
    by: ['you', 'ai', 'dogs'].includes(raw?.by) ? raw.by : 'you',
    enabled: raw?.enabled !== false,
  };
}

function load() {
  try {
    const saved = JSON.parse(localStorage.getItem(STORE_DECK) ?? 'null');
    const offered = new Set(JSON.parse(localStorage.getItem(STORE_SEEDS) ?? 'null') ?? (Array.isArray(saved) ? seed.slice(0, FIRST_SEEDS).map((c) => c.id) : []));
    const fresh = seed.filter((c) => !offered.has(c.id));
    if (Array.isArray(saved)) {
      const merged = [...fresh, ...saved].slice(0, MAX_CARDS).map(cleanCard);
      // Offered and kept together: a starter card is never marked offered without being in the saved deck.
      localStorage.setItem(STORE_DECK, JSON.stringify(merged));
      localStorage.setItem(STORE_SEEDS, JSON.stringify(seed.map((c) => c.id)));
      return merged;
    }
  } catch {
    // Unreadable: start from the starter deck.
  }
  const starter = seed.map(cleanCard);
  try {
    localStorage.setItem(STORE_DECK, JSON.stringify(starter));
    localStorage.setItem(STORE_SEEDS, JSON.stringify(seed.map((c) => c.id)));
  } catch {
    // Not remembered on this device.
  }
  return starter;
}

class Deck {
  /** @type {Card[]} */
  cards = $state(load());

  #save() {
    try {
      localStorage.setItem(STORE_DECK, JSON.stringify(this.cards));
    } catch {
      // Not remembered on this device.
    }
  }

  /** Adds a card, or replaces the one with the same id. */
  put(/** @type {any} */ raw) {
    const card = cleanCard(raw);
    const i = this.cards.findIndex((c) => c.id === card.id);
    this.cards = i < 0 ? [card, ...this.cards].slice(0, MAX_CARDS) : this.cards.map((c, k) => (k === i ? card : c));
    this.#save();
    return card;
  }

  remove(/** @type {string} */ id) {
    this.cards = this.cards.filter((c) => c.id !== id);
    this.#save();
  }

  copy(/** @type {string} */ id) {
    const card = this.cards.find((c) => c.id === id);
    if (card) this.put({ ...card, id: '', name: `${card.name} copy`, by: 'you' });
  }

  toggle(/** @type {string} */ id) {
    const card = this.cards.find((c) => c.id === id);
    if (card) this.put({ ...card, enabled: !card.enabled });
  }

  /** The deck as a file a person or an AI can read and write. */
  exportJson() {
    return JSON.stringify(this.cards, null, 2);
  }

  /** Adds the cards in a file. Returns how many, or -1 when it is not a deck. */
  importJson(/** @type {string} */ text) {
    let list;
    try {
      list = JSON.parse(text);
    } catch {
      return -1;
    }
    if (!Array.isArray(list)) return -1;
    list.slice(0, MAX_CARDS).forEach((raw) => this.put(raw));
    return list.length;
  }
}

export const deck = new Deck();
