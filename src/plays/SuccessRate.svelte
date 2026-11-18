<script>
  /**
   * The play's success rate, big, at the top right of the play: the DOGS odds that the whole play
   * ends in profit at the stake on the dial (report.odds.win_pct). The number counts and the ring
   * sweeps to each new value as the dial moves; the line under it says which exit gets there.
   */
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import strings from './strings.json';
  import { fill } from './play-format.js';

  /** @type {{ odds: any }} */
  let { odds } = $props();

  const TWEEN_MS = 420;
  const STRONG_PCT = 50; // at or above: green
  const FAIR_PCT = 25; // at or above: amber; below: red
  const FULL_FILL = 100;
  const RING_LENGTH = 100;

  const win = $derived(odds?.win_pct ?? 0);
  const shown = new Tween(0, { duration: TWEEN_MS, easing: cubicOut });
  $effect(() => {
    shown.target = win;
  });

  const tone = $derived(win >= STRONG_PCT ? 'text-ok' : win >= FAIR_PCT ? 'text-warn' : 'text-bad');
  const level = $derived(strings.levels[odds?.win_at] ?? odds?.win_at ?? '');
  const filled = $derived(odds?.fill_pct ?? FULL_FILL);
  const how = $derived(win > 0 ? fill(strings.detail.winAt, { level }) : filled <= 0 ? strings.detail.winNoFill : strings.detail.winNone);
</script>

<figure data-tour="plays-rate" class="m-0 flex shrink-0 flex-col items-center text-center {tone}" aria-label={`${strings.detail.winRate} ${win}%. ${how}`}>
  <div class="relative grid size-24 place-items-center @md:size-28 @lg:size-36 @4xl:size-44">
    <svg viewBox="0 0 100 100" class="absolute inset-0 size-full -rotate-90" aria-hidden="true">
      <circle cx="50" cy="50" r="45" fill="none" class="stroke-line" stroke-width="6" />
      <circle cx="50" cy="50" r="45" fill="none" stroke="currentColor" stroke-width="6" stroke-linecap="round"
        pathLength={RING_LENGTH} stroke-dasharray={RING_LENGTH} stroke-dashoffset={RING_LENGTH - Math.min(RING_LENGTH, shown.current)}
        class="drop-shadow-[0_0_6px_currentColor]" />
    </svg>
    <span class="font-mono text-3xl leading-none font-extrabold tracking-tighter tabular-nums @md:text-4xl @lg:text-5xl @4xl:text-6xl" aria-hidden="true">
      {Math.round(shown.current)}<span class="text-[0.5em] align-top">%</span>
    </span>
  </div>
  <figcaption class="mt-1.5 grid max-w-32 gap-0.5 @lg:max-w-40">
    <span class="text-[0.62rem] font-bold tracking-[0.18em] text-white uppercase">{strings.detail.winRate}</span>
    <span class="text-[0.68rem] leading-snug text-muted">{how}</span>
    {#if filled > 0 && filled < FULL_FILL}
      <span class="text-[0.68rem] leading-snug text-warn">{fill(strings.detail.winFill, { n: filled })}</span>
    {/if}
  </figcaption>
</figure>
