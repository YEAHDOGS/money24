<script>
  /**
   * A day's events of one kind (earnings, the Fed) folded into one row: earliest time, highest impact,
   * a count and the biggest names. The row is a button that opens the single events in place.
   */
  import { untrack } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import strings from './strings.json';
  import CalendarRow, { SOON_MS, impactMeter } from './CalendarRow.svelte';
  import { CATEGORY_CLASSES, countdown, startMs, timeLabel } from './calendar-format.js';

  const NAME_SEPARATOR = ' · ';
  const PANEL_PREFIX = 'cal-group-';
  const REDUCED_MOTION = matchMedia('(prefers-reduced-motion: reduce)');

  /** @type {{ group: import('./calendar-format.js').CalGroup, id: string, open: boolean, now: number,
   *   linkTarget: string|undefined, ontoggle: () => void }} */
  let { group, id, open, now, linkTarget, ontoggle } = $props();

  // Keeps the rows in the page while the fold closes. A folded group holds no rows at all.
  let held = $state(untrack(() => open));

  const first = $derived(group.events[0]);
  const text = $derived(strings.calendar.groups[group.kind]);
  const category = $derived(CATEGORY_CLASSES[group.category] ?? CATEGORY_CLASSES.other);
  const next = $derived(group.events.find((e) => !e.all_day && startMs(e) > now));
  const left = $derived(next ? startMs(next) - now : 0);
  const past = $derived(group.events.every((e) => startMs(e) - now <= (e.all_day ? -SOON_MS : 0)));

  /** Rows are held from the click until the fold has closed. With motion off nothing has to wait. */
  function toggle() {
    held = !REDUCED_MOTION.matches;
    ontoggle();
  }

  /** The fold stopped moving: if it closed, its rows leave the page. @param {TransitionEvent} e */
  function settle(e) {
    if (open || e.target !== e.currentTarget) return;
    held = false;
  }
</script>

<!-- Stays under the day heading while its rows scroll, so a long group can be folded from anywhere in it;
     short screens have no room for a second stuck heading. The fade for past events is on the parts,
     not the button: a stuck heading has to stay solid. -->
<button type="button" onclick={toggle} aria-expanded={open} aria-controls="{PANEL_PREFIX}{id}"
  class="group grid w-full cursor-pointer grid-cols-[3.5rem_minmax(0,1fr)_auto] gap-x-3 px-4 py-2.5 text-left transition-colors duration-150 hover:bg-card-hover focus-visible:bg-card-hover focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red short:py-1.5 lg:px-6 {open ? 'sticky top-7 z-5 bg-card short:static' : 'bg-app'}">
  <time class="pt-0.5 text-right font-mono tabular-nums transition-opacity duration-500 {first.all_day ? 'text-[0.6rem] font-bold uppercase tracking-wider text-muted' : 'text-xs text-white/90'} {past ? 'opacity-45' : ''}" datetime={first.starts_at}>
    {first.all_day ? strings.calendar.allDay : timeLabel(first)}
  </time>

  <span class="min-w-0 transition-opacity duration-500 {past ? 'opacity-45' : ''}">
    <span class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      {#if group.impact}{@render impactMeter(group.impact)}{/if}
      <span class="rounded-sm border px-1.5 py-px text-[0.68rem] font-bold uppercase tracking-wider {category.text} {category.border}">{text.label}</span>
      <span class="flex items-center gap-1 font-mono tabular-nums text-white/70 group-hover:text-white">
        {group.events.length} {text.count}
        <ChevronDown class="size-3.5 transition-transform duration-300 ease-out group-active:scale-75 motion-reduce:transition-none {open ? 'rotate-180' : ''}" aria-hidden="true" />
      </span>
    </span>
    <span class="mt-1 block text-[0.95rem] leading-snug font-medium text-white line-clamp-2">
      {group.names.join(NAME_SEPARATOR)}{#if group.more}<span class="text-muted">{NAME_SEPARATOR}+{group.more} {strings.calendar.more}</span>{/if}
    </span>
  </span>

  <!-- Same fixed slot as a single row, counting down to the group's next event -->
  <span class="w-[4.75rem] pt-0.5 text-right font-mono text-[0.68rem] tabular-nums text-neon-red max-[359px]:hidden">
    {#if next && left < SOON_MS}{countdown(left)}{/if}
  </span>
</button>

<div id="{PANEL_PREFIX}{id}" ontransitionend={settle}
  class="grid transition-[grid-template-rows,opacity,visibility] duration-300 ease-out motion-reduce:transition-none {open ? 'grid-rows-[1fr] opacity-100' : 'invisible grid-rows-[0fr] opacity-0'}">
  <div class="relative min-h-0 overflow-hidden">
    {#if open || held}
      <!-- A rail in the group's color ties the rows to their heading without moving them out of line -->
      <span class="absolute inset-y-0 left-0 w-0.5 opacity-50 {category.bg}" aria-hidden="true"></span>
      <ol>
        {#each group.events as event (event.uid)}
          <li class="border-t border-line"><CalendarRow {event} {now} {linkTarget} /></li>
        {/each}
      </ol>
    {/if}
  </div>
</div>
