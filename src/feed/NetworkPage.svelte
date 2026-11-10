<script>
  /**
   * Network page: what every round and every request costs, and exactly which URLs are hit.
   * Phones: stat cards in two columns, one card per source. Tablet and up: a full table.
   */
  import strings from './strings.json';
  import { formatBytes, formatCost, formatInterval, marginalCost, projectSource } from './net-math.js';

  const PERCENT = 100;
  const STATUS_TOO_MANY = 429;
  const STATUS_NOT_MODIFIED = 304;
  const GROUPS = ['all', 'news', 'calendar'];
  const SPARK_W = 100;
  const SPARK_H = 28;
  const MIN_BAR = 0.04;
  const ROUND_BARS = 60;

  /** @type {{ client: import('./feed-client.svelte.js').FeedClient, providers: any[], providerId: string, worstCase: boolean, note: string, projection: any, cost: number, everyMs: number, linkTarget: string|undefined }} */
  let { client, providers, providerId = $bindable(), worstCase = $bindable(), note, projection, cost, everyMs, linkTarget } = $props();

  let group = $state('all');

  const provider = $derived(providers.find((p) => p.id === providerId) ?? providers[0]);
  const totals = $derived(client.totals);
  const all = $derived(Object.values(client.sources));
  const newsCount = $derived(all.filter((s) => s.group !== 'calendar').length);
  const fetched = $derived(totals?.fetched ?? { requests: 0, in: 0, out: 0 });
  const notModified = $derived(all.reduce((n, s) => n + s.not_modified, 0));
  const report = $derived(client.report);
  const rounds = $derived(client.cycles.slice(0, ROUND_BARS).toReversed?.() ?? [...client.cycles.slice(0, ROUND_BARS)].reverse());
  const roundPeak = $derived(Math.max(1, ...rounds.map((r) => r.bytes_in)));
  const avgRound = $derived(client.cycles.length ? client.cycles.reduce((n, r) => n + r.bytes_in, 0) / client.cycles.length : 0);
  const month = $derived({
    in: totals?.month?._wire?.in ?? 0,
    out: totals?.month?._wire?.out ?? 0,
    served: totals?.month?._served?.out ?? 0,
  });
  const rows = $derived(
    all
      .filter((s) => group === 'all' || (group === 'calendar') === (s.group === 'calendar'))
      .map((s) => ({ ...s, month: projectSource(s, everyMs), perRequest: marginalCost(s.avg_in, s.avg_out, provider, worstCase) }))
      .sort((a, b) => b.month.in - a.month.in),
  );
  const latest = $derived(client.history.at(-1));
  const sparkPoints = $derived.by(() => {
    const h = client.history;
    if (h.length < 2) return '';
    const peak = Math.max(1, ...h.map((p) => p.inRate));
    return h.map((p, i) => `${(i / (h.length - 1)) * SPARK_W},${SPARK_H - (p.inRate / peak) * SPARK_H}`).join(' ');
  });
  const cards = $derived([
    [strings.net.perRequest, formatBytes(fetched.requests ? fetched.in / fetched.requests : 0), `${formatBytes(fetched.requests ? fetched.out / fetched.requests : 0)} ${strings.net.sent}`],
    [strings.net.perRound, formatBytes(avgRound), `${client.cycles.length} ${strings.net.rounds}`],
    [strings.net.sourcesHit, report ? `${report.started} / ${newsCount}` : '–', report ? `${report.answered} ${strings.net.answered} · ${report.errors} ${strings.ring.failed}` : ''],
    [strings.requests, String(fetched.requests), `${fetched.requests ? Math.round((notModified / fetched.requests) * PERCENT) : 0}% ${strings.net.notModifiedShort}`],
    [strings.session, formatBytes(totals?.wire.in ?? 0), `${formatBytes(totals?.wire.out ?? 0)} ${strings.outbound} · ${formatBytes(totals?.served.out ?? 0)} ${strings.served}`],
    [strings.month, formatBytes(month.in), `${formatBytes(month.out)} ${strings.outbound} · ${formatBytes(month.served)} ${strings.served}`],
    [strings.projected, formatBytes(projection.in), `${formatBytes(projection.out + projection.served)} ${strings.outbound} ${strings.net.atInterval} ${formatInterval(everyMs)}`],
    [strings.costMonth, formatCost(cost), provider.name],
  ]);

  /** @param {any} s */
  function dotClass(s) {
    if (!s.requests) return 'bg-muted/50';
    if (s.last_status === STATUS_TOO_MANY) return 'bg-warn';
    return s.last_error ? 'bg-bad' : 'bg-ok';
  }
