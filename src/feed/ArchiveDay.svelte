<script>
  /**
   * The calendar's day view: one New York day's archived news, grouped by outlet or by story.
   * Search and the group toggle sit on top; outlet, kind and tag filters sit here on phones and
   * portrait desktops, and in the calendar's rail (ArchiveRail.svelte) everywhere else.
   * Groups list biggest first; a group opens to its articles, a page at a time.
   */
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Search from '@lucide/svelte/icons/search';
  import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
  import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
  import strings from './archive.json';
  import feedStrings from './strings.json';
  import ArchiveArticle from './ArchiveArticle.svelte';
  import { etTime, hostOf, longDay, shortCount } from './archive-format.js';

  const MAX_STAGGER = 14;
  const GROUP_BY = ['source', 'story'];
  const STORY_NONE = 'none';
  const SELECT_CLASS = 'spring min-w-0 flex-1 rounded-sm border border-line bg-card px-1.5 py-1 text-xs text-white focus-visible:outline focus-visible:outline-neon-red';
  const SHORT_SCREEN = '(max-height: 640px)'; // the short: variant in src/styles/theme.css
  /** Where the rail is hidden, the filters row shows when opened: full strings, so Tailwind sees them. */
  const FILTERS_SHOWN = 'flex md:hidden desktop-portrait:flex! landscape-phone:flex!';
  const FILTERS_HIDDEN = 'hidden';

  /** @type {{ archive: import('./archive-client.svelte.js').ArchiveDesk, linkTarget: string|undefined, onclose: () => void }} */
  let { archive, linkTarget, onclose } = $props();

  /** The filters row starts closed on short screens, where every line of the list counts. */
  let filtersOpen = $state(!matchMedia(SHORT_SCREEN).matches);

  const groups = $derived(archive.groups?.groups ?? []);
  const total = $derived(archive.groups?.total ?? archive.counts[archive.day] ?? 0);
  const filtered = $derived(Boolean(archive.query.trim() || archive.source || archive.kind || archive.tag));
  const byStory = $derived(archive.by === 'story');
  const emptyText = $derived.by(() => {
    if (archive.unavailable) return strings.unavailable;
    if (archive.loading && !archive.groups) return strings.loading;
    return filtered ? strings.empty : strings.emptyDay;
  });

  /** @param {import('./archive-client.svelte.js').Group} g */
  function groupName(g) {
    if (!byStory) return g.name;
    return g.key === STORY_NONE ? strings.noStory : g.name || g.title;
  }
</script>

