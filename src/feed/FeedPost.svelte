<script>
  /** One post: age, source label, category badge, title link, and watchlist tags. */
  import strings from './strings.json';
  import { KIND_CLASSES, FALLBACK_KIND, age } from './post-format.js';

  /** @type {{ post: import('./feed-client.svelte.js').Post, now: number, linkTarget: string|undefined, onTag: (tag: string) => void, activeTag: string }} */
  let { post, now, linkTarget, onTag, activeTag } = $props();

  const kind = $derived(KIND_CLASSES[post.kind] ?? FALLBACK_KIND);
  const when = $derived(post.published_at ?? post.seen_at);
  const tooltip = $derived(`Published ${post.published_at ? new Date(post.published_at).toLocaleString() : 'unknown'} · seen ${new Date(post.seen_at).toLocaleString()}`);
</script>

<article class="grid grid-cols-[3.25rem_minmax(0,1fr)] gap-x-3 px-4 py-3 short:py-2 lg:px-6">
  <time
    class="pt-0.5 text-right font-mono text-xs tabular-nums text-muted"
    datetime={when}
    title={tooltip}
  >{age(when, now)}</time>

  <div class="min-w-0">
    <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      <span class="truncate font-semibold tracking-wide text-white/90">{post.source}</span>
      <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold uppercase tracking-wider {kind.text} {kind.border}">
        {strings.kinds[post.kind] ?? post.kind}
      </span>
      {#if post.lang && post.lang !== 'English'}
        <span class="text-muted">{post.lang}</span>
      {/if}
    </div>

    <a
      class="spring mt-1 block text-[0.95rem] leading-snug font-medium text-white decoration-neon-red/70 underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none line-clamp-3"
      href={post.url}
      target={linkTarget}
      rel="noopener noreferrer"
    >{post.title}</a>

    {#if post.tags.length}
      <ul class="mt-1.5 flex flex-wrap gap-1.5" aria-label="Categories">
        {#each post.tags as tag (tag)}
          <li>
            <button
              type="button"
              class="spring rounded-sm px-1.5 py-px font-mono text-[0.68rem] font-semibold tracking-wider hover:bg-neon-red/25 focus-visible:outline focus-visible:outline-neon-red {activeTag === tag ? 'bg-neon-red text-app' : 'bg-white/8 text-white/80'}"
              aria-pressed={activeTag === tag}
              onclick={() => onTag(tag)}
            >{tag}</button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</article>
