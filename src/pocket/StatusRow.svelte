<script>
  /**
   * One line of the pocket: a dot for its state, its name and state word, and a detail line under
   * them. Both lines always render (an empty detail keeps its height), so a row never changes size
   * between reads. aside: what sits on the right, such as a desk's P&L.
   */
  import { TONE_CLASSES } from './tones.js';

  const STAGGER_MAX = 12;
  const EMPTY = ' ';

  /** @type {{ row: import('./pocket-model.js').Row, index: number, aside?: import('svelte').Snippet }} */
  let { row, index, aside } = $props();
</script>

<li class="cal-rise grid grid-cols-[auto_minmax(0,1fr)_auto] items-baseline gap-x-2 px-3 py-1 tv:py-2" style:--i={Math.min(index, STAGGER_MAX)} title={row.detail || row.name}>
  <span class="size-1.5 shrink-0 translate-y-[-0.1em] rounded-full {TONE_CLASSES[row.tone].dot}" aria-hidden="true"></span>
  <span class="min-w-0">
    <span class="flex min-w-0 items-baseline gap-2">
      <span class="shrink-0 text-[0.8rem] font-semibold text-white tv:text-lg">{row.name}</span>
      <span class="truncate font-mono text-[0.66rem] tv:text-sm {TONE_CLASSES[row.tone].text}">{row.state}</span>
    </span>
    <span class="block truncate font-mono text-[0.64rem] text-muted tabular-nums tv:text-sm">{row.detail || EMPTY}</span>
  </span>
  {#if aside}{@render aside()}{/if}
</li>
