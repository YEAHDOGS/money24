<script>
  /** Global poll interval slider: drag, arrow keys or scroll wheel. Shows what the choice costs. */
  import strings from './strings.json';
  import { formatBytes, formatCost, formatInterval } from './net-math.js';
  import PollRing from './PollRing.svelte';

  const SECOND = 1000;
  const MINUTE = 60 * SECOND;
  const MAX_MINUTES = 15;
  const STEPS = [SECOND, 5 * SECOND, 15 * SECOND, 30 * SECOND, 45 * SECOND,
    ...Array.from({ length: MAX_MINUTES }, (_, i) => (i + 1) * MINUTE)];
  const FAST_BELOW = 15 * SECOND;
  const MINUTES_PER_MONTH = 30 * 24 * 60;

  /** @type {{ everyMs: number|null, onChange: (ms: number) => void, projection: {requests:number, in:number, out:number, served:number}, cost: number, floored: string[], providerName: string, plan: any, report: any }} */
  let { everyMs, onChange, projection, cost, floored, providerName, plan, report } = $props();

  const index = $derived.by(() => {
    if (everyMs == null) return STEPS.indexOf(MINUTE);
    let best = 0;
    STEPS.forEach((ms, i) => { if (Math.abs(ms - everyMs) < Math.abs(STEPS[best] - everyMs)) best = i; });
    return best;
  });

  function pick(i) {
    const next = Math.min(STEPS.length - 1, Math.max(0, i));
    if (next !== index) onChange(STEPS[next]);
  }

  /** Scroll wheel nudges one step; up = slower, down = faster, like turning a dial. */
  function onWheel(e) {
    e.preventDefault();
    pick(index + (e.deltaY < 0 ? 1 : -1));
  }
</script>

<div class="flex flex-col gap-1 border-b border-line px-4 py-2.5 short:py-1.5 lg:px-6">
  <div class="flex items-center gap-3">
    <label for="poll-range" class="shrink-0 text-[0.68rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.pollLabel}</label>
    <input
      id="poll-range"
      class="poll-range min-w-0 flex-1 cursor-pointer"
      type="range"
      min="0"
      max={STEPS.length - 1}
      step="1"
      value={index}
      aria-valuetext={formatInterval(STEPS[index])}
      oninput={(e) => pick(Number(e.currentTarget.value))}
      onwheel={onWheel}
    />
    <output for="poll-range" class="w-10 shrink-0 text-right font-mono text-sm font-bold tabular-nums text-neon-red">
      {formatInterval(STEPS[index])}
    </output>
  </div>
  <div class="flex items-center gap-3">
  <div class="flex min-w-0 flex-1 flex-col gap-1">
  <p class="truncate font-mono text-[0.7rem] tabular-nums text-muted">
    ≈ {Math.round(projection.requests / MINUTES_PER_MONTH)} req/min
    · {formatBytes(projection.in)}/mo in
    · {formatBytes(projection.out + projection.served)}/mo out
    · <span class="text-white/80">{formatCost(cost)}/mo</span> on {providerName}
  </p>
  {#if STEPS[index] < FAST_BELOW}
    <p class="text-[0.7rem] text-warn">{strings.pollFast}</p>
  {/if}
  {#if floored.length}
    <p class="truncate text-[0.7rem] text-muted" title={floored.join(', ')}>{strings.pollFloor} {floored.join(', ')}</p>
  {/if}
  </div>
  <PollRing {plan} {report} />
  </div>
</div>
