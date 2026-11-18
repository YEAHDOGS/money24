<script>
  /**
   * The play in the one format every play is filed in (market, category, expected and actual
   * entry, three exits, three stop-losses, what overrode the stop), and under it the play replayed
   * over the whole timeline of prices: since its own entry, or backtested from the close of any day.
   */
  import strings from './strings.json';
  import { price, usd, pnl } from './play-format.js';
  import { curveShape } from './curve-shape.js';

  const T = strings.record;
  const CATEGORY_LIST = 'play-categories';
  const MODE_LIVE = 'live';
  const MODE_BACKTEST = 'backtest';
  const MODES = [MODE_LIVE, MODE_BACKTEST];
  const ROW = 'flex items-baseline justify-between gap-3 border-b border-line/60 py-1';
  const DAY_LENGTH = 10; // YYYY-MM-DD
  const BACKTEST_BACK_DAYS = 5; // a backtest starts this many days before the play was made, until another day is picked
  const MS_PER_DAY = 86400000;
  const EVENT_CLASS = { entry: 'fill-white', stop: 'fill-bad', time: 'fill-warn', expiry: 'fill-warn' };

  /** @type {{ item: import('./plays-client.svelte.js').Item, client: import('./plays-client.svelte.js').PlaysClient }} */
  let { item, client } = $props();

  const play = $derived(item.play);
  const record = $derived(item.report.record);
  const demo = $derived(client.status === 'demo');

  let category = $state('');
  let mode = $state(MODE_LIVE);
  /** The day a backtest enters at the close of: the play's own day until another is picked. */
  let day = $state('');
  /** @type {any} */
  let curve = $state(null);
  let busy = $state(false);
  let error = $state('');

  $effect(() => { category = play.category ?? ''; });
  // The backtest starts a few days before the play was made; the demo, from the one day it recorded.
  $effect(() => {
    if (day) return;
    const made = Date.parse(play.armed_at ?? play.created_at);
    day = demo ? client.demoBacktestFrom : new Date(made - BACKTEST_BACK_DAYS * MS_PER_DAY).toISOString().slice(0, DAY_LENGTH);
  });

  /** Every category in use, for the suggestions. */
  const categories = $derived([...new Set([...T.categories, ...client.items.map((x) => x.play.category).filter(Boolean)])]);
  const shape = $derived(curve ? curveShape(curve) : null);
  const rows = $derived([
    [T.expectedEntry, price(record.expected_entry)],
    [T.actualEntry, record.actual_entry ? price(record.actual_entry) : T.notYet],
    [T.expectedAmount, usd(record.expected_amount_usd)],
    [T.actualAmount, record.actual_amount_usd ? usd(record.actual_amount_usd) : T.notYet],
    ...record.exits.map((/** @type {number} */ x, /** @type {number} */ i) => [`${T.exit} ${i + 1}`, x ? price(x) : T.none, 'text-ok']),
    ...record.stops.map((/** @type {number} */ x, /** @type {number} */ i) => [`${T.stop} ${i + 1}`, x ? price(x) : T.none, 'text-bad']),
    [T.override, T.overrides[record.stop_override] ?? T.overrides.none, record.stop_override ? 'text-warn' : ''],
  ]);

  async function saveCategory() {
    if (category.trim() === (play.category ?? '')) return;
    error = '';
    try {
      await client.category(play.id, category.trim());
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  // The curve follows the play, the mode and the day. A stale answer for a view already left is dropped.
  $effect(() => {
    const [id, from] = [play.id, mode === MODE_BACKTEST ? day : ''];
    if (mode === MODE_BACKTEST && !from) return;
    let stale = false;
    busy = true;
    error = '';
    client.curve(id, from)
      .then((c) => { if (!stale) curve = c; })
      .catch((err) => { if (!stale) [curve, error] = [null, err instanceof Error ? err.message : String(err)]; })
      .finally(() => { if (!stale) busy = false; });
    return () => { stale = true; };
  });
</script>

<div class="grid gap-x-8 gap-y-4 @3xl:grid-cols-[minmax(0,17rem)_minmax(0,1fr)]">
  <div>
    <dl class="text-sm">
      <div class={ROW}>
        <dt class="text-muted">{T.market}</dt>
        <dd class="font-mono text-xs font-bold tracking-wider text-white uppercase">{T.markets[record.market] ?? record.market}</dd>
      </div>
      <div class={ROW}>
        <dt class="text-muted"><label for="play-category">{T.category}</label></dt>
        <dd class="min-w-0">
          <input id="play-category" bind:value={category} list={CATEGORY_LIST} onchange={saveCategory} placeholder={T.categoryPlaceholder} disabled={demo}
            autocomplete="off" class="w-36 rounded-sm border border-line bg-card px-2 py-0.5 text-right text-xs font-bold text-white placeholder:font-normal placeholder:text-muted focus:border-neon-red focus:outline-none disabled:border-transparent disabled:bg-transparent" />
          <datalist id={CATEGORY_LIST}>{#each categories as c (c)}<option value={c}></option>{/each}</datalist>
        </dd>
      </div>
      {#each rows as [label, value, tone] (label)}
        <div class={ROW}>
          <dt class="text-muted">{label}</dt>
          <dd class="font-mono tabular-nums {tone || 'text-white/90'}">{value}</dd>
        </div>
      {/each}
    </dl>
    {#if record.stops_by_rule}<p class="mt-2 text-[0.7rem] leading-snug text-muted">{T.stopsByRule}</p>{/if}
  </div>

  <!-- THE CURVE: the chart keeps its height while it loads, so nothing under it moves. -->
  <div class="min-w-0">
    <div class="flex flex-wrap items-center gap-2">
      <div class="relative grid grid-cols-2 rounded-sm border border-line" role="group" aria-label={T.curve}>
        {#each MODES as m (m)}
          <button type="button" aria-pressed={mode === m} onclick={() => { mode = m; }}
            class="spring relative z-10 px-2.5 py-1 text-[0.68rem] font-bold tracking-wider uppercase focus-visible:outline-none focus-visible:text-neon-red {mode === m ? 'text-white' : 'text-white/50 hover:text-white'}">{T.modes[m]}</button>
        {/each}
        <span class="pointer-events-none absolute inset-y-0 left-0 w-1/2 bg-white/8 transition-transform duration-300 ease-out {mode === MODE_BACKTEST ? 'translate-x-full' : ''}" aria-hidden="true"></span>
      </div>
      <label class="flex items-center gap-2 text-xs text-muted transition-opacity duration-200 {mode === MODE_BACKTEST ? 'opacity-100' : 'pointer-events-none opacity-0'}">
        {T.from}
        <input type="date" bind:value={day} disabled={demo || mode !== MODE_BACKTEST} max={new Date().toISOString().slice(0, DAY_LENGTH)}
          class="rounded-sm border border-line bg-card px-1.5 py-0.5 font-mono text-xs text-white focus:border-neon-red focus:outline-none" />
      </label>
    </div>

    <div class="relative mt-2 aspect-[10/3] min-h-28 w-full border border-line bg-card transition-opacity duration-200 {busy ? 'opacity-50' : 'opacity-100'}">
      {#if shape}
        <svg viewBox="0 0 {shape.width} {shape.height}" preserveAspectRatio="none" class="absolute inset-0 size-full" role="img" aria-label={T.curve}>
          {#each shape.levels as level (level.kind + level.y)}
            <line x1="0" x2={shape.width} y1={level.y} y2={level.y} vector-effect="non-scaling-stroke"
              class="{level.kind === 'entry' ? 'stroke-white/40' : level.kind === 'exit' ? 'stroke-ok/50' : 'stroke-bad/60'}" stroke-dasharray="4 4" />
          {/each}
          <polyline points={shape.stopLine} fill="none" vector-effect="non-scaling-stroke" class="stroke-bad" stroke-width="1" />
          <polyline points={shape.priceLine} fill="none" vector-effect="non-scaling-stroke" class="curve-line stroke-white" stroke-width="1.5" pathLength="1" />
          {#each shape.events as e (e.kind + e.x)}
            <circle cx={e.x} cy={e.y} r="3.5" vector-effect="non-scaling-stroke" class={EVENT_CLASS[e.kind] ?? 'fill-ok'}><title>{T.events[e.kind] ?? e.kind} · {price(e.price)}</title></circle>
          {/each}
        </svg>
        <span class="absolute top-1 left-1.5 font-mono text-[0.62rem] text-muted">{price(shape.top)}</span>
        <span class="absolute bottom-1 left-1.5 font-mono text-[0.62rem] text-muted">{price(shape.bottom)}</span>
      {:else if !busy}
        <p class="absolute inset-0 grid place-items-center px-4 text-center text-xs text-muted">{error || T.noCurve}</p>
      {/if}
    </div>

    {#if curve}
      <p class="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1 font-mono text-xs tabular-nums" aria-live="polite">
        <span class={curve.pnl_usd < 0 ? 'text-bad' : 'text-ok'}>{T.ended[curve.reason] ?? curve.reason}: {pnl(curve.pnl_usd)} ({curve.pnl_pct > 0 ? '+' : ''}{curve.pnl_pct}%)</span>
        <span class="text-white/80">{T.hold}: {curve.hold_pct > 0 ? '+' : ''}{curve.hold_pct}%</span>
        <span class="text-muted">{T.entryAt} {price(curve.entry)}</span>
      </p>
      <p class="mt-1 text-[0.7rem] leading-snug text-muted">{T.sources[curve.source] ?? ''} {T.caution}</p>
    {/if}
  </div>
</div>

<style>
  /* The price line draws itself from the entry forward each time a curve loads. */
  .curve-line {
    stroke-dasharray: 1;
    animation: curve-draw 700ms cubic-bezier(0.22, 1, 0.36, 1) both;
  }
  @keyframes curve-draw {
    from {
      stroke-dashoffset: 1;
    }
    to {
      stroke-dashoffset: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .curve-line {
      animation: none;
    }
  }
</style>
