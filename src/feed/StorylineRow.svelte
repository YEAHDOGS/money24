<script>
  /**
   * One of your storylines on the board: its title, how many stories and outlets sit under it, and, opened,
   * the stories themselves (drawn by the page's own story row) with a way to rename or delete the storyline.
   */
  import Layers from '@lucide/svelte/icons/layers';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { slide } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { storylines } from './storylines.svelte.js';
  import strings from './stories.json';

  const L = strings.lines;
  const OPEN_MS = 240;

  /** @type {{ line: import('./storylines.svelte.js').Storyline, members: import('./stories-client.svelte.js').Story[], member: import('svelte').Snippet<[import('./stories-client.svelte.js').Story, number]> }} */
  let { line, members, member } = $props();

  let open = $state(false);
  let renaming = $state(false);
  let draft = $state('');
  const outlets = $derived(members.reduce((n, s) => Math.max(n, s.outlets), 0));

  function rename(/** @type {SubmitEvent} */ e) {
    e.preventDefault();
    storylines.rename(line.id, draft);
    renaming = false;
  }
</script>

<div class="border-l-2 border-neon-red">
  <button type="button" aria-expanded={open} onclick={() => { open = !open; }}
    class="spring flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none">
    <Layers class="size-5 shrink-0 text-neon-red" aria-hidden="true" />
    <span class="min-w-0 flex-1">
      <span class="block truncate text-[0.95rem] leading-snug font-semibold text-white">{line.title}</span>
      <span class="block font-mono text-xs tabular-nums text-muted">
        {members.length === 1 ? L.countOne : L.count.replace('{n}', String(members.length))}{outlets ? ` · ${L.outlets.replace('{n}', String(outlets))}` : ''}{members.length ? '' : ` · ${L.gone}`}
      </span>
    </span>
    <ChevronDown class="size-4 shrink-0 text-muted transition-transform duration-200 {open ? 'rotate-180' : ''}" aria-hidden="true" />
  </button>

  {#if open}
    <div class="border-t border-line bg-card" transition:slide={{ duration: OPEN_MS, easing: cubicOut }}>
      <div class="flex flex-wrap items-center gap-2 px-4 py-2 text-xs">
        {#if renaming}
          <form onsubmit={rename} class="flex min-w-0 flex-1 items-center gap-1.5">
            <input bind:value={draft} aria-label={L.titleLabel} maxlength="80" class="min-w-0 flex-1 rounded-sm border border-line bg-app px-1.5 py-1 text-xs text-white focus-visible:outline focus-visible:outline-neon-red" />
            <button type="submit" class="spring rounded-sm bg-neon-red px-2 py-1 font-bold tracking-wider text-app uppercase focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{L.create}</button>
          </form>
        {:else}
          <button type="button" onclick={() => { draft = line.title; renaming = true; }}
            class="spring rounded-sm border border-line px-1.5 py-0.5 font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:outline focus-visible:outline-neon-red">{L.rename}</button>
          <button type="button" onclick={() => confirm(L.deleteConfirm) && storylines.remove(line.id)}
            class="spring rounded-sm border border-line px-1.5 py-0.5 font-bold tracking-wider text-white/70 uppercase hover:text-bad focus-visible:outline focus-visible:outline-neon-red">{L.delete}</button>
        {/if}
      </div>
      <ol>
        {#each members as story, i (story.id)}
          <li class="border-t border-line">{@render member(story, i)}</li>
        {/each}
      </ol>
    </div>
  {/if}
</div>
