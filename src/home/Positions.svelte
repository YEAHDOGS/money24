<script>
  /**
   * Open positions: the plays running on paper right now. One row each: the contract, its entry,
   * what is left, what it cost, the stop in force, the next exit and its open result where a
   * price is known. A row opens its play on the Plays page. Wide: a table. Narrow: two lines a row.
   * Everything here is paper trading and says so. The rows come from positions.js.
   */
  import strings from './strings.json';
  import { fill, pnl, price, shortWhen, usd } from '../plays/play-format.js';
  import { totals } from './positions.js';
  import { PLAYS_PAGE, PLAY_PARAM, pageLink } from './home-links.js';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';

  const STATUS_OFFLINE = 'offline';
  const STATUS_CONNECTING = 'connecting';
  const DASH = '—';
  const TIMES = '×';
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase tv:text-sm';
  const PAPER_CHIP = 'rounded-sm border border-paper px-1.5 py-px text-[0.6rem] font-bold tracking-wider text-paper uppercase tv:text-sm';
  /** The table's columns, shared by the heading row and every row so they line up. */
  const TABLE = '@2xl:grid-cols-[minmax(0,2.4fr)_minmax(0,0.7fr)_minmax(0,0.5fr)_minmax(0,0.9fr)_minmax(0,0.7fr)_minmax(0,1fr)_minmax(0,1.3fr)]';
  const CELL = 'flex min-w-0 flex-col @2xl:block @2xl:text-right';
  const CELL_LABEL = 'text-[0.58rem] font-bold tracking-wider text-muted uppercase @2xl:sr-only';
  const COLUMNS = ['entry', 'size', 'cost', 'stop', 'next'];

  /** @type {{ rows: import('./positions.js').Row[], status: string }} status: the runner's connection */
  let { rows, status } = $props();

  const sum = $derived(totals(rows));
  /** A dollar price per contract or share, or a dash when it is not known. */
  function dollars(/** @type {number} */ n) {
    if (!Number.isFinite(n) || n <= 0) return DASH;
    return `$${price(n)}`;
  }
</script>

