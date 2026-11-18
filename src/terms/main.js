// The Terms of Use page's only script: it swaps the plain no-JavaScript header for the app header,
// so the terms wear exactly what every app page wears, and folds the contents list on phones.
// The text itself never needs it.
import AppHeader from '../lib/AppHeader.svelte';
import { mount } from 'svelte';

const SLOT_ID = 'header-slot';
const FOLD_SELECTOR = '.toc-fold';
// Where the contents are a rail beside the text (the same query as terms.css): always open there.
const RAIL_QUERY = '(min-width: 768px) and (min-height: 501px)';
const slot = document.getElementById(SLOT_ID);

/** @type {HTMLDetailsElement | null} */
const fold = document.querySelector(FOLD_SELECTOR);
const rail = matchMedia(RAIL_QUERY);
if (fold) {
  const sync = () => { fold.open = rail.matches; };
  sync();
  rail.addEventListener('change', sync);
  // In the rail it never folds; on a phone, picking a section folds it out of the way.
  fold.addEventListener('toggle', () => { if (rail.matches && !fold.open) fold.open = true; });
  fold.addEventListener('click', (e) => { if (!rail.matches && e.target instanceof Element && e.target.closest('a')) fold.open = false; });
}

slot.replaceChildren();
export default mount(AppHeader, { target: slot, props: { page: 'terms' } });
