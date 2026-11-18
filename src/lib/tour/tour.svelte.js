// The tour: a short stepper on each app page. One pop-up at a time, a back and a next button in
// the middle of the screen, four pop-ups at most (Steps.svelte draws it; the words are tour.json).
//
// Which pages have been toured is kept in localStorage, so each page's tour shows once. The demo
// starts a page's tour by itself the first time; anywhere else it only shows when asked for: the
// Tour button in the footer, or ?tour on the address.

import copy from './tour.json';
import { demoFirst } from '../demo.js';

const STORE_TOUR = 'm24.tour.v2';
const TOUR_PARAM = 'tour';
export const MAX_STEPS = 4;

/** @returns {Record<string, boolean>} the pages whose tour was finished or skipped */
function readSeen() {
  try {
    return JSON.parse(localStorage.getItem(STORE_TOUR) ?? '') ?? {};
  } catch {
    return {};
  }
}

class Tour {
  /** The tour is showing. Off, Tour.svelte renders nothing. */
  on = $state(false);
  /** The step showing, from zero. */
  at = $state(0);
  /** The page being toured. */
  page = $state('');

  /** A page's steps, never more than MAX_STEPS. @returns {any[]} */
  steps(/** @type {string} */ page) {
    return (copy.tours[page] ?? []).slice(0, MAX_STEPS);
  }

  /** Called by each page once: starts its tour on a first visit to the demo, or when ?tour asks. */
  arrive(/** @type {string} */ page) {
    this.page = page;
    if (!this.steps(page).length) return;
    const asked = new URLSearchParams(location.search).has(TOUR_PARAM);
    if (asked || (demoFirst() && !readSeen()[page])) this.restart();
  }

  /** From the first step, as on a first visit. */
  restart() {
    if (!this.steps(this.page).length) return;
    this.at = 0;
    this.on = true;
  }

  /** One step on. Past the last one, the tour is over. */
  next() {
    if (this.at >= this.steps(this.page).length - 1) {
      this.finish();
      return;
    }
    this.at += 1;
  }

  back() {
    this.at = Math.max(0, this.at - 1);
  }

  /** Ends the tour (read to the end, or skipped) and remembers this page was toured. */
  finish() {
    this.on = false;
    try {
      localStorage.setItem(STORE_TOUR, JSON.stringify({ ...readSeen(), [this.page]: true }));
    } catch {
      // Storage blocked: the tour shows again next visit.
    }
  }
}

export const tour = new Tour();
