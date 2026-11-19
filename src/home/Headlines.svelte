<script>
  /**
   * Headlines by category: chips to pick a news category, and under them that category's
   * headlines, most watched first (stories) or newest first (plain headlines). A headline opens
   * its story on the news page, or its article. Only the list scrolls.
   */
  import strings from './strings.json';
  import storyStrings from '../feed/stories.json';
  import StoryArt from '../lib/StoryArt.svelte';
  import { age } from '../feed/post-format.js';
  import { CATEGORY_ALL, MODE_STORIES, categoryLabel } from './news-model.js';
  import { angleStyle, categoryStyle } from './news-style.js';
  import { NEWS_PAGE, STORIES_HASH, pageLink } from './home-links.js';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';

  const STATUS_OFFLINE = 'offline';
  const STATUS_CONNECTING = 'connecting';
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase tv:text-sm';
  const CHIP = 'spring rounded-sm border px-2 py-0.5 text-[0.68rem] font-bold tracking-wider uppercase focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:px-3 tv:py-1 tv:text-sm';
  const CHIP_OFF = 'border-line text-white/55 hover:text-white';

  /**
   * @type {{ client: import('./home-client.svelte.js').HomeClient, now: number, linkTarget: string | undefined }}
   * linkTarget: where an article opens.
   */
  let { client, now, linkTarget } = $props();

  const mode = $derived(client.news.mode);
  const newsLink = $derived(pageLink(NEWS_PAGE, {}, mode === MODE_STORIES ? STORIES_HASH : ''));
</script>

<section aria-labelledby="headlines-title" class="flex min-h-0 flex-1 flex-col">
  <header class="flex items-center gap-2 border-b border-line px-3 py-2 lg:px-4">
    <h2 id="headlines-title" class={LABEL}>{strings.news.title}</h2>
    <a href={newsLink} class="spring ml-auto flex items-center gap-1 rounded-sm text-[0.68rem] font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:text-sm">
      {strings.news.more}<ArrowRight class="size-3 tv:size-4" aria-hidden="true" />
    </a>
  </header>

  <div role="group" aria-label={strings.news.categories} class="flex flex-wrap gap-1.5 border-b border-line px-3 py-2 lg:px-4 short:py-1.5">
    <button type="button" aria-pressed={client.category === CATEGORY_ALL} onclick={() => client.pick(CATEGORY_ALL)}
      class="{CHIP} {client.category === CATEGORY_ALL ? 'border-neon-red bg-white/5 text-neon-red' : CHIP_OFF}">
      {strings.news.all} <span class="font-mono text-muted tabular-nums">{client.news.headlines.length}</span>
    </button>
    {#each client.news.categories as c (c.id)}
      {@const style = categoryStyle(mode, c.id)}
      <button type="button" aria-pressed={client.category === c.id} onclick={() => client.pick(c.id)}
        class="{CHIP} {client.category === c.id ? `${style.border} ${style.text} bg-white/5` : CHIP_OFF}">
        {categoryLabel(mode, c.id)} <span class="font-mono text-muted tabular-nums">{c.count}</span>
      </button>
    {/each}
  </div>

  <ol class="list-scroll min-h-0 flex-1 divide-y divide-line md:overflow-y-auto desktop-portrait:grid desktop-portrait:grid-cols-2 desktop-portrait:content-start desktop-portrait:gap-x-px">
    {#each client.headlines as h (h.id)}
      {@const style = categoryStyle(mode, h.category)}
      {@const angle = angleStyle(h.angle)}
      <li class="row-far">
        <a href={h.href} target={h.external ? linkTarget : undefined} rel={h.external ? 'noopener noreferrer' : undefined}
          class="spring grid grid-cols-[2.25rem_minmax(0,1fr)_auto] gap-x-2.5 px-3 py-2.5 hover:bg-white/3 focus-visible:bg-white/5 focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red lg:px-4 short:py-2 tv:grid-cols-[3.5rem_minmax(0,1fr)_auto] tv:py-4">
          <time class="pt-0.5 text-right font-mono text-xs text-muted tabular-nums tv:text-base" datetime={h.at}>{age(h.at, now)}</time>
          <span class="min-w-0">
            <span class="line-clamp-2 text-sm leading-snug font-medium text-white tv:text-lg">{h.title}</span>
            <span class="mt-1 flex min-w-0 items-center gap-x-2 overflow-hidden text-[0.68rem] whitespace-nowrap text-muted tv:text-sm">
              <span class="shrink-0 font-bold tracking-wider uppercase {style.text}">{categoryLabel(mode, h.category)}</span>
              {#if h.angle}<span class="shrink-0 font-bold {angle.text}">{angle.mark} {storyStrings.angles[h.angle] ?? h.angle}</span>{/if}
              {#if h.source}<span class="truncate">{h.source}</span>{/if}
              {#if h.outlets}<span class="shrink-0 font-mono tabular-nums">{h.outlets} {h.outlets === 1 ? strings.carousel.outlet : strings.carousel.outlets}</span>{/if}
              {#each h.tickers as ticker (ticker.symbol)}<span class="shrink-0 font-mono font-semibold tracking-wider text-white/70">{ticker.symbol}</span>{/each}
            </span>
          </span>
          <StoryArt image={h.image} seed={h.seed} category={h.category} class="aspect-4/3 w-14 rounded-sm tv:w-24" />
        </a>
      </li>
    {:else}
      <li class="px-3 py-5 text-sm lg:px-4 {client.newsStatus === STATUS_OFFLINE ? 'text-bad' : 'text-muted'}" role={client.newsStatus === STATUS_OFFLINE ? 'alert' : undefined}>
        {#if client.newsStatus === STATUS_OFFLINE}{strings.news.offline}{:else if client.newsStatus === STATUS_CONNECTING || !client.news.headlines.length}{strings.news.emptyLive}{:else}{strings.news.empty}{/if}
      </li>
    {/each}
    {#if client.headlines.length}
      <li>
        <a href={newsLink} class="spring flex items-center gap-1.5 px-3 py-3 text-[0.68rem] font-bold tracking-wider text-white/70 uppercase hover:bg-white/3 hover:text-white focus-visible:bg-white/5 focus-visible:text-white focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red lg:px-4 tv:text-sm">
          {strings.news.moreRow}<ArrowRight class="size-3 tv:size-4" aria-hidden="true" />
        </a>
      </li>
    {/if}
  </ol>
</section>
