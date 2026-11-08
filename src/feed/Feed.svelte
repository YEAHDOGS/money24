<script>
  /**
   * Money 24 live news feed. Desktop and tablet: rail | feed | network monitor, split by 1px lines.
   * Phones: one column with Feed / Network tabs at the bottom, within thumb reach.
   */
  import { onMount } from 'svelte';
  import { flip } from 'svelte/animate';
  import { cubicOut } from 'svelte/easing';
  import strings from './strings.json';
  import pricing from './pricing.json';
  import { FeedClient } from './feed-client.svelte.js';
  import { KIND_CLASSES, FALLBACK_KIND } from './post-format.js';
  import { formatInterval, monthlyCost, projectMonth } from './net-math.js';
  import FeedPost from './FeedPost.svelte';
  import PollSlider from './PollSlider.svelte';
  import NetworkMonitor from './NetworkMonitor.svelte';

  const CLOCK_TICK_MS = 5000;
  const HOLD_PX = 24;
  const ENTER_MS = 460;
  const ENTER_STAGGER_MS = 45;
  const ENTER_OFFSET_PX = 18;
  const FLIP_MS = 320;
  const TOP_TAGS = 14;
  const DEFAULT_EVERY_MS = 60000;
  const MS_PER_SECOND = 1000;
  const KIND_ALL = 'all';
  const KINDS = Object.keys(strings.kinds).filter((k) => k !== KIND_ALL);
  const TAB_FEED = 'feed';
  const TAB_NETWORK = 'network';
  const STORE_PROVIDER = 'm24.news.provider';
  const STORE_WORST = 'm24.news.worstCase';
  const IS_TAURI = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
  // In the desktop app, links navigate and the shell hands them to the system browser.
  const LINK_TARGET = IS_TAURI ? undefined : '_blank';
  const STATUS_DOT = { live: 'text-ok', connecting: 'text-warn', reconnecting: 'text-warn', offline: 'text-bad' };

  const client = new FeedClient();
  let now = $state(Date.now());
  let kind = $state(KIND_ALL);
  let tag = $state('');
  let feed = $state('');
  let tab = $state(TAB_FEED);
  let providerId = $state(localStorage.getItem(STORE_PROVIDER) ?? pricing.providers[0].id);
  let worstCase = $state(localStorage.getItem(STORE_WORST) !== 'false');
  /** @type {HTMLElement | undefined} */
  let listEl = $state();

  $effect(() => localStorage.setItem(STORE_PROVIDER, providerId));
  $effect(() => localStorage.setItem(STORE_WORST, String(worstCase)));

  onMount(() => {
    client.start();
    const tick = setInterval(() => { now = Date.now(); }, CLOCK_TICK_MS);
    return () => {
      clearInterval(tick);
      client.stop();
    };
  });

  const visible = $derived(client.items.filter((p) =>
    (kind === KIND_ALL || p.kind === kind) && (!tag || p.tags.includes(tag)) && (!feed || p.feed === feed)));
  const kindCounts = $derived(client.items.reduce((acc, p) => {
    acc[p.kind] = (acc[p.kind] ?? 0) + 1;
    return acc;
  }, /** @type {Record<string, number>} */ ({})));
  const topTags = $derived.by(() => {
    /** @type {Record<string, number>} */
    const counts = {};
    for (const p of client.items) for (const t of p.tags) counts[t] = (counts[t] ?? 0) + 1;
    return Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, TOP_TAGS);
  });

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
    .filter((s) => s.min_every_ms > everyMs)
    .map((s) => `${s.label.split(' ')[0]} ≥ ${formatInterval(s.min_every_ms)}`))]);
  const feedLabel = $derived(client.sources[feed]?.label ?? feed);

  /** New posts slide down into place and fade in; only the first few of a burst animate. */
  function enter(node, { post }) {
    if (!post.fresh) return { duration: 0 };
    return {
      delay: post.order * ENTER_STAGGER_MS,
      duration: ENTER_MS,
      easing: cubicOut,
      css: (t) => `transform: translateY(${(t - 1) * ENTER_OFFSET_PX}px); opacity: ${t}`,
    };
  }

  function onListScroll() {
    client.holding = (listEl?.scrollTop ?? 0) > HOLD_PX;
    if (!client.holding) client.release();
  }

  function showNew() {
    listEl?.scrollTo({ top: 0, behavior: 'smooth' });
    client.holding = false;
    client.release();
  }

</script>

<svelte:head><title>{strings.title}</title></svelte:head>

