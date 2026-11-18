<script>
  /**
   * The tour as a stepper: one pop-up at a time, pointing at a real part of the page (or sitting
   * in the middle when it is about the whole thing), with a back and a next button at mid-screen.
   * Arrow keys step too, and Escape skips. Each step's target is ringed, and measured again every
   * so often so the pop-up follows a layout that changes. Words: tour.json. Where it goes: place.js.
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
    return place(rect, { w, h }, step.side ?? DEFAULT_SIDE, step.at ?? DEFAULT_AT, bounds);
  });

  function measure() {
    const next = { w: window.innerWidth, h: window.innerHeight };
    if (next.w !== bounds.w || next.h !== bounds.h) bounds = next;
    const box = step.target ? document.querySelector(`[data-tour="${step.target}"]`)?.getBoundingClientRect() : null;
    const found = box && onScreen(box, next) ? { left: box.left, top: box.top, right: box.right, bottom: box.bottom, width: box.width, height: box.height } : null;
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
  <button type="button" class="skip" onclick={() => tour.finish()}>{copy.skip}</button>
</section>

<!-- BACK and NEXT: mid-screen, either side of the middle, always in the same place. -->
<button type="button" class="arrow arrow-back" onclick={() => tour.back()} disabled={tour.at === 0} aria-label={copy.back}>
  <ArrowLeft size={22} strokeWidth={2.5} aria-hidden="true" />
</button>
<button type="button" class="arrow arrow-next" onclick={() => tour.next()} aria-label={last ? copy.done : copy.next}>
  {#if last}<Check size={22} strokeWidth={2.5} aria-hidden="true" />{:else}<ArrowRight size={22} strokeWidth={2.5} aria-hidden="true" />{/if}
</button>

<style>
  .ring {
    position: fixed;
    top: 0;
    left: 0;
    z-index: 49;
    border: 1px solid var(--color-neon-red);
    box-shadow: 0 0 0 4px var(--color-neon-red-faint);
    pointer-events: none;
    opacity: 0;
    transition: transform 320ms cubic-bezier(0.22, 1, 0.36, 1), width 320ms cubic-bezier(0.22, 1, 0.36, 1),
      height 320ms cubic-bezier(0.22, 1, 0.36, 1), opacity 200ms ease;
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
    padding: 0.7rem 0.9rem 0.5rem;
    border: 1px solid var(--color-neon-red);
    background: var(--color-card);
    box-shadow: 5px 5px 0 var(--color-neon-red-faint);
    color: var(--color-white);
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
    width: 0.6rem;
    height: 0.6rem;
    border: solid var(--color-neon-red);
    background: var(--color-card);
    transform: translate(-50%, -50%) rotate(45deg);
  }
  .side-bottom .tail { left: var(--tx); top: 0; border-width: 1px 0 0 1px; }
  .side-top .tail { left: var(--tx); top: 100%; border-width: 0 1px 1px 0; }
  .side-right .tail { left: 0; top: var(--ty); border-width: 0 0 1px 1px; }
  .side-left .tail { left: 100%; top: var(--ty); border-width: 1px 1px 0 0; }

  /* Each step's words rise in; the box around them only moves. */
  .words {
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
    font-weight: 700;
    letter-spacing: 0.2em;
    color: var(--color-neon-red);
  }
  .title {
    margin-top: 0.15rem;
    font-size: 1.15rem;
    font-weight: 800;
    line-height: 1.1;
    letter-spacing: -0.01em;
  }
  .text {
    margin-top: 0.3rem;
    font-size: 0.85rem;
    line-height: 1.4;
  }
  .skip {
    margin-top: 0.4rem;
    font-size: 0.6rem;
    font-weight: 700;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--color-muted);
    transition: color 150ms ease;
  }
  .skip:hover,
  .skip:focus-visible {
    color: var(--color-white);
  }

  /* Big enough to hit without aiming, and they never move: you can step through without looking. */
  .arrow {
    position: fixed;
    top: 50%;
    z-index: 51;
    display: grid;
    place-items: center;
    width: 3.2rem;
    height: 3.2rem;
    margin-top: -1.6rem;
    border: 1px solid var(--color-neon-red);
    border-radius: 999px;
    background: var(--color-neon-red);
    color: var(--color-white);
    box-shadow: 0 6px 24px var(--color-app);
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease, color 150ms ease, opacity 150ms ease;
  }
  .arrow-back {
    left: max(0.5rem, calc(50% - 17rem));
    background: var(--color-card);
  }
  .arrow-next {
    right: max(0.5rem, calc(50% - 17rem));
  }
  .arrow:hover:not(:disabled),
  .arrow:focus-visible {
    background: var(--color-white);
    color: var(--color-app);
    transform: scale(1.08);
  }
  .arrow:active:not(:disabled) {
    transform: scale(0.94);
  }
  .arrow:disabled {
    opacity: 0.35;
  }
  .arrow:focus-visible,
  .skip:focus-visible {
    outline: 2px solid var(--color-white);
    outline-offset: 3px;
  }

  @media (max-height: 640px) {
    .arrow {
      width: 2.4rem;
      height: 2.4rem;
      margin-top: -1.2rem;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .ring,
    .pop.ready,
    .arrow,
    .skip {
      transition: none;
    }
    .words {
      animation: none;
    }
  }
</style>
