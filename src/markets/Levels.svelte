<script>
  /**
   * A symbol's levels as a card: the line that turns it positive, the line that turns it negative, the
   * band between them, where a normal day reaches, and what each reading means for a play. The math is
   * in levels.js; this only lays it out. Colors say direction only (green up, red down), never money.
   */
  import strings from './strings.json';
  import { levelsFor } from './levels.js';
  import { price as fmt } from './market-format.js';

  /** @type {{ daily: any, live?: number, onMarks?: (marks: any[]) => void, class?: string, titleClass?: string }} */
  let { daily, live, onMarks, class: frame = '', titleClass = '' } = $props();

  const L = strings.levels;
  const TONES = { bullish: 'text-ok', neutral: 'text-white/80', bearish: 'text-bad' };
  const info = $derived(levelsFor(daily?.candles ?? [], live));

  $effect(() => {
    onMarks?.(info?.marks ?? []);
  });
</script>

{#if info}
  <section class="{frame} pb-4" aria-label={L.title}>
    <h3 class={titleClass}>{L.title}</h3>
    <p class="mt-2 px-4 text-lg font-semibold {TONES[info.bias]}">{L.bias[info.bias]}</p>
    <p class="px-4 text-xs leading-snug text-muted">{L.sub}</p>

    <!-- THE BAND: negative line, neutral between, positive line. Fixed height, nothing moves when it loads. -->
    <dl class="mt-3 grid grid-cols-3 gap-px border-y border-line bg-line text-center">
      <div class="bg-card px-2 py-2"><dt class="text-[0.6rem] font-bold tracking-wider text-bad uppercase">{L.negative}</dt><dd class="mt-0.5 font-mono text-sm font-semibold tabular-nums">{info.negative === null ? '—' : fmt(info.negative)}</dd></div>
      <div class="bg-card px-2 py-2"><dt class="text-[0.6rem] font-bold tracking-wider text-muted uppercase">{L.neutral}</dt><dd class="mt-0.5 font-mono text-sm tabular-nums text-white/70">{fmt(info.price)}</dd></div>
      <div class="bg-card px-2 py-2"><dt class="text-[0.6rem] font-bold tracking-wider text-ok uppercase">{L.positive}</dt><dd class="mt-0.5 font-mono text-sm font-semibold tabular-nums">{info.positive === null ? '—' : fmt(info.positive)}</dd></div>
    </dl>
    <p class="mt-2 px-4 text-xs leading-snug text-white/80">{L.means[info.bias]}</p>
    <p class="mt-2 px-4 text-xs text-muted">{L.expected}: <span class="font-mono tabular-nums text-white/85">{fmt(info.expected.low)} – {fmt(info.expected.high)}</span></p>

    {#each [['resistance', info.resistance, 'text-bad'], ['support', info.support, 'text-ok']] as [key, list, tone] (key)}
      <h4 class="mt-3 px-4 text-[0.6rem] font-bold tracking-wider uppercase {tone}">{L[key]}</h4>
      <ul class="px-4">
        {#each list as l (l.price)}
          <li class="flex items-baseline justify-between gap-2 border-b border-line py-1 text-xs last:border-b-0">
            <span class="font-mono font-semibold tabular-nums">{fmt(l.price)}</span>
            <span class="text-muted">{L.touches.replace('{n}', l.touches)}</span>
            <span class="font-mono tabular-nums text-white/70">{l.pct > 0 ? '+' : ''}{l.pct.toFixed(1)}%</span>
          </li>
        {:else}
          <li class="py-1 text-xs text-muted">{L.none}</li>
        {/each}
      </ul>
    {/each}

    <p class="mt-3 px-4 text-xs text-muted">
      {L.pivot} <span class="font-mono tabular-nums text-white/80">{fmt(info.pivots.p)}</span> ·
      {L.yearHigh} <span class="font-mono tabular-nums text-white/80">{fmt(info.yearHigh)}</span> ·
      {L.yearLow} <span class="font-mono tabular-nums text-white/80">{fmt(info.yearLow)}</span>
    </p>
    <p class="mt-1 px-4 text-[0.65rem] text-muted">{L.outlets}</p>
  </section>
{/if}
