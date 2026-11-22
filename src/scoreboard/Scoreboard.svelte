<script>
  import { onMount } from 'svelte';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import AppHeader from '../lib/AppHeader.svelte';
  import AppFooter from '../lib/AppFooter.svelte';
  import { demoFirst, DEMO_STATUS } from '../lib/demo.js';
  import { fill, pnl } from '../plays/play-format.js';
  import { date, rate, readScoreboard, sortRows, SORT_DEFAULT, VERDICT_TONE } from './model.js';
  import strings from './strings.json';

  const PAGE = 'scoreboard';
  const CONNECTING = 'connecting';
  const LIVE = 'live';
  const OFFLINE = 'offline';
  const UNKNOWN = 'unknown';
  const STAGGER_MAX = 12;
  const LABEL = 'block text-xs font-semibold text-muted tv:text-base';
  const CONTROL = 'spring min-h-10 w-full min-w-0 rounded-sm border border-line bg-card px-2 text-sm text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:min-h-12 tv:text-lg';
  const VALUE = 'font-mono text-sm font-semibold tabular-nums break-words tv:text-lg';
  const SORT_OPTIONS = Object.entries(strings.sorts);

  let rows = $state.raw(null);
  let sort = $state(SORT_DEFAULT);
  let desk = $state('');
  let status = $state(CONNECTING);
  let busy = $state(false);
  let request;
  const visible = $derived(sortRows(rows ?? [], sort, desk));
  const desks = $derived([...new Set((rows ?? []).map(row => row.desk))].sort((a, b) => (strings.desks[a] ?? a).localeCompare(strings.desks[b] ?? b)));

  async function refresh() {
    if (busy) return;
    busy = true;
    request = new AbortController();
    const signal = request.signal;
    try {
      rows = await readScoreboard(signal);
      status = LIVE;
    } catch {
      if (!signal.aborted) status = OFFLINE;
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    if (demoFirst()) {
      status = DEMO_STATUS;
    } else {
      refresh();
    }
    return () => request?.abort();
  });
</script>

<svelte:head><title>Money 24 {strings.title}</title></svelte:head>

