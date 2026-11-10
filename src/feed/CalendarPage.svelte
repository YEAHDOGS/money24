<script>
  /**
   * Calendar page: scheduled events by week. Tablet and up: filter rail | agenda | next-up panel.
   * Phones and portrait desktops: one column with a next-up bar and compact filters on top.
   * Phones on their side: agenda | next-up panel.
   */
  import { fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import strings from './strings.json';
  import CalendarRow from './CalendarRow.svelte';
  import {
    CATEGORY_CLASSES, IMPACT_RANK, countdown, dateKey, groupByDay, matches, startMs, weekDays, weekRange, whenLabel,
  } from './calendar-format.js';

  const SECOND_MS = 1000;
  const WEEK_MIN = -1; // the engine keeps one week back
  const WEEK_MAX = 4; // and serves four weeks ahead
  const MAX_STAGGER = 14;
  const COMING_UP = 8;
  const TOP_REGIONS = 12;
  const MAX_DOTS = 3;
  const SLIDE_PX = 28;
  const SLIDE_MS = 260;
  const SWIPE_PX = 48;
  const HERO_MIN_RANK = 2;
  const CATEGORY_ALL = 'all';
  const CATEGORIES = Object.keys(strings.calendar.categories);
  const IMPACT_FILTERS = { all: 0, medium: 2, high: 3 };
  const DEFAULT_IMPACT = 'medium';
  const STORE_IMPACT = 'm24.calendar.impact';
  const GROUP_CALENDAR = 'calendar';
  const SELECT_CLASS = 'spring min-w-0 flex-1 rounded-sm border border-line bg-card px-1.5 py-1 text-xs text-white focus-visible:outline focus-visible:outline-neon-red';
  const LABEL_CLASS = 'text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted';

  /** @type {{ client: import('./feed-client.svelte.js').FeedClient, now: number, linkTarget: string|undefined }} */
  let { client, now, linkTarget } = $props();

  let category = $state(CATEGORY_ALL);
  let impact = $state(localStorage.getItem(STORE_IMPACT) in IMPACT_FILTERS ? localStorage.getItem(STORE_IMPACT) : DEFAULT_IMPACT);
  let region = $state('');
  let weekOffset = $state(0);
  let direction = $state(1);
  let selectedDay = $state('');
  let clock = $state(Date.now()); // ticks every second, for the countdowns only
  let swipeStart = 0;
  /** @type {HTMLElement | undefined} */
  let listEl = $state();

  $effect(() => localStorage.setItem(STORE_IMPACT, impact));
  $effect(() => {
    const timer = setInterval(() => { clock = Date.now(); }, SECOND_MS);
    return () => clearInterval(timer);
  });

  const minImpact = $derived(IMPACT_FILTERS[impact] ?? 0);
  const filtered = $derived(client.calendar.filter((e) => matches(e, { category, minImpact, region })));
  const byDay = $derived(groupByDay(filtered));
  const todayKey = $derived(dateKey(new Date(now)));
  const days = $derived(weekDays(now, weekOffset).map((d) => {
    const events = byDay[d.key] ?? [];
    return { ...d, events, high: events.filter((e) => e.impact === 'high').length };
  }));
  /** Days that get a section: every day with events, plus today. `first` is the day's place in the reveal stagger. */
  const agenda = $derived.by(() => {
    let first = 0;
    return days.filter((d) => d.events.length || d.key === todayKey).map((d) => {
      const section = { ...d, first, nowIndex: d.events.findIndex((e) => e.all_day || startMs(e) > now) };
      first += d.events.length;
      return section;
    });
  });
  const weekCount = $derived(days.reduce((n, d) => n + d.events.length, 0));

  /** Counts beside each event type, under the current impact and region filters. */
  const categoryCounts = $derived.by(() => {
    /** @type {Record<string, number>} */
    const counts = { [CATEGORY_ALL]: 0 };
    for (const e of client.calendar) {
      if (!matches(e, { category: CATEGORY_ALL, minImpact, region })) continue;
      counts[CATEGORY_ALL] += 1;
      counts[e.category] = (counts[e.category] ?? 0) + 1;
    }
    return counts;
  });
  const regions = $derived.by(() => {
    /** @type {Record<string, number>} */
    const counts = {};
    for (const e of client.calendar) {
      if (e.region && matches(e, { category, minImpact, region: '' })) counts[e.region] = (counts[e.region] ?? 0) + 1;
    }
    return Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, TOP_REGIONS);
  });

  const upcoming = $derived(filtered.filter((e) => !e.all_day && startMs(e) > clock));
  const hero = $derived(upcoming.find((e) => (IMPACT_RANK[e.impact ?? ''] ?? 0) >= HERO_MIN_RANK) ?? upcoming[0]);
  const comingUp = $derived(upcoming.filter((e) => e !== hero).slice(0, COMING_UP));
  const calendarSources = $derived(Object.values(client.sources).filter((s) => s.group === GROUP_CALENDAR));

  /** @param {number} delta weeks to move, negative for back */
  function shiftWeek(delta) {
    const next = Math.min(WEEK_MAX, Math.max(WEEK_MIN, weekOffset + delta));
    if (next === weekOffset) return;
    direction = Math.sign(delta);
    weekOffset = next;
    selectedDay = '';
    listEl?.scrollTo({ top: 0 });
  }

  /** @param {string} key day to bring to the top of the agenda */
  function jumpTo(key) {
    selectedDay = key;
    document.getElementById(`cal-day-${key}`)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }

  /** Swiping the week strip sideways turns the page. @param {PointerEvent} e */
  function endSwipe(e) {
    const moved = e.clientX - swipeStart;
    if (Math.abs(moved) >= SWIPE_PX) shiftWeek(moved < 0 ? 1 : -1);
  }

  /** @param {KeyboardEvent} e */
  function onStripKey(e) {
    if (e.key === 'ArrowLeft') shiftWeek(-1);
    if (e.key === 'ArrowRight') shiftWeek(1);
  }
