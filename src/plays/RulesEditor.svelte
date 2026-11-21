<script>
  /**
   * The rulebook: every rule (on or off, a block or a warning, its settings), the evidence weights,
   * the news filter, the bracket new plays start with, and the desk switches. DOGS defaults ship
   * on; everything is editable, and reset brings the defaults back.
   */
  import strings from './strings.json';
  import { num } from './play-format.js';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';

  /** @type {{ client: import('./plays-client.svelte.js').PlaysClient }} */
  let { client } = $props();

  const DOGS_RULES = ['exits_required', 'crowd_evidence', 'news_trust', 'volume_first'];
  const WEIGHTS = ['volume', 'news', 'open_interest', 'technicals', 'signals'];
  const INPUT = 'w-full min-w-0 rounded-sm border border-line bg-card px-2 py-1 font-mono text-xs tabular-nums text-white focus:border-neon-red focus:outline-none disabled:opacity-40';
  const FIELD = 'grid gap-1 text-[0.68rem] text-muted';
  const LABEL = 'text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted';
  const TEMPLATE_NUMBERS = [
    ['budget_usd', 'budget', '1'], ['stop_pct', 'stopPct', '1'], ['runner_gain_pct', 'runnerGain', '10'],
    ['exit_days_before_expiry', 'exitDays', '1'], ['exit_hour', 'exitHour', '1'], ['hold_days', 'holdDays', '1'],
    ['sideways_minutes', 'sideways', '15'], ['sideways_min_gain_pct', 'sidewaysGain', '1'],
    ['no_quote_minutes', 'noQuote', '1'], ['gap_pct', 'gap', '0.5'],
  ];

  /** The finder's numbers: rulebook key, label in strings.json, input step. */
  const FINDER_NUMBERS = [
    ['min_attention', 'minAttention', '5'], ['min_validity', 'minValidity', '5'], ['min_sided', 'minSided', '1'],
    ['max_per_day', 'maxPerDay', '1'], ['min_days', 'minDays', '1'], ['otm_pct', 'otmPct', '0.5'],
    ['max_day_loss_usd', 'maxDayLoss', '10'],
  ];

  /** @type {any} */
  let draft = $state(null);
  let busy = $state(false);
  let message = $state('');
  let failed = $state(false);

  /** The rulebook the draft was copied from: not reactive, only compared. */
  let source = null;

  // Start from the runner's rulebook, and again after each save or reset.
  $effect.pre(() => {
    const rules = client.rules;
    if (!rules || rules === source) return;
    source = rules;
    draft = structuredClone($state.snapshot(rules));
  });

  const dirty = $derived(!!draft && JSON.stringify($state.snapshot(draft)) !== JSON.stringify($state.snapshot(client.rules)));

  /** The rulebook with every number coerced from what was typed. */
  function normalized() {
    const r = structuredClone($state.snapshot(draft));
    r.bankroll_usd = num(r.bankroll_usd);
    r.news_min_sources = num(r.news_min_sources);
    r.news_min_trust_pct = num(r.news_min_trust_pct);
    for (const rule of r.rules) for (const k of Object.keys(rule.params)) rule.params[k] = num(rule.params[k]);
    for (const k of WEIGHTS) r.weights[k] = num(r.weights[k]);
    for (const [k] of TEMPLATE_NUMBERS) r.template[k] = num(r.template[k]);
    if (r.finder) for (const [k] of FINDER_NUMBERS) r.finder[k] = num(r.finder[k]);
    r.template.ladder = r.template.ladder.map((/** @type {any} */ x) => ({ pct_of_position: num(x.pct_of_position), gain_pct: num(x.gain_pct) }));
    return r;
  }

  async function run(/** @type {() => Promise<any>} */ fn) {
    busy = true;
    message = '';
    try {
      await fn();
      failed = false;
      message = strings.rulesEditor.saved;
    } catch (err) {
      failed = true;
      message = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }

  function reset() {
    if (!confirm(strings.rulesEditor.resetConfirm)) return;
    run(() => client.resetRules());
  }
</script>

<div class="list-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto [scrollbar-gutter:stable]">
  {#if !draft}
    <p class="px-6 py-10 text-sm text-muted">{strings.rulesEditor.loading}</p>
  {:else}
    <form class="grid gap-5 px-4 py-4 lg:px-6" onsubmit={(e) => { e.preventDefault(); run(() => client.saveRules(normalized())); }}>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h2 class="text-lg font-bold text-white">{strings.rulesEditor.title} <span class="font-mono text-sm text-muted">· {draft.name}</span></h2>
        <button type="button" onclick={reset} disabled={busy}
          class="spring flex items-center gap-1 rounded-sm border border-line px-2.5 py-1 text-xs text-white/70 hover:border-white/40 hover:text-white focus-visible:border-neon-red focus-visible:outline-none disabled:opacity-40">
          <RotateCcw class="size-3.5" aria-hidden="true" />{strings.rulesEditor.reset}
        </button>
      </div>

      <section class="grid gap-2">
        <label class="flex items-start gap-2 text-sm text-white"><input type="checkbox" bind:checked={draft.auto_paper} class="mt-1 accent-neon-red" />{strings.rulesEditor.autoPaper}</label>
        <label class="flex items-start gap-2 text-sm text-white"><input type="checkbox" bind:checked={draft.paper_ignores_blocks} class="mt-1 accent-neon-red" />{strings.rulesEditor.paperIgnores}</label>
        <label class="{FIELD} max-w-48">{strings.rulesEditor.bankroll}<input bind:value={draft.bankroll_usd} type="number" min="0" step="50" class={INPUT} /></label>
      </section>

      <!-- THE FINDER: plays the engine opens on its own from the news. An older runner sends no finder. -->
      {#if draft.finder}
        <section class="grid gap-2">
          <h3 class={LABEL}>{strings.rulesEditor.finder.title}</h3>
          <label class="flex items-start gap-2 text-sm text-white"><input type="checkbox" bind:checked={draft.finder.on} class="mt-1 accent-neon-red" />{strings.rulesEditor.finder.on}</label>
          <label class="flex items-start gap-2 text-sm text-white"><input type="checkbox" bind:checked={draft.finder.move_must_agree} disabled={!draft.finder.on} class="mt-1 accent-neon-red" />{strings.rulesEditor.finder.moveMustAgree}</label>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {#each FINDER_NUMBERS as [key, label, step] (key)}
              <label class={FIELD}>{strings.rulesEditor.finder[label]}<input bind:value={draft.finder[key]} type="number" min="0" {step} disabled={!draft.finder.on} class={INPUT} /></label>
            {/each}
          </div>
          <p class="text-xs leading-snug text-muted">{strings.rulesEditor.finder.note}</p>
        </section>
      {/if}

      <section>
        <h3 class={LABEL}>{strings.detail.checks}</h3>
        <ul class="mt-2 divide-y divide-line border-y border-line">
          {#each draft.rules as rule (rule.id)}
            {@const dogs = DOGS_RULES.includes(rule.id)}
            <li class="grid gap-2 py-2.5 {rule.enabled ? '' : 'opacity-60'}">
              <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                <label class="flex items-center gap-2 text-sm font-semibold text-white">
                  <input type="checkbox" bind:checked={rule.enabled} class="accent-neon-red" aria-label="{strings.rulesEditor.enabled}: {strings.rules[rule.id]?.label}" />
                  {strings.rules[rule.id]?.label ?? rule.id}
                </label>
                <span class="rounded-sm border px-1 text-[0.58rem] font-bold uppercase tracking-wider {dogs ? 'border-neon-red text-neon-red' : 'border-line text-muted'}">{dogs ? strings.rulesEditor.dogs : strings.rulesEditor.suggested}</span>
                <span class="ml-auto inline-flex overflow-hidden rounded-sm border border-line text-[0.62rem] font-bold uppercase tracking-wider" role="group">
                  <button type="button" disabled={!rule.enabled} aria-pressed={rule.blocking} onclick={() => { rule.blocking = true; }}
                    class="spring px-2 py-0.5 focus-visible:outline-none focus-visible:bg-white/10 {rule.blocking ? 'bg-bad/15 text-bad' : 'text-white/50'}">{strings.rulesEditor.block}</button>
                  <button type="button" disabled={!rule.enabled} aria-pressed={!rule.blocking} onclick={() => { rule.blocking = false; }}
                    class="spring border-l border-line px-2 py-0.5 focus-visible:outline-none focus-visible:bg-white/10 {!rule.blocking ? 'bg-warn/15 text-warn' : 'text-white/50'}">{strings.rulesEditor.warn}</button>
                </span>
              </div>
              <p class="text-xs leading-snug text-white/60 max-md:truncate">{strings.rules[rule.id]?.about}</p>
              {#if Object.keys(rule.params).length}
                <div class="grid grid-cols-2 gap-2 @md:grid-cols-4">
                  {#each Object.keys(rule.params) as k (k)}
                    <label class={FIELD}>{strings.params[k] ?? k}<input bind:value={rule.params[k]} type="number" step="any" min="0" disabled={!rule.enabled} class={INPUT} /></label>
                  {/each}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      </section>

      <section>
        <h3 class={LABEL}>{strings.rulesEditor.weights}</h3>
        <div class="mt-2 grid grid-cols-2 gap-2 @md:grid-cols-5">
          {#each WEIGHTS as k (k)}
            <label class={FIELD}>{strings.weights[k]}<input bind:value={draft.weights[k]} type="number" min="0" step="5" class={INPUT} /></label>
          {/each}
        </div>
      </section>

      <section>
        <h3 class={LABEL}>{strings.rulesEditor.news}</h3>
        <div class="mt-2 grid grid-cols-2 gap-2 @md:grid-cols-4">
          <label class={FIELD}>{strings.rulesEditor.newsMinSources}<input bind:value={draft.news_min_sources} type="number" min="0" step="1" class={INPUT} /></label>
          <label class={FIELD}>{strings.rulesEditor.newsMinTrust}<input bind:value={draft.news_min_trust_pct} type="number" min="0" max="100" step="5" class={INPUT} /></label>
        </div>
        <label class="mt-2 flex items-start gap-2 text-xs text-white/80"><input type="checkbox" bind:checked={draft.breaking_overrides_technicals} class="mt-0.5 accent-neon-red" />{strings.rulesEditor.breaking}</label>
      </section>

      <section>
        <h3 class={LABEL}>{strings.rulesEditor.template}</h3>
        <div class="mt-2 grid grid-cols-2 gap-2 @md:grid-cols-4">
          {#each TEMPLATE_NUMBERS as [key, label, step] (key)}
            <label class={FIELD}>{strings.rulesEditor[label]}<input bind:value={draft.template[key]} type="number" min="0" {step} class={INPUT} /></label>
          {/each}
        </div>
        <div class="mt-2 grid gap-1.5">
          {#each draft.template.ladder as rung, i (i)}
            <div class="grid grid-cols-[2rem_minmax(0,1fr)_minmax(0,1fr)_1.75rem] items-end gap-2">
              <span class="pb-1 font-mono text-xs text-ok">T{i + 1}</span>
              <label class={FIELD}>{strings.rulesEditor.ladderPct}<input bind:value={rung.pct_of_position} type="number" min="0" max="100" step="1" class={INPUT} /></label>
              <label class={FIELD}>{strings.rulesEditor.ladderGain}<input bind:value={rung.gain_pct} type="number" min="0" step="10" class={INPUT} /></label>
              <button type="button" onclick={() => { draft.template.ladder.splice(i, 1); }} aria-label={strings.bracket.removeRung}
                class="spring grid size-7 place-items-center rounded-sm border border-line text-white/50 hover:border-bad hover:text-bad focus-visible:border-bad focus-visible:outline-none">
                <X class="size-3.5" aria-hidden="true" />
              </button>
            </div>
          {/each}
          <button type="button" onclick={() => { draft.template.ladder.push({ pct_of_position: 25, gain_pct: 200 }); }}
            class="spring flex w-fit items-center gap-1 rounded-sm border border-line px-2.5 py-1 text-xs text-white/70 hover:border-white/40 hover:text-white focus-visible:border-neon-red focus-visible:outline-none">
            <Plus class="size-3.5" aria-hidden="true" />{strings.bracket.addRung}
          </button>
        </div>
        <label class="{FIELD} mt-2">{strings.rulesEditor.calamity}<textarea bind:value={draft.template.calamity} rows="2" class="{INPUT} resize-y font-sans"></textarea></label>
      </section>

      <div class="sticky bottom-0 -mx-4 flex items-center gap-3 border-t border-line bg-app px-4 py-2.5 lg:-mx-6 lg:px-6">
        <button type="submit" disabled={busy || !dirty}
          class="spring rounded-sm bg-neon-red px-3.5 py-1.5 text-xs font-bold uppercase tracking-wider text-app hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-white disabled:opacity-40">{strings.rulesEditor.save}</button>
        <span class="text-xs {failed ? 'text-bad' : 'text-ok'}" role="status">{message}</span>
      </div>
    </form>
  {/if}
</div>
