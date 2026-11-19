<script>
  /**
   * The quote ribbon: a slim stripe of indexes, crypto, commodities and the symbols of open plays
   * that runs sideways without stopping. Hovering it or tabbing into it holds it still. The pause
   * button, an arrow, the wheel or a drag turns it into a row to move through by hand, at the very
   * place it was; play sets it running again from there. With reduced motion it never runs: it is
   * that row from the start. Each quote opens its symbol on the Markets page.
   * The arithmetic is ribbon-motion.js; the run itself is CSS (home.css).
   */
  import { flushSync } from 'svelte';
  import strings from './strings.json';
  import ribbon from './ribbon.json';
  import { fill } from '../plays/play-format.js';
  import { percent, price } from '../markets/market-format.js';
  import { MARKETS_PAGE, SYMBOL_PARAM, pageLink } from './home-links.js';
  import { copiesFor, loopSeconds, mouseDrag, resumeDelay, stepTo, wrap } from './ribbon-motion.js';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Pause from '@lucide/svelte/icons/pause';
  import Play from '@lucide/svelte/icons/play';

  const REDUCED_QUERY = '(prefers-reduced-motion: reduce)';
  const WHEEL_LINE_PX = 16; // browsers that report the wheel in lines
  const WHEEL_LINES = 1;
  const BACK = -1;
  const FORWARD = 1;
  const UP = '▲';
  const DOWN = '▼';
  const NO_DELAY = '0ms';
  const CONTROL = 'spring grid w-7 place-items-center text-white/60 hover:bg-white/5 hover:text-white focus-visible:bg-white/5 focus-visible:text-white focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red tv:w-12';

  /** @type {{ quotes: import('./home-client.svelte.js').Tick[] }} */
  let { quotes } = $props();

  /** @type {HTMLElement | undefined} */
  let view = $state();
  /** @type {HTMLElement | undefined} */
  let track = $state();
  let viewWidth = $state(0);
  let copyWidth = $state(0);
  let reduced = $state(matchMedia(REDUCED_QUERY).matches);
  /** Held still by the reader: a row to move through by hand. */
  let held = $state(false);
  /** Where in its loop the run starts, so it picks up where the row was held. */
  let delay = $state(NO_DELAY);

  const running = $derived(!held && !reduced && copyWidth > 0);
  const copies = $derived(Array.from({ length: copiesFor(viewWidth, copyWidth) }, (_, i) => i));

  $effect(() => {
    const media = matchMedia(REDUCED_QUERY);
    const sync = () => { reduced = media.matches; };
    media.addEventListener('change', sync);
    return () => media.removeEventListener('change', sync);
  });

  // The view and one copy of the list are measured whenever either changes size.
  $effect(() => {
    if (!view || !track) return;
    const first = track.firstElementChild;
    const measure = () => {
      viewWidth = view?.clientWidth ?? 0;
      copyWidth = first?.getBoundingClientRect().width ?? 0;
    };
    measure();
    const watcher = new ResizeObserver(measure);
    watcher.observe(view);
    if (first) watcher.observe(first);
    return () => watcher.disconnect();
  });

  /** Stops the run where it is: the row is now scrolled to the place the run had reached. */
  function hold() {
    if (!running || !view || !track) return;
    const reached = -new DOMMatrixReadOnly(getComputedStyle(track).transform).m41;
    flushSync(() => { held = true; });
    view.scrollLeft = reached;
  }

  function resume() {
    if (!view) return;
    delay = resumeDelay(view.scrollLeft, copyWidth);
    view.scrollLeft = 0;
    held = false;
  }

  function step(/** @type {number} */ direction) {
    if (!view) return;
    hold();
    const { from, to } = stepTo(view.scrollLeft, direction, viewWidth, copyWidth);
    view.scrollLeft = from;
    view.scrollTo({ left: to, behavior: reduced ? 'auto' : 'smooth' });
  }

  function onWheel(/** @type {WheelEvent} */ event) {
    if (!view) return;
    const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
    if (!delta) return;
    event.preventDefault();
    hold();
    view.scrollLeft = wrap(view.scrollLeft + delta * (event.deltaMode === WHEEL_LINES ? WHEEL_LINE_PX : 1), copyWidth);
  }

  /** A mouse drag: it holds the ribbon where it is, then the row follows the mouse. */
  const dragRow = {
    grab() {
      hold();
      return view?.scrollLeft ?? 0;
    },
    move(/** @type {number} */ offset) {
      if (view) view.scrollLeft = wrap(offset, copyWidth);
    },
  };

  /** Tabbing through the running ribbon can leave it scrolled: put it back when focus leaves. */
  function onFocusOut(/** @type {FocusEvent} */ event) {
    if (!running || !view || view.contains(/** @type {Node | null} */ (event.relatedTarget))) return;
    view.scrollLeft = 0;
  }
