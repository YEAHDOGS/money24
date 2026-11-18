// The tour: what a first visit to the demo is told, and what it has already been told.
//
// Two layers (Tour.svelte). First the cards: one to three big ones, shown once, that say what
// Money 24 is, that this is a snapshot, and how to get around. Then the blurbs: bubbles anchored
// to real parts of the page (tour.json), each put away on its own or all at once.
//
// What has been seen is kept in localStorage and shared by every page, so a second visit, or the
// next page, is left alone. The demo turns the tour on by itself the first time; anywhere else it
// only shows when asked for: the Tour button in the footer, or ?tour on the address.

import { SvelteSet } from 'svelte/reactivity';
import copy from './tour.json';
import { demoFirst } from '../demo.js';

const STORE_TOUR = 'm24.tour.v1';
const TOUR_PARAM = 'tour';

function readSaved() {
  try {
    return JSON.parse(localStorage.getItem(STORE_TOUR) ?? '') ?? {};
  } catch {
    return {};
  }
}

class Tour {
  /** The tour is showing (either layer). Off, Tour.svelte renders nothing. */
  on = $state(false);
  /** The cards are the layer showing; false means the blurbs are. */
  carousel = $state(false);
  /** Ids of blurbs that were put away. @type {SvelteSet<string>} */
  dismissed = new SvelteSet();

  constructor() {
    const saved = readSaved();
    for (const id of saved.dismissed ?? []) this.dismissed.add(id);
    if (new URLSearchParams(location.search).has(TOUR_PARAM)) {
      this.restart();
      return;
    }
    // A first visit to the demo gets the whole tour; one that has put it all away does not.
    if (demoFirst() && !saved.done) {
      this.on = true;
      this.carousel = !saved.cardsSeen;
    }
  }

  /** The cards are over (skipped or read to the end): the blurbs take over. */
  endCarousel() {
    this.carousel = false;
    this.#save({ cardsSeen: true });
  }

  /** Puts one blurb away for good. Once every blurb is gone, the tour is over. */
  dismiss(/** @type {string} */ id) {
    this.dismissed.add(id);
    const done = copy.blurbs.every((b) => this.dismissed.has(b.id));
    if (done) this.on = false;
    this.#save({ dismissed: [...this.dismissed], done });
  }

  /** Puts every blurb away and ends the tour. */
  dismissAll() {
    for (const b of copy.blurbs) this.dismissed.add(b.id);
    this.on = false;
    this.#save({ dismissed: [...this.dismissed], done: true });
  }

  /** From the first card, as on a first visit. */
  restart() {
    this.dismissed.clear();
    this.#save({ cardsSeen: false, dismissed: [], done: false });
    this.carousel = true;
    this.on = true;
  }

  #save(/** @type {Record<string, any>} */ changes) {
    try {
      localStorage.setItem(STORE_TOUR, JSON.stringify({ ...readSaved(), ...changes }));
    } catch {
      // Storage blocked: the tour shows again next visit.
    }
  }
}

export const tour = new Tour();
