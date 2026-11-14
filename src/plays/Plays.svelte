<script>
  /**
   * Money 24 Plays: the home screen. Every play with what has to happen for it to pay, a link inbox
   * that turns a pasted Robinhood or Webull link into a paper trade at once, and the rulebook.
   * Wide screens: rail (inbox, history) | plays | detail, split by 1px lines.
   * Phones: one pane at a time with tabs at the bottom. Phones on their side: icon rail | list | detail.
   */
  import { onMount, untrack } from 'svelte';
  import strings from './strings.json';
  import { PlaysClient, DEFAULT_RUNNER, STORE_RUNNER, STORE_SECRET, stored } from './plays-client.svelte.js';
  import { failing } from './play-format.js';
  import LinkInbox from './LinkInbox.svelte';
  import PlayCard from './PlayCard.svelte';
  import PlayDetail from './PlayDetail.svelte';
  import RulesEditor from './RulesEditor.svelte';
  import RunnerSettings from '../trading/RunnerSettings.svelte';
  import Zap from '@lucide/svelte/icons/zap';
  import Inbox from '@lucide/svelte/icons/inbox';
  import Scale from '@lucide/svelte/icons/scale';
  import Crosshair from '@lucide/svelte/icons/crosshair';

  const CLOCK_TICK_MS = 1000;
  const PHONE_QUERY = '(max-width: 767px)';
  const LANDSCAPE_QUERY = '(orientation: landscape) and (max-height: 500px) and (max-width: 1023px)';
  const MODE_WIDE = 'wide';
  const MODE_PHONE = 'phone';
  const MODE_LANDSCAPE = 'landscape';
  const TAB_PLAYS = 'plays';
  const TAB_PLAY = 'play';
  const TAB_INBOX = 'inbox';
  const TAB_RULES = 'rules';
  const PHONE_TABS = [
    { id: TAB_PLAYS, icon: Zap },
    { id: TAB_PLAY, icon: Crosshair },
    { id: TAB_INBOX, icon: Inbox },
    { id: TAB_RULES, icon: Scale },
  ];
  const FILTER_ALL = 'all';
  const FILTERS = [FILTER_ALL, 'armed', 'draft', 'closed'];
  const STATUS_DOT = { live: 'text-ok', connecting: 'text-warn', offline: 'text-bad' };
  const NAV = [
    { href: 'trading.html', key: 'desk' },
    { href: 'markets.html', key: 'markets' },
    { href: 'feed.html', key: 'news' },
    { href: 'terms.html', key: 'terms' },
  ];

  let runnerUrl = $state(stored(STORE_RUNNER, DEFAULT_RUNNER));
  let runnerSecret = $state(stored(STORE_SECRET, ''));
  let client = $state(untrack(() => new PlaysClient(runnerUrl, runnerSecret)));
  let now = $state(Date.now());
  let mode = $state(MODE_WIDE);
  let tab = $state(TAB_PLAYS);
  let filter = $state(FILTER_ALL);
  let selectedId = $state('');

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
    client = new PlaysClient(runnerUrl, runnerSecret);
    client.start();
  }

  function pick(/** @type {string} */ id) {
    selectedId = id;
    tab = TAB_PLAY;
  }

  onMount(() => {
    client.start();
    const phone = matchMedia(PHONE_QUERY);
    const landscape = matchMedia(LANDSCAPE_QUERY);
    const sync = () => {
      mode = landscape.matches ? MODE_LANDSCAPE : phone.matches ? MODE_PHONE : MODE_WIDE;
    };
    sync();
    phone.addEventListener('change', sync);
    landscape.addEventListener('change', sync);
    const tick = setInterval(() => { now = Date.now(); }, CLOCK_TICK_MS);
    return () => {
      clearInterval(tick);
      client.stop();
      phone.removeEventListener('change', sync);
      landscape.removeEventListener('change', sync);
    };
  });

  const counts = $derived(client.items.reduce((acc, x) => {
    acc[x.play.status] = (acc[x.play.status] ?? 0) + 1;
    return acc;
  }, /** @type {Record<string, number>} */ ({})));
  const visible = $derived(filter === FILTER_ALL ? client.items : client.items.filter((x) => x.play.status === filter));
  const selected = $derived(client.items.find((x) => x.play.id === selectedId) ?? client.items[0]);
  const showInboxPane = $derived(mode === MODE_WIDE || tab === TAB_INBOX);
  const showList = $derived(mode === MODE_WIDE || (mode === MODE_LANDSCAPE ? tab !== TAB_INBOX : tab === TAB_PLAYS));
  const showDetail = $derived(mode !== MODE_PHONE || tab === TAB_PLAY || tab === TAB_RULES);
  const detailIsRules = $derived(tab === TAB_RULES);
