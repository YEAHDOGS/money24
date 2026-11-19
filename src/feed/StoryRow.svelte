<script>
  /**
   * One story on the board: how loud it is (score and sparkline), its headline, what kind of story
   * it is, which way it leans, how many outlets carry it, and coverage set against trading volume.
   * The whole row is one button that opens the story.
   */
  import TrendingUp from '@lucide/svelte/icons/trending-up';
  import TrendingDown from '@lucide/svelte/icons/trending-down';
  import Minus from '@lucide/svelte/icons/minus';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import strings from './stories.json';
  import { ANGLE_CLASSES, READING_CLASSES, TREND_CLASSES, sparkPath } from './story-format.js';
  import { age } from './post-format.js';

  const SPARK_WIDTH = 44;
  const SPARK_HEIGHT = 14;
  const TREND_ICONS = { rising: TrendingUp, steady: Minus, fading: TrendingDown };
  const MAX_TAGS = 3;
  const QUIET_READINGS = ['quiet', 'none'];

  /** @type {{ story: import('./stories-client.svelte.js').Story, open: boolean, now: number, onToggle: () => void }} */
  let { story, open, now, onToggle } = $props();

  const angle = $derived(ANGLE_CLASSES[story.angle.label] ?? ANGLE_CLASSES.unclear);
  const Trend = $derived(TREND_ICONS[story.attention.trend] ?? Minus);
  const spark = $derived(sparkPath(story.spark, SPARK_WIDTH, SPARK_HEIGHT));
</script>

<button
  type="button"
  onclick={onToggle}
  aria-expanded={open}
  aria-controls="story-{story.id}"
  class="spring grid w-full grid-cols-[2.75rem_minmax(0,1fr)_auto] items-start gap-x-3 px-4 py-3 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none max-sm:gap-x-2.5 max-sm:px-3 short:py-2 lg:px-6 {open ? 'bg-white/5' : ''}"
>
  <!-- ATTENTION: the score, its direction and its last readings -->
  <span class="flex flex-col items-end gap-0.5 pt-0.5" title="{strings.attention} {story.attention.score}">
    <span class="flex items-center gap-1 font-mono text-sm leading-none font-bold tabular-nums text-white">
      <Trend class="size-3 shrink-0 {TREND_CLASSES[story.attention.trend] ?? 'text-muted'}" aria-hidden="true" />{Math.round(story.attention.score)}
    </span>
    <svg viewBox="0 0 {SPARK_WIDTH} {SPARK_HEIGHT}" class="h-3.5 w-11 overflow-visible" aria-hidden="true">
      <path d={spark} fill="none" class="stroke-neon-red" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
    </svg>
  </span>

  <span class="min-w-0">
    <span class="block text-[0.95rem] leading-snug font-medium text-white line-clamp-2">{story.title}</span>
    <span class="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      <span class="rounded-sm border border-line px-1.5 py-px text-[0.68rem] font-bold tracking-wider text-white/75 uppercase">{strings.categories[story.category] ?? story.category}</span>
      <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold tracking-wider uppercase {angle.text} {angle.border}">{strings.angles[story.angle.label] ?? story.angle.label}</span>
      <span class="font-mono tabular-nums text-muted">{story.outlets} {story.outlets === 1 ? strings.outlet : strings.outlets} · {age(story.first_seen, now)}</span>
      {#each story.tags.slice(0, MAX_TAGS) as tag (tag)}
        <span class="rounded-sm bg-white/8 px-1.5 py-px font-mono text-[0.68rem] font-semibold tracking-wider text-white/80 max-[359px]:hidden">{tag}</span>
      {/each}
    </span>
  </span>

  <span class="flex flex-col items-end gap-1.5 pt-0.5">
    <!-- A quiet reading keeps its room and only fades, so a chip appearing never moves the row. -->
    <span class="rounded-sm border px-1.5 py-px text-[0.62rem] font-bold tracking-wider whitespace-nowrap uppercase {READING_CLASSES[story.reading.label] ?? READING_CLASSES.none} {QUIET_READINGS.includes(story.reading.label) ? 'opacity-60' : ''}">
      {strings.readings[story.reading.label] ?? story.reading.label}
    </span>
    <ChevronDown class="size-4 text-muted transition-transform duration-200 {open ? 'rotate-180' : ''}" aria-hidden="true" />
  </span>
</button>
