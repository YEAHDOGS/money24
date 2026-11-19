<script>
  /**
   * An open story, with every number shown beside what it was made from: how loud and how valid
   * it is, the headline words its angle was read from, the instruments it can move with their
   * volume, the prediction markets that ask about it, the places it touches and its articles.
   */
  import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
  import ArrowDownRight from '@lucide/svelte/icons/arrow-down-right';
  import Minus from '@lucide/svelte/icons/minus';
  import Check from '@lucide/svelte/icons/check';
  import strings from './stories.json';
  import { ANGLE_CLASSES, READING_CLASSES, marketHref, pace, placeName, shortDollars, signedPct } from './story-format.js';
  import { age } from './post-format.js';

  const PERCENT = 100;
  const MAX_ARTICLES = 40;
  const ANGLE_ICONS = { bullish: ArrowUpRight, bearish: ArrowDownRight };
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase';
  const SECTION = 'border-t border-line px-4 py-3 lg:px-6';

  /** @type {{ story: import('./stories-client.svelte.js').Story, detail?: import('./stories-client.svelte.js').StoryDetail, now: number, linkTarget: string|undefined }} */
  let { story, detail, now, linkTarget } = $props();

  const angle = $derived(ANGLE_CLASSES[story.angle.label] ?? ANGLE_CLASSES.unclear);
  const meters = $derived([
    { label: strings.attention, score: story.attention.score, parts: [
      `${story.attention.last_hour} ${strings.lastHour}`, `${story.attention.prev_hour} ${strings.prevHour}`,
      `${story.outlets} ${story.outlets === 1 ? strings.outlet : strings.outlets}`,
    ] },
    { label: strings.validity, score: story.validity.score, parts: [
      `${story.validity.independent} ${strings.independent}`, `${story.validity.known} ${strings.known}`,
      story.validity.primary ? strings.primaryYes : strings.primaryNo,
    ] },
  ]);
  const places = $derived([[strings.from, story.from], [strings.about, story.about]].filter(([, list]) => list.length));
</script>

