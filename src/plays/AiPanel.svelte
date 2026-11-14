<script>
  /**
   * AI reads of a play, newest first. A model reads the play, the market, the report and tagged
   * headlines as data, with no tools and no web, and answers in a fixed shape: a verdict, what must
   * happen, the risks, evidence (added unverified) and an optional better bracket. Applying the
   * bracket is the user's call, on drafts only.
   */
  import strings from './strings.json';
  import { clock, price, num } from './play-format.js';
  import LoaderCircle from '@lucide/svelte/icons/loader-circle';
  import CircleX from '@lucide/svelte/icons/circle-x';

  /** @type {{ play: any, draft: boolean, onSave: (play: any) => Promise<void> }} */
  let { play, draft, onSave } = $props();

  const VERDICT_CLASS = { take: 'border-ok text-ok', wait: 'border-warn text-warn', pass: 'border-bad text-bad' };
  const PERCENT = 100;

  let applied = $state(-1);

  const reads = $derived([...(play.analyses ?? [])].map((a, i) => ({ a, i })).reverse());

  /** The suggestion merged into the play; quantities follow the suggested ladder and runner. */
  async function apply(/** @type {any} */ s, /** @type {number} */ i) {
    const ladder = (s.ladder ?? []).filter((/** @type {any} */ r) => num(r.qty) > 0);
    const runner = { qty: num(s.runner_qty), target: num(s.runner_target) };
    const next = { ...play, ladder, runner, qty: ladder.reduce((/** @type {number} */ t, /** @type {any} */ r) => t + r.qty, runner.qty) };
    if (s.entry > 0) next.entry = s.entry;
    if (s.stop > 0) next.stop = s.stop;
    if (s.exit_by && Date.parse(s.exit_by) > 0) next.failsafes = { ...play.failsafes, exit_by: s.exit_by };
    await onSave(next);
    applied = i;
  }
</script>

{#if play.analyzing}
  <p class="mt-2 flex items-center gap-2 text-xs text-kind-insider" role="status">
    <LoaderCircle class="spin size-3.5" aria-hidden="true" />{strings.detail.analyzing} · {play.analyzing}
  </p>
{/if}

<ol class="mt-2 grid gap-4">
  {#each reads as { a, i } (i)}
    <li class="border-l border-kind-insider/40 pl-3 text-xs leading-snug">
      <p class="flex flex-wrap items-center gap-2 text-[0.65rem] text-muted">
        <span class="font-mono">{clock(a.at)} · {a.provider}{a.model ? ` · ${a.model}` : ''}</span>
        {#if a.verdict}<span class="rounded-sm border px-1.5 py-px font-bold uppercase tracking-wider {VERDICT_CLASS[a.verdict] ?? 'border-line text-white'}">{strings.ai.verdicts[a.verdict] ?? a.verdict}</span>{/if}
        {#if a.confidence}<span class="tabular-nums">{strings.ai.confidence} {Math.round(a.confidence * PERCENT)}%</span>{/if}
      </p>
      {#if a.error}
        <p class="mt-1 flex items-start gap-1 text-bad"><CircleX class="mt-px size-3.5 shrink-0" aria-hidden="true" />{strings.ai.error}: {a.error}</p>
      {:else}
        {#if a.summary}<p class="mt-1 text-sm text-white">{a.summary}</p>{/if}
        {#if a.must_happen}<p class="mt-1.5"><span class="font-bold text-white/80">{strings.ai.mustHappen}:</span> <span class="text-white/75">{a.must_happen}</span></p>{/if}
        {#if a.thesis}<p class="mt-1.5"><span class="font-bold text-white/80">{strings.ai.thesis}:</span> <span class="text-white/75">{a.thesis}</span></p>{/if}
        {#if a.risks?.length}
          <p class="mt-1.5 font-bold text-white/80">{strings.ai.risks}</p>
          <ul class="mt-0.5 grid list-disc gap-0.5 pl-4 text-white/70">{#each a.risks as r, k (k)}<li>{r}</li>{/each}</ul>
        {/if}
        {#if a.evidence?.length}<p class="mt-1.5 text-kind-insider">{a.evidence.length} {strings.ai.evidence}</p>{/if}
        {#if a.suggestion && (a.suggestion.entry || a.suggestion.ladder?.length)}
          {@const s = a.suggestion}
          <p class="mt-1.5 font-mono tabular-nums text-white/70">
            <span class="font-sans font-bold text-white/80">{strings.ai.suggestion}:</span>
            {s.entry ? `${strings.bracket.entry} ${price(s.entry)}` : ''}
            {s.stop ? ` · ${strings.bracket.stop} ${price(s.stop)}` : ''}
            {(s.ladder ?? []).map((/** @type {any} */ r, /** @type {number} */ k) => ` · T${k + 1} ${r.qty}@${price(r.price)}`).join('')}
            {s.runner_qty ? ` · ${strings.bracket.runner} ${s.runner_qty}@${price(s.runner_target)}` : ''}
          </p>
          {#if draft}
            <button type="button" onclick={() => apply(s, i)}
              class="spring mt-1.5 rounded-sm border border-kind-insider/60 px-2.5 py-1 text-[0.68rem] font-bold uppercase tracking-wider text-kind-insider hover:bg-kind-insider/10 focus-visible:bg-kind-insider/10 focus-visible:outline-none">
              {applied === i ? strings.ai.applied : strings.ai.apply}
            </button>
          {/if}
        {/if}
      {/if}
    </li>
  {:else}
    {#if !play.analyzing}<li class="text-xs text-muted">{strings.ai.none}</li>{/if}
  {/each}
</ol>
