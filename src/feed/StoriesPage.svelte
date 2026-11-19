<script>
  /**
   * Stories page: every article about the same event as one row, loudest first.
   * Tablet: story types | board. Desktop and up: story types | board | the open story.
   * Phones, phones on their side and portrait desktops: one column; a story opens in place.
   */
  import { untrack } from 'svelte';
  import { slide } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Search from '@lucide/svelte/icons/search';
  import strings from './stories.json';
  import feedStrings from './strings.json';
  import { marketHref, pace, signedPct } from './story-format.js';
  import { age } from './post-format.js';
  import StoryRow from './StoryRow.svelte';
  import StoryDetail from './StoryDetail.svelte';

  const CATEGORY_ALL = 'all';
  const OPEN_MS = 240;
  const MAX_STAGGER = 14;
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase';
  const FIELD = 'spring min-w-0 rounded-sm border border-line bg-card px-1.5 py-1 text-xs text-white focus-visible:outline focus-visible:outline-neon-red';

  /** @type {{ client: import('./feed-client.svelte.js').FeedClient, now: number, linkTarget: string|undefined, storyId: number }} */
  let { client, now, linkTarget, storyId = $bindable(0) } = $props();

  let category = $state(CATEGORY_ALL);
  let query = $state('');

  const board = $derived(client.stories.board);
  const all = $derived(board?.stories ?? []);
  const wanted = $derived(query.trim().toLowerCase());
  const visible = $derived(all.filter((s) =>
    (category === CATEGORY_ALL || s.category === category) && (!wanted || s.title.toLowerCase().includes(wanted) || s.tags.some((t) => t.toLowerCase() === wanted))));
  /** Every category on the board with its count, the fullest first, under "All". */
  const categories = $derived([{ id: CATEGORY_ALL, stories: (board?.categories ?? []).reduce((n, c) => n + c.stories, 0) }, ...(board?.categories ?? [])]);
  const selected = $derived(all.find((s) => s.id === storyId) ?? client.stories.details[storyId]?.story);
  const movers = $derived(board?.movers ?? []);
  const emptyText = $derived.by(() => {
    if (client.stories.outdated) return strings.emptyOld;
    if (client.status === 'offline') return feedStrings.emptyOffline;
    return all.length ? strings.emptyFilter : strings.emptyLive;
  });

  /** Changes when another story is opened or the open one gains an article. */
  const openKey = $derived(selected ? `${selected.id}:${selected.articles}` : '');
  // An open story's articles load once, and again when the board says it grew.
  $effect(() => {
    if (openKey) untrack(() => client.stories.open(storyId));
  });

  /** @param {number} id */
  function toggle(id) {
    storyId = storyId === id ? 0 : id;
  }
</script>

