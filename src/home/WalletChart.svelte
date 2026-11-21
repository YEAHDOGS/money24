<script>
  /**
   * The account's history: starting capital plus every closed trade, as a line
   * over the picked range (1W, 1M, 6M, 1Y, All), with the account's value now, its change over the
   * range, its deepest fall and how many trades closed. There is no real wallet yet (deposits are
   * closed), so this is the desk's book and says so. With few closed trades the line is flat, because the
   * account was. Under the numbers is the cash account as the runner keeps it: cash, what of it is
   * settled and can be spent, what is in open positions and what is still settling.
   * The arithmetic is equity.js; this only draws. Pointing at the line reads a moment.
   */
  import strings from './strings.json';
  import { fill, pnl, shortWhen, usd } from '../plays/play-format.js';
  import { percent } from '../markets/market-format.js';
  import { DEFAULT_RANGE, RANGES, chartShape, equityCurve, rangeStart, realizedSteps, valueAt } from './equity.js';
  import { WALLET_PAGE, pageLink } from './home-links.js';
  import { probe } from './probe.js';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';

  const STATUS_OFFLINE = 'offline';
  const PERCENT = 100;
  const DASH = '—';
  const UP = '▲';
  const DOWN = '▼';
  const DOT_RADIUS = 3.5;
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase tv:text-sm';
  const PAPER_CHIP = 'rounded-sm border border-paper px-1.5 py-px text-[0.6rem] font-bold tracking-wider text-paper uppercase tv:text-sm';
  const STAT = 'min-w-0 bg-app px-3 py-2 lg:px-4';
  const STAT_VALUE = 'mt-0.5 truncate font-mono text-sm font-semibold tabular-nums max-[419px]:text-xs tv:text-xl';
  /** The line takes the color of the range's change; a flat line stays neutral. Money never takes the desk color. */
  const TONES = { up: 'text-ok', down: 'text-bad', flat: 'text-white/80' };
  const FADE_ID = 'equity-fade';
  const FADE_TOP = 0.22; // how strong the fill is right under the line; it fades to nothing at the bottom
  /** @type {Intl.DateTimeFormatOptions} */
  const DAY = { month: 'short', day: 'numeric', year: 'numeric' };
  /** @type {Intl.DateTimeFormatOptions} */
  const MOMENT = { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' };

  /**
   * @type {{ journal: any[], capital: number, ready: boolean, status: string, now: number, account?: any }}
   * ready: the journal has been read. status: the runner's connection. now: epoch ms.
   * account: the runner's cash account, null from a runner that keeps none.
   */
  let { journal, capital, ready, status, now, account = null } = $props();

  let range = $state(DEFAULT_RANGE);
  let width = $state(0);
  let height = $state(0);
  /** @type {number | null} where the pointer is over the chart, in pixels from its left edge */
  let pointerX = $state(null);

  const steps = $derived(realizedSteps(journal));
  const from = $derived(rangeStart(range, Date.parse(journal[0]?.at), now));
  const curve = $derived(equityCurve(steps, capital, from, now));
  const shape = $derived(width && height ? chartShape(curve.points, width, height) : null);
  const tone = $derived(TONES[curve.change > 0 ? 'up' : curve.change < 0 ? 'down' : 'flat']);
  const rangeIndex = $derived(RANGES.findIndex((r) => r.id === range));
  const baseline = $derived(shape && capital > shape.lo && capital < shape.hi ? shape.y(capital) : null);
  /** The moment under the pointer: its place on the line and what the account held then. */
  const reading = $derived.by(() => {
    if (pointerX === null || !shape) return null;
    const t = shape.t(pointerX);
    const value = valueAt(curve.points, t);
    return { x: shape.x(t), y: shape.y(value), value, when: new Date(t).toLocaleString(undefined, MOMENT) };
  });
  /** The cash account's four numbers, in the order they are read. */
  const cash = $derived([
    { id: 'cash', value: account?.cash_usd, note: '' },
    { id: 'buyingPower', value: account?.buying_power_usd, note: '' },
    { id: 'invested', value: account?.invested_usd, note: '' },
    { id: 'settling', value: account?.unsettled_usd, note: account?.settles?.[0] ? fill(strings.wallet.settlesAt, { when: shortWhen(account.settles[0].at) }) : '' },
  ]);
  const summary = $derived(fill(strings.wallet.chartLabel, { range: strings.wallet.rangeNames[range], start: usd(curve.start), end: usd(curve.end) }));
</script>

<section aria-labelledby="wallet-title" class="flex flex-[1_0_auto] flex-col max-md:border-b max-md:border-line ultrawide:min-h-0">
  <header class="flex flex-wrap items-center gap-x-2.5 gap-y-2 border-b border-line px-3 py-2 lg:px-4">
    <h2 id="wallet-title" class={LABEL}>{strings.wallet.title}</h2>
    <span class={PAPER_CHIP} title={strings.paperNote}>{strings.paper}</span>
    <a href={pageLink(WALLET_PAGE)} class="spring flex items-center gap-1 rounded-sm text-[0.68rem] font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red max-[359px]:hidden tv:text-sm">
      {strings.wallet.wallet}<ArrowRight class="size-3 tv:size-4" aria-hidden="true" />
    </a>

    <!-- RANGE: one marker slides between the buttons. -->
    <div role="group" aria-label={strings.wallet.rangesLabel} class="relative ml-auto grid rounded-sm border border-line" style:grid-template-columns="repeat({RANGES.length}, minmax(0, 1fr))">
      <span aria-hidden="true" class="pointer-events-none absolute inset-y-0 left-0 bg-white/8 transition-transform duration-300 ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-reduce:transition-none"
        style:width="{PERCENT / RANGES.length}%" style:transform="translateX({rangeIndex * PERCENT}%)">
        <span class="absolute inset-x-1.5 bottom-0 h-0.5 bg-neon-red"></span>
      </span>
      {#each RANGES as r (r.id)}
        <button type="button" aria-pressed={range === r.id} onclick={() => { range = r.id; }}
          class="spring relative px-2.5 py-1 font-mono text-[0.68rem] font-bold tracking-wider uppercase focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red max-[359px]:px-2 tv:px-4 tv:text-base {range === r.id ? 'text-white' : 'text-white/55 hover:text-white'}">
          {strings.wallet.ranges[r.id]}
        </button>
      {/each}
    </div>
  </header>

  <!-- Every number sits in its own equal cell and is tabular, so a new range changes digits, never widths. -->
  <dl class="grid grid-cols-3 gap-px border-b border-line bg-line lg:grid-cols-[minmax(0,1.5fr)_repeat(3,minmax(0,1fr))]">
    <div class="col-span-3 {STAT} lg:col-span-1">
      <dt class={LABEL}>{strings.wallet.equity}</dt>
      <dd class="mt-0.5 font-mono text-3xl leading-none font-semibold text-white tabular-nums short:text-2xl tv:text-5xl">{ready ? usd(curve.end) : DASH}</dd>
    </div>
    <div class={STAT}>
      <dt class={LABEL}>{strings.wallet.change}</dt>
      <dd class="{STAT_VALUE} {tone}">
        {#if ready}
          {curve.change > 0 ? UP : curve.change < 0 ? DOWN : ''} {pnl(curve.change)} <span class="font-normal opacity-80 max-xl:block tv:block">{percent(curve.changePct)}</span>
        {:else}{DASH}{/if}
      </dd>
    </div>
    <div class={STAT}>
      <dt class={LABEL}>{strings.wallet.drawdown}</dt>
      <dd class="{STAT_VALUE} {ready && curve.drawdownPct > 0 ? 'text-bad' : 'text-white/80'}">{ready ? percent(curve.drawdownPct) : DASH}</dd>
    </div>
    <div class={STAT}>
      <dt class={LABEL}>{strings.wallet.trades}</dt>
      <dd class="{STAT_VALUE} text-white/80">{ready ? curve.trades : DASH}</dd>
    </div>
  </dl>

  <!-- THE CASH ACCOUNT: the row keeps its place before the runner has answered, so nothing jumps when it does. -->
  <dl class="grid grid-cols-4 gap-px border-b border-line bg-line max-[419px]:grid-cols-2" title={strings.wallet.cashNote}>
    {#each cash as cell (cell.id)}
      <div class={STAT}>
        <dt class={LABEL}>{strings.wallet[cell.id]}</dt>
        <dd class="{STAT_VALUE} text-white/80" title={cell.note || undefined}>{Number.isFinite(cell.value) ? usd(cell.value) : DASH}</dd>
      </div>
    {/each}
  </dl>

  <!-- THE LINE. Its box is measured, so it is drawn in real pixels and nothing stretches. -->
  <div class="relative h-44 min-h-36 touch-pan-y overflow-hidden md:h-auto md:flex-1 desktop-portrait:h-[32dvh] desktop-portrait:flex-none landscape-phone:h-36 landscape-phone:flex-none" bind:clientWidth={width} bind:clientHeight={height}
    use:probe={(x) => { pointerX = x; }}>
    {#if ready && shape}
      <svg {width} {height} viewBox="0 0 {width} {height}" role="img" aria-label={summary} class="absolute inset-0 block {tone}">
        <defs>
          <linearGradient id={FADE_ID} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stop-color="currentColor" stop-opacity={FADE_TOP} />
            <stop offset="1" stop-color="currentColor" stop-opacity="0" />
          </linearGradient>
        </defs>
        {#if baseline !== null}
          <line x1="0" x2={width} y1={baseline} y2={baseline} class="stroke-muted/50" stroke-width="1" stroke-dasharray="3 5" />
        {/if}
        {#key range}
          <path d={shape.area} class="equity-area" fill="url(#{FADE_ID})" />
          <path d={shape.line} pathLength="1" class="equity-line fill-none stroke-current" stroke-width="2" stroke-linejoin="round" />
        {/key}
        <circle cx={width - DOT_RADIUS} cy={shape.y(curve.end)} r={DOT_RADIUS} class="fill-current" />
        {#if reading}
          <line x1={reading.x} x2={reading.x} y1="0" y2={height} class="stroke-white/30" stroke-width="1" />
          <circle cx={reading.x} cy={reading.y} r={DOT_RADIUS} class="fill-white" />
        {/if}
      </svg>
      <!-- Scale and dates sit over the chart's corners; the readout takes the place of the top scale. -->
      <p class="pointer-events-none absolute top-1 left-3 font-mono text-[0.62rem] text-muted tabular-nums lg:left-4 tv:text-sm">
        {#if reading}<span class="text-white">{usd(reading.value)}</span> · {reading.when}{:else}{usd(shape.hi)}{/if}
      </p>
      <p class="pointer-events-none absolute inset-x-3 bottom-1 flex justify-between font-mono text-[0.62rem] text-muted tabular-nums lg:inset-x-4 tv:text-sm">
        <span>{usd(shape.lo)} · {new Date(from).toLocaleDateString(undefined, DAY)}</span>
        <span>{strings.wallet.now}</span>
      </p>
    {:else if status === STATUS_OFFLINE}
      <p class="px-3 py-5 text-sm text-bad lg:px-4" role="alert">{strings.wallet.unknown}</p>
    {/if}
  </div>

  <p class="border-t border-line px-3 py-1.5 text-xs leading-snug text-muted lg:px-4 short:hidden tv:text-base">
    {#if ready && !curve.trades}<span class="text-white/70">{strings.wallet.flat}</span>{/if}
    {fill(strings.wallet.note, { capital: usd(capital) })}
  </p>
</section>
