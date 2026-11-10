<script>
  /**
   * Money 24 markets: one symbol's quote, chart, statistics, news and related tickers.
   * Desktop and tablet: quote and chart on the left, a rail of cards on the right, split by 1px lines.
   * Phones: one column with Chart / Stats / News tabs at the bottom, within thumb reach.
   */
  import { onMount } from 'svelte';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import Check from '@lucide/svelte/icons/check';
  import Plus from '@lucide/svelte/icons/plus';
  import strings from './strings.json';
  import { MarketClient, RANGES, INTERVALS } from './market-client.svelte.js';
  import { age, compact, marketTime, percent, price, signed } from './market-format.js';
  import ProChart from './ProChart.svelte';
  import SymbolSearch from './SymbolSearch.svelte';
  import TickList from './TickList.svelte';

  const CLOCK_TICK_MS = 30000;
  const PERCENT = 100;
  const QUARTERS_PER_YEAR = 4;
  const MS_PER_SECOND = 1000;
  const DAY_RANGE = '1d';
  const TAB_CHART = 'chart';
  const TAB_STATS = 'stats';
  const TAB_FEED = 'feed';
  const TABS = [TAB_CHART, TAB_STATS, TAB_FEED];
  const LIST_RELATED = 'related';
  const LIST_FOLLOWING = 'following';
  const LISTS = [LIST_RELATED, LIST_FOLLOWING];
  const CARD = 'cal-rise mb-px break-inside-avoid border-b border-line';
  const CARD_TITLE = 'px-4 pt-4 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted';

  const client = new MarketClient();
  let tab = $state(TAB_CHART);
  let list = $state(LIST_RELATED);
  let now = $state(Date.now());

  onMount(() => {
    client.start();
    const tick = setInterval(() => { now = Date.now(); }, CLOCK_TICK_MS);
    return () => {
      clearInterval(tick);
      client.stop();
    };
  });

  const s = $derived(client.summary);
  const zone = $derived(client.chart?.timezone ?? '');
  const followed = $derived(client.following.includes(client.symbol));
  /** The move shown beside the price: today's, or the whole range's when a longer one is open. */
  const move = $derived.by(() => {
    if (!s) return null;
    const base = client.chart?.previous_close;
    if (client.range === DAY_RANGE || !base || client.chart?.range !== client.range) return { change: s.change, pct: s.change_pct, label: strings.today };
    return { change: s.price - base, pct: ((s.price - base) / base) * PERCENT, label: strings.rangeChange[client.range] };
  });
  const stats = $derived(s ? [
    [strings.stats.open, price(s.open)],
    [strings.stats.high, price(s.day_high)],
    [strings.stats.low, price(s.day_low)],
    [strings.stats.previousClose, price(s.previous_close)],
    [strings.stats.marketCap, compact(s.market_cap)],
    [strings.stats.pe, price(s.pe)],
    [strings.stats.high52, price(s.high_52)],
    [strings.stats.low52, price(s.low_52)],
    [strings.stats.dividend, percent(s.dividend_pct || undefined)],
    [strings.stats.dividendQuarter, price(s.dividend_rate / QUARTERS_PER_YEAR)],
    [strings.stats.volume, compact(s.volume)],
    [strings.stats.avgVolume, compact(s.avg_volume)],
  ] : []);
  const quarters = $derived(s?.quarters ?? []);
  const lastQuarter = $derived(quarters.at(-1));
  const topRevenue = $derived(Math.max(1, ...quarters.map((/** @type {any} */ q) => q.revenue)));
  /** How far the last quarter's revenue moved against the one before, in percent. */
  const revenueMove = $derived.by(() => {
    const before = quarters.at(-2)?.revenue;
    return lastQuarter && before ? ((lastQuarter.revenue - before) / before) * PERCENT : null;
  });
  /** How far the last quarter's earnings per share landed from what analysts expected, in percent. */
  const surprise = $derived(lastQuarter?.eps_estimate ? ((lastQuarter.eps - lastQuarter.eps_estimate) / Math.abs(lastQuarter.eps_estimate)) * PERCENT : null);
  const headline = $derived(client.news[0]);
</script>

<svelte:head><title>{client.symbol ? `${client.symbol} · ` : ''}{strings.title}</title></svelte:head>

