<script>
  /**
   * The stake dial. It starts at the play's minimum deposit (one unit for every rung and the runner)
   * and scales the whole bracket above it: every dollar, level and check on the page follows the
   * dial through the runner's preview. Releasing it saves the size on a draft; on a play already
   * paper trading it is a what-if and saves nothing. Drag, arrow keys, PageUp/PageDown, the wheel.
   */
  import strings from './strings.json';
  import { usd, price, pnl } from './play-format.js';
  import Minus from '@lucide/svelte/icons/minus';
  import Plus from '@lucide/svelte/icons/plus';

  /**
   * @type {{ item: import('./plays-client.svelte.js').Item, client: import('./plays-client.svelte.js').PlaysClient,
   *   draft: boolean, bankroll: number, sizing: any,
   *   onPreview: (p: import('./plays-client.svelte.js').Item | null) => void, onSaved: () => void }}
   */
  let { item, client, draft, bankroll, sizing, onPreview, onSaved } = $props();

  const DEBOUNCE_MS = 60;
  const PERCENT = 100;
  const WHEEL_FINE = 1;
  const WHEEL_COARSE_SHARE = 0.05; // Shift + wheel moves 5% of the range

  // The dial's own position: follows the saved size until the user moves it.
  let units = $state(0);
  let saved = $state(false);
  let timer = 0;
  let commitTimer = 0;
  /** @type {AbortController | null} */
  let inflight = null;

  // A derived only changes when the number does, so a poll with the same size leaves the dial alone.
  const savedQty = $derived(item.play.qty);
  $effect.pre(() => {
    units = savedQty;
  });

  const base = $derived(item.report.sizing);
  const min = $derived(base.min_units);
  const max = $derived(Math.max(base.slider_max, min + 1));
  const at = $derived((/** @type {number} */ u) => ((u - min) / (max - min)) * PERCENT);
  const stake = $derived(units * base.unit_cost_usd);
  const ofBankroll = $derived(bankroll > 0 ? (stake / bankroll) * PERCENT : 0);

  function preview(/** @type {number} */ next) {
    clearTimeout(timer);
    inflight?.abort();
    if (next === savedQty) {
      onPreview(null);
      return;
    }
    timer = setTimeout(() => {
      inflight = new AbortController();
      client.preview(item.play.id, next, inflight.signal).then(onPreview).catch(() => {});
    }, DEBOUNCE_MS);
  }

  function set(/** @type {number} */ next) {
    units = Math.round(Math.min(max, Math.max(min, next)));
    saved = false;
    preview(units);
  }

  async function commit() {
    if (!draft || units === savedQty) return;
    clearTimeout(timer);
    inflight?.abort();
    await client.size(item.play.id, units);
    saved = true;
    onSaved();
  }

  /** The wheel nudges the dial; listeners are attached non-passive so the page doesn't scroll instead. */
  function wheel(/** @type {HTMLElement} */ node) {
    const onWheel = (/** @type {WheelEvent} */ e) => {
      e.preventDefault();
      const step = e.shiftKey ? Math.max(1, Math.round((max - min) * WHEEL_COARSE_SHARE)) : WHEEL_FINE;
      set(units + (e.deltaY < 0 || e.deltaX > 0 ? step : -step));
      clearTimeout(commitTimer);
      commitTimer = setTimeout(commit, DEBOUNCE_MS * 8);
    };
    node.addEventListener('wheel', onWheel, { passive: false });
    return () => node.removeEventListener('wheel', onWheel);
  }
</script>

<div {@attach wheel}>
  <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
    <label for="stake-dial" class="text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.dial.label}</label>
    <p class="font-mono tabular-nums">
      <span class="text-2xl font-bold text-white">{usd(stake)}</span>
      <span class="ml-1 text-xs text-muted">{units} {strings.dial.units} × {usd(base.unit_cost_usd)}</span>
    </p>
  </div>

  <div class="mt-2 flex items-center gap-2">
    <button type="button" onclick={() => { set(units - 1); commit(); }} aria-label={strings.dial.less} disabled={units <= min}
      class="spring grid size-8 shrink-0 place-items-center rounded-sm border border-line text-white/80 hover:border-white/40 focus-visible:border-neon-red focus-visible:outline-none disabled:opacity-30">
      <Minus class="size-4" aria-hidden="true" />
    </button>
    <div class="relative min-w-0 flex-1">
      <input id="stake-dial" type="range" class="poll-range block w-full" {min} {max} step="1" value={units}
        oninput={(e) => set(Number(e.currentTarget.value))} onchange={commit}
        aria-valuetext={`${usd(stake)}, ${units} ${strings.dial.units}`} />
      {#if sizing.liquidity_capped && base.liquidity_max > min && base.liquidity_max < max}
        <span class="dial-mark pointer-events-none absolute top-0 h-1.5 w-px bg-warn" style="--at:{at(base.liquidity_max)}%" aria-hidden="true"></span>
      {/if}
    </div>
    <button type="button" onclick={() => { set(units + 1); commit(); }} aria-label={strings.dial.more} disabled={units >= max}
      class="spring grid size-8 shrink-0 place-items-center rounded-sm border border-line text-white/80 hover:border-white/40 focus-visible:border-neon-red focus-visible:outline-none disabled:opacity-30">
      <Plus class="size-4" aria-hidden="true" />
    </button>
  </div>

  <div class="mt-1 flex justify-between gap-2 font-mono text-[0.65rem] tabular-nums text-muted">
    <span>{strings.dial.minDeposit} {usd(base.min_deposit_usd)} · {min} {strings.dial.units}</span>
    {#if sizing.liquidity_capped}<span class="text-warn">{strings.dial.liquidityCap} {base.liquidity_max} {strings.dial.units}</span>{/if}
    <span>{usd(max * base.unit_cost_usd)}</span>
  </div>

  <dl class="mt-3 grid grid-cols-[repeat(auto-fill,minmax(8.5rem,1fr))] gap-x-4 gap-y-1 font-mono text-xs tabular-nums">
    {#if bankroll > 0}
      <div><dt class="text-muted">{strings.dial.ofBankroll}</dt><dd class={ofBankroll > PERCENT ? 'text-warn' : 'text-white'}>{ofBankroll.toFixed(0)}% of {usd(bankroll)}</dd></div>
    {/if}
    <div><dt class="text-muted">{strings.dial.ofVolume}</dt><dd class="text-white">{sizing.pct_of_volume.toFixed(1)}%</dd></div>
    {#if item.play.contract.market === 'option'}
      <div><dt class="text-muted">{strings.dial.ofOI}</dt><dd class="text-white">{sizing.pct_of_oi.toFixed(1)}%</dd></div>
    {/if}
    <div><dt class="text-muted">{strings.dial.estFill}</dt><dd class={sizing.est_slip_usd > 0 ? 'text-warn' : 'text-white'}>{price(sizing.est_entry)}{sizing.est_slip_usd > 0 ? ` · ${strings.dial.slip} ${pnl(-sizing.est_slip_usd)}` : ''}</dd></div>
  </dl>

  <p class="mt-2 min-h-4 text-[0.7rem] leading-snug text-muted" role="status">
    {#if !draft}{strings.dial.whatIf}{:else if saved}<span class="text-ok">{strings.dial.saved}</span>{/if}
  </p>
</div>