</script>

<section aria-label={strings.ribbon.label} class="flex h-8 shrink-0 items-stretch border-b border-line select-none short:h-7 tv:h-12">
  <div bind:this={view} class="ribbon-view min-w-0 flex-1 overflow-y-hidden {running ? 'overflow-x-hidden' : 'overflow-x-auto'}"
    use:mouseDrag={dragRow} onwheel={onWheel} onfocusout={onFocusOut}>
    {#if quotes.length}
      <div bind:this={track} class="flex h-full w-max {running ? 'ribbon-run' : ''}"
        style:--ribbon-shift="-{copyWidth}px" style:animation-duration="{loopSeconds(copyWidth)}s" style:animation-delay={delay}>
        {#each copies as copy (copy)}
          <!-- Only the first copy is read out and tabbed through; the others are the same quotes again. -->
          <ul class="flex shrink-0 items-stretch" aria-hidden={copy ? 'true' : undefined}>
            {#each quotes as q (q.symbol)}
              {@const name = ribbon.names[q.symbol] ?? q.symbol}
              {@const down = q.change_pct < 0}
              <li class="flex">
                <a href={pageLink(MARKETS_PAGE, { [SYMBOL_PARAM]: q.symbol })} draggable="false" tabindex={copy ? -1 : undefined}
                  title={fill(strings.ribbon.open, { name: q.name || q.symbol })}
                  class="spring flex items-center gap-2 border-r border-line px-3 font-mono text-xs whitespace-nowrap tabular-nums hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red tv:px-5 tv:text-base">
                  <span class="font-sans font-bold tracking-wide text-white">{name}</span>
                  <span class="text-white/80">{price(q.price)}</span>
                  <span class={down ? 'text-bad' : 'text-ok'}>{down ? DOWN : UP} {percent(q.change_pct)}</span>
                </a>
              </li>
            {/each}
          </ul>
        {/each}
      </div>
    {:else}
      <p class="flex h-full items-center px-3 text-xs text-muted">{strings.ribbon.empty}</p>
    {/if}
  </div>

  <div class="flex shrink-0 items-stretch border-l border-line">
    <button type="button" class={CONTROL} aria-label={strings.ribbon.back} title={strings.ribbon.back} onclick={() => step(BACK)}>
      <ChevronLeft class="size-4 tv:size-6" aria-hidden="true" />
    </button>
    <button type="button" class={CONTROL} aria-label={strings.ribbon.forward} title={strings.ribbon.forward} onclick={() => step(FORWARD)}>
      <ChevronRight class="size-4 tv:size-6" aria-hidden="true" />
    </button>
    <!-- With reduced motion the ribbon never runs, so there is nothing to pause. -->
    {#if !reduced}
      <button type="button" class={CONTROL} aria-label={held ? strings.ribbon.play : strings.ribbon.pause} title={held ? strings.ribbon.play : strings.ribbon.pause} onclick={() => (held ? resume() : hold())}>
        {#if held}<Play class="size-3.5 tv:size-5" aria-hidden="true" />{:else}<Pause class="size-3.5 tv:size-5" aria-hidden="true" />{/if}
      </button>
    {/if}
  </div>
</section>
