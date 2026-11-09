<script>
  /**
   * The bracket on one price line: stop on the left, entry, then each ladder target.
   * Positions are percent of the stop→top-target range, so any price scale fits.
   */
  import strings from './strings.json';

  /** @type {{ plan: import('./trading-client.svelte.js').Plan }} */
  let { plan } = $props();

  const PAD_PCT = 6;
  const top = $derived(Math.max(...plan.targets.map((t) => t.price), plan.entry));
  const range = $derived(top - plan.stop || 1);
  /** @param {number} price */
  const at = (price) => PAD_PCT + ((price - plan.stop) / range) * (100 - 2 * PAD_PCT);
  const money = (/** @type {number} */ n) => n.toFixed(2);
</script>

<div class="relative h-16 select-none" role="img" aria-label="Stop {money(plan.stop)}, entry {money(plan.entry)}, targets {plan.targets.map((t) => money(t.price)).join(', ')}">
  <div class="absolute inset-x-0 top-7 h-px bg-line"></div>
  <div class="absolute top-7 h-px bg-bad" style="left:{at(plan.stop)}%; width:{at(plan.entry) - at(plan.stop)}%"></div>
  <div class="absolute top-7 h-px bg-ok" style="left:{at(plan.entry)}%; width:{at(top) - at(plan.entry)}%"></div>

  <div class="absolute top-0 flex -translate-x-1/2 flex-col items-center" style="left:{at(plan.stop)}%">
    <span class="font-mono text-[0.68rem] tabular-nums text-bad">{money(plan.stop)}</span>
    <span class="mt-1 size-2.5 rounded-full border-2 border-bad bg-app"></span>
    <span class="mt-1 text-[0.6rem] font-bold uppercase tracking-wider text-muted">{strings.stop}</span>
  </div>
  <div class="absolute top-0 flex -translate-x-1/2 flex-col items-center" style="left:{at(plan.entry)}%">
    <span class="font-mono text-[0.68rem] tabular-nums text-white">{money(plan.entry)}</span>
    <span class="mt-1 size-2.5 rounded-full bg-white"></span>
    <span class="mt-1 text-[0.6rem] font-bold uppercase tracking-wider text-muted">{strings.entry}</span>
  </div>
  {#each plan.targets as t, i (i)}
    <div class="absolute top-0 flex -translate-x-1/2 flex-col items-center" style="left:{at(t.price)}%">
      <span class="font-mono text-[0.68rem] tabular-nums text-ok">{money(t.price)}</span>
      <span class="mt-1 size-2.5 rounded-full bg-ok"></span>
      <span class="mt-1 font-mono text-[0.6rem] font-bold tabular-nums text-muted">×{t.qty}</span>
    </div>
  {/each}
</div>