<section class="flex min-h-0 flex-1 flex-col" aria-label="{strings.openDay}: {longDay(archive.day)}">
  <!-- Day, back to the events, and how many articles match -->
  <div class="flex items-center gap-2 border-b border-line pr-4 lg:pr-6">
    <button type="button" onclick={onclose} aria-label={strings.backLabel}
      class="spring flex shrink-0 items-center gap-1 self-stretch border-r border-line px-2.5 text-xs font-semibold text-white/70 hover:bg-white/5 hover:text-white focus-visible:bg-white/5 focus-visible:outline-none">
      <ChevronLeft class="size-4" aria-hidden="true" />
      <span class="max-[359px]:sr-only">{strings.back}</span>
    </button>
    <h2 class="min-w-0 flex-1 truncate py-1.5 text-xs font-bold short:py-1">
      {longDay(archive.day)} <span class="font-normal text-muted short:hidden">· {strings.zoneNote}</span>
    </h2>
    <span class="shrink-0 font-mono text-xs tabular-nums text-muted" aria-live="polite">{shortCount(total)} {total === 1 ? strings.article : strings.articles}</span>
  </div>

  <!-- Search and grouping -->
  <div class="flex items-center gap-2 border-b border-line px-4 py-1.5 short:py-1 lg:px-6">
    <label class="spring flex min-w-0 flex-1 items-center gap-1.5 rounded-sm border border-line bg-card px-2 focus-within:border-neon-red">
      <Search class="size-3.5 shrink-0 text-muted" aria-hidden="true" />
      <input type="search" value={archive.query} oninput={(e) => archive.search(e.currentTarget.value)}
        placeholder={strings.search} aria-label={strings.searchLabel} spellcheck="false" autocomplete="off"
        class="min-w-0 flex-1 bg-transparent py-1 text-xs text-white placeholder:text-muted focus:outline-none" />
    </label>
    <div class="grid shrink-0 grid-cols-2 gap-px overflow-hidden rounded-sm border border-line bg-line" role="group" aria-label={strings.groupBy}>
      {#each GROUP_BY as by (by)}
        <button type="button" onclick={() => archive.setBy(by)} aria-pressed={archive.by === by}
          class="spring px-2 py-1 text-[0.68rem] font-bold uppercase tracking-wide focus-visible:outline focus-visible:outline-neon-red {archive.by === by ? 'bg-neon-red text-app' : 'bg-app text-white/60 hover:text-white'}">
          {strings.by[by]}
        </button>
      {/each}
    </div>
    <button type="button" onclick={() => { filtersOpen = !filtersOpen; }} aria-expanded={filtersOpen} aria-controls="archive-filters" aria-label={strings.filters}
      class="spring relative inline-flex shrink-0 items-center self-stretch rounded-sm border px-1.5 focus-visible:outline focus-visible:outline-neon-red md:hidden desktop-portrait:inline-flex! landscape-phone:inline-flex! {filtersOpen ? 'border-neon-red text-white' : 'border-line text-white/60 hover:text-white'}">
      <SlidersHorizontal class="size-3.5" aria-hidden="true" />
      <span class="absolute -top-0.5 -right-0.5 size-1.5 rounded-full bg-neon-red transition-transform duration-150 {archive.source || archive.kind || archive.tag ? 'scale-100' : 'scale-0'}"></span>
    </button>
  </div>

  <!-- Filters where the rail is hidden -->
  <div id="archive-filters" class="items-center gap-2 border-b border-line px-4 py-1.5 short:py-1 {filtersOpen ? FILTERS_SHOWN : FILTERS_HIDDEN}">
    <select value={archive.source} onchange={(e) => archive.setFilter('source', e.currentTarget.value)} class={SELECT_CLASS} aria-label={strings.outlets}>
      <option value="">{strings.allOutlets}</option>
      {#each archive.facets?.sources ?? [] as f (f.name)}<option value={f.name}>{f.name} ({f.n})</option>{/each}
    </select>
    <select value={archive.kind} onchange={(e) => archive.setFilter('kind', e.currentTarget.value)} class={SELECT_CLASS} aria-label={strings.kinds}>
      <option value="">{strings.allKinds}</option>
      {#each archive.facets?.kinds ?? [] as f (f.name)}<option value={f.name}>{feedStrings.kinds[f.name] ?? f.name} ({f.n})</option>{/each}
    </select>
    <select value={archive.tag} onchange={(e) => archive.setFilter('tag', e.currentTarget.value)} class={SELECT_CLASS} aria-label={strings.tags}>
      <option value="">{strings.allTags}</option>
      {#each archive.facets?.tags ?? [] as f (f.name)}<option value={f.name}>{f.name} ({f.n})</option>{/each}
    </select>
  </div>

  <div class="list-scroll min-h-0 flex-1 overflow-y-auto transition-opacity duration-150 {archive.loading ? 'opacity-60' : ''}">
    {#each groups as g, gi (`${archive.by}:${g.key}`)}
      {@const open = archive.open[g.key]}
      <section class="cal-rise" style:--i={Math.min(gi, MAX_STAGGER)} aria-label={groupName(g)}>
        <h3 class="sticky top-0 z-10 border-b border-line bg-app/95 backdrop-blur-sm">
          <button type="button" onclick={() => archive.toggle(g.key)} aria-expanded={Boolean(open)}
            class="spring grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 px-4 py-2 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none short:py-1.5 lg:px-6">
            <ChevronRight class="size-3.5 text-muted transition-transform duration-150 {open ? 'rotate-90 text-neon-red' : ''}" aria-hidden="true" />
            <span class="min-w-0">
              <span class="block truncate text-sm font-semibold {g.key === STORY_NONE ? 'text-white/70' : 'text-white'}">{groupName(g)}</span>
              <span class="block truncate text-[0.7rem] font-normal text-muted {open ? 'short:hidden' : ''}">
                <span class="font-mono tabular-nums">{etTime(g.latest)}</span> {g.title}
              </span>
            </span>
            <span class="font-mono text-xs tabular-nums text-muted">{shortCount(g.n)}</span>
          </button>
        </h3>
        {#if open}
          {#if !byStory && open.articles[0]?.outlet_url}
            <a href={open.articles[0].outlet_url} target={linkTarget} rel="noopener noreferrer"
              class="spring flex items-center gap-1 border-b border-line px-4 py-1 font-mono text-[0.66rem] text-white/60 hover:text-white focus-visible:text-white focus-visible:outline-none lg:px-6">
              {strings.outletSite} <span class="text-muted">{hostOf(open.articles[0].outlet_url)}</span><ArrowUpRight class="size-3" aria-hidden="true" />
            </a>
          {/if}
          <ol>
            {#each open.articles as a, i (a.id)}
              <li class="cal-rise border-b border-line" style:--i={Math.min(i, MAX_STAGGER)}>
                <ArchiveArticle article={a} showOutlet={byStory} {linkTarget} />
              </li>
            {/each}
          </ol>
          {#if open.next || open.loading}
            <button type="button" onclick={() => archive.more(g.key)} disabled={open.loading}
              class="spring block w-full border-b border-line px-4 py-2 text-center text-xs font-semibold text-neon-red hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none disabled:text-muted">
              {open.loading ? strings.loading : `${strings.showMore} · ${shortCount(g.n - open.articles.length)} ${strings.left}`}
            </button>
          {/if}
        {/if}
      </section>
    {:else}
      <div class="px-6 py-16 text-center text-sm text-muted">
        <p>{emptyText}</p>
        {#if filtered && !archive.loading}
          <button type="button" onclick={() => archive.clearFilters()} class="spring mt-3 rounded-sm border border-line px-3 py-1 text-xs font-semibold text-white hover:border-neon-red focus-visible:outline focus-visible:outline-neon-red">{strings.clearFilters}</button>
        {/if}
      </div>
    {/each}
    {#if archive.groups?.more}
      <p class="px-4 py-3 text-center text-[0.7rem] text-muted lg:px-6">+{shortCount(archive.groups.more)} {strings.moreGroups}</p>
    {/if}
  </div>
</section>
