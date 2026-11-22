<script>
  /** The calendar's rail while a day's news is open (tablet and up): the day's outlets, kinds and
   * tags with their counts. A click narrows the day view to one; a second click clears it. */
  import strings from './archive.json';
  import feedStrings from './strings.json';
  import { FALLBACK_KIND, KIND_CLASSES } from './post-format.js';
  import { shortCount } from './archive-format.js';

  const LABEL_CLASS = 'text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted';

  /** @type {{ archive: import('./archive-client.svelte.js').ArchiveDesk }} */
  let { archive } = $props();

  const facets = $derived(archive.facets);

  /** @param {'source'|'kind'|'tag'} key @param {string} value */
  function pick(key, value) {
    archive.setFilter(key, archive[key] === value ? '' : value);
  }
</script>

<div class="py-2">
  <h2 class="px-4 pb-1 {LABEL_CLASS}">{strings.kinds}</h2>
  <ul>
    {#each facets?.kinds ?? [] as f (f.name)}
      {@const colors = KIND_CLASSES[f.name] ?? FALLBACK_KIND}
      <li>
        <button type="button" onclick={() => pick('kind', f.name)} aria-pressed={archive.kind === f.name}
          class="spring flex w-full items-center gap-2.5 px-4 py-1 text-left text-sm hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {archive.kind === f.name ? 'text-white' : 'text-white/60'}">
          <span class="h-3 w-0.5 rounded-full {colors.bg} {archive.kind === f.name ? 'opacity-100' : 'opacity-30'}"></span>
          <span class="flex-1 truncate">{feedStrings.kinds[f.name] ?? f.name}</span>
          <span class="font-mono text-xs tabular-nums text-muted">{shortCount(f.n)}</span>
        </button>
      </li>
    {/each}
  </ul>
</div>

{#if facets?.tags.length}
  <div class="border-t border-line px-4 py-3">
    <h2 class={LABEL_CLASS}>{strings.tags}</h2>
    <ul class="mt-2 flex flex-wrap gap-1.5">
      {#each facets.tags as f (f.name)}
        <li>
          <button type="button" onclick={() => pick('tag', f.name)} aria-pressed={archive.tag === f.name}
            class="spring rounded-sm px-1.5 py-0.5 font-mono text-[0.68rem] font-semibold tracking-wider focus-visible:outline focus-visible:outline-neon-red {archive.tag === f.name ? 'bg-neon-red text-app' : 'bg-white/8 text-white/75 hover:bg-neon-red/25'}">
            {f.name} <span class="opacity-60">{shortCount(f.n)}</span>
          </button>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<div class="border-t border-line py-2">
  <h2 class="px-4 pb-1 {LABEL_CLASS}">{strings.outlets}</h2>
  <ul>
    {#each facets?.sources ?? [] as f (f.name)}
      <li>
        <button type="button" onclick={() => pick('source', f.name)} aria-pressed={archive.source === f.name}
          class="spring flex w-full items-center gap-2 px-4 py-1 text-left text-xs hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {archive.source === f.name ? 'bg-white/5 text-white' : 'text-white/65'}">
          <span class="size-1.5 shrink-0 rounded-full {archive.source === f.name ? 'bg-neon-red' : 'bg-white/20'}"></span>
          <span class="flex-1 truncate">{f.name}</span>
          <span class="font-mono tabular-nums text-muted">{shortCount(f.n)}</span>
        </button>
      </li>
    {/each}
  </ul>
</div>
