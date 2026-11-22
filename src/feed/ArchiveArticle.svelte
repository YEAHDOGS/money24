<script>
  /** One archived article: its time (New York), outlet, author, kind, headline, excerpt, and the links
   * to the article and to the feed it was fetched from. The excerpt opens to its full length on a click. */
  import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
  import strings from './archive.json';
  import feedStrings from './strings.json';
  import { FALLBACK_KIND, KIND_CLASSES } from './post-format.js';
  import { etTime, hostOf, isRedirect } from './archive-format.js';

  const MAX_TAGS = 3;
  const LINK_CLASS = 'spring inline-flex min-w-0 items-center gap-1 text-white/60 decoration-neon-red/70 underline-offset-2 hover:text-white hover:underline focus-visible:text-white focus-visible:underline focus-visible:outline-none';

  /** @type {{ article: import('./archive-client.svelte.js').Article, showOutlet: boolean, linkTarget: string|undefined }} */
  let { article, showOutlet, linkTarget } = $props();

  let full = $state(false);
  const kind = $derived(KIND_CLASSES[article.kind] ?? FALLBACK_KIND);
  const redirect = $derived(isRedirect(article.url));
</script>

<article class="grid grid-cols-[3.25rem_minmax(0,1fr)] gap-x-3 px-4 py-2.5 short:py-1.5 lg:px-6 tv:grid-cols-[5rem_minmax(0,1fr)] tv:py-4">
  <time class="pt-0.5 text-right font-mono text-xs tabular-nums text-white/90 tv:text-base" datetime={article.at}>{etTime(article.at)}</time>

  <div class="min-w-0">
    <p class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs tv:text-sm">
      {#if showOutlet}
        {#if article.outlet_url}
          <a class="{LINK_CLASS} font-bold text-white/90!" href={article.outlet_url} target={linkTarget} rel="noopener noreferrer" title={strings.outletSite}>{article.source}</a>
        {:else}
          <span class="font-bold text-white/90">{article.source}</span>
        {/if}
      {/if}
      {#if article.author}<span class="min-w-0 truncate text-muted">{strings.by_author} {article.author}</span>{/if}
      <span class="rounded-sm border px-1 py-px text-[0.6rem] font-bold uppercase tracking-wider {kind.text} {kind.border}">{feedStrings.kinds[article.kind] ?? article.kind}</span>
      {#each article.tags.slice(0, MAX_TAGS) as t (t)}<span class="font-mono text-[0.66rem] font-semibold text-white/55">{t}</span>{/each}
    </p>

    <a class="spring mt-0.5 block text-[0.92rem] leading-snug font-medium text-white decoration-neon-red/70 underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none tv:text-lg"
      href={article.url} target={linkTarget} rel="noopener noreferrer">{article.title}</a>

    {#if article.excerpt}
      <button type="button" onclick={() => { full = !full; }} aria-expanded={full}
        class="spring mt-0.5 block w-full text-left text-xs leading-relaxed text-white/65 hover:text-white/85 focus-visible:text-white/85 focus-visible:outline-none tv:text-sm {full ? '' : 'line-clamp-2 short:line-clamp-1'}">
        {article.excerpt}
      </button>
    {/if}

    <p class="mt-1 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 font-mono text-[0.66rem] tv:text-xs">
      <a class={LINK_CLASS} href={article.url} target={linkTarget} rel="noopener noreferrer" title={redirect ? strings.redirectNote : article.url}>
        <span>{strings.readArticle}</span><span class="truncate text-muted">{hostOf(article.url)}</span><ArrowUpRight class="size-3 shrink-0" aria-hidden="true" />
      </a>
      {#if article.feed_url}
        <a class={LINK_CLASS} href={article.feed_url} target={linkTarget} rel="noopener noreferrer" title={article.feed_url}>
          <span>{strings.readFeed}</span><span class="truncate text-muted">{hostOf(article.feed_url)}</span><ArrowUpRight class="size-3 shrink-0" aria-hidden="true" />
        </a>
      {:else}
        <span class="text-muted" title={article.feed}>{strings.readFeed} {article.feed}</span>
      {/if}
    </p>
  </div>
</article>
