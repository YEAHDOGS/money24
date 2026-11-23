<script>
  /**
   * Money 24 Plays: the home screen. Every play with what has to happen for it to pay, a link inbox
   * that turns a pasted Robinhood or Webull link into a trade at once, and the rulebook.
   * Wide screens: rail (inbox, history) | plays | detail, split by 1px lines.
   * Phones: one pane at a time with tabs at the bottom. Phones on their side: icon rail | list | detail.
   */
  import { onMount, untrack } from 'svelte';
  import strings from './strings.json';
  import { PlaysClient, DEFAULT_RUNNER, STORE_RUNNER, STORE_SECRET, stored } from './plays-client.svelte.js';
  import { failing, fill } from './play-format.js';
  import LinkInbox from './LinkInbox.svelte';
  import PlayCard from './PlayCard.svelte';
  import PlayDetail from './PlayDetail.svelte';
  import RulesEditor from './RulesEditor.svelte';
  import AppHeader from '../lib/AppHeader.svelte';
  import AppFooter from '../lib/AppFooter.svelte';
  import RunnerSettings from '../trading/RunnerSettings.svelte';
  import Zap from '@lucide/svelte/icons/zap';
  import Inbox from '@lucide/svelte/icons/inbox';
  import Scale from '@lucide/svelte/icons/scale';
  import Crosshair from '@lucide/svelte/icons/crosshair';
  import Layers from '@lucide/svelte/icons/layers';
  import ScrollText from '@lucide/svelte/icons/scroll-text';
  import { demoHref } from '../lib/demo.js';

  const CLOCK_TICK_MS = 1000;
  const JOURNAL_PAGE = 'trading.html'; // the journal: filed under the Plays tab
  const DECK_PAGE = 'deck.html'; // the strategy deck: filed under the Plays tab
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
  const PLAY_PARAM = 'play'; // plays.html?play=<id> opens on that play (Home's open positions link here)
  const FILTER_ALL = 'all';
  const FILTERS = [FILTER_ALL, 'armed', 'draft', 'closed'];
  /** @type {Intl.DateTimeFormatOptions} */
  const DEMO_DATE = { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' };

  let runnerUrl = $state(stored(STORE_RUNNER, DEFAULT_RUNNER));
  let runnerSecret = $state(stored(STORE_SECRET, ''));
  let client = $state(untrack(() => new PlaysClient(runnerUrl, runnerSecret)));
  let now = $state(Date.now());
  let mode = $state(MODE_WIDE);
  const linkedId = new URLSearchParams(location.search).get(PLAY_PARAM) ?? '';
  let tab = $state(linkedId ? TAB_PLAY : TAB_PLAYS);
  let filter = $state(FILTER_ALL);
  let selectedId = $state(linkedId);

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

{#snippet offline()}
  {#if client.status === 'offline'}
    <p class="border-b border-line px-4 py-2 text-xs leading-snug text-bad" role="alert">{strings.offlineHint}</p>
  {:else if client.status === 'demo'}
    <p class="shrink-0 border-b border-line px-4 py-1.5 max-md:truncate text-xs leading-snug text-paper short:py-1" role="status">
      {fill(strings.demo.hint, { when: new Date(client.demoTakenAt).toLocaleString(undefined, DEMO_DATE) })}
    </p>
  {/if}
{/snippet}

{#snippet playsList()}
  <section aria-label={strings.tabs.plays} data-tour="plays-list" class="flex min-h-0 flex-col">
    <div class="flex flex-wrap items-center gap-1.5 border-b border-line px-3 py-2 lg:px-4">
      {#each FILTERS as f (f)}
        <button type="button" aria-pressed={filter === f} onclick={() => { filter = f; }}
          class="spring rounded-sm border px-2 py-0.5 text-[0.68rem] font-bold uppercase tracking-wider focus-visible:outline-none focus-visible:border-neon-red {filter === f ? 'border-neon-red bg-white/5 text-white' : 'border-line text-white/55 hover:text-white'}">
          {strings.filters[f]} <span class="font-mono tabular-nums text-muted">{f === FILTER_ALL ? client.items.length : counts[f] ?? 0}</span>
        </button>
      {/each}
      <a href={demoHref(JOURNAL_PAGE)} class="spring ml-auto flex items-center gap-1 rounded-sm border border-line px-2 py-0.5 text-[0.68rem] font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:border-neon-red focus-visible:outline-none">
        <ScrollText class="size-3" aria-hidden="true" />{strings.journal}
      </a>
      <a href={demoHref(DECK_PAGE)} class="spring flex items-center gap-1 rounded-sm border border-line px-2 py-0.5 text-[0.68rem] font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:border-neon-red focus-visible:outline-none">
        <Layers class="size-3" aria-hidden="true" />{strings.deck}
      </a>
      {#if mode === MODE_WIDE}
        <button type="button" aria-pressed={detailIsRules} onclick={() => { tab = detailIsRules ? TAB_PLAY : TAB_RULES; }}
          class="spring flex items-center gap-1 rounded-sm border px-2 py-0.5 text-[0.68rem] font-bold uppercase tracking-wider focus-visible:outline-none focus-visible:border-neon-red {detailIsRules ? 'border-neon-red text-neon-red' : 'border-line text-white/70 hover:text-white'}">
          <Scale class="size-3" aria-hidden="true" />{strings.tabs.rules}
        </button>
      {/if}
    </div>
    <ul class="list-scroll min-h-0 flex-1 divide-y divide-line overflow-x-hidden overflow-y-auto [scrollbar-gutter:stable]">
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
  <div data-tour="plays-inbox" class="flex min-h-0 flex-1 flex-col">
    <LinkInbox {client} onPick={pick} />
  </div>
{/snippet}

{#snippet detail()}
  <section aria-label={detailIsRules ? strings.tabs.rules : strings.tabs.play} class="@container flex min-h-0 flex-col border-line">
    {#if detailIsRules}
      <RulesEditor {client} />
    {:else if selected}
      {#key selected.play.id}
        <PlayDetail item={selected} {client} {now} compact={mode !== MODE_WIDE} />
      {/key}
    {:else}
      <p class="px-6 py-10 text-sm text-muted">{client.status === 'offline' ? strings.offlineHint : strings.pickOne}</p>
    {/if}
  </section>
{/snippet}

<div class="flex h-dvh w-full flex-col">
<AppHeader page="plays" status={client.status} />

{#if mode === MODE_WIDE}
  <div class="mx-auto grid min-h-0 w-full max-w-7xl flex-1 grid-cols-[12rem_15rem_minmax(0,1fr)] border-line select-text lg:grid-cols-[16rem_20rem_minmax(0,1fr)] xl:border-x desktop-portrait:grid-cols-[16rem_minmax(0,1fr)]! desktop-portrait:grid-rows-[minmax(0,36%)_minmax(0,1fr)] ultrawide:max-w-none ultrawide:grid-cols-[22rem_28rem_minmax(0,1fr)]">
    <aside class="flex min-h-0 flex-col border-r border-line">
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
  <div class="grid min-h-0 w-full flex-1 grid-cols-[3rem_15rem_minmax(0,1fr)] select-text">
    <nav class="flex flex-col items-center gap-1 border-r border-line py-2" aria-label="Sections">
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
  <div class="grid min-h-0 w-full flex-1 grid-rows-[minmax(0,1fr)_auto] select-text">
    <div class="flex min-h-0 flex-col">
      {@render offline()}
      {#if tab === TAB_INBOX}
        {@render inboxPane()}
        <RunnerSettings url={runnerUrl} secret={runnerSecret} onSave={saveRunner} />
      {:else if showList}
        {@render playsList()}
      {:else if showDetail}
        {@render detail()}
      {/if}
    </div>
    <nav data-tour="plays-tabs" class="grid grid-cols-4 border-t border-line" aria-label="Sections">
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
<AppFooter recorded={client.demoTakenAt} />
</div>
