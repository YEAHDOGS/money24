<script>
  /**
   * The prediction desk's rows for the desk's Odds tab: a paper book of Kalshi event contracts at
   * Kalshi's live prices (engine/predict). First the desk's state and cash, then the rule's record in
   * each price band (every favorite it found, followed to its result, traded or not), then the open
   * positions soonest to close, then the settled ones. This only draws; the runner does the work.
   */
  import strings from './strings.json';
  import { fill, pnl, signedPct, shortWhen, usd, units } from '../plays/play-format.js';
  import { until } from './desk.js';

  const PERCENT = 100;
  const STATE_LOOKING = 'looking';
  const STATUS_OFFLINE = 'offline';
  const ROW = 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-baseline gap-x-2.5 px-3 py-2 lg:px-4 tv:py-3';
  const STAT = 'block font-mono text-[0.62rem] leading-snug text-muted tabular-nums tv:text-sm';
  const ROW_STAGGER_MAX = 12;

  /** @type {{ predict: any, status: string, now: number }} predict: GET /api/predict, null until read. */
  let { predict, status, now } = $props();

  const t = strings.desk.predict;
  const rows = $derived([...(predict?.open ?? []), ...(predict?.closed ?? [])]);
  const looking = $derived(predict?.state === STATE_LOOKING);

  /** 0.9368 → "93.7¢" */
  const cents = (/** @type {number} */ n) => `${Number((n * PERCENT).toFixed(1))}¢`;
</script>

{#if predict}
  <li class="px-3 py-1.5 lg:px-4">
    <span class="flex flex-wrap items-center gap-x-2 gap-y-0.5 font-mono text-[0.62rem] text-muted tabular-nums tv:text-sm" title={predict.why || predict.last_error || undefined}>
      <span class="relative size-1.5 shrink-0 rounded-full {looking ? 'live-dot bg-ok text-ok' : 'bg-muted/60'}" aria-hidden="true"></span>
      <span class="font-sans font-bold text-white">{fill(t.head, { venue: predict.venue })}</span>
      <span class={looking ? 'text-ok' : 'text-white/70'}>{strings.desk.states[predict.state] ?? predict.state}</span>
      {#if looking && until(predict.next_at, now)}<span>· {fill(strings.desk.nextScan, { time: until(predict.next_at, now) })}</span>{/if}
      <span class="max-[419px]:hidden">· {predict.keys_set ? t.keysSet : t.keysNone}</span>
    </span>
    <span class={STAT}>{fill(t.account, { cash: usd(predict.account.cash_usd), open: usd(predict.account.open_usd), mark: usd(predict.account.mark_usd), realized: pnl(predict.account.realized_usd), won: predict.account.won, settled: predict.account.settled })}</span>
    {#each predict.bands as b (b.from)}
      <span class={STAT} title={t.bandNote}>
        <span class="font-sans font-bold text-white/80">{fill(t.band, { from: Math.round(b.from * PERCENT), to: Math.round(b.to * PERCENT) })}</span>
        {b.settled ? fill(t.bandStat, { watched: b.watched, settled: b.settled, won: b.win_pct, needs: Number((b.avg_price * PERCENT).toFixed(1)), edge: signedPct(b.edge_pct) }) : fill(t.bandWaiting, { watched: b.watched })}
      </span>
    {/each}
  </li>
{/if}
{#each rows as p, i (p.buy.ticker)}
  {@const won = p.pnl_usd > 0}
  <li class="overflow-hidden">
    <div class="cal-rise {ROW}" style:--i={Math.min(i, ROW_STAGGER_MAX)}>
      <span class="size-1.5 shrink-0 translate-y-[-0.15em] rounded-full {p.settled ? (won ? 'bg-ok' : 'bg-bad') : 'live-dot relative bg-paper text-paper'}" aria-hidden="true"></span>
      <span class="min-w-0">
        <span class="block truncate text-sm font-semibold text-white tv:text-lg" title={p.buy.title}>{p.buy.title}</span>
        <span class="block truncate font-mono text-[0.68rem] text-muted tabular-nums tv:text-sm">
          {fill(t.plan, { side: t.sides[p.buy.side] ?? p.buy.side, qty: units(p.buy.qty), price: cents(p.buy.price), cost: usd(p.buy.cost_usd), fee: usd(p.buy.fee_usd), close: shortWhen(p.buy.close_at) })}
        </span>
      </span>
      <span class="text-right font-mono tabular-nums">
        {#if p.settled}
          <span class="block text-sm font-bold tv:text-lg {won ? 'text-ok' : 'text-bad'}">{pnl(p.pnl_usd)}</span>
          <span class="block text-[0.62rem] text-muted tv:text-sm">{won ? t.won : t.lost}</span>
        {:else}
          <span class="block text-sm font-bold text-paper tv:text-lg">{cents(p.mark)}</span>
          <span class="block text-[0.62rem] text-muted tv:text-sm">{t.bid}</span>
        {/if}
      </span>
    </div>
  </li>
{:else}
  <li class="px-3 py-4 text-sm text-muted lg:px-4">
    {#if status === STATUS_OFFLINE}{strings.offline}
    {:else if predict}{predict.why || fill(t.empty, { every: predict.rules.every_minutes, min: Math.round(predict.rules.min_price * PERCENT), max: Math.round(predict.rules.max_price * PERCENT), hours: predict.rules.within_hours })}
    {:else}{t.loading}{/if}
  </li>
{/each}