<div class="flex h-dvh min-w-0 flex-col">
  <AppHeader page={PAGE} {status} />
  <main class="mx-auto flex min-h-0 w-full max-w-7xl flex-1 flex-col xl:border-x xl:border-line ultrawide:max-w-none">
    <header class="flex shrink-0 items-center justify-between gap-3 border-b border-line px-3 py-2 lg:px-5 lg:py-3 tv:px-8 tv:py-5">
      <div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-0.5">
        <h1 class="text-xl leading-tight font-extrabold tracking-tight sm:text-2xl tv:text-4xl">{strings.title}</h1>
        <span class="text-xs font-semibold text-paper tv:text-lg">{strings.paper}</span>
      </div>
      {#if status !== DEMO_STATUS}
        <button type="button" onclick={refresh} aria-disabled={busy}
          class="spring flex min-h-9 shrink-0 items-center gap-2 rounded-sm border border-line px-2.5 text-xs font-semibold hover:border-muted focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red aria-disabled:text-muted tv:min-h-12 tv:text-lg">
          <RefreshCw class="size-3.5 motion-safe:aria-disabled:animate-spin tv:size-5" aria-hidden="true" />
          {strings.refresh}
        </button>
      {/if}
    </header>

    <div class="flex min-h-0 flex-1 flex-col grid-rows-[minmax(0,1fr)] md:grid md:grid-cols-[10rem_minmax(0,1fr)] lg:grid-cols-[12rem_minmax(0,1fr)] landscape-phone:grid landscape-phone:grid-cols-[9rem_minmax(0,1fr)] tv:grid-cols-[17rem_minmax(0,1fr)]">
      <aside class="flex shrink-0 gap-2 border-b border-line px-3 py-1.5 md:flex-col md:gap-4 md:border-r md:border-b-0 md:py-4 landscape-phone:flex-col landscape-phone:border-r landscape-phone:border-b-0 tv:px-5">
        <label class="min-w-0 flex-1 md:flex-none landscape-phone:flex-none">
          <span class={LABEL}>{strings.desk}</span>
          <select class={CONTROL} bind:value={desk}>
            <option value="">{strings.allDesks}</option>
            {#each desks as id (id)}<option value={id}>{strings.desks[id] ?? id}</option>{/each}
          </select>
        </label>
        <label class="min-w-0 flex-1 md:flex-none landscape-phone:flex-none">
          <span class={LABEL}>{strings.sort}</span>
          <select class={CONTROL} bind:value={sort}>
            {#each SORT_OPTIONS as [id, name] (id)}<option value={id}>{name}</option>{/each}
          </select>
        </label>
        <div class="hidden space-y-2 border-t border-line pt-3 text-sm leading-snug text-muted md:block landscape-phone:hidden! tv:text-lg">
          <h2 class="font-semibold text-white">{strings.rules}</h2>
          <p>{strings.method}</p>
          <p>{strings.evidence}</p>
        </div>
      </aside>

      <!-- svelte-ignore a11y_no_noninteractive_tabindex (the strategy list needs keyboard scrolling on older browsers) -->
      <section tabindex="0" aria-label={strings.list}
        class="list-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-contain focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red">
        <details class="border-b border-line px-3 py-1.5 text-xs text-muted md:hidden landscape-phone:block!">
          <summary class="spring w-fit cursor-pointer rounded-sm font-semibold text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{strings.rules}</summary>
          <p class="mt-2">{strings.method}</p>
          <p class="mt-2">{strings.evidence}</p>
        </details>
        {#if status === OFFLINE || !rows}
          <p role="status" class="px-3 py-3 text-sm lg:px-5 tv:text-lg {status === OFFLINE ? 'text-warn' : 'text-muted'}">
            {status === DEMO_STATUS ? strings.demo : status === OFFLINE ? (rows ? strings.stale : strings.offline) : strings.loading}
          </p>
        {/if}

        {#if rows}
          <ul class="grid min-w-0 border-line ultrawide:grid-cols-2">
            {#each visible as row, i (row.desk + '/' + row.id)}
              <li class="min-w-0 overflow-hidden border-b border-line ultrawide:odd:border-r">
                <article class="cal-rise grid min-w-0 gap-3 px-3 py-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)] lg:px-5 desktop-portrait:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)_minmax(0,1fr)] landscape-phone:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] landscape-phone:gap-2 landscape-phone:py-2 tv:gap-5 tv:px-8 tv:py-5"
                  style:--i={Math.min(i, STAGGER_MAX)}>
                  <div class="min-w-0 space-y-1">
                    <p class="text-xs font-semibold text-paper tv:text-base">{strings.desks[row.desk] ?? row.desk}</p>
                    <h2 class="text-base leading-snug font-bold break-words tv:text-2xl">{row.name}</h2>
                    <p class="text-xs text-muted capitalize tv:text-base">{row.state} · {row.risk === UNKNOWN ? strings.unknownRisk : fill(strings.risk, { risk: row.risk })}</p>
                    <p class="text-xs text-muted break-words tv:text-base">{row.basis}</p>
                  </div>

                  <dl class="grid min-w-0 grid-cols-2 gap-x-2 gap-y-1.5">
                    <div><dt class={LABEL}>{strings.settled}</dt><dd class={VALUE}>{row.settled}</dd></div>
                    <div><dt class={LABEL}>{strings.mean}</dt><dd class={VALUE}>{rate(row.mean_return, row.unit)}</dd></div>
                    <div><dt class={LABEL}>{strings.winRate}</dt><dd class={VALUE}>{rate(row.win_rate)}</dd></div>
                    <div><dt class={LABEL}>{strings.breakEven}</dt><dd class={VALUE}>{rate(row.break_even_rate)}</dd></div>
                    <div><dt class={LABEL}>{strings.first}</dt><dd class="text-xs tabular-nums tv:text-base">{date(row.first_at)}</dd></div>
                    <div><dt class={LABEL}>{strings.latest}</dt><dd class="text-xs tabular-nums tv:text-base">{date(row.latest_at)}</dd></div>
                  </dl>

                  <div class="min-w-0 space-y-1 border-t border-line pt-2 md:border-0 md:pt-0 landscape-phone:col-span-2 landscape-phone:flex landscape-phone:flex-wrap landscape-phone:items-baseline landscape-phone:gap-x-3 landscape-phone:space-y-0 landscape-phone:pt-1">
                    <p class="text-sm font-bold {VERDICT_TONE[row.verdict] ?? 'text-muted'} tv:text-xl">{row.verdict}</p>
                    <p class="text-xs leading-snug text-muted tv:text-base">{row.reason}</p>
                    <p class="text-xs font-semibold leading-snug tabular-nums tv:text-base">
                      <span class="text-muted">{strings.pnl}</span>
                      <span class="font-mono {row.realized_pnl_usd < 0 ? 'text-bad' : row.realized_pnl_usd > 0 ? 'text-ok' : 'text-white'}">{pnl(row.realized_pnl_usd)}</span>
                    </p>
                    {#if row.lower_95 != null}
                      <p class="text-xs leading-snug text-muted tabular-nums tv:text-base">{strings.range} {rate(row.lower_95, row.unit)} to {rate(row.upper_95, row.unit)} · {strings.recent} {rate(row.recent_mean, row.unit)}</p>
                    {/if}
                  </div>
                </article>
              </li>
            {:else}
              <li class="px-3 py-3 text-sm text-muted lg:px-5">{strings.empty}</li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  </main>
  <AppFooter />
</div>
