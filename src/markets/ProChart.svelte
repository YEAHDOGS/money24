<script>
  /**
   * The chart and its controls: candle size, chart type, indicators, drawing tools and scale.
   * The drawing itself lives in chart-engine.js. Choices are remembered on this device; drawings
   * are remembered per symbol.
   */
  import { onMount, untrack } from 'svelte';
  import Activity from '@lucide/svelte/icons/activity';
  import ChartArea from '@lucide/svelte/icons/chart-area';
  import ChartCandlestick from '@lucide/svelte/icons/chart-candlestick';
  import ChartLine from '@lucide/svelte/icons/chart-line';
  import ChartBars from '@lucide/svelte/icons/chart-no-axes-column';
  import Eraser from '@lucide/svelte/icons/eraser';
  import Minus from '@lucide/svelte/icons/minus';
  import Scan from '@lucide/svelte/icons/scan';
  import TrendingUp from '@lucide/svelte/icons/trending-up';
  import strings from './strings.json';
  import { compact, percent, price, signed } from './market-format.js';
  import {
    ChartEngine, STUDIES, SCALE_LINEAR, SCALE_LOG, SCALE_PERCENT, TOOL_LEVEL, TOOL_TREND, TYPE_AREA, TYPE_BARS, TYPE_CANDLES, TYPE_LINE,
  } from './chart-engine.js';

  const STORE_PREFS = 'm24.markets.chart';
  const STORE_DRAWINGS = 'm24.markets.drawings.';
  const DEFAULT_STUDIES = ['volume'];
  const BASELINE_RANGE = '1d'; // only the one-day view marks the previous close
  const TYPES = [
    { id: TYPE_AREA, icon: ChartArea },
    { id: TYPE_CANDLES, icon: ChartCandlestick },
    { id: TYPE_BARS, icon: ChartBars },
    { id: TYPE_LINE, icon: ChartLine },
  ];
  const TOOLS = [
    { id: TOOL_LEVEL, icon: Minus, label: strings.chart.level, hint: strings.chart.hintLevel },
    { id: TOOL_TREND, icon: TrendingUp, label: strings.chart.trend, hint: strings.chart.hintTrend },
  ];
  const BUTTON = 'spring flex h-7 min-w-7 items-center justify-center rounded-sm border px-1.5 text-xs font-bold focus-visible:outline focus-visible:outline-1 focus-visible:outline-white';
  const BUTTON_ON = 'border-neon-red bg-neon-red/15 text-white';
  const BUTTON_OFF = 'border-line text-white/60 hover:bg-white/5 hover:text-white';

  /** @type {{ symbol: string, chart: any, isNew: boolean, interval: string, intervals: string[], onInterval: (i: string) => void }} */
  let { symbol, chart, isNew, interval, intervals, onInterval } = $props();

  /** Saved values can be missing, unreadable or from an older build: anything odd falls back. */
  function saved(/** @type {string} */ key, /** @type {any} */ fallback) {
    try {
      return JSON.parse(localStorage.getItem(key) ?? 'null') ?? fallback;
    } catch {
      return fallback;
    }
  }

  function save(/** @type {string} */ key, /** @type {any} */ value) {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch {
      // Not persisted on this device; still used for this visit.
    }
  }

  const prefs = saved(STORE_PREFS, {});
  let type = $state(TYPES.some((t) => t.id === prefs.type) ? prefs.type : TYPE_AREA);
  let studies = $state(Array.isArray(prefs.studies) ? prefs.studies.filter((/** @type {string} */ id) => STUDIES.some((s) => s.id === id)) : DEFAULT_STUDIES);
  let scale = $state([SCALE_LOG, SCALE_PERCENT].includes(prefs.scale) ? prefs.scale : SCALE_LINEAR);
  let tool = $state('');
  let menuOpen = $state(false);
  /** @type {any} the bar under the crosshair and each indicator's value there */
  let legend = $state(null);
  let drawings = $state(/** @type {any[]} */ ([]));
  /** @type {HTMLDivElement} */
  let el;
  /** @type {HTMLDivElement} */
  let menu;
  /** @type {ChartEngine|undefined} */
  let engine = $state();

  onMount(() => {
    const made = new ChartEngine(el, {
      onLegend: (l) => { legend = l; },
      onTool: (t) => { tool = t; },
      onDrawings: (d) => {
        drawings = d;
        save(STORE_DRAWINGS + symbol, d);
      },
    });
    made.setType(type);
    made.setStudies(studies);
    made.setScale(scale);
    engine = made;
    return () => made.destroy();
  });

  // Another symbol brings its own drawings.
  $effect(() => {
    const list = saved(STORE_DRAWINGS + symbol, []);
    drawings = Array.isArray(list) ? list : [];
    engine?.setDrawings(untrack(() => $state.snapshot(drawings)));
  });

  $effect(() => {
    if (!engine || !chart) return;
    engine.load(chart.candles, {
      interval: chart.interval,
      timezone: chart.timezone,
      previousClose: chart.previous_close,
      baseline: chart.range === BASELINE_RANGE,
      regular: chart.regular,
      fit: untrack(() => isNew),
    });
  });

  function remember() {
    save(STORE_PREFS, { type, studies, scale });
  }

  function pickType(/** @type {string} */ id) {
    type = id;
    engine?.setType(id);
    remember();
  }

  function toggleStudy(/** @type {string} */ id) {
    studies = studies.includes(id) ? studies.filter((/** @type {string} */ s) => s !== id) : [...studies, id];
    engine?.setStudies($state.snapshot(studies));
    remember();
  }

  function pickScale(/** @type {string} */ wanted) {
    scale = scale === wanted ? SCALE_LINEAR : wanted;
    engine?.setScale(scale);
    remember();
  }

  function pickTool(/** @type {string} */ id) {
    tool = tool === id ? '' : id;
    engine?.setTool(tool);
  }

  function clearDrawings() {
    drawings = [];
    engine?.setDrawings([]);
    save(STORE_DRAWINGS + symbol, []);
  }

  /** The indicator menu closes on a click anywhere else and on Escape. */
  function onWindowDown(/** @type {PointerEvent} */ event) {
    if (menuOpen && !menu.contains(/** @type {Node} */ (event.target))) menuOpen = false;
  }

  function onKey(/** @type {KeyboardEvent} */ event) {
    if (event.key !== 'Escape') return;
    menuOpen = false;
    if (tool) pickTool(tool);
  }

  const hint = $derived(TOOLS.find((t) => t.id === tool)?.hint ?? '');
  const empty = $derived(Boolean(chart) && chart.candles.length === 0);
