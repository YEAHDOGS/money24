<script>
  import { onMount, untrack } from 'svelte';
  import Terminal from '@lucide/svelte/icons/terminal';
  import strings from './scribe.json';
  import { ScribeClient, defaultWindow, requestWindow, displayTime, sourceURL, startBlock } from './scribe-client.svelte.js';

  const MAX_TIP_LENGTH = 4000;
  const FIELD_CLASS = 'mt-1 block w-full min-w-0 rounded-sm border border-line bg-card px-2.5 py-2 text-sm text-white focus:border-neon-red focus:outline-none';
  let { runnerUrl, runnerSecret, providers, status } = $props();
  const client = untrack(() => new ScribeClient(runnerUrl, runnerSecret));
  const initial = defaultWindow();
  let tip = $state('');
  let provider = $state('');
  let entry = $state(initial.entry);
  let exit = $state(initial.exit);
  let selectedId = $state('');
  let message = $state('');
  let paperError = $state('');
  const connected = $derived(providers.filter((item) => item.connected));
  const available = $derived(status === 'live' && connected.length > 0);
  const selected = $derived(client.records.find((item) => item.id === selectedId) ?? client.records[0]);
  const blocked = $derived(selected ? startBlock(selected, client.now) : '');
  const unavailable = $derived(status === 'demo' ? strings.demo : status !== 'live' ? strings.offline : strings.noProvider);

  $effect(() => {
    if (status === 'live') client.refresh();
  });
  onMount(() => () => client.stop());

  /** Submit a research request, preserving the tip on errors. */
  async function submit(event) {
    event.preventDefault();
    if (!available || client.busy || !tip.trim()) return;
    message = '';
    try {
      const window = requestWindow(entry, exit);
      const record = await client.submit({ tip: tip.trim(), provider: provider || connected[0].id, ...window });
      if (!record) return;
      selectedId = record.id;
      tip = '';
    } catch (error) {
      message = error instanceof Error ? error.message : strings.failed;
    }
  }

  /** Starts a checked contract on the paper prediction desk; the desk reports why it cannot. */
  async function paper(ticker) {
    paperError = '';
    try {
      await client.paper(selected.id, ticker);
    } catch (error) {
      paperError = error instanceof Error ? error.message : strings.failed;
    }
  }
</script>

