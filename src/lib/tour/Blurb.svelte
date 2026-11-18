<script>
  /**
   * One tour bubble, pointing at a part of the page: a black card on a red line with a tail to
   * the thing it talks about, one or two sentences, and an X. Where it goes is Blurbs.svelte's
   * call (through place.js), once this bubble has reported its size. It pops in from its tail.
   */
  import { untrack } from 'svelte';
  import { scale } from 'svelte/transition';
  import X from '@lucide/svelte/icons/x';
  import copy from './tour.json';

  const FIRST_MS = 200; // before the first bubble
  const STAGGER_MS = 240; // between one bubble and the next
  const LEAVE_MS = 140;
  const LEAVE_SCALE = 0.85;

  /**
   * @type {{
   *   blurb: { id: string, text: string },
   *   pos: { left: number, top: number, side: string, tx: number, ty: number },
   *   index?: number,
   *   onsize: (w: number, h: number) => void,
   *   onclose: () => void,
   *   onskip: () => void,
   * }} index: its place in the stagger. onsize: its size, whenever that changes.
   */
  let { blurb, pos, index = 0, onsize, onclose, onskip } = $props();

  let w = $state(0);
  let h = $state(0);

  // Only the size is watched: the callback itself changes whenever the parent places again.
  $effect(() => {
    const size = [w, h];
    untrack(() => onsize(size[0], size[1]));
  });
</script>

<div
  class="blurb side-{pos.side}"
  class:ready={w > 0}
  bind:offsetWidth={w}
  bind:offsetHeight={h}
  style:left="{pos.left}px"
  style:top="{pos.top}px"
  style:--tx="{pos.tx}px"
  style:--ty="{pos.ty}px"
  style:--delay="{FIRST_MS + index * STAGGER_MS}ms"
  out:scale={{ duration: LEAVE_MS, start: LEAVE_SCALE }}
  role="note"
>
  <span class="tail" aria-hidden="true"></span>
  <p class="text">{blurb.text}</p>
  <button type="button" class="skip" onclick={onskip}>{copy.skipTour}</button>
  <button type="button" class="close" onclick={onclose} aria-label={copy.close}>
    <X size={13} strokeWidth={2.5} aria-hidden="true" />
  </button>
</div>

<style>
  .blurb {
    position: fixed;
    z-index: 50;
    width: max-content;
    max-width: min(17rem, calc(100vw - 16px));
    padding: 0.6rem 2rem 0.45rem 0.8rem;
    border: 1px solid var(--color-neon-red);
    background: var(--color-card);
    box-shadow: 4px 4px 0 var(--color-neon-red-faint);
    color: var(--color-white);
    font-size: 0.8rem;
    line-height: 1.4;
    user-select: none;
    /* Hidden until measured and placed, then it pops out of its tail. */
    visibility: hidden;
  }
  .blurb.ready {
    visibility: visible;
    transform-origin: var(--tx) var(--ty);
    animation: blurb-in 360ms cubic-bezier(0.34, 1.56, 0.64, 1) var(--delay) both;
  }
  @keyframes blurb-in {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }

  /* The tail: a small square on its corner, half under the card's edge, on the side that faces the target. */
  .tail {
    position: absolute;
    width: 0.6rem;
    height: 0.6rem;
    border: solid var(--color-neon-red);
    background: var(--color-card);
    transform: translate(-50%, -50%) rotate(45deg);
  }
  .side-bottom .tail {
    left: var(--tx);
    top: 0;
    border-width: 1px 0 0 1px;
  }
  .side-top .tail {
    left: var(--tx);
    top: 100%;
    border-width: 0 1px 1px 0;
  }
  .side-right .tail {
    left: 0;
    top: var(--ty);
    border-width: 0 0 1px 1px;
  }
  .side-left .tail {
    left: 100%;
    top: var(--ty);
    border-width: 1px 1px 0 0;
  }

  .text {
    margin: 0;
  }
  .skip {
    margin-top: 0.3rem;
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
  .close {
    position: absolute;
    top: 0.3rem;
    right: 0.3rem;
    display: grid;
    place-items: center;
    width: 1.4rem;
    height: 1.4rem;
    color: var(--color-muted);
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), color 150ms ease, background-color 150ms ease;
  }
  .close:hover,
  .close:focus-visible {
    color: var(--color-white);
    background: var(--color-neon-red);
  }
  .close:active {
    transform: scale(0.9);
  }
  .skip:focus-visible,
  .close:focus-visible {
    outline: 1px solid var(--color-white);
    outline-offset: 2px;
  }

  @media (prefers-reduced-motion: reduce) {
    .blurb.ready {
      animation: none;
    }
  }
</style>
