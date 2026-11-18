<script>
  /**
   * The layer of tour bubbles over a page (tour.json says what and where). Only the bubbles for
   * this page appear, and only those whose target is on screen: a pane that is hidden on a phone
   * gets its bubble when it shows. Targets are measured again every so often, so a bubble follows
   * a layout that changes. Each bubble is placed clear of the ones before it (place.js), and a
   * phone shows only a couple at a time, the next arriving as one is put away.
   */
  import { onMount } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import copy from './tour.json';
  import { tour } from './tour.svelte.js';
  import { onScreen, place } from './place.js';
  import Blurb from './Blurb.svelte';

  const SETTLE_MS = 500; // a page fades in and its data lands before anything is measured
  const RECHECK_MS = 400; // how often the targets are measured again
  const NARROW_PX = 768; // under this width (a phone), only a few bubbles show at once
  const NARROW_MAX = 2;
  const DEFAULT_AT = 0.5;

  /** @type {{ page: string }} */
  let { page } = $props();

  let ready = $state(false);
  /** @type {{ blurb: any, rect: any }[]} the blurbs whose target is on screen right now. Raw: the list is replaced whole and compared by identity. */
  let shown = $state.raw([]);
  let bounds = $state({ w: 0, h: 0 });
  /** @type {SvelteMap<string, { w: number, h: number }>} each bubble's size, reported once drawn */
  const sizes = new SvelteMap();

  /** A bubble's size, kept only when it changed, or placing would never settle. */
  function sized(/** @type {string} */ id, /** @type {number} */ w, /** @type {number} */ h) {
    const was = sizes.get(id);
    if (was && was.w === w && was.h === h) return;
    sizes.set(id, { w, h });
  }

  // A bubble just put away leaves at once, before the next measurement.
  const visible = $derived.by(() => {
    const live = shown.filter((s) => !tour.dismissed.has(s.blurb.id));
    return bounds.w < NARROW_PX ? live.slice(0, NARROW_MAX) : live;
  });
  // Each bubble is placed after the ones before it, and keeps clear of them.
  const placed = $derived.by(() => {
    /** @type {{ left: number, top: number, w: number, h: number }[]} */
    const taken = [];
    return visible.map((item) => {
      const size = sizes.get(item.blurb.id) ?? { w: 0, h: 0 };
      const pos = place(item.rect, size, item.blurb.side, item.blurb.at ?? DEFAULT_AT, bounds, taken);
      if (size.w) taken.push({ left: pos.left, top: pos.top, w: size.w, h: size.h });
      return { ...item, pos };
    });
  });

  const box = (/** @type {DOMRect} */ r) => ({ left: r.left, top: r.top, right: r.right, bottom: r.bottom, width: r.width, height: r.height });
  const same = (/** @type {any[]} */ a, /** @type {any[]} */ b) => a.length === b.length && a.every((s, i) => s.blurb === b[i].blurb && JSON.stringify(s.rect) === JSON.stringify(b[i].rect));

  function measure() {
    const next = { w: window.innerWidth, h: window.innerHeight };
    const list = [];
    for (const blurb of copy.blurbs) {
      if (tour.dismissed.has(blurb.id)) continue;
      if (blurb.page && blurb.page !== page) continue;
      const rect = document.querySelector(`[data-tour="${blurb.target}"]`)?.getBoundingClientRect();
      if (rect && onScreen(rect, next)) list.push({ blurb, rect: box(rect) });
    }
    if (next.w !== bounds.w || next.h !== bounds.h) bounds = next;
    if (!same(list, shown)) shown = list;
  }

  // Let the page settle, then keep looking: a pane opens, the window changes size, a list scrolls.
  onMount(() => {
    let recheck = 0;
    const settle = setTimeout(() => {
      ready = true;
      measure();
      recheck = setInterval(measure, RECHECK_MS);
    }, SETTLE_MS);
    window.addEventListener('resize', measure);
    return () => {
      clearTimeout(settle);
      clearInterval(recheck);
      window.removeEventListener('resize', measure);
    };
  });
</script>

{#if ready}
  {#each placed as item, i (item.blurb.id)}
    <Blurb
      blurb={item.blurb}
      pos={item.pos}
      index={i}
      onsize={(w, h) => sized(item.blurb.id, w, h)}
      onclose={() => tour.dismiss(item.blurb.id)}
      onskip={() => tour.dismissAll()}
    />
  {/each}
{/if}