</script>

<svelte:head><title>{strings.title}</title></svelte:head>

{#snippet brand(/** @type {boolean} */ compact)}
  <h1 class="leading-none font-extrabold tracking-tight {compact ? 'text-lg' : 'text-2xl md:text-3xl'}">
    {strings.brand}<span class="text-neon-red">{strings.brandAccent}</span>
    <span class="ml-1 align-middle text-[0.6rem] font-bold tracking-[0.3em] text-muted">{strings.section}</span>
  </h1>
{/snippet}

{#snippet statusLine()}
  <p class="flex items-center gap-2 text-xs font-semibold {STATUS_DOT[client.status]}" role="status">
    <span class="relative size-2 rounded-full bg-current {client.status === 'live' ? 'live-dot' : ''}"></span>
    {strings.status[client.status]}
  </p>
  <span class="whitespace-nowrap rounded-sm border border-warn px-1.5 py-px text-[0.62rem] font-bold uppercase tracking-wider text-warn">{strings.paper}</span>
{/snippet}

{#snippet navLinks()}
  <nav class="flex flex-wrap gap-x-3 gap-y-1" aria-label="Pages">
    {#each NAV as n (n.href)}
      <a href={n.href} class="spring text-xs font-semibold text-white/60 underline-offset-4 hover:text-white hover:underline focus-visible:text-white focus-visible:outline-none">{strings.links[n.key]}&nbsp;→</a>
    {/each}
  </nav>
{/snippet}

{#snippet offline()}
  {#if client.status === 'offline'}
    <p class="border-b border-line px-4 py-2 text-xs leading-snug text-bad" role="alert">{strings.offlineHint}</p>
  {/if}
{/snippet}

{#snippet playsList()}
  <section aria-label={strings.tabs.plays} class="flex min-h-0 flex-col">
    <div class="flex flex-wrap items-center gap-1.5 border-b border-line px-3 py-2 lg:px-4">
      {#each FILTERS as f (f)}
        <button type="button" aria-pressed={filter === f} onclick={() => { filter = f; }}
          class="spring rounded-sm border px-2 py-0.5 text-[0.68rem] font-bold uppercase tracking-wider focus-visible:outline-none focus-visible:border-neon-red {filter === f ? 'border-neon-red bg-white/5 text-white' : 'border-line text-white/55 hover:text-white'}">
          {strings.filters[f]} <span class="font-mono tabular-nums text-muted">{f === FILTER_ALL ? client.items.length : counts[f] ?? 0}</span>
        </button>
      {/each}
      {#if mode === MODE_WIDE}
        <button type="button" aria-pressed={detailIsRules} onclick={() => { tab = detailIsRules ? TAB_PLAY : TAB_RULES; }}
          class="spring ml-auto flex items-center gap-1 rounded-sm border px-2 py-0.5 text-[0.68rem] font-bold uppercase tracking-wider focus-visible:outline-none focus-visible:border-neon-red {detailIsRules ? 'border-neon-red text-neon-red' : 'border-line text-white/70 hover:text-white'}">
          <Scale class="size-3" aria-hidden="true" />{strings.tabs.rules}
        </button>
      {/if}
    </div>
    <ul class="list-scroll min-h-0 flex-1 divide-y divide-line overflow-y-auto">
      {#each visible as item (item.play.id)}
        <li>
          <PlayCard {item} selected={!detailIsRules && selected?.play.id === item.play.id} onPick={() => pick(item.play.id)} />
        </li>
      {:else}
        <li class="px-5 py-10 text-sm text-muted">{client.status === 'offline' ? strings.offlineHint : strings.empty}</li>
      {/each}
    </ul>
  </section>
{/snippet}

{#snippet inboxPane()}
  <div class="flex min-h-0 flex-1 flex-col">
    <LinkInbox {client} onPick={pick} />
  </div>
{/snippet}

{#snippet detail()}
  <section aria-label={detailIsRules ? strings.tabs.rules : strings.tabs.play} class="@container flex min-h-0 flex-col border-line">
    {#if detailIsRules}
      <RulesEditor {client} />
    {:else if selected}
      {#key selected.play.id}
        <PlayDetail item={selected} {client} {now} />
      {/key}
    {:else}
      <p class="px-6 py-10 text-sm text-muted">{client.status === 'offline' ? strings.offlineHint : strings.pickOne}</p>
    {/if}
  </section>
{/snippet}

{#if mode === MODE_WIDE}
  <div class="mx-auto grid h-dvh w-full max-w-7xl grid-cols-[12rem_15rem_minmax(0,1fr)] border-line select-text lg:grid-cols-[16rem_20rem_minmax(0,1fr)] xl:border-x desktop-portrait:grid-cols-[16rem_minmax(0,1fr)]! desktop-portrait:grid-rows-[minmax(0,36%)_minmax(0,1fr)] ultrawide:max-w-none ultrawide:grid-cols-[22rem_28rem_minmax(0,1fr)]">
    <aside class="flex min-h-0 flex-col border-r border-line">
      <div class="flex flex-col gap-2 px-4 py-4">
        {@render brand(false)}
        <div class="flex flex-wrap items-center gap-2">{@render statusLine()}</div>
        {@render navLinks()}
      </div>
      {@render offline()}
      {#if showInboxPane}{@render inboxPane()}{/if}
      <RunnerSettings url={runnerUrl} secret={runnerSecret} onSave={saveRunner} />
    </aside>
    <div class="flex min-h-0 flex-col border-line lg:border-r desktop-portrait:border-r-0">
      {@render playsList()}
    </div>
    <div class="flex min-h-0 flex-col max-lg:border-l max-lg:border-line desktop-portrait:col-span-2 desktop-portrait:border-t desktop-portrait:border-l-0">
      {@render detail()}
    </div>
  </div>
{:else if mode === MODE_LANDSCAPE}
  <div class="grid h-dvh w-full grid-cols-[3rem_15rem_minmax(0,1fr)] select-text">
    <nav class="flex flex-col items-center gap-1 border-r border-line py-2" aria-label="Sections">
      <span class="mb-1 font-extrabold text-neon-red" aria-hidden="true">24</span>
      <span class="relative mb-2 size-2 rounded-full bg-current {STATUS_DOT[client.status]} {client.status === 'live' ? 'live-dot' : ''}" title={strings.status[client.status]}></span>
      {#each PHONE_TABS.filter((t) => t.id !== TAB_PLAY) as t (t.id)}
        <button type="button" aria-pressed={tab === t.id || (t.id === TAB_PLAYS && tab === TAB_PLAY)} aria-label={strings.tabs[t.id]} onclick={() => { tab = t.id; }}
          class="spring grid size-10 place-items-center rounded-sm focus-visible:outline focus-visible:outline-neon-red {tab === t.id || (t.id === TAB_PLAYS && tab === TAB_PLAY) ? 'bg-white/5 text-neon-red' : 'text-white/55'}">
          <t.icon class="size-5" aria-hidden="true" />
        </button>
      {/each}
    </nav>
    <div class="flex min-h-0 flex-col border-r border-line">
      {@render offline()}
      {#if tab === TAB_INBOX}{@render inboxPane()}{:else}{@render playsList()}{/if}
    </div>
    {@render detail()}
  </div>
{:else}
  <div class="grid h-dvh w-full grid-rows-[auto_minmax(0,1fr)_auto] select-text">
    <header class="flex items-center justify-between gap-3 border-b border-line px-4 py-2.5">
      {@render brand(true)}
      <div class="flex items-center gap-2">{@render statusLine()}</div>
    </header>
    <div class="flex min-h-0 flex-col">
      {@render offline()}
      {#if tab === TAB_INBOX}
        {@render inboxPane()}
        <div class="border-t border-line px-4 py-2">{@render navLinks()}</div>
        <RunnerSettings url={runnerUrl} secret={runnerSecret} onSave={saveRunner} />
      {:else if showList}
        {@render playsList()}
      {:else if showDetail}
        {@render detail()}
      {/if}
    </div>
    <nav class="grid grid-cols-4 border-t border-line" aria-label="Sections">
      {#each PHONE_TABS as t (t.id)}
        {@const alert = t.id === TAB_PLAY && selected ? failing(selected.report).blocks : 0}
        <button type="button" aria-pressed={tab === t.id} onclick={() => { tab = t.id; }}
          class="spring relative flex flex-col items-center gap-0.5 py-2.5 text-[0.65rem] font-bold uppercase tracking-wider focus-visible:bg-white/5 focus-visible:outline-none {tab === t.id ? 'text-neon-red' : 'text-white/55'}">
          <t.icon class="size-5" aria-hidden="true" />
          {strings.tabs[t.id]}
          {#if alert}<span class="absolute top-1.5 right-[calc(50%-1.4rem)] size-1.5 rounded-full bg-warn" aria-hidden="true"></span>{/if}
        </button>
      {/each}
    </nav>
  </div>
{/if}
