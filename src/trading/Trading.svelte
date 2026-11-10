<script>
  /**
   * Money 24 trading desk: what the paper runner saw and what it planned, live.
   * Desktop and tablet: rail | journal | plan, split by 1px lines.
   * Phones: one column with Journal / Plan / Sources tabs at the bottom, within thumb reach.
   */
  import { onMount, untrack } from 'svelte';
  import { cubicOut } from 'svelte/easing';
  import strings from './strings.json';
  import { TradingClient, DEFAULT_RUNNER, bucket, contractLabel } from './trading-client.svelte.js';
  import PlanLadder from './PlanLadder.svelte';
  import ShareBox from './ShareBox.svelte';
  import RunnerSettings from './RunnerSettings.svelte';

  const CLOCK_TICK_MS = 1000;
  const ENTER_MS = 420;
  const ENTER_OFFSET_PX = 16;
  const MS_PER_MINUTE = 60000;
  const MINUTES_PER_HOUR = 60;
  const HOURS_PER_DAY = 24;
  const HOURS_SHOWN_MAX = 48; // past two days, ages read in days
  const PERCENT = 100;
  const ALL = 'all';
  const BUCKETS = ['entry', 'exit', 'trim', 'add', 'rejected', 'noise'];
  const TAB_JOURNAL = 'journal';
  const TAB_DETAIL = 'detail';
  const TAB_SOURCES = 'sources';
  const TABS = [TAB_JOURNAL, TAB_DETAIL, TAB_SOURCES];
  const STORE_RUNNER = 'm24.trading.runner';
  const STORE_SECRET = 'm24.trading.secret';
  // Android share sheet (PWA share_target) and links land here as ?title=&text=&url=.
  const SHARE_PARAMS = ['title', 'text', 'url'];
  const STATUS_DOT = { live: 'text-ok', connecting: 'text-warn', reconnecting: 'text-warn', offline: 'text-bad' };
  // Literal class strings so Tailwind sees them. Colors come from theme tokens.
  const BUCKET_CLASSES = {
    entry: 'text-ok border-ok',
    exit: 'text-kind-news border-kind-news',
    trim: 'text-kind-filing border-kind-filing',
    add: 'text-kind-insider border-kind-insider',
    rejected: 'text-bad border-bad',
    noise: 'text-muted border-line',
  };

  /** Browser storage can be missing or throw (private mode, some WebViews): fall back to defaults. */
  function stored(/** @type {string} */ key, /** @type {string} */ fallback) {
    try {
      return localStorage.getItem(key) ?? fallback;
    } catch {
      return fallback;
    }
  }

  let runnerUrl = $state(stored(STORE_RUNNER, DEFAULT_RUNNER));
  let runnerSecret = $state(stored(STORE_SECRET, ''));
  let client = $state(untrack(() => new TradingClient(runnerUrl, runnerSecret)));
  /** The post id (or raw text) of the last share, so its journal line opens when it lands. */
  let awaiting = '';
  /** A share that arrived through the share sheet, prefilled into the box and sent once. */
  const sharedIn = SHARE_PARAMS.map((k) => new URLSearchParams(location.search).get(k) ?? '').filter(Boolean).join(' ');
  let now = $state(Date.now());
  let filter = $state(ALL);
  let tab = $state(TAB_JOURNAL);
  let selectedId = $state(0);

  function connect() {
    client.onEntry = (e) => {
      if (!awaiting || (e.signal.post_id || e.signal.raw) !== awaiting) return;
      awaiting = '';
      pick(e.id);
    };
    client.start();
  }

  function saveRunner(/** @type {string} */ url, /** @type {string} */ secret) {
    try {
      localStorage.setItem(STORE_RUNNER, url || DEFAULT_RUNNER);
      localStorage.setItem(STORE_SECRET, secret);
    } catch {
      // Not persisted on this device; still used for this visit.
    }
    client.stop();
    runnerUrl = url || DEFAULT_RUNNER;
    runnerSecret = secret;
    client = new TradingClient(runnerUrl, runnerSecret);
    connect();
  }

  /** Sends one post to the runner and reports how it was read. Its plan opens when the journal line lands. */
  async function share(/** @type {string} */ input) {
    const sig = await client.share(input);
    tab = TAB_JOURNAL;
    awaiting = sig.post_id || sig.raw;
    // The stream can beat the HTTP reply: open the plan now if its line already landed.
    const landed = client.items.find((e) => (e.signal.post_id || e.signal.raw) === awaiting);
    if (landed) {
      awaiting = '';
      pick(landed.id);
    }
    if (sig.kind === 'noise') return strings.shareNoise;
    return `${strings.shareRead} ${strings.actions[sig.kind] ?? sig.kind} · ${contractLabel(sig)} · ${sig.source_id}`;
  }

  onMount(() => {
    connect();
    const tick = setInterval(() => { now = Date.now(); }, CLOCK_TICK_MS);
    if (sharedIn) {
      history.replaceState(null, '', location.pathname);
      share(sharedIn).catch(() => {});
    }
    return () => {
      clearInterval(tick);
      client.stop();
    };
  });

  const counts = $derived(client.items.reduce((acc, e) => {
    const b = bucket(e);
    acc[b] = (acc[b] ?? 0) + 1;
    return acc;
  }, /** @type {Record<string, number>} */ ({})));
  const visible = $derived(filter === ALL ? client.items : client.items.filter((e) => bucket(e) === filter));
  const selected = $derived(client.items.find((e) => e.id === selectedId) ?? client.items.find((e) => e.plan));
  const sourcesByRole = $derived(client.sources.reduce((acc, src) => {
    const role = src.role ?? 'callouts';
    (acc[role] ??= []).push(src);
    return acc;
  }, /** @type {Record<string, any[]>} */ ({})));

  /** "4m", "2h 10m", "in 3h 59m" style clocks for ages and countdowns. */
  function span(/** @type {number} */ ms) {
    const m = Math.max(0, Math.round(ms / MS_PER_MINUTE));
    if (m < MINUTES_PER_HOUR) return `${m}m`;
    const h = Math.floor(m / MINUTES_PER_HOUR);
    if (h >= HOURS_SHOWN_MAX) return `${Math.floor(h / HOURS_PER_DAY)}d`;
    return `${h}h ${m % MINUTES_PER_HOUR}m`;
  }

  /** New journal lines slide down into place and fade in. Transform and opacity only. */
  function enter(node, { entry }) {
    if (!entry.fresh) return { duration: 0 };
    return {
      duration: ENTER_MS,
      easing: cubicOut,
      css: (t) => `transform: translateY(${(t - 1) * ENTER_OFFSET_PX}px); opacity: ${t}`,
    };
  }

  function pick(/** @type {number} */ id) {
    selectedId = id;
    tab = TAB_DETAIL;
  }
