<script>
  /**
   * One folding section of a play, for phones: a full-width heading you tap, a few words of what is
   * inside, and the content sliding open under it. A play is then one screen of headings instead of
   * a long scroll through everything. Wide screens do not fold; PlayDetail lays the sections out there.
   */
  import { untrack } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';

  /**
   * @type {{ id: string, title: string, note?: string, tone?: string, open?: boolean, i?: number, children: import('svelte').Snippet }}
   * note: a few words shown beside the title; tone: its text color class; i: its place in the reveal stagger
   */
  let { id, title, note = '', tone = 'text-muted', open = false, i = 0, children } = $props();

  // Starts as asked, then belongs to the reader.
  let shown = $state(untrack(() => open));
</script>

<section class="reveal border-b border-line" style="--i:{i}" aria-labelledby={id}>
  <h3 {id}>
    <button type="button" aria-expanded={shown} aria-controls="{id}-body" onclick={() => { shown = !shown; }}
      class="spring flex w-full items-center gap-2 px-4 py-3 text-left hover:bg-white/3 focus-visible:bg-white/5 focus-visible:outline-none">
      <span class="text-[0.68rem] font-bold tracking-[0.18em] uppercase {shown ? 'text-white' : 'text-white/75'}">{title}</span>
      <span class="ml-auto truncate font-mono text-[0.68rem] tabular-nums {tone}">{note}</span>
      <ChevronDown class="size-4 shrink-0 text-muted transition-transform duration-200 {shown ? 'rotate-180 text-neon-red' : ''}" aria-hidden="true" />
    </button>
  </h3>
  <div id="{id}-body" class="fold" class:is-open={shown}>
    <div class="fold-inner"><div class="px-4 pb-4">{@render children()}</div></div>
  </div>
</section>

<style>
  /* Opens by growing its one grid row from nothing: the content keeps its own height, so nothing inside reflows. */
  .fold {
    display: grid;
    grid-template-rows: 0fr;
    transition: grid-template-rows 260ms cubic-bezier(0.22, 1, 0.36, 1);
  }
  .fold.is-open {
    grid-template-rows: 1fr;
  }
  /* Folded content is hidden from the keyboard and screen readers too, once it has closed. */
  .fold-inner {
    min-height: 0;
    overflow: hidden;
    visibility: hidden;
    transition: visibility 0s linear 260ms;
  }
  .fold.is-open .fold-inner {
    visibility: visible;
    transition-delay: 0s;
  }
  @media (prefers-reduced-motion: reduce) {
    .fold,
    .fold-inner {
      transition: none;
    }
  }
</style>
