<script>
  /**
   * The bracket on one price line: stop on the left, entry, then each ladder target.
   * Positions are percent of the stop→top-target range, so any price scale fits. Points that would
   * land on top of each other are moved apart, in order, so every price stays readable on a phone.
   */
  import strings from './strings.json';
  import { priceLabel } from './trading-client.svelte.js';

  /** @type {{ plan: import('./trading-client.svelte.js').Plan }} */
  let { plan } = $props();

  const PAD_PCT = 6;
  const FULL_PCT = 100;
  const MIN_GAP_PCT = 15; // closer than this and two prices print over each other
  const top = $derived(Math.max(...plan.targets.map((t) => t.price), plan.entry));
  const range = $derived(top - plan.stop || 1);
  /** @param {number} price */
  const at = (price) => PAD_PCT + ((price - plan.stop) / range) * (FULL_PCT - 2 * PAD_PCT);

  /** Where each point is drawn: its place on the scale, pushed right of the one before it, then back inside the line. */
  function spread(/** @type {number[]} */ places) {
    const gap = Math.min(MIN_GAP_PCT, (FULL_PCT - 2 * PAD_PCT) / Math.max(1, places.length - 1));
    const out = [...places];
    for (let i = 1; i < out.length; i += 1) out[i] = Math.max(out[i], out[i - 1] + gap);
    for (let i = out.length - 1; i >= 0; i -= 1) out[i] = Math.min(out[i], FULL_PCT - PAD_PCT - (out.length - 1 - i) * gap);
    return out;
  }

  const spots = $derived(spread([plan.stop, plan.entry, ...plan.targets.map((t) => t.price)].map(at)));
  const stopAt = $derived(spots[0]);
  const entryAt = $derived(spots[1]);
  const topAt = $derived(Math.max(...spots));

</script>

<div class="relative h-16 select-none" role="img" aria-label="Stop {priceLabel(plan.stop)}, entry {priceLabel(plan.entry)}, targets {plan.targets.map((t) => priceLabel(t.price)).join(', ')}">
  <div class="absolute inset-x-0 top-7 h-px bg-line"></div>
  <div class="absolute top-7 h-px bg-bad" style="left:{stopAt}%; width:{entryAt - stopAt}%"></div>
  <div class="absolute top-7 h-px bg-ok" style="left:{entryAt}%; width:{topAt - entryAt}%"></div>

  <div class="absolute top-0 flex -translate-x-1/2 flex-col items-center" style="left:{stopAt}%">
    <span class="font-mono text-[0.68rem] tabular-nums text-bad">{priceLabel(plan.stop)}</span>
    <span class="mt-1 size-2.5 rounded-full border-2 border-bad bg-app"></span>
    <span class="mt-1 text-[0.6rem] font-bold uppercase tracking-wider text-muted">{strings.stop}</span>
  </div>
  <div class="absolute top-0 flex -translate-x-1/2 flex-col items-center" style="left:{entryAt}%">
    <span class="font-mono text-[0.68rem] tabular-nums text-white">{priceLabel(plan.entry)}</span>
    <span class="mt-1 size-2.5 rounded-full bg-white"></span>
    <span class="mt-1 text-[0.6rem] font-bold uppercase tracking-wider text-muted">{strings.entry}</span>
  </div>
  {#each plan.targets as t, i (i)}
    <div class="absolute top-0 flex -translate-x-1/2 flex-col items-center" style="left:{spots[i + 2]}%">
      <span class="font-mono text-[0.68rem] tabular-nums text-ok">{priceLabel(t.price)}</span>
      <span class="mt-1 size-2.5 rounded-full bg-ok"></span>
      <span class="mt-1 font-mono text-[0.6rem] font-bold tabular-nums text-muted">×{t.qty}</span>
    </div>
  {/each}
</div>