<div class="text-sm">
  <!-- COVERAGE AGAINST VOLUME -->
  <section class="px-4 py-3 lg:px-6">
    <h3 class={LABEL}>{strings.reading}</h3>
    <p class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1">
      <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold tracking-wider uppercase {READING_CLASSES[story.reading.label] ?? READING_CLASSES.none}">{strings.readings[story.reading.label] ?? story.reading.label}</span>
      {#if story.reading.symbol}<span class="font-mono text-xs font-bold text-white/90">{story.reading.symbol}</span>{/if}
      <span class="text-white/70">{strings.readingNotes[story.reading.label] ?? ''}</span>
    </p>
    <dl class="mt-3 grid gap-3 sm:grid-cols-2">
      {#each meters as meter (meter.label)}
        <div>
          <dt class="flex items-baseline justify-between gap-2 text-xs text-white/80">
            <span>{meter.label}</span><span class="font-mono font-bold tabular-nums text-white">{Math.round(meter.score)}</span>
          </dt>
          <dd>
            <span class="mt-1 block h-px bg-line" aria-hidden="true">
              <span class="block h-px origin-left bg-neon-red transition-transform duration-500 ease-out" style:transform="scaleX({meter.score / PERCENT})"></span>
            </span>
            <span class="mt-1.5 block text-xs leading-snug text-muted">{meter.parts.join(' · ')}</span>
          </dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- ANGLE -->
  <section class={SECTION}>
    <h3 class={LABEL}>{strings.angle}</h3>
    <p class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1.5 text-xs">
      <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold tracking-wider uppercase {angle.text} {angle.border}">{strings.angles[story.angle.label] ?? story.angle.label}</span>
      <span class="text-muted">{story.angle.terms.length ? strings.angleBy : strings.angleNone}</span>
      {#each story.angle.terms as term (term.term)}
        <span class="rounded-sm bg-white/8 px-1.5 py-px font-mono {(ANGLE_CLASSES[term.side] ?? ANGLE_CLASSES.unclear).text}">{term.term} <span class="opacity-60">{term.n}</span></span>
      {/each}
    </p>
  </section>

  <!-- WHAT IT CAN MOVE -->
  <section class={SECTION}>
    <h3 class={LABEL}>{strings.links}</h3>
    {#if story.links.length}
      <ul class="mt-1.5 grid gap-px">
        {#each story.links as link (link.symbol)}
          {@const Icon = ANGLE_ICONS[link.angle] ?? Minus}
          <li class="grid grid-cols-[1rem_4.5rem_minmax(0,1fr)_auto] items-baseline gap-x-2 py-1">
            <Icon class="size-3.5 self-center {(ANGLE_CLASSES[link.angle] ?? ANGLE_CLASSES.unclear).text}" aria-label={strings.angles[link.angle] ?? link.angle} />
            <a href={marketHref(link.symbol)} class="spring truncate font-mono text-xs font-bold text-white underline decoration-line underline-offset-4 hover:decoration-neon-red focus-visible:decoration-neon-red focus-visible:outline-none">{link.symbol}</a>
            <span class="min-w-0 truncate text-xs text-muted" title={link.why}><span class="text-white/70">{strings.relations[link.relation] ?? link.relation}</span> · {link.why}</span>
            <span class="text-right font-mono text-xs tabular-nums">
              {#if link.quoted}
                <span class={link.move_pct < 0 ? 'text-bad' : 'text-ok'}>{signedPct(link.move_pct)}</span>
                {#if link.rel_volume}<span class="ml-1.5 text-white/70" title="{strings.volume}: {pace(link.rel_volume)} {strings.volumeNote}">{pace(link.rel_volume)}</span>{/if}
              {/if}
            </span>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="mt-1.5 text-xs text-muted">{strings.linksNone}</p>
    {/if}
  </section>

  <!-- PREDICTION MARKETS -->
  {#if story.markets.length}
    <section class={SECTION}>
      <h3 class={LABEL}>{strings.bets}</h3>
      <ul class="mt-1.5 grid gap-1.5">
        {#each story.markets as bet (bet.id)}
          <li class="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3">
            <a href={bet.url} target={linkTarget} rel="noopener noreferrer" class="spring truncate text-xs text-white/90 underline decoration-line underline-offset-4 hover:decoration-neon-red focus-visible:decoration-neon-red focus-visible:outline-none">{bet.question}</a>
            <span class="font-mono text-xs tabular-nums text-muted"><span class="font-bold text-white">{strings.betYes} {Math.round(bet.yes * PERCENT)}%</span> · {shortDollars(bet.volume)} {strings.betVolume}</span>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  <!-- PLACES -->
  {#if places.length}
    <section class="{SECTION} grid gap-2">
      {#each places as [label, list] (label)}
        <p class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
          <span class={LABEL}>{label}</span>
          {#each list as place (place.id)}
            <span class="rounded-sm bg-white/8 px-1.5 py-px text-white/80">{placeName(place.id)} <span class="font-mono opacity-60">{place.n}</span></span>
          {/each}
        </p>
      {/each}
    </section>
  {/if}

  <!-- ARTICLES -->
  <section class={SECTION}>
    <h3 class={LABEL}>{strings.coverage} <span class="font-mono tracking-normal">{story.articles}</span></h3>
    <!-- Room for a few rows is held while they load, so the list does not jump in under the reader. -->
    <ol class="mt-1.5 grid min-h-16 gap-1.5">
      {#each (detail?.events ?? []).slice(0, MAX_ARTICLES) as post (post.id)}
        <li class="grid grid-cols-[2.25rem_minmax(0,1fr)] gap-x-2 text-xs">
          <time class="text-right font-mono tabular-nums text-muted" datetime={post.seen_at}>{age(post.published_at ?? post.seen_at, now)}</time>
          <span class="min-w-0">
            <a href={post.url} target={linkTarget} rel="noopener noreferrer" class="spring text-white/90 underline decoration-line underline-offset-4 hover:decoration-neon-red focus-visible:decoration-neon-red focus-visible:outline-none">{post.title}</a>
            <span class="text-muted"> · {post.source}{#if post.kind !== 'news'} <Check class="inline size-3 text-ok" aria-label={strings.primary} />{/if}</span>
          </span>
        </li>
      {/each}
    </ol>
  </section>
</div>
