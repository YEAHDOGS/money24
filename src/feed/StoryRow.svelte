<script>
  /**
   * One story on the board: its picture with how loud it is on the corner, its headline, what kind
   * of story it is, which way it leans, how many outlets carry it, and coverage set against
   * trading volume. The whole row is one button that opens the story.
   */
  import TrendingUp from '@lucide/svelte/icons/trending-up';
  import TrendingDown from '@lucide/svelte/icons/trending-down';
  import Minus from '@lucide/svelte/icons/minus';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import strings from './stories.json';
  import StoryArt from '../lib/StoryArt.svelte';
  import { ANGLE_CLASSES, READING_CLASSES, TREND_CLASSES, sparkPath } from './story-format.js';
  import { age } from './post-format.js';

  const SPARK_WIDTH = 44;
  const SPARK_HEIGHT = 10;
  const TREND_ICONS = { rising: TrendingUp, steady: Minus, fading: TrendingDown };
  const MAX_TAGS = 3;
  const QUIET_READINGS = ['quiet', 'none'];
  const CHIP = 'rounded-sm border px-1.5 py-px text-[0.68rem] font-bold tracking-wider uppercase';
  const READING = 'rounded-sm border px-1.5 py-px text-[0.62rem] font-bold tracking-wider whitespace-nowrap uppercase';

  /** @type {{ story: import('./stories-client.svelte.js').Story, open: boolean, now: number, onToggle: () => void }} */
  let { story, open, now, onToggle } = $props();

  const angle = $derived(ANGLE_CLASSES[story.angle.label] ?? ANGLE_CLASSES.unclear);
  const Trend = $derived(TREND_ICONS[story.attention.trend] ?? Minus);
  const spark = $derived(sparkPath(story.spark, SPARK_WIDTH, SPARK_HEIGHT));
  /** A quiet reading keeps its room and only fades, so a chip appearing never moves the row. */
  const reading = $derived(`${READING_CLASSES[story.reading.label] ?? READING_CLASSES.none} ${QUIET_READINGS.includes(story.reading.label) ? 'opacity-60' : ''}`);
</script>

<button
  type="button"
  onclick={onToggle}
  aria-expanded={open}
  aria-controls="story-{story.id}"
  class="spring group grid w-full grid-cols-[4.5rem_minmax(0,1fr)_auto] items-start gap-x-3 px-4 py-3 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none max-sm:grid-cols-[3.75rem_minmax(0,1fr)_auto] max-sm:gap-x-2.5 max-sm:px-3 short:py-2 lg:grid-cols-[6rem_minmax(0,1fr)_auto] lg:px-6 {open ? 'bg-white/5' : ''}"
>
  <!-- PICTURE, with attention on its corner: the score, its direction and its last readings -->
  <span class="relative block" title="{strings.attention} {story.attention.score}">
    <StoryArt image={story.image} seed={story.id} category={story.category} class="aspect-4/3 w-full rounded-sm transition-transform duration-300 ease-out group-hover:scale-[1.03] motion-reduce:transition-none" />
    <span class="absolute bottom-0 left-0 flex items-center gap-1 rounded-tr-sm rounded-bl-sm bg-app/85 px-1 py-0.5 font-mono text-xs leading-none font-bold tabular-nums text-white">
      <Trend class="size-3 shrink-0 {TREND_CLASSES[story.attention.trend] ?? 'text-muted'}" aria-hidden="true" />{Math.round(story.attention.score)}
    </span>
    <svg viewBox="0 0 {SPARK_WIDTH} {SPARK_HEIGHT}" preserveAspectRatio="none" class="mt-1 h-2.5 w-full overflow-visible" aria-hidden="true">
      <path d={spark} fill="none" class="stroke-neon-red" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round" vector-effect="non-scaling-stroke" />
    </svg>
  </span>

  <span class="min-w-0">
    <span class="block text-[0.95rem] leading-snug font-medium text-white line-clamp-2 max-sm:line-clamp-3">{story.title}</span>
    <span class="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      <span class="{CHIP} border-line text-white/75">{strings.categories[story.category] ?? story.category}</span>
      <span class="{CHIP} {angle.text} {angle.border}">{strings.angles[story.angle.label] ?? story.angle.label}</span>
      <!-- Phones: the reading sits with the other chips, where there is room for it -->
      <span class="{READING} sm:hidden {reading}">{strings.readings[story.reading.label] ?? story.reading.label}</span>
      <span class="font-mono tabular-nums text-muted">{story.outlets} {story.outlets === 1 ? strings.outlet : strings.outlets} · {age(story.first_seen, now)}</span>
      {#each story.tags.slice(0, MAX_TAGS) as tag (tag)}
        <span class="rounded-sm bg-white/8 px-1.5 py-px font-mono text-[0.68rem] font-semibold tracking-wider text-white/80 max-sm:hidden">{tag}</span>
      {/each}
    </span>
  </span>

  <span class="flex flex-col items-end gap-1.5 pt-0.5">
    <span class="{READING} max-sm:hidden {reading}">{strings.readings[story.reading.label] ?? story.reading.label}</span>
    <ChevronDown class="size-4 text-muted transition-transform duration-200 {open ? 'rotate-180' : ''}" aria-hidden="true" />
  </span>
</button>
