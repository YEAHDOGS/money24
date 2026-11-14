<script>
  /**
   * The bracket of a draft: entry, stop, ladder rungs, the runner and every failsafe, plus the
   * theme and thesis. Saving sends the whole play back; the rules and the math rerun on it.
   */
  import { untrack } from 'svelte';
  import strings from './strings.json';
  import { num } from './play-format.js';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';

  /** @type {{ play: any, onSave: (play: any) => Promise<void> }} */
  let { play, onSave } = $props();

  const INPUT = 'w-full min-w-0 rounded-sm border border-line bg-card px-2 py-1 font-mono text-xs tabular-nums text-white focus:border-neon-red focus:outline-none';
  const FIELD = 'grid gap-1 text-[0.68rem] text-muted';
  const MINUTE_MS = 60000;
  const LOCAL_LEN = 16; // "YYYY-MM-DDTHH:mm"

  /** ISO time → the value a datetime-local input shows, in this device's time zone. */
  function toLocal(/** @type {string} */ iso) {
    const t = Date.parse(iso);
    if (!(t > 0)) return '';
    const d = new Date(t - new Date(t).getTimezoneOffset() * MINUTE_MS);
    return d.toISOString().slice(0, LOCAL_LEN);
  }

  /** The editable copy: numbers as typed, until saved. */
  function fromPlay(/** @type {any} */ p) {
    return {
      entry: p.entry, stop: p.stop, theme: p.theme ?? '', thesis: p.thesis ?? '',
      ladder: (p.ladder ?? []).map((/** @type {any} */ r) => ({ ...r })),
      runnerQty: p.runner?.qty ?? 0, runnerTarget: p.runner?.target ?? 0,
      exitBy: toLocal(p.failsafes?.exit_by), sideways: p.failsafes?.sideways_minutes ?? 0,
      sidewaysGain: p.failsafes?.sideways_min_gain_pct ?? 0, noQuote: p.failsafes?.no_quote_minutes ?? 0,
      gap: p.failsafes?.gap_pct ?? 0, calamity: p.failsafes?.calamity ?? '',
    };
  }

  let f = $state(untrack(() => fromPlay(play)));
  let saved = $state(false);
  let busy = $state(false);

  async function save(/** @type {SubmitEvent} */ event) {
    event.preventDefault();
    busy = true;
    const ladder = f.ladder.map((r) => ({ qty: num(r.qty), price: num(r.price) })).filter((r) => r.qty > 0);
    const runner = { qty: num(f.runnerQty), target: num(f.runnerTarget) };
    const qty = ladder.reduce((s, r) => s + r.qty, runner.qty);
    const exitBy = f.exitBy ? new Date(f.exitBy).toISOString() : '0001-01-01T00:00:00Z';
    try {
      await onSave({
        ...play, qty, entry: num(f.entry), stop: num(f.stop), ladder, runner, theme: f.theme.trim(), thesis: f.thesis.trim(),
        failsafes: { exit_by: exitBy, sideways_minutes: num(f.sideways), sideways_min_gain_pct: num(f.sidewaysGain),
          no_quote_minutes: num(f.noQuote), gap_pct: num(f.gap), calamity: f.calamity.trim() },
      });
      saved = true;
    } finally {
      busy = false;
    }
  }
</script>

