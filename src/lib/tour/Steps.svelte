<script>
  /**
   * The tour as a stepper: one pop-up at a time, pointing at a real part of the page (or sitting
   * in the middle when it is about the whole thing). Back and Next sit at the far left and right
   * edges of the window, Skip tutorial at the top right, and the page behind is blurred a little so
   * the tutorial is what you look at; the thing a step points at stays sharp. Everything the tour
   * draws is one color nothing else on the site uses (--color-tour). Arrow keys step, Escape skips.
   * Words: tour.json. Where the pop-up goes: place.js.
   */
  import { onMount } from 'svelte';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import Check from '@lucide/svelte/icons/check';
  import copy from './tour.json';
  import { tour } from './tour.svelte.js';
  import { onScreen, place } from './place.js';
  import { demoFirst, loadDemo } from '../demo.js';

  const RECHECK_MS = 300; // how often the target is measured again
  const SNAPSHOT_FILE = 'markets/index.json'; // the smallest recorded file that carries the snapshot's time
  const DEFAULT_SIDE = 'bottom';
  const DEFAULT_AT = 0.5;
  const HALF = 2;
  const ARROW_ROOM = 68; // the pop-up keeps this far from the side edges when it is level with the arrows
  const ARROW_BAND = 44; // how far above and below mid-height the arrows reach
  /** @type {Intl.DateTimeFormatOptions} */
  const SNAPSHOT_DATE = { month: 'long', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' };

  const steps = $derived(tour.steps(tour.page));
  const step = $derived(steps[Math.min(tour.at, steps.length - 1)]);
  const last = $derived(tour.at >= steps.length - 1);

  let takenAt = $state('');
  /** @type {{ left: number, top: number, right: number, bottom: number, width: number, height: number } | null} the target's box, when it is on screen */
  let rect = $state.raw(null);
  let bounds = $state.raw({ w: 0, h: 0 });
  let w = $state(0);
  let h = $state(0);

  const text = $derived(step.textPending && !takenAt ? step.textPending : step.text.replace('{when}', new Date(takenAt).toLocaleString(undefined, SNAPSHOT_DATE)));
  /** Where the pop-up goes: beside its target, or the middle of the screen when it has none in view. */
  const pos = $derived.by(() => {
    if (!rect || !w) return { left: (bounds.w - w) / HALF, top: (bounds.h - h) / HALF, side: '', tx: 0, ty: 0 };
    const p = place(rect, { w, h }, step.side ?? DEFAULT_SIDE, step.at ?? DEFAULT_AT, bounds);
    // Level with the arrows and room to spare: slide clear of them, and keep the tail on the target.
    const mid = bounds.h / HALF;
    const level = p.top < mid + ARROW_BAND && p.top + h > mid - ARROW_BAND;
    if (!level || bounds.w - ARROW_ROOM * HALF < w) return p;
    const left = Math.min(Math.max(p.left, ARROW_ROOM), bounds.w - w - ARROW_ROOM);
    return { ...p, left, tx: p.tx + p.left - left };
  });
  /**
   * The blur is four panels around the target, so the target itself stays sharp. With no target
   * the first panel covers the whole window and the rest have no size.
   */
  const veil = $derived(rect
    ? [
      { left: 0, top: 0, width: bounds.w, height: rect.top },
      { left: 0, top: rect.bottom, width: bounds.w, height: Math.max(0, bounds.h - rect.bottom) },
      { left: 0, top: rect.top, width: rect.left, height: rect.height },
      { left: rect.right, top: rect.top, width: Math.max(0, bounds.w - rect.right), height: rect.height },
    ]
    : [{ left: 0, top: 0, width: bounds.w, height: bounds.h }]);

  function measure() {
    const next = { w: window.innerWidth, h: window.innerHeight };
    if (next.w !== bounds.w || next.h !== bounds.h) bounds = next;
    const box = step.target ? document.querySelector(`[data-tour="${step.target}"]`)?.getBoundingClientRect() : null;
    const found = box && onScreen(box, next) ? { left: Math.max(0, box.left), top: Math.max(0, box.top), right: box.right, bottom: box.bottom, width: box.width, height: box.height } : null;
    if (JSON.stringify(found) !== JSON.stringify(rect)) rect = found;
  }

  function onKey(/** @type {KeyboardEvent} */ e) {
    if (e.key === 'ArrowRight') tour.next();
    else if (e.key === 'ArrowLeft') tour.back();
    else if (e.key === 'Escape') tour.finish();
    else return;
    e.preventDefault();
  }

  // A new step: look at its target at once.
  $effect(() => {
    step;
    measure();
  });

  onMount(() => {
    const recheck = setInterval(measure, RECHECK_MS);
    window.addEventListener('resize', measure);
    window.addEventListener('keydown', onKey);
    if (demoFirst()) loadDemo(SNAPSHOT_FILE).then((index) => { takenAt = index.taken_at; }).catch(() => {});
    return () => {
      clearInterval(recheck);
      window.removeEventListener('resize', measure);
      window.removeEventListener('keydown', onKey);
    };
  });
</script>

<!-- THE VEIL: the page, blurred and dimmed a little, everywhere but the step's target. -->
{#each veil as panel, i (i)}
  <div class="veil" aria-hidden="true" style:left="{panel.left}px" style:top="{panel.top}px" style:width="{panel.width}px" style:height="{panel.height}px"></div>
{/each}

<!-- The ring around what the step is about. It glides from one target to the next. -->
<div class="ring" class:shown={rect} aria-hidden="true"
  style:transform="translate({rect?.left ?? bounds.w / HALF}px, {rect?.top ?? bounds.h / HALF}px)"
  style:width="{rect?.width ?? 0}px" style:height="{rect?.height ?? 0}px"></div>

<section class="pop side-{pos.side}" class:ready={w > 0} bind:offsetWidth={w} bind:offsetHeight={h} aria-label={copy.label} aria-live="polite"
  style:transform="translate({Math.round(pos.left)}px, {Math.round(pos.top)}px)" style:--tx="{pos.tx}px" style:--ty="{pos.ty}px">
  {#if pos.side}<span class="tail" aria-hidden="true"></span>{/if}
  {#key step.id}
    <div class="words">
      <p class="count">{copy.count.replace('{n}', String(tour.at + 1)).replace('{of}', String(steps.length))}</p>
      <h2 class="title">{step.title}</h2>
      <p class="text">{text}</p>
    </div>
  {/key}
</section>

<!-- SKIP: top right, whenever a tutorial is showing. -->
<button type="button" class="skip" onclick={() => tour.finish()}>{copy.skip}</button>

<!-- BACK and NEXT: mid-height at the far left and right edges, always in the same place. -->
<button type="button" class="arrow arrow-back" onclick={() => tour.back()} disabled={tour.at === 0} aria-label={copy.back}>
  <ArrowLeft size={24} strokeWidth={2.5} aria-hidden="true" />
</button>
<button type="button" class="arrow arrow-next" onclick={() => tour.next()} aria-label={last ? copy.done : copy.next}>
  {#if last}<Check size={24} strokeWidth={2.5} aria-hidden="true" />{:else}<ArrowRight size={24} strokeWidth={2.5} aria-hidden="true" />{/if}
</button>

<style>
  .veil {
    position: fixed;
    z-index: 48;
    background: var(--tour-veil);
    backdrop-filter: blur(3px);
    animation: veil-in 220ms ease-out both;
  }
  @keyframes veil-in {
    from {
      opacity: 0;
    }
  }

  .ring {
    position: fixed;
    top: 0;
    left: 0;
    z-index: 49;
    border: 2px solid var(--color-tour);
    pointer-events: none;
    opacity: 0;
    transition: opacity 200ms ease;
  }
  .ring.shown {
    opacity: 1;
  }

  .pop {
    position: fixed;
    top: 0;
    left: 0;
    z-index: 50;
    width: max-content;
    max-width: min(20rem, calc(100vw - 16px));
    padding: 0.75rem 0.95rem 0.8rem;
    background: var(--color-tour);
    color: var(--color-app);
    box-shadow: 0 10px 40px var(--color-app);
    user-select: none;
    /* Hidden until measured; then it glides from one step's place to the next. */
    visibility: hidden;
  }
  .pop.ready {
    visibility: visible;
    transition: transform 320ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  /* The tail: a small square on its corner, half under the edge that faces the target. */
  .tail {
    position: absolute;
    width: 0.7rem;
    height: 0.7rem;
    background: var(--color-tour);
    transform: translate(-50%, -50%) rotate(45deg);
  }
  .side-bottom .tail { left: var(--tx); top: 0; }
  .side-top .tail { left: var(--tx); top: 100%; }
  .side-right .tail { left: 0; top: var(--ty); }
  .side-left .tail { left: 100%; top: var(--ty); }

  /* Each step's words rise in; the box around them only moves. */
  .words {
    position: relative;
    animation: words-in 260ms cubic-bezier(0.22, 1, 0.36, 1) both;
  }
  @keyframes words-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
  .count {
    font-size: 0.62rem;
    font-weight: 800;
    letter-spacing: 0.2em;
  }
  .title {
    margin-top: 0.15rem;
    font-size: 1.2rem;
    font-weight: 800;
    line-height: 1.1;
    letter-spacing: -0.01em;
  }
  .text {
    margin-top: 0.3rem;
    font-size: 0.88rem;
    font-weight: 500;
    line-height: 1.4;
  }

  .skip,
  .arrow {
    position: fixed;
    z-index: 51;
    background: var(--color-tour);
    color: var(--color-app);
    box-shadow: 0 6px 24px var(--color-app);
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease, opacity 150ms ease;
  }
  .skip {
    top: 2.2rem;
    right: 0.75rem;
    padding: 0.5rem 0.9rem;
    font-size: 0.68rem;
    font-weight: 800;
    letter-spacing: 0.18em;
    text-transform: uppercase;
  }
  /* Big enough to hit without aiming, and they never move: you can step through without looking. */
  .arrow {
    top: 50%;
    display: grid;
    place-items: center;
    width: 3.4rem;
    height: 3.4rem;
    margin-top: -1.7rem;
    border-radius: 999px;
  }
  .arrow-back {
    left: 0.75rem;
  }
  .arrow-next {
    right: 0.75rem;
  }
  .skip:hover,
  .skip:focus-visible,
  .arrow:hover:not(:disabled),
  .arrow:focus-visible {
    background: var(--color-white);
    transform: scale(1.06);
  }
  .skip:active,
  .arrow:active:not(:disabled) {
    transform: scale(0.94);
  }
  .arrow:disabled {
    opacity: 0.4;
  }
  .skip:focus-visible,
  .arrow:focus-visible {
    outline: 2px solid var(--color-white);
    outline-offset: 3px;
  }

  @media (max-height: 640px), (max-width: 480px) {
    .arrow {
      width: 2.6rem;
      height: 2.6rem;
      margin-top: -1.3rem;
    }
    .arrow-back {
      left: 0.3rem;
    }
    .arrow-next,
    .skip {
      right: 0.3rem;
    }
    .skip {
      top: 1.8rem;
      padding: 0.3rem 0.6rem;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .ring,
    .pop.ready,
    .arrow,
    .skip {
      transition: none;
    }
    .words,
    .veil {
      animation: none;
    }
  }
</style>
