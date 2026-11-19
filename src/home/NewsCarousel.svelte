<script>
  /**
   * The news banner: the top stories, one a slide. Each shows its category, its angle, how many
   * outlets carry it, its attention score and the tickers it can move. It changes by itself, and
   * holds while it is pointed at or has focus; the arrows, the dots, the arrow keys and a sideways
   * swipe change it by hand, and the pause button stops it. Slides are stacked in one fixed frame
   * and change by transform and opacity, so nothing around the banner ever moves.
   * The state and gestures are carousel.svelte.js.
   */
  import strings from './strings.json';
  import storyStrings from '../feed/stories.json';
  import StoryArt from '../lib/StoryArt.svelte';
  import { fill } from '../plays/play-format.js';
  import { age } from '../feed/post-format.js';
  import { BACK, Carousel, FORWARD, SIDE_AFTER, SIDE_BEFORE, SIDE_HERE, STEP_ATTR, STEP_BACK, STEP_FORWARD, gestures } from './carousel.svelte.js';
  import { MODE_STORIES, categoryLabel } from './news-model.js';
  import { angleStyle, categoryStyle } from './news-style.js';
  import { MARKETS_PAGE, SYMBOL_PARAM, pageLink } from './home-links.js';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Pause from '@lucide/svelte/icons/pause';
  import Play from '@lucide/svelte/icons/play';

  const TRENDS = { rising: '▲', steady: '', fading: '▼' };
  const SIDES = {
    [SIDE_HERE]: 'visible translate-x-0 opacity-100',
    [SIDE_BEFORE]: 'invisible -translate-x-6 opacity-0',
    [SIDE_AFTER]: 'invisible translate-x-6 opacity-0',
  };
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase tv:text-sm';
  const CHIP = 'rounded-sm border px-1.5 py-px text-[0.6rem] font-bold tracking-wider uppercase tv:text-sm';
  const TICKER = 'rounded-sm bg-white/8 px-1.5 py-px font-mono text-[0.68rem] font-semibold tracking-wider text-white/85 tv:text-sm';
  const CONTROL = 'spring grid w-8 shrink-0 place-items-center text-white/60 hover:bg-white/5 hover:text-white focus-visible:bg-white/5 focus-visible:text-white focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red disabled:opacity-30 tv:w-12';

  /**
   * @type {{ slides: import('./news-model.js').Headline[], mode: string, now: number, linkTarget: string | undefined }}
   * mode: stories or plain headlines. linkTarget: where an article opens.
   */
  let { slides, mode, now, linkTarget } = $props();

  const carousel = new Carousel();
  const title = $derived(mode === MODE_STORIES ? strings.carousel.label : strings.carousel.labelEvents);
  const single = $derived(slides.length < 2);
  /** Not changing by itself: stopped by the reader, or reduced motion. The button then offers to play. */
  const still = $derived(carousel.stopped || carousel.reduced);

  $effect(() => carousel.size(slides.length));
  $effect(() => (carousel.playing ? carousel.run() : undefined));
</script>