<div class="mx-auto grid h-dvh w-full max-w-7xl grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)_auto] select-text border-line md:grid-cols-[13rem_minmax(0,1fr)_17rem] md:grid-rows-1 lg:grid-cols-[15rem_minmax(0,1fr)_22rem] xl:border-x desktop-portrait:grid-cols-[15rem_minmax(0,1fr)]! desktop-portrait:grid-rows-[minmax(0,1fr)_minmax(0,36%)]! landscape-phone:grid-cols-[minmax(0,1fr)_15rem]! landscape-phone:grid-rows-[auto_minmax(0,1fr)]! ultrawide:max-w-none ultrawide:grid-cols-[20rem_minmax(0,1fr)_34rem]">

  <!-- RAIL: brand and status on every size; filters and source health from tablet up -->
  <aside class="flex min-h-0 flex-col border-b border-line md:border-r md:border-b-0 landscape-phone:col-span-2 landscape-phone:border-r-0! landscape-phone:border-b!">
    <div class="flex items-center justify-between gap-3 px-4 py-3 short:py-1.5 md:flex-col md:items-start md:py-5 landscape-phone:flex-row! landscape-phone:items-center! landscape-phone:py-1.5!">
      <h1 class="text-2xl leading-none font-extrabold tracking-tight short:text-lg md:text-3xl">
        {strings.brand}<span class="text-neon-red">{strings.brandAccent}</span>
        <span class="ml-1 align-middle text-[0.6rem] font-bold tracking-[0.3em] text-muted">NEWS</span>
      </h1>
      <p class="flex items-center gap-2 text-xs font-semibold {STATUS_DOT[client.status]}" role="status">
        <span class="relative size-2 rounded-full bg-current {client.status === 'live' ? 'live-dot' : ''}"></span>
        {strings.status[client.status]}
      </p>
    </div>

    <div class="list-scroll hidden min-h-0 flex-1 overflow-y-auto md:block landscape-phone:hidden!">
      <nav aria-label="Categories" class="border-t border-line py-2">
        {#each [KIND_ALL, ...KINDS] as k (k)}
          {@const c = KIND_CLASSES[k] ?? FALLBACK_KIND}
          <button
            type="button"
            class="spring flex w-full items-center gap-2.5 px-4 py-1.5 text-left text-sm hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {kind === k ? 'text-white' : 'text-white/60'}"
            aria-pressed={kind === k}
            onclick={() => { kind = k; }}
          >
            <span class="h-3 w-0.5 rounded-full {k === KIND_ALL ? 'bg-neon-red' : c.bg} {kind === k ? 'opacity-100' : 'opacity-30'}"></span>
            <span class="flex-1 truncate">{strings.kinds[k]}</span>
            <span class="font-mono text-xs tabular-nums text-muted">{k === KIND_ALL ? client.items.length : kindCounts[k] ?? 0}</span>
          </button>
        {/each}
      </nav>

      {#if topTags.length}
        <div class="border-t border-line px-4 py-3">
          <ul class="flex flex-wrap gap-1.5" aria-label="Top tags">
            {#each topTags as [t, n] (t)}
              <li>
                <button type="button" onclick={() => { tag = tag === t ? '' : t; }} aria-pressed={tag === t}
                  class="spring rounded-sm px-1.5 py-0.5 font-mono text-[0.68rem] font-semibold tracking-wider focus-visible:outline focus-visible:outline-neon-red {tag === t ? 'bg-neon-red text-app' : 'bg-white/8 text-white/75 hover:bg-neon-red/25'}">
                  {t} <span class="opacity-60">{n}</span>
                </button>
              </li>
            {/each}
          </ul>
        </div>
      {/if}

      <div class="border-t border-line py-2">
        <h2 class="px-4 pb-1 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.sources}</h2>
        <ul>
          {#each sourceList as s (s.id)}
            <li>
              <button type="button" onclick={() => { feed = feed === s.id ? '' : s.id; }} aria-pressed={feed === s.id}
                title={s.last_error || ''}
                class="spring flex w-full items-center gap-2 px-4 py-1 text-left text-xs hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {feed === s.id ? 'text-white' : 'text-white/60'}">
                <span class="size-1.5 shrink-0 rounded-full {!s.requests ? 'bg-muted/50' : s.last_error ? 'bg-bad' : 'bg-ok'}"></span>
                <span class="flex-1 truncate">{s.label}</span>
                <span class="font-mono tabular-nums text-muted">{s.added}</span>
              </button>
            </li>
          {/each}
        </ul>
      </div>
    </div>
  </aside>

  <!-- FEED -->
  <main class="flex min-h-0 flex-col border-line md:border-r landscape-phone:flex! landscape-phone:border-r {tab === TAB_NETWORK ? 'max-md:hidden' : ''}">
    <PollSlider
      {everyMs}
      onChange={(ms) => client.setPollEvery(ms)}
      {projection}
      {cost}
      {floored}
      providerName={provider.name}
    />

    <div class="flex flex-wrap gap-1.5 border-b border-line px-4 py-2 short:py-1 md:hidden landscape-phone:flex!" role="group" aria-label="Categories">
      {#each [KIND_ALL, ...KINDS] as k (k)}
        {@const c = KIND_CLASSES[k] ?? FALLBACK_KIND}
        <button type="button" onclick={() => { kind = k; }} aria-pressed={kind === k}
          class="spring rounded-sm border px-2 py-0.5 text-[0.7rem] font-bold uppercase tracking-wider {kind === k ? `${k === KIND_ALL ? 'border-neon-red text-neon-red' : `${c.border} ${c.text}`} bg-white/5` : 'border-line text-white/55'}">
          {strings.kinds[k]}
        </button>
      {/each}
    </div>

    {#if tag || feed}
      <div class="flex items-center gap-2 border-b border-line px-4 py-1.5 text-xs">
        {#if feed}<span class="rounded-sm bg-white/8 px-1.5 py-px">{feedLabel}</span>{/if}
        {#if tag}<span class="rounded-sm bg-neon-red px-1.5 py-px font-mono font-semibold text-app">{tag}</span>{/if}
        <button type="button" class="spring ml-auto text-muted hover:text-white" onclick={() => { kind = KIND_ALL; tag = ''; feed = ''; }}>{strings.clearFilter}</button>
      </div>
    {/if}

    <div class="relative min-h-0 flex-1">
      <!-- The pill floats over the list, so showing it never shifts a post -->
      <div class="pointer-events-none absolute inset-x-0 top-3 z-10 flex justify-center">
        <button
          type="button"
          onclick={showNew}
          class="spring pointer-events-auto rounded-full bg-neon-red px-3 py-1 text-xs font-bold text-app shadow-lg {client.pending.length ? 'translate-y-0 opacity-100' : 'pointer-events-none! -translate-y-3 opacity-0'}"
          tabindex={client.pending.length ? 0 : -1}
          aria-hidden={!client.pending.length}
        >↑ {client.pending.length} {strings.newPosts}</button>
      </div>

      <ol bind:this={listEl} onscroll={onListScroll} class="list-scroll h-full overflow-y-auto">
        {#each visible as post (post.id)}
          <li
            class="relative border-b border-line {post.fresh ? 'post-fresh' : ''}"
            style:--flash-delay="{(post.order ?? 0) * ENTER_STAGGER_MS}ms"
            in:enter={{ post }}
            animate:flip={{ duration: FLIP_MS, easing: cubicOut }}
          >
            <FeedPost {post} {now} linkTarget={LINK_TARGET} onTag={(t) => { tag = tag === t ? '' : t; }} activeTag={tag} />
          </li>
        {:else}
          <li class="px-6 py-16 text-center text-sm text-muted">
            {#if client.status === 'offline'}{strings.emptyOffline}{:else if client.items.length}{strings.emptyFilter}{:else}{strings.emptyLive}{/if}
          </li>
        {/each}
      </ol>
    </div>
  </main>

  <!-- NETWORK MONITOR -->
  <div class="min-h-0 border-line desktop-portrait:col-span-2 desktop-portrait:border-t landscape-phone:block! {tab === TAB_FEED ? 'max-md:hidden' : ''}">
    <NetworkMonitor
      {client}
      providers={pricing.providers}
      bind:providerId
      bind:worstCase
      note={pricing.note}
      {projection}
      {cost}
      {everyMs}
    />
  </div>

  <!-- PHONE TABS: bottom of the screen, within thumb reach -->
  <nav class="grid grid-cols-2 border-t border-line md:hidden landscape-phone:hidden!" aria-label="Views">
    {#each [[TAB_FEED, strings.tabFeed], [TAB_NETWORK, strings.tabNetwork]] as [id, label] (id)}
      <button type="button" onclick={() => { tab = id; }} aria-pressed={tab === id}
        class="spring relative py-3 text-xs font-bold uppercase tracking-[0.2em] short:py-2 {tab === id ? 'text-white' : 'text-muted'}">
        <span class="absolute inset-x-6 top-0 h-px bg-neon-red transition-transform duration-150 {tab === id ? 'scale-x-100' : 'scale-x-0'}"></span>
        {label}
        {#if id === TAB_FEED && client.pending.length}<span class="ml-1 text-neon-red">•</span>{/if}
      </button>
    {/each}
  </nav>
</div>