</script>

{#snippet nowLine()}
  <li class="now-line flex items-center gap-2 px-4 py-1 lg:px-6" aria-label="{strings.calendar.now} {new Date(now).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}">
    <span class="font-mono text-[0.6rem] font-bold uppercase tracking-[0.2em] text-neon-red">{strings.calendar.now}</span>
    <span class="h-px flex-1 bg-neon-red"></span>
  </li>
{/snippet}

{#snippet heroValues()}
  {#if hero?.forecast || hero?.previous}
    <span class="font-mono text-xs tabular-nums text-muted">
      {#if hero.forecast}{strings.calendar.forecast} <span class="text-white/85">{hero.forecast}</span>{/if}
      {#if hero.previous}&nbsp;{strings.calendar.previous} <span class="text-white/85">{hero.previous}</span>{/if}
    </span>
  {/if}
{/snippet}

<div class="mx-auto grid h-full w-full max-w-7xl grid-cols-[minmax(0,1fr)] md:grid-cols-[12rem_minmax(0,1fr)_14rem] lg:grid-cols-[15rem_minmax(0,1fr)_19rem] xl:border-x xl:border-line desktop-portrait:grid-cols-[minmax(0,1fr)]! landscape-phone:grid-cols-[minmax(0,1fr)_14rem]! ultrawide:max-w-none ultrawide:grid-cols-[19rem_minmax(0,1fr)_30rem]">

  <!-- FILTER RAIL (tablet and up) -->
  <aside class="list-scroll hidden min-h-0 overflow-y-auto border-r border-line md:block desktop-portrait:hidden! landscape-phone:hidden!" aria-label={strings.calendar.categoriesLabel}>
    <nav aria-label={strings.calendar.categoriesLabel} class="py-2">
      {#each CATEGORIES as c (c)}
        {@const colors = CATEGORY_CLASSES[c]}
        <button type="button" onclick={() => { category = c; }} aria-pressed={category === c}
          class="spring flex w-full items-center gap-2.5 px-4 py-1.5 text-left text-sm hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {category === c ? 'text-white' : 'text-white/60'}">
          <span class="h-3 w-0.5 rounded-full {colors ? colors.bg : 'bg-neon-red'} {category === c ? 'opacity-100' : 'opacity-30'}"></span>
          <span class="flex-1 truncate">{strings.calendar.categories[c]}</span>
          <span class="font-mono text-xs tabular-nums text-muted">{categoryCounts[c] ?? 0}</span>
        </button>
      {/each}
    </nav>

    <div class="border-t border-line px-4 py-3">
      <h2 class={LABEL_CLASS}>{strings.calendar.impact}</h2>
      <div class="mt-2 grid grid-cols-3 gap-px overflow-hidden rounded-sm border border-line bg-line" role="group" aria-label={strings.calendar.impact}>
        {#each Object.keys(IMPACT_FILTERS) as level (level)}
          <button type="button" onclick={() => { impact = level; }} aria-pressed={impact === level}
            class="spring truncate px-1 py-1 text-[0.68rem] font-bold uppercase tracking-wide focus-visible:outline focus-visible:outline-neon-red {impact === level ? 'bg-neon-red text-app' : 'bg-app text-white/60 hover:text-white'}">
            {strings.calendar.impacts[level]}
          </button>
        {/each}
      </div>
    </div>

    {#if regions.length}
      <div class="border-t border-line px-4 py-3">
        <h2 class={LABEL_CLASS}>{strings.calendar.regions}</h2>
        <ul class="mt-2 flex flex-wrap gap-1.5">
          {#each regions as [r, n] (r)}
            <li>
              <button type="button" onclick={() => { region = region === r ? '' : r; }} aria-pressed={region === r}
                class="spring rounded-sm px-1.5 py-0.5 font-mono text-[0.68rem] font-semibold tracking-wider focus-visible:outline focus-visible:outline-neon-red {region === r ? 'bg-neon-red text-app' : 'bg-white/8 text-white/75 hover:bg-neon-red/25'}">
                {r} <span class="opacity-60">{n}</span>
              </button>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    <div class="border-t border-line py-2">
      <h2 class="px-4 pb-1 {LABEL_CLASS}">{strings.calendar.sources}</h2>
      <ul>
        {#each calendarSources as s (s.id)}
          <li class="flex items-center gap-2 px-4 py-1 text-xs text-white/60" title={s.last_error || ''}>
            <span class="size-1.5 shrink-0 rounded-full {!s.requests ? 'bg-muted/50' : s.last_error ? 'bg-bad' : 'bg-ok'}"></span>
            <span class="flex-1 truncate">{s.label}</span>
          </li>
        {/each}
      </ul>
    </div>
  </aside>

  <!-- AGENDA -->
  <section class="flex min-h-0 flex-col" aria-label={strings.tabCalendar}>
    <!-- Next-up bar: where there is no side panel. Fixed height, so the countdown never moves the page. -->
    <p class="flex h-9 items-center gap-2 border-b border-line px-4 text-xs short:h-7 md:hidden desktop-portrait:flex! landscape-phone:hidden!" aria-live="off">
      <span class="shrink-0 {LABEL_CLASS}">{strings.calendar.nextUp}</span>
      {#if hero}
        <span class="min-w-0 flex-1 truncate font-semibold">{hero.symbol || hero.region} {hero.title}</span>
        <span class="shrink-0 font-mono font-bold tabular-nums text-neon-red">{countdown(startMs(hero) - clock)}</span>
      {:else}
        <span class="min-w-0 flex-1 truncate text-muted">{strings.calendar.nothingNext}</span>
      {/if}
    </p>

    <!-- Week strip: arrows, swipe or the arrow keys turn the week; a day jumps the agenda to it -->
    <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] border-b border-line" role="group" aria-label="{strings.calendar.week} {weekRange(days)}">
      <button type="button" onclick={() => shiftWeek(-1)} disabled={weekOffset <= WEEK_MIN} aria-label={strings.calendar.prevWeek}
        class="spring grid w-9 place-items-center border-r border-line text-white/70 hover:bg-white/5 hover:text-white focus-visible:bg-white/5 focus-visible:outline-none disabled:opacity-25">
        <ChevronLeft class="size-4" aria-hidden="true" />
      </button>
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="grid touch-pan-y overflow-hidden" onpointerdown={(e) => { swipeStart = e.clientX; }} onpointerup={endSwipe} onkeydown={onStripKey}>
        {#key weekOffset}
          <div class="col-start-1 row-start-1 grid grid-cols-7" in:fly={{ x: direction * SLIDE_PX, duration: SLIDE_MS, easing: cubicOut }}>
            {#each days as d (d.key)}
              <button type="button" onclick={() => jumpTo(d.key)} disabled={!d.events.length && d.key !== todayKey} aria-pressed={selectedDay === d.key} title={d.long}
                class="spring relative flex min-w-0 flex-col items-center gap-0.5 py-1.5 hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none disabled:opacity-35 short:py-1">
                <span class="absolute inset-x-2 top-0 h-px bg-neon-red transition-transform duration-150 {selectedDay === d.key ? 'scale-x-100' : 'scale-x-0'}"></span>
                <span class="text-[0.58rem] font-bold uppercase tracking-[0.14em] {d.key === todayKey ? 'text-neon-red' : 'text-muted'}">{d.weekday}</span>
                <span class="font-mono text-sm leading-none font-bold tabular-nums {d.key === todayKey ? 'text-neon-red' : 'text-white'}">{d.number}</span>
                <span class="flex h-1 items-center gap-0.5" aria-hidden="true">
                  {#each { length: Math.min(d.high, MAX_DOTS) } as _, i (i)}<span class="size-1 rounded-full bg-neon-red"></span>{/each}
                  {#if !d.high && d.events.length}<span class="size-1 rounded-full bg-white/30"></span>{/if}
                </span>
              </button>
            {/each}
          </div>
        {/key}
      </div>
      <button type="button" onclick={() => shiftWeek(1)} disabled={weekOffset >= WEEK_MAX} aria-label={strings.calendar.nextWeek}
        class="spring grid w-9 place-items-center border-l border-line text-white/70 hover:bg-white/5 hover:text-white focus-visible:bg-white/5 focus-visible:outline-none disabled:opacity-25">
        <ChevronRight class="size-4" aria-hidden="true" />
      </button>
    </div>

    <!-- Compact filters where the rail is hidden -->
    <div class="flex items-center gap-2 border-b border-line px-4 py-1.5 short:py-1 md:hidden desktop-portrait:flex! landscape-phone:flex!">
      <select bind:value={category} class={SELECT_CLASS} aria-label={strings.calendar.categoriesLabel}>
        {#each CATEGORIES as c (c)}<option value={c}>{strings.calendar.categories[c]} ({categoryCounts[c] ?? 0})</option>{/each}
      </select>
      <select bind:value={impact} class={SELECT_CLASS} aria-label={strings.calendar.impact}>
        {#each Object.keys(IMPACT_FILTERS) as level (level)}<option value={level}>{strings.calendar.impact}: {strings.calendar.impacts[level]}</option>{/each}
      </select>
      <select bind:value={region} class={SELECT_CLASS} aria-label={strings.calendar.regions}>
        <option value="">{strings.calendar.regions}</option>
        {#each regions as [r] (r)}<option value={r}>{r}</option>{/each}
      </select>
    </div>

    <div class="flex items-baseline justify-between gap-2 border-b border-line px-4 py-1.5 text-xs short:hidden lg:px-6">
      <span class="font-semibold">{weekOffset === 0 ? strings.calendar.thisWeek : strings.calendar.week} <span class="text-muted">{weekRange(days)}</span></span>
      <span class="font-mono tabular-nums text-muted">{weekCount} {strings.calendar.events}</span>
    </div>

    <div bind:this={listEl} class="list-scroll min-h-0 flex-1 overflow-y-auto">
      {#key weekOffset}
        <div in:fly={{ x: direction * SLIDE_PX, duration: SLIDE_MS, easing: cubicOut }}>
          {#each agenda as d (d.key)}
            <section id="cal-day-{d.key}" aria-label={d.long}>
              <h3 class="sticky top-0 z-10 flex items-baseline gap-2 border-b border-line bg-app/95 px-4 py-1.5 text-xs font-bold backdrop-blur-sm lg:px-6">
                <span class={d.key === todayKey ? 'text-neon-red' : 'text-white'}>{d.long}</span>
                {#if d.key === todayKey}<span class="text-[0.6rem] uppercase tracking-[0.2em] text-neon-red">{strings.calendar.today}</span>{/if}
                <span class="ml-auto font-mono font-normal tabular-nums text-muted">{d.events.length}</span>
              </h3>
              <ol>
                {#each d.events as event, i (event.uid)}
                  {#if d.key === todayKey && i === d.nowIndex}{@render nowLine()}{/if}
                  <li class="cal-rise border-b border-line" style:--i={Math.min(d.first + i, MAX_STAGGER)}>
                    <CalendarRow {event} {now} {linkTarget} />
                  </li>
                {/each}
                {#if d.key === todayKey && d.nowIndex === -1}{@render nowLine()}{/if}
              </ol>
            </section>
          {:else}
            <p class="px-6 py-16 text-center text-sm text-muted">
              {#if client.status === 'offline'}{strings.emptyOffline}{:else if !client.calendarLoaded}{strings.calendar.loading}{:else}{strings.calendar.empty}{/if}
            </p>
          {/each}
        </div>
      {/key}
    </div>
  </section>

  <!-- NEXT UP (tablet and up, and phones on their side) -->
  <aside class="list-scroll hidden min-h-0 overflow-y-auto border-l border-line md:block desktop-portrait:hidden! landscape-phone:block!" aria-label={strings.calendar.nextUp}>
    <div class="border-b border-line px-4 py-4 short:py-2">
      <h2 class={LABEL_CLASS}>{strings.calendar.nextUp}</h2>
      {#if hero}
        <p class="mt-2 font-mono text-3xl leading-none font-extrabold tabular-nums text-neon-red short:mt-1 short:text-xl" role="timer">{countdown(startMs(hero) - clock)}</p>
        <p class="mt-2 text-sm leading-snug font-semibold short:mt-1">
          {#if hero.symbol || hero.region}<span class="font-mono text-white/70">{hero.symbol || hero.region}</span>{/if}
          {hero.title}
        </p>
        <p class="mt-1 flex flex-wrap items-baseline gap-x-3 font-mono text-xs tabular-nums text-muted">
          <span>{whenLabel(hero)}</span>
          {@render heroValues()}
        </p>
      {:else}
        <p class="mt-2 text-sm text-muted">{strings.calendar.nothingNext}</p>
      {/if}
    </div>

    {#if comingUp.length}
      <h2 class="px-4 pt-3 pb-1 {LABEL_CLASS}">{strings.calendar.comingUp}</h2>
      <ol>
        {#each comingUp as e (e.uid)}
          <li class="grid grid-cols-[minmax(0,1fr)_auto] gap-x-2 border-b border-line px-4 py-2 text-xs">
            <span class="truncate font-medium text-white/90">
              {#if e.symbol || e.region}<span class="font-mono text-white/60">{e.symbol || e.region}</span>{/if}
              {e.title}
            </span>
            <span class="font-mono tabular-nums text-neon-red">{countdown(startMs(e) - clock)}</span>
            <span class="col-span-2 font-mono text-[0.68rem] tabular-nums text-muted">{whenLabel(e)}{#if e.forecast} · {strings.calendar.forecast} {e.forecast}{/if}</span>
          </li>
        {/each}
      </ol>
    {/if}
  </aside>
</div>