</script>

<svelte:head><title>{strings.title}</title></svelte:head>

<div class="mx-auto grid h-dvh w-full max-w-7xl grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)_auto] select-text border-line md:grid-cols-[13rem_minmax(0,1fr)_19rem] md:grid-rows-1 lg:grid-cols-[15rem_minmax(0,1fr)_24rem] xl:border-x desktop-portrait:grid-cols-[15rem_minmax(0,1fr)]! desktop-portrait:grid-rows-[minmax(0,1fr)_minmax(0,40%)]! landscape-phone:grid-cols-[minmax(0,1fr)_17rem]! landscape-phone:grid-rows-[auto_minmax(0,1fr)]! ultrawide:max-w-none ultrawide:grid-cols-[20rem_minmax(0,1fr)_34rem]">

  <!-- RAIL: brand, status and counts on every size; sources from tablet up -->
  <aside class="flex min-h-0 flex-col border-b border-line md:border-r md:border-b-0 landscape-phone:col-span-2 landscape-phone:border-r-0! landscape-phone:border-b! {tab === TAB_SOURCES ? 'max-md:row-span-2' : ''}">
    <div class="flex items-center justify-between gap-3 px-4 py-3 short:py-1.5 md:flex-col md:items-start md:py-5 landscape-phone:flex-row! landscape-phone:items-center! landscape-phone:py-1.5!">
      <h1 class="text-2xl leading-none font-extrabold tracking-tight short:text-lg md:text-3xl">
        {strings.brand}<span class="text-neon-red">{strings.brandAccent}</span>
        <span class="ml-1 align-middle text-[0.6rem] font-bold tracking-[0.3em] text-muted">{strings.section}</span>
      </h1>
      <div class="flex items-center gap-3 md:flex-col md:items-start md:gap-1.5 landscape-phone:flex-row! landscape-phone:items-center!">
        <p class="flex items-center gap-2 text-xs font-semibold {STATUS_DOT[client.status]}" role="status">
          <span class="relative size-2 rounded-full bg-current {client.status === 'live' ? 'live-dot' : ''}"></span>
          {strings.status[client.status]}
        </p>
        <span class="whitespace-nowrap rounded-sm border border-warn px-1.5 py-px text-[0.62rem] font-bold uppercase tracking-wider text-warn">{strings.paper}</span>
        <a href="feed.html" class="spring text-xs font-semibold text-white/60 underline-offset-4 hover:text-white hover:underline focus-visible:text-white focus-visible:outline-none">{strings.newsLink}&nbsp;→</a>
      </div>
    </div>

    <nav aria-label="Filter" class="hidden border-t border-line py-2 md:block landscape-phone:hidden!">
      {#each [ALL, ...BUCKETS] as b (b)}
        <button type="button" aria-pressed={filter === b} onclick={() => { filter = b; }}
          class="spring flex w-full items-center gap-2.5 px-4 py-1.5 text-left text-sm hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {filter === b ? 'text-white' : 'text-white/60'}">
          <span class="h-3 w-0.5 rounded-full border {b === ALL ? 'border-neon-red bg-neon-red' : BUCKET_CLASSES[b]} {filter === b ? 'opacity-100' : 'opacity-40'}"></span>
          <span class="flex-1 truncate">{strings.actions[b]}</span>
          <span class="font-mono text-xs tabular-nums text-muted">{b === ALL ? client.items.length : counts[b] ?? 0}</span>
        </button>
      {/each}
    </nav>

    <div class="list-scroll min-h-0 flex-1 overflow-y-auto border-t border-line py-2 {tab === TAB_SOURCES ? '' : 'max-md:hidden'} landscape-phone:hidden!">
      <h2 class="px-4 pb-1 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.sources} · {client.sources.length}</h2>
      {#each Object.entries(sourcesByRole) as [role, list] (role)}
        <h3 class="px-4 pt-2 pb-0.5 text-[0.6rem] font-bold uppercase tracking-[0.14em] text-white/40">{strings.roles[role] ?? role}</h3>
        <ul>
          {#each list as s (s.type + (s.handle ?? s.venue ?? ''))}
            <li class="flex items-center gap-2 px-4 py-0.5 text-xs text-white/70" title={s.note ?? ''}>
              <span class="flex-1 truncate">{s.handle ? `@${s.handle}` : s.venue ?? s.type}</span>
              <span class="truncate font-mono text-[0.62rem] uppercase text-muted">{s.markets?.join(' ')}</span>
            </li>
          {/each}
        </ul>
      {/each}
    </div>
    <div class="hidden md:block landscape-phone:hidden! {tab === TAB_SOURCES ? 'max-md:block!' : ''}">
      <RunnerSettings url={runnerUrl} secret={runnerSecret} onSave={saveRunner} />
    </div>
  </aside>

  <!-- JOURNAL -->
  <main class="flex min-h-0 flex-col border-line md:border-r landscape-phone:flex! {tab === TAB_JOURNAL ? '' : 'max-md:hidden'}">
    <ShareBox onShare={share} initial={sharedIn} />
    <div class="flex flex-wrap gap-1.5 border-b border-line px-4 py-2 short:py-1 md:hidden landscape-phone:flex!" role="group" aria-label="Filter">
      {#each [ALL, ...BUCKETS] as b (b)}
        <button type="button" aria-pressed={filter === b} onclick={() => { filter = b; }}
          class="spring rounded-sm border px-2 py-0.5 text-[0.7rem] font-bold uppercase tracking-wider {filter === b ? `${b === ALL ? 'border-neon-red text-neon-red' : BUCKET_CLASSES[b]} bg-white/5` : 'border-line text-white/55'}">
          {strings.actions[b]}
        </button>
      {/each}
    </div>

    <ol class="list-scroll min-h-0 flex-1 divide-y divide-line overflow-y-auto" aria-live="polite">
      {#each visible as e (e.id)}
        {@const b = bucket(e)}
        <li in:enter={{ entry: e }}>
          <button type="button" onclick={() => pick(e.id)} aria-pressed={selected?.id === e.id}
            class="spring grid w-full grid-cols-[3.25rem_minmax(0,1fr)] gap-x-3 px-4 py-3 text-left short:py-2 hover:bg-white/3 focus-visible:bg-white/5 focus-visible:outline-none lg:px-6 {selected?.id === e.id ? 'bg-white/5' : ''}">
            <time class="pt-0.5 text-right font-mono text-xs tabular-nums text-muted" datetime={e.at}>{span(now - Date.parse(e.at))}</time>
            <span class="min-w-0">
              <span class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold uppercase tracking-wider {BUCKET_CLASSES[b]}">{strings.actions[b]}</span>
                <span class="font-mono font-semibold text-white">{contractLabel(e.signal)}</span>
                <span class="truncate text-muted">{e.signal.source_id}</span>
                {#if e.signal.manual}
                  <span class="rounded-sm bg-white/8 px-1.5 py-px text-[0.62rem] font-bold uppercase tracking-wider text-white/70">{strings.shared}</span>
                {/if}
              </span>
              {#if e.plan}
                <span class="mt-1 block font-mono text-xs tabular-nums text-white/80">
                  {e.plan.qty} @ {e.plan.entry.toFixed(2)} · {strings.stop} {e.plan.stop.toFixed(2)} · {e.plan.targets.map((t) => `${t.qty}→${t.price.toFixed(2)}`).join(' ')}
                </span>
              {:else if e.rejected}
                <span class="mt-1 block text-xs text-bad/90">{e.rejected}</span>
              {/if}
              <span class="mt-1 block truncate text-[0.8rem] text-white/55">{e.signal.raw}</span>
            </span>
          </button>
        </li>
      {:else}
        <li class="px-6 py-10 text-sm text-muted">
          {client.status === 'offline' ? strings.offlineHint : strings.empty}
        </li>
      {/each}
    </ol>
  </main>

  <!-- PLAN DETAIL -->
  <section aria-label={strings.tabs.detail} class="list-scroll min-h-0 overflow-y-auto border-t border-line md:border-t-0 desktop-portrait:col-span-2 desktop-portrait:border-t! landscape-phone:block {tab === TAB_DETAIL ? '' : 'max-md:hidden'}">
    {#if selected}
      {@const s = selected.signal}
      <div class="px-4 py-4 lg:px-6">
        <div class="flex items-baseline justify-between gap-3">
          <h2 class="font-mono text-xl font-bold text-white">{contractLabel(s)}</h2>
          <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold uppercase tracking-wider {BUCKET_CLASSES[bucket(selected)]}">{strings.actions[bucket(selected)]}</span>
        </div>
        <p class="mt-1 text-xs text-muted">
          {s.source_id} · {new Date(selected.at).toLocaleString()}
          {#if s.url}· <a href={s.url} target="_blank" rel="noopener noreferrer" class="spring text-white/70 underline-offset-4 hover:text-white hover:underline">{strings.openPost}</a>{/if}
        </p>
        {#if selected.as_of}
          <p class="mt-1 text-xs text-warn">{strings.asOf}: {new Date(selected.as_of).toLocaleString()} ({span(now - Date.parse(selected.as_of))} ago)</p>
        {/if}

        {#if selected.plan}
          {@const p = selected.plan}
          <div class="mt-5"><PlanLadder plan={p} /></div>
          <dl class="mt-5 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
            <dt class="text-muted">{strings.entry}</dt>
            <dd class="text-right font-mono tabular-nums">{p.qty} {s.right ? strings.contracts : strings.units} @ {p.entry.toFixed(2)}</dd>
            <dt class="text-muted">{strings.cost}</dt>
            <dd class="text-right font-mono tabular-nums">${p.cost_usd.toFixed(2)}</dd>
            <dt class="text-muted">{strings.stop}</dt>
            <dd class="text-right font-mono tabular-nums text-bad">{p.stop.toFixed(2)} ({(((p.stop - p.entry) / p.entry) * PERCENT).toFixed(0)}%)</dd>
            {#each p.targets as t, i (i)}
              <dt class="text-muted">T{i + 1}</dt>
              <dd class="text-right font-mono tabular-nums text-ok">{t.qty} → {t.price.toFixed(2)} (+{(((t.price - p.entry) / p.entry) * PERCENT).toFixed(0)}%)</dd>
            {/each}
            <dt class="text-muted">{strings.runner}</dt>
            <dd class="text-right font-mono tabular-nums">{p.runner}</dd>
            <dt class="text-muted">{strings.exitBy}</dt>
            <dd class="text-right font-mono tabular-nums">
              {#if p.exit_by && Date.parse(p.exit_by) > 0}
                {new Date(p.exit_by).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
                <span class="text-muted">({Date.parse(p.exit_by) > now ? `in ${span(Date.parse(p.exit_by) - now)}` : 'passed'})</span>
              {:else}
                <span class="text-muted">{strings.noTimeRule}</span>
              {/if}
            </dd>
            {#if p.time_stop_at}
              <dt class="text-muted">{strings.timeStop}</dt>
              <dd class="text-right font-mono tabular-nums">
                {new Date(p.time_stop_at).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
                <span class="text-muted">{strings.timeStopRule} {p.time_stop_above?.toFixed(2)}</span>
              </dd>
            {/if}
          </dl>
        {:else if selected.rejected}
          <h3 class="mt-5 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.reason}</h3>
          <p class="mt-1 text-sm text-bad">{selected.rejected}</p>
        {/if}

        <dl class="mt-5 grid grid-cols-2 gap-x-4 gap-y-2 border-t border-line pt-4 text-sm">
          {#if s.price}
            <dt class="text-muted">{strings.called}</dt>
            <dd class="text-right font-mono tabular-nums">{s.price.toFixed(2)}</dd>
          {/if}
          <dt class="text-muted">{strings.confidence}</dt>
          <dd class="text-right font-mono tabular-nums">{(s.confidence * PERCENT).toFixed(0)}%</dd>
        </dl>
        <h3 class="mt-4 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.raw}</h3>
        <p class="mt-1 rounded-sm bg-white/5 px-3 py-2 font-mono text-xs leading-relaxed break-words text-white/80">{s.raw}</p>
      </div>
    {:else}
      <p class="px-6 py-10 text-sm text-muted">{strings.pickOne}</p>
    {/if}
  </section>

  <!-- PHONE TABS: within thumb reach -->
  <nav class="grid grid-cols-3 border-t border-line md:hidden landscape-phone:hidden!" aria-label="Sections">
    {#each TABS as t (t)}
      <button type="button" aria-pressed={tab === t} onclick={() => { tab = t; }}
        class="spring py-3 text-xs font-bold uppercase tracking-wider focus-visible:outline-none focus-visible:bg-white/5 {tab === t ? 'text-neon-red' : 'text-white/55'}">
        {strings.tabs[t]}
      </button>
    {/each}
  </nav>
</div>
