<script>
  /**
   * Money 24 news app shell: a slim header, one page at a time, and a tab bar along the bottom on
   * every screen. A page is one entry in TABS plus its component below; pages stay mounted, so
   * switching tabs keeps each one's scroll position and filters.
   */
  import { onMount } from 'svelte';
  import Newspaper from '@lucide/svelte/icons/newspaper';
  import CalendarDays from '@lucide/svelte/icons/calendar-days';
  import Activity from '@lucide/svelte/icons/activity';
  import strings from './strings.json';
  import pricing from './pricing.json';
  import { FeedClient } from './feed-client.svelte.js';
  import { formatInterval, monthlyCost, projectMonth } from './net-math.js';
  import NewsPage from './NewsPage.svelte';
  import CalendarPage from './CalendarPage.svelte';
  import NetworkPage from './NetworkPage.svelte';

  const CLOCK_TICK_MS = 5000;
  const DEFAULT_EVERY_MS = 60000;
  const MS_PER_SECOND = 1000;
  const PERCENT = 100;
  const TAB_NEWS = 'news';
  const TAB_CALENDAR = 'calendar';
  const TAB_NETWORK = 'network';
  /** The tab bar, left to right. Add a page here and render it in the panel loop below. */
  const TABS = [
    { id: TAB_NEWS, label: strings.tabNews, icon: Newspaper },
    { id: TAB_CALENDAR, label: strings.tabCalendar, icon: CalendarDays },
    { id: TAB_NETWORK, label: strings.tabNetwork, icon: Activity },
  ];
  const STORE_TAB = 'm24.news.tab';
  const STORE_PROVIDER = 'm24.news.provider';
  const STORE_WORST = 'm24.news.worstCase';
  const IS_TAURI = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
  // In the desktop app, links navigate and the shell hands them to the system browser.
  const LINK_TARGET = IS_TAURI ? undefined : '_blank';
  const STATUS_DOT = { live: 'text-ok', connecting: 'text-warn', reconnecting: 'text-warn', offline: 'text-bad' };

  /** The tab named in the address bar (#calendar), else the last one used, else the first. */
  function currentTab() {
    const wanted = [location.hash.slice(1), localStorage.getItem(STORE_TAB)];
    return wanted.find((id) => TABS.some((t) => t.id === id)) ?? TABS[0].id;
  }

  const client = new FeedClient();
  let now = $state(Date.now());
  let tab = $state(currentTab());
  let providerId = $state(localStorage.getItem(STORE_PROVIDER) ?? pricing.providers[0].id);
  let worstCase = $state(localStorage.getItem(STORE_WORST) !== 'false');

  $effect(() => localStorage.setItem(STORE_TAB, tab));
  $effect(() => localStorage.setItem(STORE_PROVIDER, providerId));
  $effect(() => localStorage.setItem(STORE_WORST, String(worstCase)));

  onMount(() => {
    client.start();
    const tick = setInterval(() => { now = Date.now(); }, CLOCK_TICK_MS);
    // Tabs are links to #id, so the browser's back button and a phone's back-swipe step through them.
    const onHash = () => { tab = currentTab(); };
    window.addEventListener('hashchange', onHash);
    return () => {
      clearInterval(tick);
      window.removeEventListener('hashchange', onHash);
      client.stop();
    };
  });

  const activeIndex = $derived(TABS.findIndex((t) => t.id === tab));
  const everyMs = $derived(client.settings?.poll_every_ms ?? DEFAULT_EVERY_MS);
  const sourceList = $derived(Object.values(client.sources));
  const provider = $derived(pricing.providers.find((p) => p.id === providerId) ?? pricing.providers[0]);
  /** Recent bytes/ms streamed to this UI; the session average would be skewed by the first page load. */
  const servedPerMs = $derived.by(() => {
    const h = client.history;
    if (!h.length) return 0;
    return h.reduce((sum, p) => sum + p.servedRate, 0) / h.length / MS_PER_SECOND;
  });
  const projection = $derived(projectMonth(sourceList, everyMs, servedPerMs));
  const cost = $derived(monthlyCost(projection, provider, worstCase));
  /** Sources whose politeness floor overrides the slider, grouped by family ("GDELT ≥ 5m"). */
  const floored = $derived([...new Set(sourceList
    .filter((s) => s.group !== TAB_CALENDAR && s.min_every_ms > everyMs)
    .map((s) => `${s.label.split(' ')[0]} ≥ ${formatInterval(s.min_every_ms)}`))]);
  const slider = $derived({
    everyMs,
    onChange: (ms) => client.setPollEvery(ms),
    projection,
    cost,
    floored,
    providerName: provider.name,
    plan: client.plan,
    report: client.report,
  });
