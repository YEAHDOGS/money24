<script>
  /**
   * The evidence that other money is in this trade, and the facts around it. Market data fills the
   * first lines on its own; anything an AI model found stays unweighed until a person verifies it.
   * On a draft, lines can be added, verified and removed.
   */
  import strings from './strings.json';
  import { num } from './play-format.js';
  import BadgeCheck from '@lucide/svelte/icons/badge-check';
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import Zap from '@lucide/svelte/icons/zap';

  /** @type {{ play: any, draft: boolean, onSave: (play: any) => Promise<void> }} */
  let { play, draft, onSave } = $props();

  const KINDS = ['volume', 'open_interest', 'onchain', 'news', 'signal', 'technical'];
  const STANCES = ['for', 'against', 'neutral'];
  const STANCE_MARK = { for: '▲', against: '▼', neutral: '•' };
  const STANCE_TONE = { for: 'text-ok', against: 'text-bad', neutral: 'text-muted' };
  const INPUT = 'min-w-0 rounded-sm border border-line bg-card px-2 py-1 text-xs text-white placeholder:text-muted focus:border-neon-red focus:outline-none';
  const BLANK = { kind: 'news', label: '', stance: 'for', url: '', source: '', source_count: 1, trust_pct: 0, verified: false, breaking: false };

  let adding = $state(false);
  let form = $state({ ...BLANK });

  async function add(/** @type {SubmitEvent} */ event) {
    event.preventDefault();
    if (!form.label.trim()) return;
    const line = { ...form, label: form.label.trim(), source_count: num(form.source_count), trust_pct: num(form.trust_pct), at: new Date().toISOString() };
    await onSave({ ...play, evidence: [...(play.evidence ?? []), line] });
    form = { ...BLANK };
    adding = false;
  }
</script>