{#snippet statGrid(/** @type {string} */ classes)}
  <dl class="grid gap-x-6 gap-y-1.5 px-4 py-3 text-sm lg:px-6 {classes}">
    {#each stats as [label, value] (label)}
      <div class="flex items-baseline justify-between gap-2 border-b border-line/60 pb-1">
        <dt class="truncate text-muted">{label}</dt>
        <dd class="font-mono tabular-nums text-white/90">{value}</dd>
      </div>
    {/each}
  </dl>
{/snippet}

{#snippet arrow(/** @type {boolean} */ down, /** @type {string} */ classes)}
  {#if down}<ArrowDown class={classes} aria-hidden="true" />{:else}<ArrowUp class={classes} aria-hidden="true" />{/if}
{/snippet}

<div class="mx-auto grid h-dvh w-full max-w-7xl grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)_auto] select-text border-line md:grid-cols-[minmax(0,1fr)_19rem] md:grid-rows-[auto_minmax(0,1fr)] lg:grid-cols-[minmax(0,1fr)_23rem] xl:border-x desktop-portrait:grid-cols-[minmax(0,1fr)]! desktop-portrait:grid-rows-[auto_minmax(0,1fr)_minmax(0,34%)]! landscape-phone:grid-cols-[minmax(0,1fr)_16rem]! landscape-phone:grid-rows-[auto_minmax(0,1fr)]! ultrawide:max-w-none ultrawide:grid-cols-[minmax(0,1fr)_46rem]">

  <!-- TOP BAR: brand, search and the way to the other desks -->
  <header class="flex items-center gap-3 border-b border-line px-4 py-2 short:py-1 md:col-span-2 lg:px-6 desktop-portrait:col-span-1! landscape-phone:col-span-2">
    <h1 class="text-xl leading-none font-extrabold tracking-tight short:text-base md:text-2xl">
      {strings.brand}<span class="text-neon-red">{strings.brandAccent}</span>
      <span class="ml-1 align-middle text-[0.6rem] font-bold tracking-[0.3em] text-muted max-sm:hidden">{strings.section}</span>
    </h1>
    <SymbolSearch matches={client.matches} onSearch={(q) => client.search(q)} onPick={(symbol) => { tab = TAB_CHART; client.open(symbol); }} />
    <nav class="flex shrink-0 gap-3 text-xs font-semibold">
      <a href="trading.html" class="spring text-white/60 underline-offset-4 hover:text-white hover:underline focus-visible:text-white focus-visible:outline-none">{strings.tradingLink}</a>
      <a href="feed.html" class="spring text-white/60 underline-offset-4 hover:text-white hover:underline focus-visible:text-white focus-visible:outline-none">{strings.newsLink}</a>
    </nav>
  </header>

  <!-- QUOTE AND CHART -->
  <main class="flex min-h-0 min-w-0 flex-col border-line md:border-r desktop-portrait:border-r-0! landscape-phone:flex! {tab === TAB_CHART ? '' : 'max-md:hidden'}">
    <div class="flex items-start justify-between gap-3 px-4 pt-3 short:pt-1.5 lg:px-6">
      <div class="min-w-0">
        <h2 class="truncate text-xl leading-tight font-semibold text-white short:text-base md:text-2xl">{s?.name ?? client.symbol}</h2>
        <p class="truncate text-xs font-semibold uppercase tracking-wider text-muted short:hidden">{s ? `${s.exchange}: ${s.symbol}` : strings.loading}</p>
      </div>
      <button type="button" aria-pressed={followed} disabled={!client.symbol} onclick={() => client.toggleFollow(client.symbol)}
        class="spring flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-bold focus-visible:outline focus-visible:outline-1 focus-visible:outline-white {followed ? 'border-neon-red bg-neon-red/15 text-white' : 'border-line text-white/70 hover:bg-white/5'}">
        {#if followed}<Check class="size-3.5" aria-hidden="true" />{:else}<Plus class="size-3.5" aria-hidden="true" />{/if}
        {followed ? strings.following : strings.follow}
      </button>
    </div>

    <!-- The price block keeps its height while loading, so the chart below never jumps. -->
    <div class="min-h-20 px-4 pt-1 short:min-h-8 lg:px-6" aria-live="polite">
      {#if s && move}
        {@const down = move.change < 0}
        <p class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span class="font-mono text-4xl leading-none font-semibold tabular-nums text-white short:text-2xl">{price(s.price)}</span>
          <span class="text-sm font-semibold text-muted">{s.currency}</span>
          <span class="flex items-center gap-1 self-center rounded-sm px-2 py-0.5 font-mono text-sm font-bold tabular-nums text-app {down ? 'bg-bad' : 'bg-ok'}">
            {@render arrow(down, 'size-4')}{percent(move.pct)}
          </span>
          <span class="font-mono text-sm tabular-nums {down ? 'text-bad' : 'text-ok'}">{signed(move.change)} <span class="font-sans">{move.label}</span></span>
        </p>
        <p class="mt-1.5 truncate text-xs text-muted short:hidden">
          {strings.state[s.market_state] ?? s.market_state}: {marketTime(s.market_time, zone)}
          {#if s.delay_minutes}· {strings.delayed.replace('{n}', s.delay_minutes)}{/if}
        </p>
        {#if s.ext_session}
          <p class="truncate font-mono text-xs tabular-nums text-muted short:hidden">
            <span class="font-sans">{strings.ext[s.ext_session]}</span> {price(s.ext_price)}
            <span class={s.ext_change < 0 ? 'text-bad' : 'text-ok'}>{signed(s.ext_change)} ({percent(s.ext_change_pct)})</span>
          </p>
        {/if}
      {:else if client.problem}
        <p class="pt-2 text-sm text-bad" role="alert">{strings[client.problem]}</p>
      {/if}
    </div>

    <!-- RANGES: the red bar slides under the open one -->
    <div class="relative mt-2 grid grid-cols-8 border-b border-line short:mt-1" role="group" aria-label={strings.ranges[client.range]}>
      {#each RANGES as r (r)}
        <button type="button" aria-pressed={client.range === r} onclick={() => client.setRange(r)}
          class="spring py-2 text-xs font-bold tracking-wider short:py-1 focus-visible:bg-white/5 focus-visible:outline-none {client.range === r ? 'text-white' : 'text-white/50 hover:text-white'}">
          {strings.ranges[r]}
        </button>
      {/each}
      <span class="pointer-events-none absolute bottom-0 left-0 h-0.5 w-1/8 bg-neon-red transition-transform duration-300 ease-out" style:transform="translateX({RANGES.indexOf(client.range) * PERCENT}%)"></span>
    </div>

    <ProChart symbol={client.symbol} chart={client.chart} isNew={client.chartIsNew} interval={client.interval} intervals={INTERVALS[client.range]} onInterval={(i) => client.setInterval(i)} />

    {@render statGrid('grid-cols-2 border-t border-line max-md:hidden short:hidden lg:grid-cols-3 xl:grid-cols-4 landscape-phone:hidden!')}
  </main>

  <!-- RAIL: news, lists, financials, earnings, overview. The only part of the page that scrolls. -->
  <aside class="list-scroll min-h-0 overflow-y-auto desktop-portrait:columns-3 desktop-portrait:border-t desktop-portrait:border-line landscape-phone:block! ultrawide:columns-2 {tab === TAB_CHART ? 'max-md:hidden' : ''}">
    <section class="{CARD} md:hidden md:short:block landscape-phone:block! {tab === TAB_STATS ? '' : 'max-md:hidden'}">
      {@render statGrid('grid-cols-2 landscape-phone:grid-cols-1')}
    </section>

    <section class="{CARD} landscape-phone:block! {tab === TAB_FEED ? '' : 'max-md:hidden'}" aria-label={strings.news}>
      {#if headline}
        <a href={headline.url} target="_blank" rel="noopener noreferrer" class="group block focus-visible:outline-none">
          {#if headline.thumb}
            <img src={headline.thumb} alt="" loading="lazy" referrerpolicy="no-referrer" class="aspect-[16/9] w-full object-cover" />
          {/if}
          <span class="block px-4 pt-3 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{headline.publisher} · {age(headline.at, now)}</span>
          <span class="block px-4 pt-1 pb-3 text-base leading-snug font-semibold text-white underline-offset-4 group-hover:underline group-focus-visible:underline">{headline.title}</span>
        </a>
        <ul class="divide-y divide-line border-t border-line">
          {#each client.news.slice(1) as n (n.url)}
            <li>
              <a href={n.url} target="_blank" rel="noopener noreferrer" class="spring block px-4 py-2.5 hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none">
                <span class="block text-sm leading-snug text-white/85">{n.title}</span>
                <span class="mt-0.5 block text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{n.publisher} · {age(n.at, now)}</span>
              </a>
            </li>
          {/each}
        </ul>
      {:else}
        <h3 class={CARD_TITLE}>{strings.news}</h3>
        <p class="px-4 py-4 text-sm text-muted">{strings.noNews}</p>
      {/if}
    </section>

    <section class="{CARD} landscape-phone:block! {tab === TAB_FEED ? '' : 'max-md:hidden'}">
      <div class="relative grid grid-cols-2 border-b border-line" role="group">
        {#each LISTS as l (l)}
          <button type="button" aria-pressed={list === l} onclick={() => { list = l; }}
            class="spring py-2.5 text-xs font-bold uppercase tracking-wider focus-visible:bg-white/5 focus-visible:outline-none {list === l ? 'text-white' : 'text-white/50 hover:text-white'}">
            {strings[l]}
          </button>
        {/each}
        <span class="pointer-events-none absolute bottom-0 left-0 h-0.5 w-1/2 bg-neon-red transition-transform duration-300 ease-out" style:transform="translateX({LISTS.indexOf(list) * PERCENT}%)"></span>
      </div>
      <TickList ticks={list === LIST_RELATED ? client.related : client.followed} current={client.symbol} onPick={(symbol) => { tab = TAB_CHART; client.open(symbol); }} />
    </section>

    {#if lastQuarter}
      <section class="{CARD} pb-4 landscape-phone:block! {tab === TAB_STATS ? '' : 'max-md:hidden'}">
        <h3 class={CARD_TITLE}>{strings.financials} · {lastQuarter.label}</h3>
        <p class="px-4 pt-2 font-mono text-2xl font-semibold tabular-nums text-white">{compact(lastQuarter.revenue)}</p>
        <p class="px-4 text-sm text-white/80">
          {#if revenueMove !== null}
            <span class="font-mono tabular-nums {revenueMove < 0 ? 'text-bad' : 'text-ok'}">{signed(revenueMove)}%</span>
          {/if}
          {strings.revenue} {strings.versusLast} · {strings.netIncome} <span class="font-mono tabular-nums">{compact(lastQuarter.earnings)}</span>
        </p>
        <!-- Revenue by quarter: bars grow from the floor with a transform, in a box of fixed height -->
        <ol class="mt-3 grid h-24 grid-cols-4 items-end gap-2 px-4">
          {#each quarters as q (q.label)}
            <li class="flex h-full flex-col justify-end gap-1 text-center">
              <span class="h-16 origin-bottom rounded-t-sm bg-neon-red/70 transition-transform duration-500 ease-out" style:transform="scaleY({q.revenue / topRevenue})" title="{q.label}: {compact(q.revenue)}"></span>
              <span class="font-mono text-[0.62rem] tabular-nums text-muted">{q.label}</span>
            </li>
          {/each}
        </ol>
      </section>

      <section class="{CARD} pb-4 landscape-phone:block! {tab === TAB_STATS ? '' : 'max-md:hidden'}">
        <h3 class={CARD_TITLE}>{strings.earnings} · {lastQuarter.label}</h3>
        {#if surprise !== null}
          <p class="px-4 pt-2 font-mono text-2xl font-semibold tabular-nums {surprise < 0 ? 'text-bad' : 'text-ok'}">{signed(surprise)}%</p>
          <p class="px-4 text-sm text-white/80">
            {surprise < 0 ? strings.epsMiss : strings.epsBeat} · {strings.eps} <span class="font-mono tabular-nums">{price(lastQuarter.eps)}</span>,
            {strings.expected} <span class="font-mono tabular-nums">{price(lastQuarter.eps_estimate)}</span>
          </p>
        {/if}
        {#if s.next_earnings}
          <p class="px-4 pt-2 text-sm text-muted">{strings.nextEarnings}: <span class="text-white/85">{new Date(s.next_earnings * MS_PER_SECOND).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</span></p>
        {/if}
      </section>
    {/if}

    {#if s?.about}
      <section class="{CARD} pb-4 landscape-phone:block! {tab === TAB_STATS ? '' : 'max-md:hidden'}">
        <h3 class={CARD_TITLE}>{strings.about}</h3>
        {#if s.sector}
          <p class="px-4 pt-2 text-xs font-semibold text-white/70">{s.sector} · {s.industry}</p>
        {/if}
        <p class="px-4 pt-2 text-sm leading-relaxed text-white/75">{s.about}</p>
        {#if s.website}
          <a href={s.website} target="_blank" rel="noopener noreferrer" class="spring mt-2 inline-block px-4 text-sm font-semibold text-white underline underline-offset-4 hover:text-neon-red">{strings.website}</a>
        {/if}
      </section>
    {/if}
  </aside>

  <!-- PHONE TABS: within thumb reach -->
  <nav class="grid grid-cols-3 border-t border-line md:hidden landscape-phone:hidden!" aria-label="Sections">
    {#each TABS as t (t)}
      <button type="button" aria-pressed={tab === t} onclick={() => { tab = t; }}
        class="spring py-3 text-xs font-bold uppercase tracking-wider short:py-2 focus-visible:bg-white/5 focus-visible:outline-none {tab === t ? 'text-neon-red' : 'text-white/55'}">
        {strings.tabs[t]}
      </button>
    {/each}
  </nav>
</div>
