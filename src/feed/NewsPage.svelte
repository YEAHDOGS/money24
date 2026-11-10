<script>
  /**
   * News page. Tablet: filter rail | feed. Desktop and up: rail | feed | source health.
   * Phones, phones on their side and portrait desktops: category chips on top of the feed.
   */
  import { flip } from 'svelte/animate';
  import { cubicOut } from 'svelte/easing';
  import strings from './strings.json';
  import { KIND_CLASSES, FALLBACK_KIND } from './post-format.js';
  import FeedPost from './FeedPost.svelte';
  import PollSlider from './PollSlider.svelte';

  const HOLD_PX = 24;
  const ENTER_MS = 460;
  const ENTER_STAGGER_MS = 45;
  const ENTER_OFFSET_PX = 18;
  const FLIP_MS = 320;
  const TOP_TAGS = 14;
  const KIND_ALL = 'all';
  const KINDS = Object.keys(strings.kinds).filter((k) => k !== KIND_ALL);

  /** @type {{ client: import('./feed-client.svelte.js').FeedClient, now: number, linkTarget: string|undefined, slider: any }} */
  let { client, now, linkTarget, slider } = $props();

  let kind = $state(KIND_ALL);
  let tag = $state('');
  let feed = $state('');
  /** @type {HTMLElement | undefined} */
  let listEl = $state();

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
  const newsSources = $derived(Object.values(client.sources).filter((s) => s.group !== 'calendar'));
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

{#snippet sourceHealth()}
  <h2 class="px-4 pb-1 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.sources}</h2>
  <ul>
    {#each newsSources as s (s.id)}
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
{/snippet}

<div class="mx-auto grid h-full w-full max-w-7xl grid-cols-[minmax(0,1fr)] md:grid-cols-[13rem_minmax(0,1fr)] lg:grid-cols-[15rem_minmax(0,1fr)_16rem] xl:border-x xl:border-line desktop-portrait:grid-cols-[minmax(0,1fr)]! landscape-phone:grid-cols-[minmax(0,1fr)]! ultrawide:max-w-none ultrawide:grid-cols-[19rem_minmax(0,1fr)_24rem]">

  <!-- FILTER RAIL -->
  <aside class="list-scroll hidden min-h-0 overflow-y-auto border-r border-line md:block desktop-portrait:hidden! landscape-phone:hidden!" aria-label="Filters">
    <nav aria-label="Categories" class="py-2">
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

    <!-- Source health sits in the rail on tablets; desktops give it its own column -->
    <div class="border-t border-line py-2 lg:hidden">{@render sourceHealth()}</div>
  </aside>

  <!-- FEED -->
  <section class="flex min-h-0 flex-col" aria-label={strings.tabNews}>
    <PollSlider {...slider} />

    <div class="flex flex-wrap gap-1.5 border-b border-line px-4 py-2 short:py-1 md:hidden desktop-portrait:flex! landscape-phone:flex!" role="group" aria-label="Categories">
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
            <FeedPost {post} {now} {linkTarget} onTag={(t) => { tag = tag === t ? '' : t; }} activeTag={tag} />
          </li>
        {:else}
          <li class="px-6 py-16 text-center text-sm text-muted">
            {#if client.status === 'offline'}{strings.emptyOffline}{:else if client.items.length}{strings.emptyFilter}{:else}{strings.emptyLive}{/if}
          </li>
        {/each}
      </ol>
    </div>
  </section>

  <!-- SOURCE HEALTH (desktop and up) -->
  <aside class="list-scroll hidden min-h-0 overflow-y-auto border-l border-line py-2 lg:block desktop-portrait:hidden! landscape-phone:hidden!" aria-label={strings.sources}>
    {@render sourceHealth()}
  </aside>
</div>
