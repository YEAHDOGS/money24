<script>
  /**
   * One source's record: how far it can be trusted, and every entry it called with what the
   * market did afterwards. Opened by picking a source in the rail.
   */
  import strings from './strings.json';
  import { contractLabel, priceLabel } from './trading-client.svelte.js';

  const T = strings.source;
  const X_PREFIX = 'x:';
  const PERCENT = 100;
  const HALF = 50; // the range bar's zero line sits in the middle
  const MIN_SPAN_PCT = 5; // the smallest move the range bars are scaled to, so tiny moves stay tiny
  const SMALL_SAMPLE = 20; // fewer scored calls than this and the win rate is mostly luck
  const DECIMALS = 1;
  const PLUS = '+';

  /** @type {{ id: string, source?: any, client: import('./trading-client.svelte.js').TradingClient, ranked: number, onLoaded: () => void }} */
  let { id, source, client, ranked, onLoaded } = $props();

  /** @type {{trust: any, outcomes: any[]}|null} */
  let record = $state(null);
  let busy = $state(false);
  let message = $state('');
  let failed = $state(false);

  async function read(/** @type {string} */ sourceId) {
    try {
      const got = await client.sourceRecord(sourceId);
      if (sourceId === id) record = got;
    } catch {
      if (sourceId === id) [message, failed] = [T.offline, true];
    }
  }

  // Another source: drop the old record at once, so stale numbers never sit under a new name.
  $effect(() => {
    record = null;
    message = '';
    failed = false;
    read(id);
  });

  async function load() {
    busy = true;
    message = '';
    try {
      const { added } = await client.loadHistory(id);
      [message, failed] = [T.loaded.replace('{n}', added), false];
      await read(id);
      onLoaded();
    } catch (err) {
      [message, failed] = [err instanceof Error ? err.message : String(err), true];
    } finally {
      busy = false;
    }
  }

  /** "+11.8%" or "-1.6%". */
  function pct(/** @type {number} */ n) {
    return `${n > 0 ? PLUS : ''}${n.toFixed(DECIMALS)}%`;
  }

  const trust = $derived(record?.trust);
  const outcomes = $derived(record?.outcomes ?? []);
  /** The largest move in either direction across all calls: every range bar shares this scale. */
  const span = $derived(Math.max(MIN_SPAN_PCT, ...outcomes.filter((o) => o.scored).flatMap((o) => [Math.abs(o.peak_pct), Math.abs(o.trough_pct)])));
  /** Where a percent move sits on a range bar, 0 to 100 from left to right. */
  const place = (/** @type {number} */ move) => HALF + (Math.max(-span, Math.min(span, move)) / span) * HALF;

  function verdict(/** @type {any} */ o) {
    if (!o.scored) return { label: T.unpriced, classes: 'text-muted border-line' };
    if (o.open) return { label: T.open, classes: 'text-warn border-warn' };
    return o.win ? { label: T.win, classes: 'text-ok border-ok' } : { label: T.loss, classes: 'text-bad border-bad' };
  }
</script>

