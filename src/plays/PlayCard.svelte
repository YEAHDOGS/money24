<script>
  /**
   * One play in the list: status, the DOGS odds, what the stock needs to do by when, what it can
   * make and lose, and how many rules it is failing or broke on its way to paper.
   */
  import strings from './strings.json';
  import { needsLine, failing, pct, pnl, usd, realized } from './play-format.js';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';

  /** @type {{ item: import('./plays-client.svelte.js').Item, selected: boolean, onPick: () => void }} */
  let { item, selected, onPick } = $props();

  const STATUS_CLASS = { draft: 'border-line text-white/70', armed: 'border-paper text-paper', closed: 'border-muted text-muted' };

  const play = $derived(item.play);
  const report = $derived(item.report);
  const fails = $derived(failing(report));
  const broke = $derived(play.broke_rules?.length ?? 0);
  const made = $derived(realized(play));
  const showMade = $derived(play.status !== 'draft' && (play.result || play.fills?.length));
</script>

<button type="button" onclick={onPick} aria-pressed={selected}
  class="spring relative grid w-full grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1 px-3 py-3 text-left hover:bg-white/3 focus-visible:bg-white/5 focus-visible:outline-none lg:px-4 {selected ? 'bg-white/5' : ''}">
  {#if selected}<span class="absolute inset-y-0 left-0 w-0.5 bg-neon-red" aria-hidden="true"></span>{/if}
  <span class="flex min-w-0 items-center gap-2">
    <span class="truncate font-mono text-sm font-bold text-white">{play.title}</span>
    <span class="shrink-0 rounded-sm border px-1.5 py-px text-[0.6rem] font-bold uppercase tracking-wider {STATUS_CLASS[play.status] ?? STATUS_CLASS.draft}">{strings.playStatus[play.status] ?? play.status}</span>
  </span>
  <span class="text-right font-mono text-sm font-bold tabular-nums text-neon-red" title={strings.card.odds}>
    {report.odds.dogs_pct ? pct(report.odds.dogs_pct) : '—'}
  </span>
  <span class="col-span-2 truncate font-mono text-xs tabular-nums text-white/80">{needsLine(play, report) || '—'}</span>
  <span class="col-span-2 flex flex-wrap items-center gap-x-3 gap-y-0.5 font-mono text-[0.7rem] tabular-nums">
    {#if showMade}
      <span class={made < 0 ? 'text-bad' : 'text-ok'}>{pnl(made)}{play.result ? '' : ' …'}</span>
    {/if}
    <span class="text-ok">{strings.card.maxProfit} +{usd(report.payoff.max_profit_usd)}</span>
    <span class="text-bad">{strings.card.plannedLoss} −{usd(report.payoff.planned_loss_usd)}</span>
    {#if play.theme}<span class="truncate text-muted">{play.theme}</span>{/if}
    {#if broke || fails.blocks}
      <span class="flex items-center gap-1 text-warn">
        <TriangleAlert class="size-3" aria-hidden="true" />
        {broke ? `${broke} ${broke === 1 ? strings.card.ruleBroken : strings.card.rulesBroken}` : `${fails.blocks} ${fails.blocks === 1 ? strings.card.ruleFailing : strings.card.rulesFailing}`}
      </span>
    {/if}
  </span>
</button>
