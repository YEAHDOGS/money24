<script>
  /**
   * Map page: where the news is loudest. Counted two ways: where the outlets covering a story
   * publish from, or the places the headlines name. Wide screens: map | ranked places and their
   * stories. Phones: the map on top, the list scrolling under it.
   */
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import strings from './stories.json';
  import feedStrings from './strings.json';
  import { ANGLE_CLASSES, placeName } from './story-format.js';
  import WorldMap from './WorldMap.svelte';

  const MODES = ['from', 'about'];
  const STORE_MODE = 'm24.news.mapMode';
  const MAX_PLACES = 40;
  const MAX_ARCS = 8;
  const MAX_STAGGER = 14;
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase';

  /** @type {{ client: import('./feed-client.svelte.js').FeedClient, active: boolean, onOpenStory: (id: number) => void }} */
  let { client, active, onOpenStory } = $props();

  let mode = $state(MODES.includes(localStorage.getItem(STORE_MODE) ?? '') ? localStorage.getItem(STORE_MODE) : MODES[0]);
  let country = $state('');
  let hovered = $state('');
  let storyHover = $state(0);
  /** @type {any} the country shapes: a file of its own, fetched the first time the map is shown */
  let world = $state(null);

  $effect(() => localStorage.setItem(STORE_MODE, mode));
  $effect(() => {
    if (active && !world) import('./world.json').then((m) => { world = m.default; });
  });

  /** @type {import('./stories-client.svelte.js').MapPlace[]} */
  const places = $derived(client.stories.world?.[mode] ?? []);
  const loudest = $derived(places[0]?.attention || 1);
  /** Heat per country, 0 to 1. A square root keeps quiet countries visible beside the loudest. */
  const heat = $derived(Object.fromEntries(places.map((p) => [p.id, Math.sqrt(p.attention / loudest)])));
  const place = $derived(places.find((p) => p.id === country));
  const pointed = $derived(places.find((p) => p.id === hovered));
  /** The story the reader is pointing at: its arcs run from where it is broadcast to what it is about. */
  const arcs = $derived.by(() => {
    const story = client.stories.board?.stories.find((s) => s.id === storyHover);
    if (!story) return [];
    return story.from.flatMap((f) => story.about.map((a) => /** @type {[string, string]} */ ([f.id, a.id]))).slice(0, MAX_ARCS);
  });
  const emptyText = $derived.by(() => {
    if (client.stories.outdated) return strings.emptyOld;
    if (client.status === 'offline') return feedStrings.emptyOffline;
    return strings.emptyLive;
  });
</script>