</script>

<svelte:window onpointerdown={onWindowDown} onkeydown={onKey} />

<div class="flex min-h-0 flex-1 flex-col">
  <!-- TOOLBAR: wraps onto more rows on narrow screens, so nothing scrolls sideways -->
  <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-line px-4 py-1.5 short:py-1 lg:px-6" role="toolbar" aria-label={strings.chart.studies}>
    <div class="flex gap-1" role="group" aria-label={strings.chart.interval}>
      {#each intervals as i (i)}
        <button type="button" aria-pressed={interval === i} onclick={() => onInterval(i)} class="{BUTTON} font-mono {interval === i ? BUTTON_ON : BUTTON_OFF}">{strings.intervals[i]}</button>
      {/each}
    </div>
    <div class="flex gap-1" role="group">
      {#each TYPES as t (t.id)}
        <button type="button" aria-pressed={type === t.id} title={strings.chart.types[t.id]} aria-label={strings.chart.types[t.id]} onclick={() => pickType(t.id)} class="{BUTTON} {type === t.id ? BUTTON_ON : BUTTON_OFF}">
          <t.icon class="size-4" aria-hidden="true" />
        </button>
      {/each}
    </div>
    <div class="relative" bind:this={menu}>
      <button type="button" aria-expanded={menuOpen} aria-haspopup="true" onclick={() => { menuOpen = !menuOpen; }} class="{BUTTON} gap-1.5 {menuOpen || studies.length ? BUTTON_ON : BUTTON_OFF}">
        <Activity class="size-4" aria-hidden="true" />
        <span class="max-sm:sr-only">{strings.chart.studies}</span>
        <span class="font-mono tabular-nums">{studies.length}</span>
      </button>
      <div class="menu absolute top-full left-0 z-20 mt-1 w-52 rounded-sm border border-line bg-card py-1 shadow-lg {menuOpen ? 'menu-open' : ''}" inert={!menuOpen}>
        {#each STUDIES as s (s.id)}
          <label class="spring flex cursor-pointer items-center gap-2.5 px-3 py-1.5 text-sm text-white/80 hover:bg-white/5">
            <input type="checkbox" checked={studies.includes(s.id)} onchange={() => toggleStudy(s.id)} class="size-3.5 accent-neon-red" />
            <span class="flex-1">{strings.chart.study[s.id]}</span>
            <span class="h-0.5 w-4 rounded-full" style:background="var({s.token})"></span>
          </label>
        {/each}
      </div>
    </div>
    <!-- Drawing and scale controls step aside on very short screens, where the chart needs the room -->
    <div class="flex gap-1 short:hidden" role="group">
      {#each TOOLS as t (t.id)}
        <button type="button" aria-pressed={tool === t.id} title={t.label} aria-label={t.label} onclick={() => pickTool(t.id)} class="{BUTTON} {tool === t.id ? BUTTON_ON : BUTTON_OFF}">
          <t.icon class="size-4" aria-hidden="true" />
        </button>
      {/each}
      <button type="button" disabled={!drawings.length} title={strings.chart.clear} aria-label={strings.chart.clear} onclick={clearDrawings} class="{BUTTON} {BUTTON_OFF} disabled:opacity-30">
        <Eraser class="size-4" aria-hidden="true" />
      </button>
    </div>
    <div class="flex gap-1 short:hidden" role="group">
      <button type="button" aria-pressed={scale === SCALE_LOG} title={strings.chart.logTitle} onclick={() => pickScale(SCALE_LOG)} class="{BUTTON} {scale === SCALE_LOG ? BUTTON_ON : BUTTON_OFF}">{strings.chart.log}</button>
      <button type="button" aria-pressed={scale === SCALE_PERCENT} title={strings.chart.percentTitle} onclick={() => pickScale(SCALE_PERCENT)} class="{BUTTON} {scale === SCALE_PERCENT ? BUTTON_ON : BUTTON_OFF}">{strings.chart.percent}</button>
      <button type="button" title={strings.chart.fit} aria-label={strings.chart.fit} onclick={() => engine?.fit()} class="{BUTTON} {BUTTON_OFF}">
        <Scan class="size-4" aria-hidden="true" />
      </button>
    </div>
  </div>

  <!-- LEGEND: one fixed-height line, so the chart below never moves as the crosshair does -->
  <p class="flex h-6 shrink-0 items-center gap-x-3 overflow-hidden px-4 font-mono text-[0.7rem] whitespace-nowrap tabular-nums text-muted short:h-5 lg:px-6" aria-live="off">
    {#if hint}
      <span class="font-sans font-semibold text-warn">{hint}</span>
    {:else if legend}
      {@const b = legend.bar}
      <span>O <span class="text-white/85">{price(b.open)}</span></span>
      <span>H <span class="text-white/85">{price(b.high)}</span></span>
      <span>L <span class="text-white/85">{price(b.low)}</span></span>
      <span>C <span class="text-white/85">{price(b.close)}</span></span>
      <span class={legend.change < 0 ? 'text-bad' : 'text-ok'}>{signed(legend.change)} ({percent(legend.changePct)})</span>
      <span>{strings.chart.volume} <span class="text-white/85">{compact(b.volume)}</span></span>
      {#each legend.studies as s (s.study + s.label)}
        {#if s.value !== null && s.value !== undefined}
          <span style:color={s.color}>{strings.chart.study[s.study].split(' ')[0]} {s.label} {price(s.value)}</span>
        {/if}
      {/each}
    {/if}
  </p>

  <div class="relative min-h-0 flex-1">
    <div bind:this={el} class="absolute inset-0 font-mono transition-opacity duration-300 {chart ? 'opacity-100' : 'opacity-0'} {tool ? 'cursor-crosshair' : ''}"></div>
    {#if empty}
      <p class="absolute inset-0 flex items-center justify-center text-sm text-muted">{strings.chart.empty}</p>
    {/if}
  </div>
</div>

<style>
  /* The indicator menu drops in: transform and opacity only, and it holds no space when closed. */
  .menu {
    opacity: 0;
    transform: translateY(-6px) scale(0.98);
    transform-origin: top left;
    pointer-events: none;
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), opacity 150ms ease;
  }
  .menu-open {
    opacity: 1;
    transform: none;
    pointer-events: auto;
  }
</style>