<section aria-labelledby="positions-title" class="@container flex min-h-0 shrink-0 flex-col border-b border-line ultrawide:border-b-0">
  <header class="flex flex-wrap items-center gap-x-2.5 gap-y-1 border-b border-line px-3 py-2 lg:px-4">
    <h2 id="positions-title" class={LABEL}>{strings.positions.title}</h2>
    <span class={PAPER_CHIP} title={strings.paperNote}>{strings.paper}</span>
    <span class="font-mono text-xs font-bold text-white tabular-nums tv:text-base">{rows.length}</span>
    <a href={pageLink(PLAYS_PAGE)} class="spring ml-auto flex items-center gap-1 rounded-sm text-[0.68rem] font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:text-sm">
      {strings.positions.all}<ArrowRight class="size-3 tv:size-4" aria-hidden="true" />
    </a>
    <!-- The totals keep their line whether or not there is anything to add up, so the list never jumps. -->
    <p class="order-last flex basis-full gap-x-4 font-mono text-xs tabular-nums text-white/80 @2xl:order-none @2xl:ml-3 @2xl:basis-auto tv:text-base">
      <span><span class="text-muted">{strings.positions.cost}</span> {rows.length ? usd(sum.cost) : DASH}</span>
      <span title={sum.marked < rows.length ? fill(strings.positions.openOf, { n: sum.marked, of: rows.length }) : undefined}>
        <span class="text-muted">{strings.positions.open}</span>
        <span class={!sum.marked ? '' : sum.open < 0 ? 'text-bad' : 'text-ok'}>{sum.marked ? pnl(sum.open) : DASH}</span>
      </span>
    </p>
  </header>

  {#if rows.length}
    <div aria-hidden="true" class="hidden gap-x-3 border-b border-line px-3 py-1 text-[0.58rem] font-bold tracking-wider text-muted uppercase @2xl:grid {TABLE} lg:px-4 tv:text-xs">
      <span>{strings.positions.cols.contract}</span>
      {#each COLUMNS as column (column)}<span class="text-right">{strings.positions.cols[column]}</span>{/each}
      <span class="text-right">{strings.positions.cols.result}</span>
    </div>
  {/if}

  <ul class="list-scroll min-h-0 divide-y divide-line md:max-h-[36dvh] md:overflow-y-auto ultrawide:max-h-none">
    {#each rows as row, i (row.id)}
      <!-- The row rises inside its own clipped box, so the list never grows a scrollbar while it does. -->
      <li class="overflow-hidden">
        <div class="cal-rise" style:--i={i}>
        <a href={pageLink(PLAYS_PAGE, { [PLAY_PARAM]: row.id })} title={fill(strings.positions.openPlay, { title: row.title })}
          class="spring grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1.5 px-3 py-2.5 hover:bg-white/3 focus-visible:bg-white/5 focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red {TABLE} lg:px-4 tv:py-4">
          <span class="flex min-w-0 items-center gap-2">
            <span class="live-dot relative size-2 shrink-0 rounded-full bg-paper text-paper" aria-hidden="true"></span>
            <span class="truncate font-mono text-sm font-bold text-white tv:text-lg">{row.title}</span>
          </span>

          <!-- The result: top right on a narrow row, the last column of the table. -->
          <span class="text-right font-mono text-sm font-bold tabular-nums @2xl:order-last tv:text-lg">
            <span class="sr-only">{strings.positions.cols.result}</span>
            {#if row.mark}
              <span class={row.mark.pnl < 0 ? 'text-bad' : 'text-ok'}>{pnl(row.mark.pnl)}</span>
              {#if row.mark.modelled}<span class="ml-1 align-middle text-[0.55rem] font-bold tracking-wider text-muted uppercase" title={strings.positions.modelNote}>{strings.positions.model}</span>{/if}
            {:else}
              <span class="text-muted" title={strings.positions.noMark}>{DASH}</span>
            {/if}
            {#if row.sold}<span class="block text-[0.62rem] font-normal {row.banked < 0 ? 'text-bad' : 'text-ok'}">{pnl(row.banked)} {strings.positions.banked}</span>{/if}
          </span>

          <span class="col-span-2 grid grid-cols-5 gap-x-2 font-mono text-xs tabular-nums text-white/85 @2xl:contents tv:text-base">
            <span class={CELL}><span class={CELL_LABEL}>{strings.positions.cols.entry}</span>{dollars(row.entry)}</span>
            <span class={CELL}><span class={CELL_LABEL}>{strings.positions.cols.size}</span>{row.qty}</span>
            <span class={CELL}><span class={CELL_LABEL}>{strings.positions.cols.cost}</span>{Number.isFinite(row.cost) ? usd(row.cost) : DASH}</span>
            <span class={CELL}><span class={CELL_LABEL}>{strings.positions.cols.stop}</span><span class="text-bad">{dollars(row.stop)}</span></span>
            <span class={CELL}>
              <span class={CELL_LABEL}>{strings.positions.cols.next}</span>
              {#if row.next}
                <span class="truncate text-ok">{dollars(row.next.price)}<span class="text-muted @max-2xl:hidden"> {TIMES}{row.next.qty}</span></span>
              {:else if row.exitBy}
                <span class="truncate text-white/70" title={strings.positions.timeExit}>{shortWhen(row.exitBy)}</span>
              {:else}
                {DASH}
              {/if}
            </span>
          </span>
        </a>
        </div>
      </li>
    {:else}
      <li class="px-3 py-5 text-sm lg:px-4" role={status === STATUS_OFFLINE ? 'alert' : undefined}>
        {#if status === STATUS_OFFLINE}
          <span class="text-bad">{strings.offline}</span>
          <a href={pageLink(PLAYS_PAGE)} class="spring ml-1 font-semibold text-white underline decoration-neon-red underline-offset-4 hover:text-neon-red">{strings.offlineLink}</a>
        {:else if status === STATUS_CONNECTING}
          <span class="text-muted">{strings.loading}</span>
        {:else}
          <span class="text-muted">{strings.positions.empty}</span>
          <a href={pageLink(PLAYS_PAGE)} class="spring ml-1 font-semibold text-white underline decoration-neon-red underline-offset-4 hover:text-neon-red">{strings.positions.emptyLink}</a>
        {/if}
      </li>
    {/each}
  </ul>
</section>