</script>

<svelte:head><title>{strings.title}</title></svelte:head>

<div class="grid h-dvh w-full grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)_auto] select-text">

  <!-- HEADER: brand, engine status, link to the trading desk -->
  <header class="border-b border-line">
    <div class="mx-auto flex max-w-7xl items-center gap-3 px-4 py-2 short:py-1 ultrawide:max-w-none">
      <h1 class="text-xl leading-none font-extrabold tracking-tight short:text-base md:text-2xl">
        {strings.brand}<span class="text-neon-red">{strings.brandAccent}</span>
        <span class="ml-1 align-middle text-[0.6rem] font-bold tracking-[0.3em] text-muted">NEWS</span>
      </h1>
      <p class="ml-auto flex items-center gap-2 text-xs font-semibold {STATUS_DOT[client.status]}" role="status">
        <span class="relative size-2 rounded-full bg-current {client.status === 'live' ? 'live-dot' : ''}"></span>
        {strings.status[client.status]}
      </p>
      <a href="trading.html" class="spring text-xs font-semibold text-white/60 underline-offset-4 hover:text-white hover:underline focus-visible:text-white focus-visible:outline-none">{strings.tradingLink}&nbsp;→</a>
    </div>
  </header>

  <!-- PAGES: stacked in one cell; the active one slides in, so nothing around them moves -->
  <main class="relative min-h-0 overflow-hidden">
    {#each TABS as t, i (t.id)}
      <div
        id="panel-{t.id}"
        role="tabpanel"
        aria-labelledby="tab-{t.id}"
        class="absolute inset-0 transition-[transform,opacity,visibility] duration-300 ease-out {tab === t.id ? 'visible translate-x-0 opacity-100' : `invisible opacity-0 ${i < activeIndex ? '-translate-x-8' : 'translate-x-8'}`}"
      >
        {#if t.id === TAB_NEWS}
          <NewsPage {client} {now} linkTarget={LINK_TARGET} {slider} />
        {:else if t.id === TAB_CALENDAR}
          <CalendarPage {client} {now} linkTarget={LINK_TARGET} />
        {:else if t.id === TAB_NETWORK}
          <NetworkPage {client} providers={pricing.providers} bind:providerId bind:worstCase note={pricing.note} {projection} {cost} {everyMs} linkTarget={LINK_TARGET} />
        {/if}
      </div>
    {/each}
  </main>

  <!-- TAB BAR: bottom of every screen, within thumb reach on phones -->
  <nav class="border-t border-line" aria-label={strings.views}>
    <div class="relative mx-auto grid max-w-7xl ultrawide:max-w-none" role="tablist" style:grid-template-columns="repeat({TABS.length}, minmax(0, 1fr))">
      <!-- One marker slides between tabs -->
      <span class="pointer-events-none absolute top-0 left-0 h-0.5 bg-neon-red transition-transform duration-300 ease-out" style:width="{PERCENT / TABS.length}%" style:transform="translateX({activeIndex * PERCENT}%)"></span>
      {#each TABS as t (t.id)}
        {@const Icon = t.icon}
        <a
          id="tab-{t.id}"
          href="#{t.id}"
          role="tab"
          aria-selected={tab === t.id}
          aria-controls="panel-{t.id}"
          class="spring flex items-center justify-center gap-2 py-3 text-xs font-bold uppercase tracking-[0.2em] hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none short:py-1.5 max-[359px]:flex-col max-[359px]:gap-0.5 max-[359px]:text-[0.6rem] max-[359px]:tracking-widest {tab === t.id ? 'text-white' : 'text-muted hover:text-white/80'}"
        >
          <Icon class="size-4 shrink-0 transition-transform duration-150 {tab === t.id ? 'scale-110 text-neon-red' : ''}" aria-hidden="true" />
          <span class="truncate">{t.label}</span>
          {#if t.id === TAB_NEWS && client.pending.length}<span class="size-1.5 rounded-full bg-neon-red" aria-hidden="true"></span>{/if}
        </a>
      {/each}
    </div>
  </nav>
</div>
