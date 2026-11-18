<script>
  /**
   * The big cards of a first visit, one at a time, docked low on the screen with the page still
   * showing behind: what Money 24 is, that this is a snapshot, how to get around. Dots, Skip,
   * Next; a finger swipes between them and the arrow keys step. Text lives in tour.json.
   */
  import { onMount } from 'svelte';
  import { fly, scale } from 'svelte/transition';
  import { backOut, cubicOut } from 'svelte/easing';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import copy from './tour.json';
  import { tour } from './tour.svelte.js';
  import { demoFirst, loadDemo } from '../demo.js';

  const SNAPSHOT_FILE = 'markets/index.json'; // the smallest recorded file that carries the snapshot's time
  const SWIPE_PX = 40; // a sideways drag longer than this steps a card
  const FLY_PX = 36;
  const FLY_MS = 260;
  const IN_MS = 320;
  const OUT_MS = 160;
  const POP_FROM = 0.9;
  const PAD = 2;
  /** @type {Intl.DateTimeFormatOptions} */
  const SNAPSHOT_DATE = { month: 'long', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' };

  const demo = demoFirst();
  const cards = copy.cards.filter((c) => !c.demoOnly || demo);

  let at = $state(0);
  let dir = $state(1); // which way the last step went, so the card flies in from that side
  let takenAt = $state('');
  /** @type {HTMLElement | undefined} */
  let card = $state();
  let dragFrom = 0;

  const current = $derived(cards[at]);
  const last = $derived(at >= cards.length - 1);
  const text = $derived(current.textPending && !takenAt
    ? current.textPending
    : current.text.replace('{when}', new Date(takenAt).toLocaleString(undefined, SNAPSHOT_DATE)));

  function go(/** @type {number} */ i) {
    if (i < 0 || i >= cards.length || i === at) return;
    dir = i > at ? 1 : -1;
    at = i;
  }

  function next() {
    if (last) tour.endCarousel();
    else go(at + 1);
  }

  function onKey(/** @type {KeyboardEvent} */ e) {
    if (e.key === 'ArrowRight') go(at + 1);
    else if (e.key === 'ArrowLeft') go(at - 1);
    else if (e.key === 'Escape') tour.endCarousel();
    else return;
    e.preventDefault();
  }

  function onPointerUp(/** @type {PointerEvent} */ e) {
    const moved = e.clientX - dragFrom;
    if (Math.abs(moved) > SWIPE_PX) go(at + (moved < 0 ? 1 : -1));
  }

  onMount(() => {
    // The arrows work straight away, and a screen reader lands on the first card.
    card?.focus();
    if (demo) loadDemo(SNAPSHOT_FILE).then((index) => { takenAt = index.taken_at; }).catch(() => {});
  });
</script>

<div class="dock">
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
  <section
    bind:this={card}
    class="card"
    tabindex="-1"
    aria-roledescription="carousel"
    aria-label={copy.label}
    onkeydown={onKey}
    onpointerdown={(e) => { dragFrom = e.clientX; }}
    onpointerup={onPointerUp}
    in:scale={{ duration: IN_MS, start: POP_FROM, easing: backOut }}
    out:scale={{ duration: OUT_MS, start: POP_FROM }}
  >
    <div class="body">
      {#key current.id}
        <div in:fly={{ x: FLY_PX * dir, duration: FLY_MS, easing: cubicOut }} aria-live="polite">
          <p class="kicker">
            <span class="count">{String(at + 1).padStart(PAD, '0')}</span> / {String(cards.length).padStart(PAD, '0')} · {current.kicker}
          </p>
          <h2 class="title">{current.title}</h2>
          <p class="text">{text}</p>
        </div>
      {/key}
    </div>

    <div class="controls">
      <button type="button" class="skip" onclick={() => tour.endCarousel()}>{copy.skip}</button>
      <div class="dots" role="tablist" aria-label={copy.label}>
        {#each cards as c, i (c.id)}
          <button type="button" class="dot" class:on={i === at} role="tab" aria-selected={i === at}
            aria-label={copy.card.replace('{n}', String(i + 1)).replace('{of}', String(cards.length))} onclick={() => go(i)}></button>
        {/each}
      </div>
      <button type="button" class="next" onclick={next}>
        {last ? copy.go : copy.next}<ArrowRight size={14} strokeWidth={2.5} aria-hidden="true" />
      </button>
    </div>
  </section>
</div>

<style>
  /* Low and centred, clear of the footer line and a phone's bottom tabs. */
  .dock {
    position: fixed;
    left: 50%;
    bottom: 4.5rem;
    z-index: 60;
    width: min(34rem, calc(100vw - 1rem));
    transform: translateX(-50%);
  }
  .card {
    border: 1px solid var(--color-neon-red);
    background: var(--color-card);
    box-shadow: 6px 6px 0 var(--color-neon-red-faint);
    color: var(--color-white);
    outline: none;
    touch-action: pan-y;
    user-select: none;
  }
  /* The tallest card sets the height, so stepping between cards moves nothing around them. */
  .body {
    display: grid;
    min-height: 9.5rem;
    padding: 1rem 1.2rem 0.6rem;
    overflow: hidden;
  }
  .kicker {
    font-size: 0.62rem;
    font-weight: 700;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: var(--color-muted);
  }
  .count {
    color: var(--color-neon-red);
  }
  .title {
    margin-top: 0.3rem;
    font-size: clamp(1.5rem, 6vw, 2.2rem);
    font-weight: 800;
    line-height: 1;
    letter-spacing: -0.02em;
  }
  .text {
    margin-top: 0.5rem;
    font-size: 0.9rem;
    line-height: 1.45;
    color: var(--color-white);
  }

  .controls {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.5rem 0.6rem 0.6rem 1.2rem;
    border-top: 1px solid var(--color-line);
  }
  .skip,
  .next {
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), color 150ms ease, background-color 150ms ease;
  }
  .skip {
    color: var(--color-muted);
  }
  .skip:hover,
  .skip:focus-visible {
    color: var(--color-white);
  }
  .next {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.5rem 0.9rem;
    background: var(--color-neon-red);
    color: var(--color-white);
  }
  .next:hover,
  .next:focus-visible {
    background: var(--color-white);
    color: var(--color-app);
  }
  .next:active,
  .skip:active {
    transform: scale(0.96);
  }
  .dots {
    display: flex;
    gap: 0.4rem;
  }
  .dot {
    width: 1.4rem;
    height: 0.2rem;
    background: var(--color-line);
    transform-origin: left;
    transition: transform 220ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease;
  }
  .dot.on {
    background: var(--color-neon-red);
    transform: scaleY(2);
  }
  .skip:focus-visible,
  .next:focus-visible,
  .dot:focus-visible {
    outline: 1px solid var(--color-white);
    outline-offset: 3px;
  }

  /* Squat windows: the card sits at the very bottom and tightens. */
  @media (max-height: 640px) {
    .dock {
      bottom: 0.5rem;
    }
    .body {
      min-height: 0;
      padding: 0.5rem 0.8rem 0.3rem;
    }
    .title {
      font-size: 1.2rem;
    }
    .text {
      margin-top: 0.2rem;
      font-size: 0.78rem;
      line-height: 1.3;
    }
    .controls {
      padding: 0.3rem 0.4rem 0.3rem 0.8rem;
    }
    .next {
      padding: 0.3rem 0.6rem;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .skip,
    .next,
    .dot {
      transition: none;
    }
  }
</style>
