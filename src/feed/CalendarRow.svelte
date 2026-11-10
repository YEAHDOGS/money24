<script>
  /** One scheduled event: time, impact meter, region or ticker, type, title, numbers and a countdown. */
  import strings from './strings.json';
  import { CATEGORY_CLASSES, IMPACT_CLASSES, IMPACT_LEVELS, IMPACT_RANK, countdown, startMs, timeLabel } from './calendar-format.js';

  const SOON_MS = 24 * 60 * 60 * 1000; // countdowns show for the next day only

  /** @type {{ event: import('./calendar-format.js').CalEvent, now: number, linkTarget: string|undefined }} */
  let { event, now, linkTarget } = $props();

  const category = $derived(CATEGORY_CLASSES[event.category] ?? CATEGORY_CLASSES.other);
  const rank = $derived(IMPACT_RANK[event.impact ?? ''] ?? 0);
  const left = $derived(startMs(event) - now);
  const past = $derived(event.all_day ? left <= -SOON_MS : left <= 0);
  const values = $derived([
    [strings.calendar.actual, event.actual],
    [strings.calendar.forecast, event.forecast],
    [strings.calendar.previous, event.previous],
  ].filter(([, v]) => v));
</script>

<article class="grid grid-cols-[3.5rem_minmax(0,1fr)_auto] gap-x-3 px-4 py-2.5 transition-opacity duration-500 short:py-1.5 lg:px-6 {past ? 'opacity-45' : ''}">
  <time class="pt-0.5 text-right font-mono tabular-nums {event.all_day ? 'text-[0.6rem] font-bold uppercase tracking-wider text-muted' : 'text-xs text-white/90'}" datetime={event.starts_at}>
    {event.all_day ? strings.calendar.allDay : timeLabel(event)}
  </time>

  <div class="min-w-0">
    <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      {#if rank}
        <span class="flex items-end gap-px" role="img" aria-label={strings.calendar.impactNames[event.impact]}>
          {#each IMPACT_LEVELS as level (level)}
            <span class="w-1 rounded-sm {level <= rank ? IMPACT_CLASSES[rank] : 'bg-white/12'} {level === 1 ? 'h-1.5' : level === 2 ? 'h-2.5' : 'h-3.5'}"></span>
          {/each}
        </span>
      {/if}
      {#if event.symbol || event.region}
        <span class="font-mono font-bold tracking-wide text-white/90">{event.symbol || event.region}</span>
      {/if}
      <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold uppercase tracking-wider {category.text} {category.border}">
        {strings.calendar.categories[event.category] ?? event.category}
      </span>
    </div>

    {#if event.url}
      <a class="spring mt-1 block text-[0.95rem] leading-snug font-medium text-white decoration-neon-red/70 underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none line-clamp-2"
        href={event.url} target={linkTarget} rel="noopener noreferrer">{event.title}</a>
    {:else}
      <p class="mt-1 text-[0.95rem] leading-snug font-medium text-white line-clamp-2">{event.title}</p>
    {/if}

    {#if values.length}
      <dl class="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 font-mono text-xs tabular-nums">
        {#each values as [label, value] (label)}
          <div class="flex gap-1.5">
            <dt class="text-muted">{label}</dt>
            <dd class={label === strings.calendar.actual ? 'font-bold text-neon-red' : 'text-white/85'}>{value}</dd>
          </div>
        {/each}
      </dl>
    {/if}
    {#if event.detail}
      <p class="mt-0.5 truncate text-[0.7rem] text-muted">{event.detail}</p>
    {/if}
  </div>

  <!-- Fixed-width slot, so a countdown appearing or ending never moves the row -->
  <span class="w-[4.75rem] pt-0.5 text-right font-mono text-[0.68rem] tabular-nums text-neon-red max-[359px]:hidden">
    {#if !event.all_day && left > 0 && left < SOON_MS}{countdown(left)}{/if}
  </span>
</article>