<section use:gestures={carousel} aria-roledescription="carousel" aria-label={title} class="flex shrink-0 flex-col border-b border-line">
  <div class="relative h-40 touch-pan-y overflow-hidden short:h-32 tv:h-56" aria-live={carousel.playing ? 'off' : 'polite'}>
    {#each slides as slide, i (slide.id)}
      {@const category = categoryStyle(mode, slide.category)}
      {@const angle = angleStyle(slide.angle)}
      <article aria-roledescription="slide" aria-label={fill(strings.carousel.slide, { n: i + 1, of: slides.length })}
        class="absolute inset-0 isolate flex flex-col gap-1.5 px-3 py-3 transition-[opacity,translate,visibility] duration-300 ease-out motion-reduce:transition-none lg:px-4 short:py-2 {SIDES[carousel.side(i)]}">
        <!-- The story's picture fills the slide; the veil keeps the words on its left readable. -->
        <StoryArt image={slide.image} seed={slide.seed} category={slide.category} overlay="veil" eager={i === 0} class="absolute! inset-0 -z-10" />
        <p class="flex min-w-0 items-center gap-2">
          <span class="{LABEL} shrink-0 max-[359px]:hidden">{title}</span>
          <span class="{CHIP} truncate {category.text} {category.border}">{categoryLabel(mode, slide.category)}</span>
          {#if slide.angle}<span class="{CHIP} shrink-0 {angle.text} {angle.border}">{angle.mark} {storyStrings.angles[slide.angle] ?? slide.angle}</span>{/if}
          <time class="ml-auto shrink-0 font-mono text-xs text-muted tabular-nums tv:text-base" datetime={slide.at}>{age(slide.at, now)}</time>
        </p>
        <a href={slide.href} target={slide.external ? linkTarget : undefined} rel={slide.external ? 'noopener noreferrer' : undefined} draggable="false"
          class="line-clamp-3 text-lg leading-snug font-semibold text-white decoration-neon-red/70 underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none short:line-clamp-2 short:text-base tv:text-2xl">{slide.title}</a>
        <p class="mt-auto flex min-w-0 items-center gap-x-3 overflow-hidden font-mono text-[0.7rem] whitespace-nowrap text-muted tabular-nums tv:text-sm">
          {#if slide.source}<span class="truncate font-sans font-semibold text-white/80">{slide.source}</span>{/if}
          {#if slide.outlets}<span class="shrink-0"><span class="text-white/85">{slide.outlets}</span> {slide.outlets === 1 ? strings.carousel.outlet : strings.carousel.outlets}</span>{/if}
          {#if slide.attention !== null}
            <span class="shrink-0">{strings.carousel.attention} <span class="text-white/85">{slide.attention}</span> {TRENDS[slide.trend] ?? ''}<span class="sr-only">{storyStrings.trends[slide.trend] ?? ''}</span></span>
          {/if}
          <span class="flex min-w-0 items-center gap-1.5 overflow-hidden">
            {#each slide.tickers as ticker (ticker.symbol)}
              {#if ticker.linked}
                <a href={pageLink(MARKETS_PAGE, { [SYMBOL_PARAM]: ticker.symbol })} draggable="false" class="spring {TICKER} hover:bg-neon-red/25 focus-visible:outline focus-visible:outline-neon-red">{ticker.symbol}</a>
              {:else}
                <span class={TICKER}>{ticker.symbol}</span>
              {/if}
            {/each}
          </span>
        </p>
      </article>
    {:else}
      <!-- Why there are none (the service is off, or still loading) is said once, by the headlines list. -->
      <p class="px-3 py-5 text-sm text-muted lg:px-4">{strings.carousel.empty}</p>
    {/each}
  </div>

  <!-- CONTROLS: always there, so the banner is the same height with one story or six. -->
  <div class="flex h-8 items-stretch border-t border-line tv:h-12">
    <button type="button" class={CONTROL} {...{ [STEP_ATTR]: STEP_BACK }} aria-label={strings.carousel.previous} title={strings.carousel.previous} disabled={single} onclick={() => carousel.go(carousel.index + BACK)}>
      <ChevronLeft class="size-4 tv:size-6" aria-hidden="true" />
    </button>
    <div class="flex min-w-0 flex-1 items-stretch justify-center">
      {#each slides as slide, i (slide.id)}
        <button type="button" aria-label={fill(strings.carousel.go, { n: i + 1 })} aria-current={carousel.index === i ? 'true' : undefined} onclick={() => carousel.go(i)}
          class="spring group grid w-5 place-items-center focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red tv:w-8">
          <span class="size-1.5 rounded-full transition-[transform,background-color] duration-200 motion-reduce:transition-none tv:size-2 {carousel.index === i ? 'scale-150 bg-neon-red' : 'bg-white/30 group-hover:bg-white/70'}"></span>
        </button>
      {/each}
    </div>
    <button type="button" class={CONTROL} aria-label={still ? strings.carousel.play : strings.carousel.pause} title={still ? strings.carousel.play : strings.carousel.pause}
      disabled={single || carousel.reduced} onclick={() => { carousel.stopped = !carousel.stopped; }}>
      {#if still}<Play class="size-3.5 tv:size-5" aria-hidden="true" />{:else}<Pause class="size-3.5 tv:size-5" aria-hidden="true" />{/if}
    </button>
    <button type="button" class={CONTROL} {...{ [STEP_ATTR]: STEP_FORWARD }} aria-label={strings.carousel.next} title={strings.carousel.next} disabled={single} onclick={() => carousel.go(carousel.index + FORWARD)}>
      <ChevronRight class="size-4 tv:size-6" aria-hidden="true" />
    </button>
  </div>
</section>
