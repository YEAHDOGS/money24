<script>
  /** Network monitor: live throughput, session and month totals, projected bill, and a meter per source. */
  import strings from './strings.json';
  import { formatBytes, formatCost, marginalCost, projectSource } from './net-math.js';

  const SPARK_W = 100;
  const SPARK_H = 28;
  const STATUS_TOO_MANY = 429;
  const PERCENT = 100;

  /** @type {{ client: import('./feed-client.svelte.js').FeedClient, providers: any[], providerId: string, worstCase: boolean, note: string, projection: {requests:number, in:number, out:number, served:number}, cost: number, everyMs: number }} */
  let { client, providers, providerId = $bindable(), worstCase = $bindable(), note, projection, cost, everyMs } = $props();

  const provider = $derived(providers.find((p) => p.id === providerId) ?? providers[0]);
  const totals = $derived(client.totals);
  const sources = $derived(Object.values(client.sources));
  const notModified = $derived(sources.reduce((n, s) => n + s.not_modified, 0));
  const fetchedRequests = $derived(totals?.fetched.requests ?? 0);
  const month = $derived({
    in: totals?.month?._wire?.in ?? 0,
    out: totals?.month?._wire?.out ?? 0,
    served: totals?.month?._served?.out ?? 0,
  });
  const rows = $derived(
    sources
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

  /** @param {any} s */
  function dotClass(s) {
    if (!s.requests) return 'bg-muted/50';
    if (s.last_status === STATUS_TOO_MANY) return 'bg-warn';
    return s.last_error ? 'bg-bad' : 'bg-ok';
  }
</script>

<section class="flex h-full min-h-0 flex-col desktop-portrait:flex-row" aria-labelledby="net-title">
  <div class="flex shrink-0 flex-col border-line desktop-portrait:w-[28rem] desktop-portrait:border-r">
  <header class="flex items-baseline justify-between gap-2 border-b border-line px-4 py-2.5 short:py-1.5">
    <h2 id="net-title" class="text-[0.68rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.network}</h2>
    <span class="font-mono text-[0.7rem] tabular-nums text-muted">{totals?.clients ?? 0} {strings.clients}</span>
  </header>

  <div class="border-b border-line px-4 py-3 short:hidden">
    <div class="flex items-baseline justify-between font-mono text-xs tabular-nums">
      <span class="text-neon-red">↓ {formatBytes(latest?.inRate ?? 0)}/s</span>
      <span class="text-white/70">↑ {formatBytes((latest?.outRate ?? 0) + (latest?.servedRate ?? 0))}/s</span>
    </div>
    <svg class="mt-1.5 h-8 w-full text-neon-red" viewBox="0 0 {SPARK_W} {SPARK_H}" preserveAspectRatio="none" aria-hidden="true">
      <line x1="0" y1={SPARK_H} x2={SPARK_W} y2={SPARK_H} class="stroke-line" vector-effect="non-scaling-stroke" />
      {#if sparkPoints}
        <polyline points={sparkPoints} fill="none" stroke="currentColor" stroke-width="1.5" vector-effect="non-scaling-stroke" stroke-linejoin="round" />
      {/if}
    </svg>
  </div>

  <dl class="grid grid-cols-3 border-b border-line text-center font-mono tabular-nums">
    {#each [
      [strings.session, totals?.wire.in, totals?.wire.out, totals?.served.out],
      [strings.month, month.in, month.out, month.served],
      [strings.projected, projection.in, projection.out, projection.served],
    ] as [label, inBytes, outBytes, served], i (label)}
      <div class="px-2 py-2 {i ? 'border-l border-line' : ''}">
        <dt class="truncate font-sans text-[0.62rem] font-bold uppercase tracking-[0.14em] text-muted">{label}</dt>
        <dd class="mt-1 text-xs"><span class="text-muted">{strings.inbound}</span> {formatBytes(inBytes ?? 0)}</dd>
        <dd class="text-xs"><span class="text-muted">{strings.outbound}</span> {formatBytes(outBytes ?? 0)}</dd>
        <dd class="text-xs"><span class="text-muted">{strings.served}</span> {formatBytes(served ?? 0)}</dd>
      </div>
    {/each}
  </dl>

  <div class="flex flex-col gap-2 border-b border-line px-4 py-3 short:py-2">
    <div class="flex items-center justify-between gap-2 font-mono text-xs tabular-nums">
      <span><span class="text-muted">{strings.requests}</span> {fetchedRequests}</span>
      <span><span class="text-muted">{strings.notModified}</span> {fetchedRequests ? Math.round((notModified / fetchedRequests) * PERCENT) : 0}%</span>
      <span class="text-white"><span class="text-muted">{strings.costMonth}</span> {formatCost(cost)}</span>
    </div>
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
    <p class="text-[0.68rem] leading-snug text-muted short:hidden">{provider.detail} {strings.ingressFree}</p>
  </div>

  </div>

  <div class="flex min-h-0 flex-1 flex-col">
  <ul class="list-scroll min-h-0 flex-1 overflow-y-auto" aria-label={strings.sources}>
    {#each rows as s (s.id)}
      <li class="grid grid-cols-[0.5rem_minmax(0,1fr)_auto] items-center gap-x-2.5 border-b border-line px-4 py-2" title={s.last_error || `HTTP ${s.last_status}`}>
        <span class="size-2 rounded-full {dotClass(s)}" aria-hidden="true"></span>
        <div class="min-w-0">
          <p class="truncate text-xs font-semibold">{s.label}</p>
          <p class="truncate font-mono text-[0.68rem] tabular-nums text-muted">
            {formatBytes(s.avg_in)}/req · {s.requests} req · {s.requests ? Math.round((s.not_modified / s.requests) * PERCENT) : 0}% 304
          </p>
        </div>
        <div class="text-right font-mono text-[0.68rem] tabular-nums">
          <p class="text-white/90">{formatBytes(s.month.in)}/mo</p>
          <p class="text-muted">{formatCost(s.perRequest)}/{strings.perRequest}</p>
        </div>
      </li>
    {/each}
  </ul>
  <p class="border-t border-line px-4 py-1.5 text-[0.62rem] leading-snug text-muted short:hidden">{note}</p>
  </div>
</section>