<form class="mt-2 grid gap-3" onsubmit={save} oninput={() => { saved = false; }}>
  <div class="grid grid-cols-2 gap-2 @md:grid-cols-4">
    <label class={FIELD}>{strings.bracket.entry}<input bind:value={f.entry} type="number" step="0.01" min="0" class={INPUT} /></label>
    <label class={FIELD}>{strings.bracket.stop}<input bind:value={f.stop} type="number" step="0.01" min="0" class={INPUT} /></label>
    <label class={FIELD}>{strings.bracket.runnerQty}<input bind:value={f.runnerQty} type="number" step="1" min="0" class={INPUT} /></label>
    <label class={FIELD}>{strings.bracket.runnerTarget}<input bind:value={f.runnerTarget} type="number" step="0.01" min="0" class={INPUT} /></label>
  </div>

  <fieldset class="grid gap-1.5">
    <legend class="text-[0.68rem] text-muted">{strings.bracket.ladder}</legend>
    {#each f.ladder as r, i (i)}
      <div class="grid grid-cols-[2rem_minmax(0,1fr)_minmax(0,1fr)_1.75rem] items-center gap-2">
        <span class="font-mono text-xs text-ok">T{i + 1}</span>
        <input bind:value={r.qty} type="number" step="1" min="0" aria-label="{strings.bracket.rung} {i + 1} {strings.bracket.qty}" placeholder={strings.bracket.qty} class={INPUT} />
        <input bind:value={r.price} type="number" step="0.01" min="0" aria-label="{strings.bracket.rung} {i + 1} {strings.bracket.price}" placeholder={strings.bracket.price} class={INPUT} />
        <button type="button" onclick={() => { f.ladder.splice(i, 1); }} aria-label={strings.bracket.removeRung}
          class="spring grid size-7 place-items-center rounded-sm border border-line text-white/50 hover:border-bad hover:text-bad focus-visible:border-bad focus-visible:outline-none">
          <X class="size-3.5" aria-hidden="true" />
        </button>
      </div>
    {/each}
    <button type="button" onclick={() => { f.ladder.push({ qty: 1, price: num(f.entry) * 2 }); }}
      class="spring flex w-fit items-center gap-1 rounded-sm border border-line px-2.5 py-1 text-xs text-white/70 hover:border-white/40 hover:text-white focus-visible:border-neon-red focus-visible:outline-none">
      <Plus class="size-3.5" aria-hidden="true" />{strings.bracket.addRung}
    </button>
  </fieldset>

  <fieldset class="grid grid-cols-2 gap-2 @md:grid-cols-3">
    <legend class="mb-1 text-[0.68rem] text-muted">{strings.bracket.failsafes}</legend>
    <label class="{FIELD} col-span-2 @md:col-span-1">{strings.bracket.exitBy}<input bind:value={f.exitBy} type="datetime-local" class={INPUT} /></label>
    <label class={FIELD}>{strings.bracket.sideways}<input bind:value={f.sideways} type="number" step="1" min="0" class={INPUT} /></label>
    <label class={FIELD}>{strings.bracket.sidewaysGain}<input bind:value={f.sidewaysGain} type="number" step="1" class={INPUT} /></label>
    <label class={FIELD}>{strings.bracket.noQuote}<input bind:value={f.noQuote} type="number" step="1" min="0" class={INPUT} /></label>
    <label class={FIELD}>{strings.bracket.gap}<input bind:value={f.gap} type="number" step="0.5" min="0" class={INPUT} /></label>
    <label class="{FIELD} col-span-2 @md:col-span-3">{strings.bracket.calamity}<textarea bind:value={f.calamity} rows="2" class="{INPUT} resize-y font-sans"></textarea></label>
  </fieldset>

  <div class="grid gap-2 @md:grid-cols-[12rem_minmax(0,1fr)]">
    <label class={FIELD}>{strings.detail.theme}<input bind:value={f.theme} class="{INPUT} font-sans" /></label>
    <label class={FIELD}>{strings.bracket.thesis}<textarea bind:value={f.thesis} rows="2" class="{INPUT} resize-y font-sans"></textarea></label>
  </div>

  <div class="flex items-center gap-3">
    <button type="submit" disabled={busy}
      class="spring rounded-sm border border-neon-red px-3 py-1.5 text-xs font-bold uppercase tracking-wider text-neon-red hover:bg-neon-red/10 focus-visible:bg-neon-red/10 focus-visible:outline-none disabled:opacity-40">{strings.bracket.save}</button>
    <span class="text-xs text-ok" role="status">{saved ? strings.bracket.saved : ''}</span>
  </div>
</form>
