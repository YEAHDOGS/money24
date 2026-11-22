<script>
  /**
   * The prediction desk's rows for the desk's Odds tab: a paper book of Kalshi event contracts at
   * Kalshi's live prices (engine/predict). First the desk's mode and cash, then every strategy in its
   * rulebook with its risk, its state and its record (every market it found, followed to its result,
   * traded or not), then the open positions soonest to close, then the settled ones. This only draws;
   * the runner does the work, and the rulebook file is where strategies are added, changed or switched off.
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
  /** Each risk label's chip: low reads calm, high reads loud; the word is always there too. */
  const RISK_CHIP = { low: 'border-ok/60 text-ok', medium: 'border-paper/70 text-paper', high: 'border-bad/70 text-bad' };

  /** @type {{ predict: any, status: string, now: number }} predict: GET /api/predict, null until read. */
  let { predict, status, now } = $props();

  const t = strings.desk.predict;
  const rows = $derived([...(predict?.open ?? []), ...(predict?.closed ?? [])]);
  /** Strategy names by id, for the position rows. */
  const names = $derived(Object.fromEntries((predict?.strategies ?? []).map((/** @type {any} */ s) => [s.config.id, s.config.name])));

  /** 0.9368 → "93.7¢" */
  const cents = (/** @type {number} */ n) => `${Number((n * PERCENT).toFixed(1))}¢`;

  /** One strategy's record in a line: what it found, how it did, and what it needed to break even. */
  function record(/** @type {any} */ r) {
    if (!r.settled) return fill(t.waiting, { watched: r.watched });
    const line = fill(t.record, { watched: r.watched, settled: r.settled, won: r.win_pct, needs: Number((r.avg_cost * PERCENT).toFixed(1)), edge: signedPct(r.edge_pct) });
    return r.avg_prob ? `${line} · ${fill(t.said, { prob: r.avg_prob })}` : line;
  }
</script>

{#if predict}
  <li class="px-3 py-1.5 lg:px-4">
    <span class="flex flex-wrap items-center gap-x-2 gap-y-0.5 font-mono text-[0.62rem] text-muted tabular-nums tv:text-sm">
      <span class="font-sans font-bold text-white">{fill(t.head, { venue: predict.venue })}</span>
      <span title={t.modeNote}>· {fill(t.mode, { mode: t.risks[predict.mode] ?? predict.mode })}</span>
      <span class="max-[419px]:hidden">· {predict.keys_set ? t.keysSet : t.keysNone}</span>
    </span>
    <span class={STAT}>{fill(t.account, { cash: usd(predict.account.cash_usd), open: usd(predict.account.open_usd), mark: usd(predict.account.mark_usd), realized: pnl(predict.account.realized_usd), won: predict.account.won, settled: predict.account.settled })}</span>
    <span class={STAT}>{fill(t.lossLimit, { lost: usd(predict.lost_today_usd), limit: usd(predict.daily_loss_usd) })}</span>
    {#if predict.rulebook_error}<span class="block font-mono text-[0.62rem] leading-snug text-bad tv:text-sm">{fill(t.rulebookError, { error: predict.rulebook_error })}</span>{/if}
  </li>
  {#each predict.strategies as s (s.config.id)}
    {@const looking = s.state === STATE_LOOKING}
    <li class="px-3 py-1.5 lg:px-4" title={s.why || s.last_error || s.config.name}>
      <span class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[0.68rem] tv:text-sm">
        <span class="relative size-1.5 shrink-0 rounded-full {looking ? 'live-dot bg-ok text-ok' : 'bg-muted/60'}" aria-hidden="true"></span>
        <span class="font-bold text-white">{s.config.name}</span>
        <span class="rounded-sm border px-1 text-[0.56rem] font-bold tracking-wider uppercase tv:text-xs {RISK_CHIP[s.config.risk] ?? 'border-line text-muted'}">{t.risks[s.config.risk] ?? s.config.risk}</span>
        <span class="font-mono {looking ? 'text-ok' : 'text-white/70'}">{t.states[s.state] ?? s.state}</span>
        {#if looking && until(s.next_at, now)}<span class="font-mono text-muted">· {fill(strings.desk.nextScan, { time: until(s.next_at, now) })}</span>{/if}
        {#if s.last_error}<span class="font-mono text-bad">· {t.error}</span>{/if}
      </span>
      <span class={STAT}>{record(s.record)}</span>
      <span class={STAT}>{fill(t.positions, { open: s.account.open, cost: usd(s.account.open_usd), realized: pnl(s.account.realized_usd), won: s.account.won, settled: s.account.settled })}</span>
      {#each (s.record.bands ?? []).filter((/** @type {any} */ b) => b.settled) as b (b.from)}
        <span class={STAT}>
          <span class="font-sans font-bold text-white/80">{fill(t.band, { from: Math.round(b.from * PERCENT), to: Math.round(b.to * PERCENT) })}</span>
          {fill(t.record, { watched: b.watched, settled: b.settled, won: b.win_pct, needs: Number((b.avg_cost * PERCENT).toFixed(1)), edge: signedPct(b.edge_pct) })}
        </span>
      {/each}
    </li>
  {/each}
{/if}
{#each rows as p, i (p.buy.ticker)}
  {@const won = p.pnl_usd > 0}
  <li class="overflow-hidden">
    <div class="cal-rise {ROW}" style:--i={Math.min(i, ROW_STAGGER_MAX)}>
      <span class="size-1.5 shrink-0 translate-y-[-0.15em] rounded-full {p.settled ? (won ? 'bg-ok' : 'bg-bad') : 'live-dot relative bg-paper text-paper'}" aria-hidden="true"></span>
      <span class="min-w-0">
        <span class="block truncate text-sm font-semibold text-white tv:text-lg" title={p.buy.note || p.buy.title}>{p.buy.title}</span>
        <span class="block truncate font-mono text-[0.68rem] text-muted tabular-nums tv:text-sm" title={p.buy.note}>
          <span class="font-sans text-white/60">{names[p.buy.strategy] ?? p.buy.strategy}</span>
          · {fill(t.plan, { side: t.sides[p.buy.side] ?? p.buy.side, qty: units(p.buy.qty), price: cents(p.buy.price), cost: usd(p.buy.cost_usd), fee: usd(p.buy.fee_usd), close: shortWhen(p.buy.close_at) })}{p.buy.prob ? ` · ${fill(t.chance, { prob: Math.round(p.buy.prob * PERCENT) })}` : ''}
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
    {:else if predict}{predict.on ? t.empty : t.off}
    {:else}{t.loading}{/if}
  </li>
{/each}
{#if predict}
  <li class="px-3 py-2 font-mono text-[0.6rem] leading-snug text-muted/80 lg:px-4 tv:text-xs">{fill(t.rulebook, { file: predict.rulebook_file })}</li>
{/if}