<ul class="mt-2 grid gap-1.5">
  {#each play.evidence ?? [] as e, i (i)}
    <li class="grid grid-cols-[1rem_minmax(0,1fr)_auto] gap-x-2 text-xs leading-snug">
      <span class="font-mono {STANCE_TONE[e.stance] ?? 'text-muted'}" title={strings.evidence.stances[e.stance]}>{STANCE_MARK[e.stance] ?? '•'}</span>
      <span class="min-w-0">
        <span class="text-white">{e.label}</span>
        <span class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[0.65rem] text-muted">
          <span class="uppercase tracking-wider">{strings.evidence.kinds[e.kind] ?? e.kind}</span>
          {#if e.auto}<span class="flex items-center gap-0.5"><Zap class="size-2.5" aria-hidden="true" />{strings.evidence.auto}</span>{/if}
          {#if e.verified}<span class="flex items-center gap-0.5 text-ok"><BadgeCheck class="size-3" aria-hidden="true" />{strings.evidence.verified}</span>{/if}
          {#if e.ai && !e.verified}<span class="flex items-center gap-0.5 text-kind-insider"><Sparkles class="size-3" aria-hidden="true" />{strings.evidence.ai}</span>{/if}
          {#if e.breaking}<span class="font-bold uppercase text-warn">{strings.evidence.breaking}</span>{/if}
          {#if e.source_count}<span class="tabular-nums">{e.source_count} {strings.evidence.sources}</span>{/if}
          {#if e.trust_pct}<span class="tabular-nums">{strings.evidence.trust} {Math.round(e.trust_pct)}</span>{/if}
          {#if e.source}<span>{e.source}</span>{/if}
          {#if e.url}<a href={e.url} target="_blank" rel="noopener noreferrer" class="spring flex items-center gap-0.5 text-white/70 hover:text-white"><ExternalLink class="size-3" aria-hidden="true" />{strings.evidence.link}</a>{/if}
        </span>
      </span>
      {#if draft && !e.auto}
        <span class="flex items-start gap-1">
          <button type="button" onclick={() => onSave({ ...play, evidence: play.evidence.map((/** @type {any} */ x, /** @type {number} */ j) => (j === i ? { ...x, verified: !x.verified } : x)) })}
            aria-label={e.verified ? strings.evidence.unverify : strings.evidence.verify} title={e.verified ? strings.evidence.unverify : strings.evidence.verify}
            class="spring grid size-6 place-items-center rounded-sm border border-line {e.verified ? 'text-ok' : 'text-white/50'} hover:border-white/40 focus-visible:border-neon-red focus-visible:outline-none">
            <BadgeCheck class="size-3.5" aria-hidden="true" />
          </button>
          <button type="button" onclick={() => onSave({ ...play, evidence: play.evidence.filter((/** @type {any} */ _, /** @type {number} */ j) => j !== i) })} aria-label={strings.evidence.remove} title={strings.evidence.remove}
            class="spring grid size-6 place-items-center rounded-sm border border-line text-white/50 hover:border-bad hover:text-bad focus-visible:border-bad focus-visible:outline-none">
            <X class="size-3.5" aria-hidden="true" />
          </button>
        </span>
      {/if}
    </li>
  {:else}
    <li class="text-xs text-muted">{strings.evidence.empty}</li>
  {/each}
</ul>

{#if draft}
  {#if adding}
    <form class="mt-3 grid gap-2 rounded-sm border border-line p-2.5 @lg:grid-cols-2" onsubmit={add}>
      <input bind:value={form.label} placeholder={strings.evidence.label} aria-label={strings.evidence.label} class="{INPUT} @lg:col-span-2" required />
      <select bind:value={form.kind} aria-label={strings.evidence.kind} class={INPUT}>
        {#each KINDS as k (k)}<option value={k}>{strings.evidence.kinds[k]}</option>{/each}
      </select>
      <select bind:value={form.stance} aria-label={strings.evidence.stance} class={INPUT}>
        {#each STANCES as s (s)}<option value={s}>{strings.evidence.stances[s]}</option>{/each}
      </select>
      <input bind:value={form.url} type="url" placeholder={strings.evidence.url} aria-label={strings.evidence.url} class={INPUT} />
      <input bind:value={form.source} placeholder={strings.evidence.source} aria-label={strings.evidence.source} class={INPUT} />
      <label class="flex items-center gap-2 text-xs text-muted">{strings.evidence.sourceCount}
        <input bind:value={form.source_count} type="number" min="0" step="1" class="{INPUT} w-16 tabular-nums" /></label>
      <label class="flex items-center gap-2 text-xs text-muted">{strings.evidence.trustPct}
        <input bind:value={form.trust_pct} type="number" min="0" max="100" step="1" class="{INPUT} w-16 tabular-nums" /></label>
      <label class="flex items-center gap-1.5 text-xs text-white/80"><input type="checkbox" bind:checked={form.verified} class="accent-neon-red" />{strings.evidence.verified}</label>
      <label class="flex items-center gap-1.5 text-xs text-white/80"><input type="checkbox" bind:checked={form.breaking} class="accent-neon-red" />{strings.evidence.breaking}</label>
      <div class="flex gap-2 @lg:col-span-2">
        <button type="submit" class="spring rounded-sm border border-neon-red px-3 py-1 text-xs font-bold uppercase tracking-wider text-neon-red hover:bg-neon-red/10 focus-visible:bg-neon-red/10 focus-visible:outline-none">{strings.evidence.save}</button>
        <button type="button" onclick={() => { adding = false; }} class="spring rounded-sm px-3 py-1 text-xs text-muted hover:text-white focus-visible:text-white focus-visible:outline-none">✕</button>
      </div>
    </form>
  {:else}
    <button type="button" onclick={() => { adding = true; }}
      class="spring mt-3 flex items-center gap-1 rounded-sm border border-line px-2.5 py-1 text-xs text-white/70 hover:border-white/40 hover:text-white focus-visible:border-neon-red focus-visible:outline-none">
      <Plus class="size-3.5" aria-hidden="true" />{strings.evidence.add}
    </button>
  {/if}
{/if}