</script>

<div class="list-scroll mx-auto h-full w-full max-w-7xl overflow-y-auto xl:border-x xl:border-line ultrawide:max-w-none">
  <!-- STAT CARDS: 1px grid lines between cells -->
  <dl class="grid grid-cols-2 gap-px border-b border-line bg-line sm:grid-cols-4 2xl:grid-cols-8 ultrawide:grid-cols-8">
    {#each cards as [label, value, sub] (label)}
      <div class="bg-app px-4 py-3 short:py-2">
        <dt class="truncate text-[0.62rem] font-bold uppercase tracking-[0.16em] text-muted">{label}</dt>
        <dd class="mt-1 truncate font-mono text-lg font-bold tabular-nums">{value}</dd>
        <dd class="truncate font-mono text-[0.65rem] tabular-nums text-muted">{sub}</dd>
      </div>
    {/each}
  </dl>

  <!-- ROUNDS, RATE AND HOSTING -->
  <div class="grid grid-cols-1 gap-px border-b border-line bg-line md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
    <div class="bg-app px-4 py-3">
      <div class="flex items-baseline justify-between gap-2">
        <h2 class="text-[0.62rem] font-bold uppercase tracking-[0.16em] text-muted">{strings.net.roundsTitle}</h2>
        <span class="font-mono text-[0.65rem] tabular-nums text-muted">↓ {formatBytes(latest?.inRate ?? 0)}/s · ↑ {formatBytes((latest?.outRate ?? 0) + (latest?.servedRate ?? 0))}/s</span>
      </div>
      <!-- One bar per round, newest on the right. Bars grow with scaleY, so the row never reflows. -->
      <div class="mt-2 flex h-16 items-end gap-px" role="img" aria-label={strings.net.roundsTitle}>
        {#each rounds as r (r.at)}
          <div
            class="h-full flex-1 origin-bottom rounded-t-sm transition-transform duration-300 {r.added > 0 ? 'bg-neon-red' : 'bg-white/25'}"
            style:transform="scaleY({Math.max(MIN_BAR, r.bytes_in / roundPeak)})"
            title={`${new Date(r.at).toLocaleTimeString()} · ${formatBytes(r.bytes_in)} · ${r.started} ${strings.ring.sources} · +${r.added}`}
          ></div>
        {:else}
          <p class="self-center text-xs text-muted">{strings.ring.waiting}</p>
        {/each}
      </div>
      <svg class="mt-2 h-6 w-full text-neon-red short:hidden" viewBox="0 0 {SPARK_W} {SPARK_H}" preserveAspectRatio="none" aria-hidden="true">
        {#if sparkPoints}
          <polyline points={sparkPoints} fill="none" stroke="currentColor" stroke-width="1.5" vector-effect="non-scaling-stroke" stroke-linejoin="round" />
        {/if}
      </svg>
    </div>

    <div class="flex flex-col gap-2 bg-app px-4 py-3">
      <label class="flex items-center gap-2 text-xs">
        <span class="shrink-0 text-muted">{strings.provider}</span>
        <select bind:value={providerId} class="spring min-w-0 flex-1 rounded-sm border border-line bg-card px-2 py-1 text-white focus-visible:outline focus-visible:outline-neon-red">
          {#each providers as p (p.id)}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
      </label>
      <label class="flex items-start gap-2 text-[0.7rem] leading-snug text-white/80">
        <input type="checkbox" bind:checked={worstCase} class="mt-0.5 accent-neon-red" />
        <span>{strings.worstCase}</span>
      </label>
      <p class="text-[0.68rem] leading-snug text-muted">{provider.detail} {strings.ingressFree}</p>
      <p class="text-[0.62rem] leading-snug text-muted/80">{note}</p>
    </div>
  </div>

  <!-- SOURCES AND URLS -->
  <div class="flex items-center gap-1.5 border-b border-line px-4 py-2" role="group" aria-label={strings.net.groups}>
    {#each GROUPS as g (g)}
      <button type="button" onclick={() => { group = g; }} aria-pressed={group === g}
        class="spring rounded-sm border px-2 py-0.5 text-[0.7rem] font-bold uppercase tracking-wider {group === g ? 'border-neon-red bg-white/5 text-neon-red' : 'border-line text-white/55'}">
        {strings.net.group[g]}
      </button>
    {/each}
    <span class="ml-auto font-mono text-[0.65rem] tabular-nums text-muted">{rows.length} {strings.ring.sources}</span>
  </div>

  <div role="table" aria-label={strings.net.urlsTitle} class="text-xs">
    <div role="row" class="sticky top-0 z-10 hidden grid-cols-[11rem_minmax(0,1fr)_3.5rem_3.5rem_3.5rem_4.5rem_4.5rem_3.5rem_5rem_6rem] gap-x-3 border-b border-line bg-app/95 px-4 py-2 text-[0.6rem] font-bold uppercase tracking-[0.14em] text-muted md:grid">
      {#each strings.net.columns as c (c)}<span role="columnheader" class="truncate">{c}</span>{/each}
    </div>
    {#each rows as s (s.id)}
      <div role="row" class="grid grid-cols-[0.5rem_minmax(0,1fr)_auto] gap-x-2.5 gap-y-1 border-b border-line px-4 py-2.5 md:grid-cols-[11rem_minmax(0,1fr)_3.5rem_3.5rem_3.5rem_4.5rem_4.5rem_3.5rem_5rem_6rem] md:gap-x-3 md:py-2" title={s.last_error || ''}>
        <span role="cell" class="flex min-w-0 items-center gap-2 max-md:contents">
          <span class="size-2 shrink-0 rounded-full {dotClass(s)}" aria-hidden="true"></span>
          <span class="truncate font-semibold">{s.label}</span>
        </span>
        <a role="cell" href={s.last_url || s.url} target={linkTarget} rel="noopener noreferrer"
          class="spring break-all font-mono text-[0.68rem] leading-snug text-white/70 hover:text-white hover:underline max-md:col-span-3 max-md:row-start-2">{s.last_url || s.url}</a>
        <span role="cell" class="font-mono tabular-nums max-md:row-start-1 max-md:col-start-3 {s.last_status === STATUS_NOT_MODIFIED ? 'text-white/60' : s.last_error ? 'text-bad' : 'text-ok'}">{s.last_status || '–'}</span>
        <span role="cell" class="font-mono tabular-nums text-muted max-md:hidden">{formatInterval(Math.max(everyMs, s.min_every_ms || 0))}</span>
        <span role="cell" class="font-mono tabular-nums max-md:hidden">{s.requests}</span>
        <span role="cell" class="font-mono tabular-nums max-md:hidden">{formatBytes(s.avg_in)}</span>
        <span role="cell" class="font-mono tabular-nums max-md:hidden">{formatBytes(s.last_in)}</span>
        <span role="cell" class="font-mono tabular-nums text-muted max-md:hidden">{s.requests ? Math.round((s.not_modified / s.requests) * PERCENT) : 0}%</span>
        <span role="cell" class="font-mono tabular-nums max-md:hidden">{formatBytes(s.month.in)}</span>
        <span role="cell" class="font-mono tabular-nums text-muted max-md:hidden">{formatCost(s.perRequest)}</span>
        <span class="font-mono text-[0.65rem] tabular-nums text-muted md:hidden max-md:col-span-3" aria-hidden="true">
          {s.requests} req · {formatBytes(s.avg_in)}/req · {formatBytes(s.month.in)}/mo · {formatCost(s.perRequest)}/req
        </span>
      </div>
    {/each}
  </div>
</div>