<div class="mx-auto grid h-full w-full max-w-7xl grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)] md:grid-cols-[minmax(0,1fr)_17rem] md:grid-rows-[minmax(0,1fr)] lg:grid-cols-[minmax(0,1fr)_21rem] xl:border-x xl:border-line landscape-phone:grid-cols-[minmax(0,1fr)_14rem]! landscape-phone:grid-rows-[minmax(0,1fr)]! ultrawide:max-w-none ultrawide:grid-cols-[minmax(0,1fr)_28rem]">

  <!-- MAP -->
  <section class="flex min-h-0 min-w-0 flex-col border-line max-md:border-b md:border-r landscape-phone:border-r landscape-phone:border-b-0" aria-label={strings.tabMap}>
    <div class="flex items-center gap-3 border-b border-line px-4 py-1.5 short:py-1 lg:px-6">
      <div class="grid shrink-0 grid-cols-2 gap-px overflow-hidden rounded-sm border border-line bg-line" role="group" aria-label={strings.map.modeLabel}>
        {#each MODES as m (m)}
          <button type="button" onclick={() => { mode = m; }} aria-pressed={mode === m}
            class="spring px-2 py-1 text-[0.68rem] font-bold tracking-wide whitespace-nowrap uppercase focus-visible:outline focus-visible:outline-neon-red {mode === m ? 'bg-neon-red text-app' : 'bg-app text-white/60 hover:text-white'}">
            {m === 'from' ? strings.map.modeFrom : strings.map.modeAbout}
          </button>
        {/each}
      </div>
      <!-- What is under the pointer: one fixed line, so pointing never moves the map -->
      <p class="min-w-0 flex-1 truncate text-right text-xs text-muted" aria-live="off">
        {#if hovered}
          <span class="font-semibold text-white">{placeName(hovered)}</span>
          {#if pointed}<span class="font-mono tabular-nums"> · {pointed.stories} {pointed.stories === 1 ? strings.map.story : strings.map.stories} · {pointed.articles} {pointed.articles === 1 ? strings.article : strings.articles}</span>{/if}
        {:else}
          {mode === 'from' ? strings.map.modeFromNote : strings.map.modeAboutNote}
        {/if}
      </p>
    </div>

    <!-- Phones: the map keeps its own shape under the bar. Wider: it fills the cell. -->
    <div class="min-h-0 w-full max-md:aspect-(--map-shape) md:flex-1 landscape-phone:aspect-auto landscape-phone:flex-1" style:--map-shape={world ? `${world.width} / ${world.height}` : '2 / 1'}>
      {#if world}
        <WorldMap {world} {heat} selected={country} {arcs} onPick={(id) => { country = country === id ? '' : id; }} onHover={(id) => { hovered = id; }} />
      {/if}
    </div>

    <p class="flex items-center justify-end gap-2 border-t border-line px-4 py-1 text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase short:hidden max-md:hidden lg:px-6">
      {strings.map.legendLow}
      <span class="h-1.5 w-24 bg-linear-to-r from-neon-red/10 to-neon-red" aria-hidden="true"></span>
      {strings.map.legendHigh}
    </p>
  </section>

  <!-- PLACES AND THEIR STORIES -->
  <aside class="list-scroll min-h-0 overflow-y-auto" aria-label={strings.map.topPlaces}>
    {#if country}
      {#key country}
        <div class="cal-rise">
          <div class="flex items-center gap-2 border-b border-line px-4 py-2">
            <button type="button" onclick={() => { country = ''; }} aria-label={strings.map.world}
              class="spring grid size-7 shrink-0 place-items-center rounded-sm border border-line text-white/80 hover:text-white focus-visible:outline focus-visible:outline-neon-red">
              <ArrowLeft class="size-4" aria-hidden="true" />
            </button>
            <h2 class="min-w-0 flex-1 truncate text-sm font-extrabold tracking-wide text-white uppercase">{placeName(country)}</h2>
            {#if place}<span class="font-mono text-xs tabular-nums text-muted">{place.stories} · {place.articles}</span>{/if}
          </div>
          <ol>
            {#each place?.top ?? [] as story (story.id)}
              {@const angle = ANGLE_CLASSES[story.angle] ?? ANGLE_CLASSES.unclear}
              <li class="border-b border-line">
                <button type="button" onclick={() => onOpenStory(story.id)}
                  onpointerenter={() => { storyHover = story.id; }} onpointerleave={() => { storyHover = 0; }}
                  onfocus={() => { storyHover = story.id; }} onblur={() => { storyHover = 0; }}
                  class="spring grid w-full grid-cols-[2rem_minmax(0,1fr)] gap-x-2 px-4 py-2 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none">
                  <span class="pt-0.5 text-right font-mono text-xs font-bold tabular-nums text-white">{Math.round(story.attention)}</span>
                  <span class="min-w-0">
                    <span class="block text-sm leading-snug text-white/90 line-clamp-2">{story.title}</span>
                    <span class="mt-1 flex flex-wrap gap-x-2 text-[0.68rem] font-bold tracking-wider uppercase">
                      <span class="text-white/60">{strings.categories[story.category] ?? story.category}</span>
                      <span class={angle.text}>{strings.angles[story.angle] ?? story.angle}</span>
                    </span>
                  </span>
                </button>
              </li>
            {:else}
              <li class="px-6 py-10 text-center text-sm text-muted">{strings.map.noStories}</li>
            {/each}
          </ol>
        </div>
      {/key}
    {:else}
      <h2 class="px-4 pt-3 pb-1 {LABEL}">{strings.map.topPlaces}</h2>
      <ol>
        {#each places.slice(0, MAX_PLACES) as p, i (p.id)}
          <li class="cal-rise" style:--i={Math.min(i, MAX_STAGGER)}>
            <button type="button" onclick={() => { country = p.id; }} onpointerenter={() => { hovered = p.id; }} onpointerleave={() => { hovered = ''; }}
              class="spring relative grid w-full grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-2 px-4 py-1.5 text-left text-sm hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none">
              <span class="truncate text-white/90">{placeName(p.id)}</span>
              <span class="font-mono text-xs tabular-nums text-muted">{p.stories}</span>
              <span class="absolute inset-x-4 bottom-0 h-px origin-left bg-neon-red transition-transform duration-500 ease-out" style:transform="scaleX({heat[p.id] ?? 0})" aria-hidden="true"></span>
            </button>
          </li>
        {:else}
          <li class="px-6 py-10 text-center text-sm text-muted">{emptyText}</li>
        {/each}
      </ol>
      {#if places.length}<p class="px-4 py-3 text-xs text-muted">{strings.map.pickPlace}</p>{/if}
    {/if}
  </aside>
</div>
