<script>
  /**
   * One open position in the pocket, on one line so the whole list fits: which desk holds it, the
   * question in a few words (the full question on hover), how long until it closes, its mark, and its
   * open result when there is one. P&L always carries its sign, not color alone.
   */
  import { pnl } from '../plays/play-format.js';

  const STAGGER_MAX = 12;
  const CENT = 0.01; // an open result under a cent is not worth the room

  /** @type {{ play: import('./pocket-model.js').PlayRow, index: number }} */
  let { play, index } = $props();

  const result = $derived(play.pnl !== null && Math.abs(play.pnl) >= CENT ? play.pnl : null);
</script>

<li class="cal-rise grid grid-cols-[auto_minmax(0,1fr)_auto_auto] items-baseline gap-x-2 px-3 py-[0.2rem] tv:py-1.5" style:--i={Math.min(index, STAGGER_MAX)}
  title={[play.full || play.title, play.closes].filter(Boolean).join(' · ')}>
  <span class="rounded-sm border border-paper/60 px-1 text-[0.5rem] font-bold tracking-wider text-paper uppercase tv:text-xs">{play.desk}</span>
  <span class="truncate text-[0.74rem] font-semibold text-white tv:text-lg">{play.title}</span>
  <span class="font-mono text-[0.6rem] text-muted tabular-nums tv:text-sm">{play.soon}</span>
  <span class="text-right font-mono text-[0.74rem] font-bold tabular-nums tv:text-lg">
    <span class="text-paper">{play.mark}</span>{#if result !== null}<span class="ml-1 text-[0.6rem] {result < 0 ? 'text-bad' : 'text-ok'}">{pnl(result)}</span>{/if}
  </span>
</li>
