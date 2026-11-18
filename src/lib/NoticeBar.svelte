<script>
  /**
   * The work-in-progress notice every Money 24 page wears: a slim hazard strip whose text runs like
   * a ticker, and the full notice in a dialog. The dialog opens by itself on a device's first visit
   * and again from the strip. On the demo both say so. Text lives in notice.json.
   */
  import { onMount } from 'svelte';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import notice from './notice.json';
  import nav from './app-nav.json';
  import { demoFirst, demoHref } from './demo.js';
  import { notice as noticeState, NOTICE_VERSION, STORE_ACK } from './notice-state.svelte.js';

  const TICKER_COPIES = [0, 1]; // the text runs twice in a row, so the loop has no seam
  const TITLE_ID = 'notice-title';

  /** @type {{ overlay?: boolean }} overlay: laid over the page top without taking space (the landing page) */
  let { overlay = false } = $props();

  /** The landing page is not the demo itself, so its strip only says work in progress. */
  const demo = $derived(!overlay && demoFirst());
  /** @type {HTMLDialogElement | undefined} */
  let dialog = $state();

  function open() {
    if (dialog?.open) return;
    // Firefox 97 has no <dialog>: the strip still carries every word, and nothing waits on it.
    if (!dialog?.showModal) {
      noticeState.open = false;
      return;
    }
    dialog.showModal();
    noticeState.open = true;
  }

  function acknowledge() {
    noticeState.open = false;
    try {
      localStorage.setItem(STORE_ACK, NOTICE_VERSION);
    } catch {
      // Not persisted on this device: the notice opens again on the next page.
    }
  }

  // A device that has not acknowledged the notice gets it opened once, on arrival.
  onMount(() => {
    if (noticeState.open) open();
  });
</script>

