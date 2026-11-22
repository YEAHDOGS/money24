<script>
  /**
   * One open position in the pocket: which desk holds it, what it is and when it closes, then its
   * mark and, where the desk gives one, its open result. P&L always carries its sign, not color alone.
   */
  import { pnl } from '../plays/play-format.js';

  const STAGGER_MAX = 12;

  /** @type {{ play: import('./pocket-model.js').PlayRow, index: number }} */
  let { play, index } = $props();
</script>

<li class="cal-rise grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 px-3 py-1 tv:py-2" style:--i={Math.min(index, STAGGER_MAX)}>
  <span class="min-w-0">
    <span class="flex min-w-0 items-baseline gap-1.5">
      <span class="shrink-0 rounded-sm border border-paper/60 px-1 text-[0.54rem] font-bold tracking-wider text-paper uppercase tv:text-xs">{play.desk}</span>
      <span class="truncate text-[0.78rem] font-semibold text-white tv:text-lg" title={play.title}>{play.title}</span>
    </span>
    <span class="block truncate font-mono text-[0.64rem] text-muted tabular-nums tv:text-sm">{play.closes || ' '}</span>
  </span>
  <span class="text-right font-mono tabular-nums">
    <span class="block text-[0.78rem] font-bold text-paper tv:text-lg">{play.mark}</span>
    <span class="block text-[0.62rem] tv:text-sm {play.pnl === null ? 'text-muted' : play.pnl < 0 ? 'text-bad' : 'text-ok'}">{play.pnl === null ? ' ' : pnl(play.pnl)}</span>
  </span>
</li>