<div class="px-4 py-4 lg:px-6">
  <div class="flex items-baseline justify-between gap-3">
    <h2 class="truncate font-mono text-xl font-bold text-white">{source?.handle ? `@${source.handle}` : id}</h2>
    <span class="shrink-0 font-mono text-xs tabular-nums text-muted">{trust?.rank ? T.rank.replace('{n}', trust.rank).replace('{of}', ranked) : T.unranked}</span>
  </div>
  {#if source?.note}<p class="mt-1 text-xs text-muted">{source.note}</p>{/if}

  <!-- The numbers keep their place while a record loads: dashes, then figures. -->
  <div class="mt-4 flex items-end justify-between gap-3">
    <p>
      <span class="block text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{T.trust}</span>
      <span class="font-mono text-4xl leading-none font-semibold tabular-nums text-white">{trust?.scored ? trust.trust_pct.toFixed(0) : '—'}</span>
      <span class="text-sm text-muted">{T.outOf}</span>
    </p>
    <p class="text-right">
      <span class="block text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{T.winRate}</span>
      <span class="font-mono text-2xl leading-none font-semibold tabular-nums {trust?.scored ? 'text-white' : 'text-muted'}">{trust?.scored ? `${trust.win_rate_pct.toFixed(0)}%` : '—'}</span>
      <span class="block font-mono text-xs tabular-nums text-muted">{trust?.wins ?? 0} / {trust?.scored ?? 0}</span>
    </p>
  </div>
  <div class="mt-2 h-1 overflow-hidden rounded-full bg-white/8" role="img" aria-label="{T.trust} {trust?.trust_pct?.toFixed(0) ?? 0} {T.outOf}">
    <div class="h-full origin-left bg-neon-red transition-transform duration-500 ease-out" style:transform="scaleX({(trust?.trust_pct ?? 0) / PERCENT})"></div>
  </div>
  {#if trust && trust.scored < SMALL_SAMPLE}
    <p class="mt-2 text-xs text-warn">{T.smallSample.replace('{n}', trust.scored)}</p>
  {/if}

  <dl class="mt-4 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
    <dt class="text-muted">{T.posts}</dt>
    <dd class="text-right font-mono tabular-nums">{trust?.posts ?? '—'}</dd>
    <dt class="text-muted">{T.callouts}</dt>
    <dd class="text-right font-mono tabular-nums">{trust?.callouts ?? '—'}</dd>
    <dt class="text-muted">{T.exits}</dt>
    <dd class="text-right font-mono tabular-nums">{trust?.exits ?? '—'}</dd>
    <dt class="text-muted">{T.avgPeak}</dt>
    <dd class="text-right font-mono tabular-nums text-ok">{trust?.scored ? pct(trust.avg_peak_pct) : '—'}</dd>
    <dt class="text-muted">{T.avgEnd}</dt>
    <dd class="text-right font-mono tabular-nums {trust?.avg_end_pct < 0 ? 'text-bad' : 'text-ok'}">{trust?.scored ? pct(trust.avg_end_pct) : '—'}</dd>
    <dt class="text-muted">{T.avgTrough}</dt>
    <dd class="text-right font-mono tabular-nums text-bad">{trust?.scored ? pct(trust.avg_trough_pct) : '—'}</dd>
    {#if trust?.open || trust?.unpriced}
      <dt class="text-muted">{T.notCounted}</dt>
      <dd class="text-right font-mono tabular-nums text-muted">{trust.open} {T.open.toLowerCase()} · {trust.unpriced} {T.unpriced.toLowerCase()}</dd>
    {/if}
  </dl>

  {#if id.startsWith(X_PREFIX)}
    <button type="button" disabled={busy} onclick={load}
      class="spring mt-4 w-full rounded-sm border border-line py-2 text-xs font-bold uppercase tracking-wider text-white/80 hover:bg-white/5 focus-visible:outline focus-visible:outline-1 focus-visible:outline-white disabled:opacity-40">
      {busy ? T.loading : T.load}
    </button>
    <p class="mt-1.5 text-xs text-muted">{T.loadHint}</p>
  {/if}
  <p class="mt-1 min-h-4 text-xs {failed ? 'text-bad' : 'text-ok'}" role="status">{message}</p>

  <h3 class="mt-4 border-t border-line pt-4 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{T.history} · {outcomes.length}</h3>
  <ol class="mt-1 divide-y divide-line">
    {#each outcomes as o (o.signal.post_id || o.signal.raw)}
      {@const v = verdict(o)}
      <li class="py-3">
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
          <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold uppercase tracking-wider {v.classes}">{v.label}</span>
          <span class="font-mono font-semibold text-white">{contractLabel(o.signal)}</span>
          <time class="text-muted" datetime={o.at}>{new Date(o.at).toLocaleDateString([], { year: 'numeric', month: 'short', day: 'numeric' })}</time>
          {#if o.scored}
            <span class="ml-auto font-mono font-bold tabular-nums {o.result_pct < 0 ? 'text-bad' : 'text-ok'}">{pct(o.result_pct)}</span>
          {/if}
        </div>
        {#if o.scored}
          <!-- Worst to best over the window, on one scale for every call. The tick is where it ended, or the source's exit. -->
          <div class="relative mt-2 h-2 rounded-full bg-white/6" role="img"
            aria-label="{T.low} {pct(o.trough_pct)}, {T.high} {pct(o.peak_pct)}, {o.has_exit ? T.exit : T.end} {pct(o.has_exit ? o.exit_pct : o.end_pct)}">
            <span class="absolute inset-y-0 rounded-full bg-bad/70" style:left="{place(o.trough_pct)}%" style:right="{PERCENT - place(Math.min(0, o.peak_pct))}%"></span>
            <span class="absolute inset-y-0 rounded-full bg-ok/70" style:left="{place(Math.max(0, o.trough_pct))}%" style:right="{PERCENT - place(o.peak_pct)}%"></span>
            <span class="absolute inset-y-0 left-1/2 w-px bg-white/40"></span>
            <span class="absolute -inset-y-1 w-0.5 rounded-full bg-white" style:left="{place(o.has_exit ? o.exit_pct : o.end_pct)}%"></span>
          </div>
          <p class="mt-1.5 flex flex-wrap gap-x-3 font-mono text-[0.7rem] tabular-nums text-muted">
            <span>{T.entry} {priceLabel(o.entry)}</span>
            <span>{T.low} <span class="text-bad">{pct(o.trough_pct)}</span></span>
            <span>{T.high} <span class="text-ok">{pct(o.peak_pct)}</span></span>
            <span>{o.has_exit ? T.exit : T.end} <span class="text-white/80">{pct(o.has_exit ? o.exit_pct : o.end_pct)}</span></span>
          </p>
        {:else}
          <p class="mt-1 text-xs text-muted">{o.note}</p>
        {/if}
        <p class="mt-1 text-[0.8rem] leading-snug break-words text-white/55">
          {o.signal.raw}
          {#if o.signal.url}<a href={o.signal.url} target="_blank" rel="noopener noreferrer" class="spring ml-1 text-white/70 underline underline-offset-4 hover:text-white">{strings.openPost}</a>{/if}
        </p>
      </li>
    {:else}
      <li class="py-6 text-sm text-muted">{record ? T.empty : T.reading}</li>
    {/each}
  </ol>

  <p class="mt-3 border-t border-line pt-3 text-xs leading-relaxed text-muted">{T.method}</p>
</div>
