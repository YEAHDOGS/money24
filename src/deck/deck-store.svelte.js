// The strategy deck: cards kept on this device (localStorage), seeded with DOGS's starter cards the first time.
// Import and export is plain JSON, so a person or an AI can write cards in from outside. Nothing leaves the device.
import seed from './seed.json';
import strings from './strings.json';

const STORE_DECK = 'm24.deck';
export const KINDS = Object.keys(strings.kinds);
const DEFAULT_KIND = 'rule';
const MAX_TEXT = 600;
const MAX_CARDS = 500;

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
    if (Array.isArray(saved)) return saved.slice(0, MAX_CARDS).map(cleanCard);
  } catch {
    // Unreadable: start from the starter deck.
  }
  return seed.map(cleanCard);
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
