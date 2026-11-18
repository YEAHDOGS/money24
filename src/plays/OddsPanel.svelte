<script>
  /**
   * Payoff and odds: how the play can end in dollars (stopped out, some rungs then the stop, every
   * exit, total calamity), the headline numbers, and the odds — the market's chance of the first
   * exit, the evidence score that moves it, and the DOGS odds that result.
   */
  import strings from './strings.json';
  import { pnl, usd, pct, signedPct, scenarioName, barPct, when } from './play-format.js';

  /** @type {{ report: any }} */
  /** bare: the caller already wrote the "How it can end" heading (a folding section on phones) */
  let { report, bare = false } = $props();

  const LABEL = 'text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted';
  const COMPONENTS = ['volume', 'news', 'open_interest', 'technicals', 'signals'];
  const SCORE_MAX = 100;
  const NEUTRAL = 50;

  const pay = $derived(report.payoff);
  const odds = $derived(report.odds);
  const biggest = $derived(Math.max(1, ...pay.scenarios.map((/** @type {any} */ s) => Math.abs(s.pnl_usd))));
</script>

{#if !bare}<h3 class={LABEL}>{strings.detail.scenarios}</h3>{/if}
<ul class="mt-2 grid gap-1">
  {#each pay.scenarios as s (s.name)}
    {@const loss = s.pnl_usd < 0}
    <li class="grid grid-cols-[minmax(0,8.5rem)_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-x-2 text-xs">
      <span class="truncate text-white/75">{scenarioName(s.name)}</span>
      <span class="flex h-2 justify-end overflow-hidden" aria-hidden="true">
        {#if loss}<span class="bar-fill bar-fill-right block h-full w-full bg-bad/70" style="transform:scaleX({barPct(s.pnl_usd, biggest) / SCORE_MAX})"></span>{/if}
      </span>
      <span class="flex h-2 overflow-hidden" aria-hidden="true">
        {#if !loss}<span class="bar-fill block h-full w-full bg-ok/70" style="transform:scaleX({barPct(s.pnl_usd, biggest) / SCORE_MAX})"></span>{/if}
      </span>
      <span class="w-[7.5rem] text-right font-mono tabular-nums {loss ? 'text-bad' : 'text-ok'}">{pnl(s.pnl_usd)} <span class="text-muted">{signedPct(s.pnl_pct)}</span></span>
    </li>
  {/each}
</ul>

<dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 font-mono text-xs tabular-nums @md:grid-cols-3">
  <div><dt class="text-muted">{strings.payoff.cost}</dt><dd class="text-white">{usd(pay.cost_usd)}</dd></div>
  <div><dt class="text-muted">{strings.payoff.maxProfit}</dt><dd class="text-ok">{pnl(pay.max_profit_usd)}</dd></div>
  <div><dt class="text-muted">{strings.payoff.plannedLoss}</dt><dd class="text-bad">{pnl(-pay.planned_loss_usd)}</dd></div>
  <div><dt class="text-muted">{strings.payoff.worstLoss}</dt><dd class="text-bad">{pnl(-pay.worst_loss_usd)}</dd></div>
  <div><dt class="text-muted">{strings.payoff.rewardRisk}</dt><dd class="text-white">{pay.reward_risk.toFixed(1)} : 1</dd></div>
  <div><dt class="text-muted">{strings.payoff.expected}</dt><dd class={pay.expected_usd < 0 ? 'text-bad' : 'text-ok'}>{pnl(pay.expected_usd)}</dd></div>
  <div class="col-span-2 @md:col-span-3"><dt class="text-muted">{strings.payoff.exitAt}</dt><dd class="text-white">{when(pay.exit_at)} ET</dd></div>
</dl>

<h3 class="{LABEL} mt-5">{strings.detail.odds}</h3>
<div class="mt-2 grid grid-cols-3 gap-2 font-mono tabular-nums">
  <div class="border-l border-line pl-2">
    <p class="text-[0.62rem] uppercase tracking-wider text-muted">{strings.oddsPanel.market}</p>
    <p class="text-lg font-bold text-white">{odds.market_pct ? pct(odds.market_pct) : '—'}</p>
  </div>
  <div class="border-l border-line pl-2">
    <p class="text-[0.62rem] uppercase tracking-wider text-muted">{strings.oddsPanel.score}</p>
    <p class="text-lg font-bold {odds.score >= NEUTRAL ? 'text-ok' : 'text-bad'}">{odds.score >= NEUTRAL ? '▲' : '▼'} {Math.round(odds.score)}</p>
  </div>
  <div class="border-l border-neon-red pl-2">
    <p class="text-[0.62rem] uppercase tracking-wider text-muted">{strings.oddsPanel.dogs}</p>
    <p class="text-lg font-bold text-neon-red">{odds.dogs_pct ? pct(odds.dogs_pct) : '—'}</p>
  </div>
</div>
<ul class="mt-3 grid gap-1">
  {#each COMPONENTS as c (c)}
    {@const score = odds.components?.[c]}
    <li class="grid grid-cols-[6.5rem_minmax(0,1fr)_5.5rem] items-center gap-x-2 text-xs">
      <span class="truncate text-white/75">{strings.oddsPanel.components[c]}</span>
      <span class="relative h-2 overflow-hidden bg-white/5" aria-hidden="true">
        <span class="absolute inset-y-0 left-1/2 w-px bg-line"></span>
        {#if score !== undefined}<span class="bar-fill block h-full w-full {score >= NEUTRAL ? 'bg-ok/70' : 'bg-bad/70'}" style="transform:scaleX({score / SCORE_MAX})"></span>{/if}
      </span>
      <span class="text-right font-mono tabular-nums text-muted">
        {score === undefined ? strings.oddsPanel.noData : Math.round(score)}
        <span class="text-white/40">· {odds.weights?.[c] ?? 0}</span>
      </span>
    </li>
  {/each}
</ul>
<p class="mt-2 text-[0.68rem] leading-snug text-muted">{strings.oddsPanel.note}</p>
{#if odds.ignored?.length}
  <details class="mt-2 text-xs">
    <summary class="cursor-pointer text-warn">{strings.oddsPanel.ignored} · {odds.ignored.length}</summary>
    <ul class="mt-1 grid gap-0.5 text-white/60">
      {#each odds.ignored as reason, i (i)}<li>{reason}</li>{/each}
    </ul>
  </details>
{/if}
