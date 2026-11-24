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
  const HORIZONS = ['day', 'week'];
  const OUTCOMES = ['up', 'flat', 'down'];
  const OUTCOME_CLASS = { up: 'bg-ok/25 text-ok', flat: 'bg-white/10 text-white/80', down: 'bg-bad/25 text-bad' };
  const pct = (/** @type {number} */ x) => `${Math.round(x * 100)}%`;
  const stretchText = $derived.by(() => {
    const t = info?.stretch;
    if (!t) return '';
    const text = L.stretch[t.direction];
    return text.replace('{run}', `${t.runPct > 0 ? '+' : ''}${t.runPct.toFixed(0)}%`).replace('{ext}', Math.abs(t.extension).toFixed(1)).replace('{rsi}', t.rsi.toFixed(0));
  });

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

    <!-- STRETCH: how far it has run. The meter fills with a transform; its words say what that means. -->
    <h4 class="mt-3 flex items-baseline justify-between px-4 text-[0.6rem] font-bold tracking-wider text-muted uppercase"><span>{L.stretch.title}</span><span class="font-mono tabular-nums {info.stretch.direction === 'none' ? 'text-white/70' : 'text-warn'}">{info.stretch.score}</span></h4>
    <span class="mx-4 mt-1 block h-1 overflow-hidden bg-line" aria-hidden="true"><span class="block h-1 origin-left bg-warn transition-transform duration-500 ease-out" style:transform="scaleX({info.stretch.score / 100})"></span></span>
    <p class="mt-1.5 px-4 text-xs leading-snug text-white/80">{stretchText}</p>

    <!-- THREE OUTCOMES: up, flat and down, each with its share. -->
    <h4 class="mt-3 px-4 text-[0.6rem] font-bold tracking-wider text-muted uppercase">{L.outcomes.title}</h4>
    {#each HORIZONS as h (h)}
      <p class="mt-1.5 px-4 text-xs text-muted">{L.outcomes[h]}</p>
      <div class="mx-4 mt-0.5 flex h-5 overflow-hidden rounded-sm text-[0.65rem] font-bold tabular-nums" role="img" aria-label={OUTCOMES.map((o) => `${L.outcomes[o]} ${pct(info.odds[h][o])}`).join(', ')}>
        {#each OUTCOMES as o (o)}
          <span class="grid min-w-0 place-items-center {OUTCOME_CLASS[o]}" style:flex="{Math.max(info.odds[h][o], 0.001)} 1 0%">{pct(info.odds[h][o])}</span>
        {/each}
      </div>
    {/each}
    <p class="mt-1.5 px-4 text-[0.65rem] leading-snug text-muted">{L.outcomes.note}</p>

    <p class="mt-3 px-4 text-xs text-muted">{L.expected}: <span class="font-mono tabular-nums text-white/85">{fmt(info.expected.low)} – {fmt(info.expected.high)}</span></p>

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