<aside aria-label={notice.label} data-tour="notice" class="notice bg-warn text-app {overlay ? 'fixed inset-x-0 top-0 z-40' : 'relative'}">
  <button type="button" onclick={open} aria-haspopup="dialog"
    class="notice-button flex h-6 w-full items-center text-left text-[0.62rem] leading-none font-extrabold tracking-[0.16em] uppercase short:h-5 focus-visible:outline-none tv:h-10 tv:text-sm">
    <span class="hazard h-full w-6 shrink-0 max-[359px]:hidden" aria-hidden="true"></span>
    <span class="flex shrink-0 items-center gap-1.5 px-2 whitespace-nowrap">
      <TriangleAlert class="size-3.5 shrink-0 tv:size-5" aria-hidden="true" />
      {#if demo}<span>{notice.demo}</span><span aria-hidden="true">·</span>{/if}
      <span>{notice.wip}</span>
    </span>
    <span class="sr-only">{notice.ticker.join('. ')}. {notice.open}</span>
    <span class="notice-window min-w-0 flex-1 overflow-hidden border-l border-app/30" aria-hidden="true">
      <span class="notice-track flex w-max">
        {#each TICKER_COPIES as copy (copy)}
          <span class="flex shrink-0">
            {#each notice.ticker as line (line)}
              <span class="px-3 whitespace-nowrap">{line}</span><span class="opacity-50">/</span>
            {/each}
          </span>
        {/each}
      </span>
    </span>
    <span class="hazard h-full w-6 shrink-0 max-[359px]:hidden" aria-hidden="true"></span>
  </button>
</aside>

<dialog bind:this={dialog} aria-labelledby={TITLE_ID} onclose={acknowledge}
  class="notice-dialog m-auto max-h-[calc(100dvh-1rem)] w-[min(46rem,calc(100vw-1rem))] overflow-y-auto border border-warn bg-app text-white select-text backdrop:bg-app/85">
  <div class="hazard h-3 short:h-1.5" aria-hidden="true"></div>
  <div class="px-5 py-6 short:py-3 sm:px-9 sm:py-8">
    <p class="rise text-[0.7rem] font-bold tracking-[0.4em] text-warn uppercase" style:--i="0">{notice.dialog.kicker}</p>
    <h2 id={TITLE_ID} class="rise notice-title mt-2 leading-[0.9] font-extrabold tracking-tight text-white uppercase" style:--i="1">{notice.dialog.title}</h2>
    {#if demo}
      <p class="rise mt-4 inline-block border border-paper px-2 py-0.5 text-xs font-bold tracking-[0.2em] text-paper uppercase short:mt-2" style:--i="2">{notice.demo}</p>
      <p class="rise mt-2 text-sm leading-snug text-white/80" style:--i="2">{notice.dialog.demoLine}</p>
    {/if}
    <ul class="mt-5 grid gap-3 max-sm:gap-2 short:mt-3 short:gap-2">
      {#each notice.dialog.points as point, i (point.lead)}
        <li class="rise border-l-2 border-warn pl-3" style:--i={i + 3}>
          <p class="text-base leading-snug font-bold text-white short:text-sm">{point.lead}</p>
          <p class="text-sm leading-snug text-white/70 max-sm:hidden">{point.text}</p>
        </li>
      {/each}
    </ul>
    <div class="rise mt-6 flex flex-wrap items-center justify-between gap-3 short:mt-3" style:--i={notice.dialog.points.length + 3}>
      <a href={demoHref(nav.terms.href)} class="notice-link text-sm font-semibold text-white underline decoration-warn underline-offset-4">{notice.dialog.terms}</a>
      <form method="dialog">
        <!-- svelte-ignore a11y_autofocus -->
        <button autofocus class="notice-accept bg-warn px-5 py-2.5 text-sm font-extrabold tracking-[0.18em] text-app uppercase short:py-1.5">{notice.dialog.accept}</button>
      </form>
    </div>
  </div>
  <div class="hazard h-3 short:h-1.5" aria-hidden="true"></div>
</dialog>

<style>
  /* Hazard stripes: the strip's two ends and the dialog's top and bottom edges. */
  .hazard {
    background: var(--bg-hazard);
  }
  .notice-button:focus-visible .notice-window,
  .notice-button:hover .notice-window {
    background: var(--color-white);
  }
  .notice-window {
    transition: background-color 150ms ease;
  }

  /* The ticker: two copies of the text, moved left by exactly one of them. Transform only. */
  .notice-track {
    animation: notice-run 46s linear infinite;
  }
  .notice-button:hover .notice-track,
  .notice-button:focus-visible .notice-track {
    animation-play-state: paused;
  }
  @keyframes notice-run {
    to {
      transform: translateX(-50%);
    }
  }

  .notice-title {
    font-size: clamp(2.4rem, 13vw, 6.5rem);
  }
  .notice-dialog {
    touch-action: pan-y;
    scrollbar-width: thin;
    scrollbar-color: var(--color-warn) transparent;
  }
  .notice-dialog[open] {
    animation: notice-in 260ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }
  .notice-dialog[open]::backdrop {
    animation: notice-fade 200ms ease-out;
  }
  @keyframes notice-in {
    from {
      opacity: 0;
      transform: translateY(14px) scale(0.97);
    }
  }
  @keyframes notice-fade {
    from {
      opacity: 0;
    }
  }
  /* Each line of the notice rises in after the one above it. */
  .notice-dialog[open] .rise {
    animation: notice-rise 420ms cubic-bezier(0.22, 1, 0.36, 1) calc(120ms + var(--i, 0) * 55ms) both;
  }
  @keyframes notice-rise {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
  }

  .notice-accept,
  .notice-link {
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease, color 150ms ease;
  }
  .notice-accept:hover,
  .notice-accept:focus-visible {
    background: var(--color-white);
  }
  .notice-accept:active {
    transform: scale(0.96);
  }
  .notice-accept:focus-visible,
  .notice-link:focus-visible {
    outline: 2px solid var(--color-white);
    outline-offset: 3px;
  }
  .notice-link:hover {
    color: var(--color-warn);
  }

  @media (prefers-reduced-motion: reduce) {
    .notice-track,
    .notice-dialog[open],
    .notice-dialog[open]::backdrop,
    .notice-dialog[open] .rise {
      animation: none;
    }
    /* Standing still, the ticker shows what fits; the dialog has every word. */
    .notice-window {
      text-overflow: ellipsis;
    }
  }
</style>
