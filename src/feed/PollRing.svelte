<script>
  /** Poll ring: fills with the engine's clock, pops on each round, and says what the round found. */
  import { onMount } from 'svelte';
  import { scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import strings from './strings.json';
  import { formatBytes } from './net-math.js';
  import { RingMotion, RING_SIZE, RING_RADIUS, RING_CIRCUMFERENCE } from './ring-motion.js';

  const CENTER = RING_SIZE / 2;
  const HEAD_Y = CENTER - RING_RADIUS;
  const LABEL_IN_MS = 420;
  const LABEL_START = 0.6;

  /** @type {{ plan: any, report: any }} */
  let { plan, report } = $props();

  /** @type {SVGSVGElement} */ let root;
  /** @type {SVGCircleElement} */ let fill;
  /** @type {SVGGElement} */ let head;
  /** @type {SVGCircleElement} */ let burst;
  /** @type {RingMotion | null} */
  let motion = $state(null);
  let lastReportAt = '';

  onMount(() => {
    const calm = matchMedia('(prefers-reduced-motion: reduce)').matches;
    motion = new RingMotion({ root, fill, head, burst, calm });
    return () => motion?.stop();
  });

  $effect(() => {
    if (motion && plan) motion.plan(plan);
  });

  $effect(() => {
    if (!motion || !report || report.at === lastReportAt) return;
    // The first report is a replay from when the window connected: show it, don't celebrate it.
    if (lastReportAt) motion.report();
    lastReportAt = report.at;
  });

  const tone = $derived(!report ? 'idle' : report.added > 0 ? 'added' : report.changed > 0 ? 'quiet' : 'still');
  const headline = $derived.by(() => {
    if (!report) return strings.ring.waiting;
    if (report.added > 0) return `+${report.added} ${report.added === 1 ? strings.ring.article : strings.ring.articles}`;
    return report.changed > 0 ? strings.ring.noNew : strings.ring.noChange;
  });
  const detail = $derived.by(() => {
    if (!report) return '';
    const sources = report.added > 0 ? report.sources_added : report.answered;
    const parts = [`${sources} ${sources === 1 ? strings.ring.source : strings.ring.sources}`, formatBytes(report.bytes_in)];
    if (report.errors) parts.push(`${report.errors} ${strings.ring.failed}`);
    return parts.join(' · ');
  });
</script>

<div class="flex shrink-0 items-center gap-2.5" role="status" aria-live="polite">
  <!-- Fixed width, so a longer or shorter result never moves the ring or the slider -->
  <div class="w-36 overflow-hidden text-right leading-tight">
    {#key report?.at}
      <div in:scale={{ start: LABEL_START, duration: LABEL_IN_MS, easing: backOut }} class="origin-right">
        <p class="truncate text-xs font-bold {tone === 'added' ? 'text-neon-red' : tone === 'idle' ? 'text-muted' : 'text-white/80'}">{headline}</p>
        <p class="truncate font-mono text-[0.62rem] tabular-nums text-muted">{detail}</p>
      </div>
    {/key}
  </div>

  <svg bind:this={root} class="size-9 shrink-0 overflow-visible" viewBox="0 0 {RING_SIZE} {RING_SIZE}" aria-hidden="true">
    <circle bind:this={burst} class="ring-part fill-none stroke-neon-red opacity-0" cx={CENTER} cy={CENTER} r={RING_RADIUS} stroke-width="1.5" />
    <circle class="fill-none stroke-line" cx={CENTER} cy={CENTER} r={RING_RADIUS} stroke-width="3" />
    <circle
      bind:this={fill}
      class="fill-none stroke-neon-red"
      cx={CENTER}
      cy={CENTER}
      r={RING_RADIUS}
      stroke-width="3"
      stroke-linecap="round"
      stroke-dasharray={RING_CIRCUMFERENCE}
      stroke-dashoffset={RING_CIRCUMFERENCE}
      transform="rotate(-90 {CENTER} {CENTER})"
    />
    <g bind:this={head} class="ring-part">
      <circle class="fill-white" cx={CENTER} cy={HEAD_Y} r="2.2" />
    </g>
    {#if tone === 'added'}
      <circle class="fill-neon-red" cx={CENTER} cy={CENTER} r="3.5" />
    {/if}
  </svg>
</div>
