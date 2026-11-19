<script>
  /**
   * A story's picture. When an outlet sent a photo with its article, that photo, loaded straight
   * from the outlet and faded in over the art. Otherwise generated art: one of four backdrops,
   * picked by the story's number so a story always wears the same one, under a large mark for its
   * category. The frame's size comes from the caller, so a photo arriving never moves anything.
   * Decorative: the headline beside it says what the story is.
   */
  import Landmark from '@lucide/svelte/icons/landmark';
  import ChartColumn from '@lucide/svelte/icons/chart-column';
  import CircleDollarSign from '@lucide/svelte/icons/circle-dollar-sign';
  import Handshake from '@lucide/svelte/icons/handshake';
  import Scale from '@lucide/svelte/icons/scale';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import Truck from '@lucide/svelte/icons/truck';
  import Rocket from '@lucide/svelte/icons/rocket';
  import Telescope from '@lucide/svelte/icons/telescope';
  import UserRound from '@lucide/svelte/icons/user-round';
  import Globe from '@lucide/svelte/icons/globe';
  import Megaphone from '@lucide/svelte/icons/megaphone';
  import ChartCandlestick from '@lucide/svelte/icons/chart-candlestick';
  import Newspaper from '@lucide/svelte/icons/newspaper';

  /** One mark per story category in src/feed/stories.json; anything else gets the newspaper. */
  const MARKS = {
    central_bank: Landmark, macro: ChartColumn, earnings: CircleDollarSign, deal: Handshake, legal: Scale,
    security: ShieldAlert, supply: Truck, product: Rocket, analyst: Telescope, executive: UserRound,
    geopolitics: Globe, rumor: Megaphone, markets: ChartCandlestick,
  };
  const BACKDROPS = 4; // --art-1 to --art-4 in src/styles/theme.css
  const OVERLAYS = { scrim: 'var(--art-scrim)', veil: 'var(--art-veil)' };
  const MARK_STROKE = 1.25;

  /**
   * @type {{ image?: string, seed?: number, category?: string, overlay?: '' | 'scrim' | 'veil', eager?: boolean, class?: string }}
   * image: a link to the outlet's photo, empty for art. seed: the story's number. overlay: a dark
   * layer for text set over the picture. eager: load at once (the first thing on screen).
   */
  let { image = '', seed = 0, category = '', overlay = '', eager = false, class: frame = '' } = $props();

  /** The photo that finished loading, and the one that failed: compared with `image`, so a reused frame starts over. */
  let loaded = $state('');
  let failed = $state('');

  const Mark = $derived(MARKS[category] ?? Newspaper);
  const backdrop = $derived(`var(--art-lines), var(--art-${(Math.abs(Math.trunc(seed)) % BACKDROPS) + 1})`);
</script>

<span class="relative block overflow-hidden bg-card {frame}" aria-hidden="true">
  <span class="absolute inset-0" style:background-image={backdrop}></span>
  <Mark class="absolute top-1/2 left-1/2 size-[52%] max-h-28 max-w-28 -translate-x-1/2 -translate-y-1/2 text-white/55" strokeWidth={MARK_STROKE} />
  {#if image && failed !== image}
    <img
      src={image}
      alt=""
      loading={eager ? 'eager' : 'lazy'}
      decoding="async"
      referrerpolicy="no-referrer"
      draggable="false"
      onload={() => { loaded = image; }}
      onerror={() => { failed = image; }}
      class="absolute inset-0 size-full object-cover transition-opacity duration-500 ease-out motion-reduce:transition-none {loaded === image ? 'opacity-100' : 'opacity-0'}"
    />
  {/if}
  {#if overlay}<span class="absolute inset-0" style:background-image={OVERLAYS[overlay]}></span>{/if}
</span>