{#snippet moverList()}
  <h2 class="px-4 {LABEL}">{strings.movers}</h2>
  <p class="px-4 pt-1 text-xs leading-snug text-muted">{movers.length ? strings.moversNote : strings.moversNone}</p>
  <ul class="pt-1.5">
    {#each movers as m (m.symbol)}
      <li>
        <a href={marketHref(m.symbol)} class="spring grid grid-cols-[minmax(0,1fr)_auto_auto] items-baseline gap-x-2 px-4 py-1 text-xs hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none">
          <span class="truncate font-mono font-bold text-white">{m.symbol}</span>
          <span class="font-mono tabular-nums {m.move_pct < 0 ? 'text-bad' : 'text-ok'}">{signedPct(m.move_pct)}</span>
          <span class="font-mono tabular-nums text-neon-red">{pace(m.rel_volume)}</span>
        </a>
      </li>
    {/each}
  </ul>
{/snippet}

<div class="mx-auto grid h-full w-full max-w-7xl grid-cols-[minmax(0,1fr)] md:grid-cols-[13rem_minmax(0,1fr)] lg:grid-cols-[14rem_minmax(0,1fr)_minmax(0,24rem)] xl:border-x xl:border-line desktop-portrait:grid-cols-[minmax(0,1fr)]! landscape-phone:grid-cols-[minmax(0,1fr)]! ultrawide:max-w-none ultrawide:grid-cols-[19rem_minmax(0,1fr)_minmax(0,40rem)]">

  <!-- STORY TYPES AND UNEXPLAINED VOLUME (tablet and up) -->
  <aside class="list-scroll hidden min-h-0 overflow-y-auto border-r border-line md:block desktop-portrait:hidden! landscape-phone:hidden!" aria-label={strings.categoriesLabel}>
    <nav aria-label={strings.categoriesLabel} class="py-2">
      {#each categories as c (c.id)}
        <button type="button" onclick={() => { category = c.id; }} aria-pressed={category === c.id}
          class="spring flex w-full items-center gap-2.5 px-4 py-1.5 text-left text-sm hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {category === c.id ? 'text-white' : 'text-white/60'}">
          <span class="h-3 w-0.5 rounded-full bg-neon-red transition-opacity duration-150 {category === c.id ? 'opacity-100' : 'opacity-25'}"></span>
          <span class="flex-1 truncate">{strings.categories[c.id] ?? c.id}</span>
          <span class="font-mono text-xs tabular-nums text-muted">{c.stories}</span>
        </button>
      {/each}
    </nav>
    <div class="border-t border-line py-3">{@render moverList()}</div>
  </aside>

  <!-- BOARD -->
  <section class="flex min-h-0 flex-col" aria-label={strings.tabStories}>
    <div class="flex items-center gap-2 border-b border-line px-4 py-1.5 short:py-1 lg:px-6">
      <select bind:value={category} class="{FIELD} max-w-[45%] md:hidden desktop-portrait:block! landscape-phone:block!" aria-label={strings.categoriesLabel}>
        {#each categories as c (c.id)}<option value={c.id}>{strings.categories[c.id] ?? c.id} ({c.stories})</option>{/each}
      </select>
      <button type="button" onclick={() => client.stories.setTradable(!client.stories.tradable)} aria-pressed={client.stories.tradable} title={strings.tradableNote}
        class="spring shrink-0 rounded-sm border px-1.5 py-1 text-[0.68rem] font-bold tracking-wide whitespace-nowrap uppercase focus-visible:outline focus-visible:outline-neon-red {client.stories.tradable ? 'border-neon-red bg-neon-red text-app' : 'border-line text-white/60 hover:text-white'}">
        {strings.tradable}
      </button>
      <label class="flex min-w-0 flex-1 items-center gap-1.5 text-muted focus-within:text-white">
        <Search class="size-3.5 shrink-0" aria-hidden="true" />
        <input type="search" bind:value={query} placeholder={strings.search} aria-label={strings.search}
          class="min-w-0 flex-1 bg-transparent py-1 text-xs text-white placeholder:text-muted focus-visible:outline-none" />
      </label>
      <span class="shrink-0 font-mono text-xs tabular-nums text-muted">{visible.length}</span>
    </div>

    <!-- Unexplained volume, where the rail is hidden: one fixed line, so it never moves the board -->
    <p class="flex h-7 items-center gap-2 border-b border-line px-4 text-xs md:hidden desktop-portrait:flex! landscape-phone:flex!">
      <span class="shrink-0 {LABEL}">{strings.movers}</span>
      <span class="min-w-0 flex-1 truncate font-mono tabular-nums text-white/85">
        {#each movers as m (m.symbol)}<a href={marketHref(m.symbol)} class="mr-3 hover:text-neon-red">{m.symbol} <span class="text-neon-red">{pace(m.rel_volume)}</span></a>{:else}<span class="font-sans text-muted">{strings.moversNone}</span>{/each}
      </span>
    </p>

    <ol class="list-scroll min-h-0 flex-1 overflow-y-auto">
      {#each visible as story, i (story.id)}
        <li class="row-far border-b border-line {i < MAX_STAGGER ? 'cal-rise' : ''}" style:--i={i}>
          <StoryRow {story} {now} open={storyId === story.id} onToggle={() => toggle(story.id)} />
          <!-- In place, where there is no third column -->
          {#if storyId === story.id}
            <div id="story-{story.id}" class="border-t border-line bg-card lg:hidden desktop-portrait:block!" transition:slide={{ duration: OPEN_MS, easing: cubicOut }}>
              <StoryDetail {story} detail={client.stories.details[story.id]} {now} {linkTarget} />
            </div>
          {/if}
        </li>
      {:else}
        <li class="px-6 py-16 text-center text-sm text-muted">{emptyText}</li>
      {/each}
    </ol>

    {#if board}
      <p class="border-t border-line px-4 py-1 text-right font-mono text-[0.68rem] tabular-nums text-muted short:hidden lg:px-6">{strings.updated} {age(board.updated_at, now)}</p>
    {/if}
  </section>

  <!-- THE OPEN STORY (desktop and up) -->
  <aside class="list-scroll hidden min-h-0 overflow-y-auto border-l border-line lg:block desktop-portrait:hidden!" aria-label={strings.open}>
    {#if selected}
      {#key selected.id}
        <div class="cal-rise">
          <h2 class="px-4 pt-4 text-base leading-snug font-semibold text-white lg:px-6">{selected.title}</h2>
          <p class="px-4 pt-1 pb-1 font-mono text-xs tabular-nums text-muted lg:px-6">{strings.firstSeen} {new Date(selected.first_seen).toLocaleString()}</p>
          <StoryDetail story={selected} detail={client.stories.details[selected.id]} {now} {linkTarget} />
        </div>
      {/key}
    {:else}
      <p class="px-6 py-16 text-center text-sm text-muted">{strings.pickStory}</p>
    {/if}
  </aside>
</div>