<div class="list-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto select-text">
  <form onsubmit={submit} class="border-b border-line p-4 lg:p-5">
    <h2 class="flex items-center gap-2 text-lg font-bold text-white"><Terminal class="size-5 text-neon-red" aria-hidden="true" />{strings.title}</h2>
    <p class="mt-1 max-w-prose text-sm leading-relaxed text-muted">{strings.intro}</p>
    <label for="scribe-tip" class="mt-4 block text-xs font-semibold text-white">{strings.tip}</label>
    <textarea id="scribe-tip" bind:value={tip} rows="3" maxlength={MAX_TIP_LENGTH} required placeholder={strings.placeholder}
      class="{FIELD_CLASS} resize-y placeholder:text-muted" disabled={client.busy}></textarea>
    <label for="scribe-provider" class="mt-3 block text-xs text-muted">{strings.provider}</label>
    <select id="scribe-provider" value={provider || connected[0]?.id || ''} onchange={(event) => { provider = event.currentTarget.value; }} class={FIELD_CLASS} disabled={!available || client.busy}>
      {#if !connected.length}<option value="">{strings.choose}</option>{/if}
      {#each connected as item (item.id)}<option value={item.id}>{item.name}</option>{/each}
    </select>
    <div class="mt-3 grid min-w-0 grid-cols-1 gap-3 @lg:grid-cols-2">
      <label class="min-w-0 text-xs text-muted">{strings.entry}<input type="datetime-local" required bind:value={entry} class={FIELD_CLASS} /></label>
      <label class="min-w-0 text-xs text-muted">{strings.exit}<input type="datetime-local" required bind:value={exit} class={FIELD_CLASS} /></label>
    </div>
    <p class="mt-2 text-xs leading-relaxed text-muted">{strings.timing}</p>
    <button type="submit" disabled={!available || client.busy || !tip.trim()}
      class="spring mt-4 rounded-sm bg-neon-red px-4 py-2.5 text-sm font-bold text-app hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-white disabled:opacity-40">
      {client.busy ? strings.submitting : strings.submit}
    </button>
    <p class="mt-2 text-xs leading-relaxed text-paper">{strings.disclaimer}</p>
    {#if !available}<p class="mt-2 text-xs leading-relaxed text-muted">{unavailable}</p>{/if}
    <p class="mt-2 min-h-4 text-xs text-bad" role="status">{message || client.error}</p>
  </form>

  <section class="border-b border-line p-4 lg:px-5" aria-label={strings.history}>
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-white">{strings.history}</h3>
      <button type="button" onclick={() => client.refresh()} disabled={status !== 'live'} class="spring rounded-sm px-2 py-1 text-xs text-muted hover:text-white focus-visible:outline focus-visible:outline-neon-red disabled:opacity-40">{strings.refresh}</button>
    </div>
    {#if client.records.length}
      <ol class="mt-2 max-h-40 overflow-y-auto divide-y divide-line">
        {#each client.records as record (record.id)}
          <li><button type="button" aria-pressed={selected?.id === record.id} onclick={() => { selectedId = record.id; }}
            class="spring block w-full rounded-sm px-2 py-2 text-left text-sm focus-visible:outline focus-visible:outline-neon-red {selected?.id === record.id ? 'bg-white/5 text-white' : 'text-muted hover:text-white'}">
            <span class="line-clamp-2 break-words">{record.tip}</span><span class="mt-1 block text-xs text-muted">{strings.statuses[record.status] ?? record.status} · {displayTime(record.created_at)}</span>
          </button></li>
        {/each}
      </ol>
    {:else}<p class="mt-2 text-sm text-muted">{strings.empty}</p>{/if}
  </section>

  {#if selected}
    <article class="break-words p-4 lg:p-5" aria-label={strings.candidates}>
      <p class="text-xs font-semibold text-warn">{strings.unverified}</p>
      <h3 class="mt-2 text-base font-semibold text-white">{selected.tip}</h3>
      <dl class="mt-3 grid grid-cols-1 gap-2 text-xs @lg:grid-cols-2">
        <div><dt class="text-muted">{strings.entry}</dt><dd class="mt-1 text-white">{displayTime(selected.expected_entry_at)}</dd></div>
        <div><dt class="text-muted">{strings.exit}</dt><dd class="mt-1 text-white">{displayTime(selected.expected_exit_at)}</dd></div>
      </dl>
      {#if selected.status === 'researching'}<p class="mt-4 text-sm text-paper" role="status">{strings.working}</p>{/if}
      {#if selected.error}<p class="mt-4 text-sm text-bad" role="status">{selected.error}</p>{/if}
      {#if selected.summary}<p class="mt-4 whitespace-pre-line text-sm leading-relaxed text-white">{selected.summary}</p>{/if}
      {#if selected.corrections?.length}
        <h4 class="mt-5 text-sm font-semibold text-white">{strings.corrections}</h4>
        <ul class="mt-2 list-disc space-y-1 pl-4 text-sm text-warn">{#each selected.corrections as correction}<li>{correction}</li>{/each}</ul>
      {/if}
      {#if selected.evidence?.length}
        <h4 class="mt-5 text-sm font-semibold text-white">{strings.evidence}</h4>
        <ul class="mt-2 space-y-2 text-sm text-muted">
          {#each selected.evidence as evidence}
            <li>{evidence.label}
              {#if sourceURL(evidence.url)}<a href={sourceURL(evidence.url)} target="_blank" rel="noopener noreferrer" class="ml-1 text-paper underline underline-offset-2">{evidence.source || evidence.url}</a>{/if}
            </li>
          {/each}
        </ul>
      {/if}
      {#if selected.status === 'ready'}
        <h4 class="mt-5 text-sm font-semibold text-white">{strings.candidates}</h4>
        {#each selected.candidates ?? [] as candidate}
          <section class="mt-3 border-t border-line pt-3">
            <p class="text-xs text-muted">{candidate.verified ? strings.contractFound : strings.contractUnverified} · {candidate.side}</p>
            <h5 class="mt-1 text-sm font-semibold text-white">{candidate.title || candidate.ticker}</h5>
            <p class="mt-2 text-sm leading-relaxed text-muted">{candidate.thesis}</p>
            <p class="mt-2 text-xs text-warn">{candidate.verification_note}</p>
            {#if candidate.quote_at}
              <p class="mt-2 font-mono text-xs text-white">{strings.bid}: {candidate.bid ?? '—'} · {strings.ask}: {candidate.ask ?? '—'}</p>
              <p class="mt-1 text-xs text-muted">{strings.quote}: {displayTime(candidate.quote_at)}</p>
            {/if}
            {#if candidate.close_at}<p class="mt-1 text-xs text-muted">{strings.closes}: {displayTime(candidate.close_at)}</p>{/if}
            {#if candidate.invalidation}<p class="mt-3 text-sm text-white">{strings.invalidation}: {candidate.invalidation}</p>{/if}
            {#if candidate.risks?.length}<ul aria-label={strings.risks} class="mt-2 list-disc space-y-1 pl-4 text-xs text-muted">{#each candidate.risks as risk}<li>{risk}</li>{/each}</ul>{/if}
            {#if sourceURL(candidate.url)}<a href={sourceURL(candidate.url)} target="_blank" rel="noopener noreferrer" class="mt-3 inline-block text-sm text-paper underline underline-offset-2">{strings.open}</a>{/if}
            {#if candidate.paper}
              <p class="mt-3 font-mono text-xs text-ok">{strings.papered}: {candidate.paper.qty} × {candidate.paper.price} · ${(candidate.paper.cost_usd + candidate.paper.fee_usd).toFixed(2)} · {displayTime(candidate.paper.at)}</p>
            {:else if candidate.verified && candidate.quote_at}
              <div class="mt-3 flex min-h-9 flex-wrap items-center gap-x-3 gap-y-1">
                <button type="button" onclick={() => paper(candidate.ticker)} disabled={!!blocked || client.busy || status !== 'live'}
                  class="spring rounded-sm border border-neon-red px-3 py-1.5 text-xs font-bold text-neon-red hover:bg-neon-red/10 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red disabled:opacity-40">{strings.paper}</button>
                <span class="text-xs text-muted">{blocked || strings.paperHint}</span>
              </div>
            {/if}
          </section>
        {:else}<p class="mt-2 text-sm text-muted">{strings.noCandidates}</p>{/each}
        <p class="mt-2 min-h-4 text-xs text-bad" role="alert">{paperError}</p>
      {/if}
    </article>
  {/if}
</div>
