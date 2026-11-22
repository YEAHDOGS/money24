<script>
  /**
   * One block of the pocket: a small heading with its count, over a list of rows. The rows are the
   * children; the block draws the 1px line under itself so blocks stack without double lines.
   */
  /** @type {{ id: string, title: string, count?: number, tone?: string, children: import('svelte').Snippet }} */
  let { id, title, count, tone = '', children } = $props();
</script>

<section aria-labelledby={id} class="border-b border-line">
  <h2 {id} class="flex items-center gap-2 px-3 pt-2.5 pb-1 text-[0.6rem] font-bold tracking-[0.18em] text-muted uppercase tv:text-sm">
    {title}
    {#if count !== undefined}<span class="font-mono tabular-nums {tone || 'text-white/80'}">{count}</span>{/if}
  </h2>
  <ul class="pb-1.5">{@render children()}</ul>
</section>
