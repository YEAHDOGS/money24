<script>
  /**
   * Today on the trading calendar: one slim stripe under the quotes that names the day (a weekend,
   * a half day, an options expiry), says which markets are open and counts down to the stock
   * session's next open or close. Pressing it drops a panel over the page with the day's labels,
   * how the desk treats a day like this, each market's state and the week ahead. The panel lies
   * over the page, so opening it moves nothing. The days come from the runner (engine/calendar).
   */
  import strings from './strings.json';
  import { fill } from '../plays/play-format.js';
  import { until } from './desk.js';
  import CalendarDays from '@lucide/svelte/icons/calendar-days';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';

  const PANEL_ID = 'day-panel';
  const KEY_CLOSE = 'Escape';
  const DAY_ZONE = 'UTC'; // a day arrives as a plain date: it is formatted as that date, in no one's zone
  /** @type {Intl.DateTimeFormatOptions} */
  const DATE = { weekday: 'short', month: 'short', day: 'numeric', timeZone: DAY_ZONE };
  const SESSION_HALF = 'half';
  const SESSION_CLOSED = 'closed';
  /** A market's dot: trading, trading thinly or for half a day, or shut. Never color alone: the state is written beside it. */
  const STATE_DOT = { open: 'bg-ok', always: 'bg-ok', half: 'bg-warn', thin: 'bg-warn', closed: 'bg-muted/60' };
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase tv:text-sm';

  /** @type {{ days: any[], now: number }} days: today first, then the days ahead. now: epoch ms. */
  let { days, now } = $props();

  let open = $state(false);
  /** @type {HTMLElement | undefined} */
  let root = $state();

  const today = $derived(days[0]);
  const ahead = $derived(days.slice(1));
  /** Inside the stock session right now: it counts down to the close. Otherwise to the next open. */
  const inSession = $derived(Boolean(today?.opens && today?.closes && now >= Date.parse(today.opens) && now < Date.parse(today.closes)));
  const countdown = $derived.by(() => {
    if (!today) return '';
    const time = until(inSession ? today.closes : today.next_open, now);
    return time ? fill(inSession ? strings.day.stocksCloseIn : strings.day.stocksOpenIn, { time }) : '';
  });

  const dateLabel = (/** @type {string} */ date) => new Date(`${date}T00:00:00Z`).toLocaleDateString('en-US', DATE);

  // While the panel is open, Escape and a press anywhere else close it.
  $effect(() => {
    if (!open) return;
    const onKey = (/** @type {KeyboardEvent} */ event) => { if (event.key === KEY_CLOSE) open = false; };
    const onPress = (/** @type {PointerEvent} */ event) => { if (!root?.contains(/** @type {Node} */ (event.target))) open = false; };
    window.addEventListener('keydown', onKey);
    window.addEventListener('pointerdown', onPress);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('pointerdown', onPress);
    };
  });
</script>

<section bind:this={root} aria-label={strings.day.label} class="relative z-20 flex h-8 shrink-0 items-stretch border-b border-line short:h-7 tv:h-12">
  {#if today}
    <button type="button" aria-expanded={open} aria-controls={PANEL_ID} title={open ? strings.day.close : strings.day.open} onclick={() => { open = !open; }}
      class="spring flex min-w-0 flex-1 items-center gap-2 px-3 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red lg:px-4 tv:gap-4">
      <CalendarDays class="size-3.5 shrink-0 text-neon-red tv:size-5" aria-hidden="true" />
      <span class="shrink-0 font-mono text-xs font-bold text-white tabular-nums tv:text-base">{dateLabel(today.date)}</span>
      <span class="min-w-0 truncate text-xs font-semibold text-white/85 tv:text-base">{today.label}</span>

      <!-- Each market's state for the day. From tablet up; on phones they are in the panel. -->
      <span class="ml-auto hidden shrink-0 items-center gap-3 lg:flex tv:gap-5">
        {#each today.markets ?? [] as m (m.market)}
          <span class="flex items-center gap-1.5 text-[0.68rem] font-semibold tracking-wide text-white/70 tv:text-sm" title="{strings.day.states[m.state] ?? m.state}: {m.note}">
            <span class="size-1.5 rounded-full {STATE_DOT[m.state] ?? 'bg-muted/60'}" aria-hidden="true"></span>
            {strings.day.markets[m.market] ?? m.market}<span class="sr-only"> {strings.day.states[m.state] ?? m.state}</span>
          </span>
        {/each}
      </span>

      <span class="shrink-0 font-mono text-[0.68rem] text-muted tabular-nums max-lg:ml-auto max-[419px]:hidden tv:text-sm">{countdown}</span>
      <ChevronDown class="size-3.5 shrink-0 text-white/60 transition-transform duration-200 ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-reduce:transition-none tv:size-5 {open ? 'rotate-180' : ''}" aria-hidden="true" />
    </button>

    {#if open}
      <div id={PANEL_ID} class="day-panel list-scroll absolute inset-x-0 top-full grid max-h-[70dvh] gap-px overflow-y-auto border-b border-line bg-line shadow-2xl shadow-app md:grid-cols-3">
        <div class="bg-card p-3 lg:p-4">
          <h3 class={LABEL}>{strings.day.today}</h3>
          <ul class="mt-2 flex flex-col gap-2">
            {#each today.tags ?? [] as tag, i (tag.id)}
              <li class="cal-rise text-xs leading-snug text-white/70 tv:text-base" style:--i={i}><span class="font-bold text-white">{tag.name}.</span> {tag.note}</li>
            {/each}
          </ul>
          <h3 class="mt-4 {LABEL}">{strings.day.playbook}</h3>
          <ul class="mt-2 flex flex-col gap-1.5">
            {#each today.playbook ?? [] as line, i (line)}
              <li class="cal-rise border-l-2 border-neon-red pl-2 text-xs leading-snug text-white/85 tv:text-base" style:--i={i}>{line}</li>
            {/each}
          </ul>
        </div>

        <div class="bg-card p-3 lg:p-4">
          <h3 class={LABEL}>{strings.day.marketsTitle}</h3>
          <ul class="mt-2 flex flex-col gap-2">
            {#each today.markets ?? [] as m, i (m.market)}
              <li class="cal-rise grid grid-cols-[auto_minmax(0,1fr)] items-baseline gap-x-2 text-xs leading-snug tv:text-base" style:--i={i}>
                <span class="flex items-center gap-1.5 font-bold text-white">
                  <span class="size-1.5 rounded-full {STATE_DOT[m.state] ?? 'bg-muted/60'}" aria-hidden="true"></span>{strings.day.markets[m.market] ?? m.market}
                </span>
                <span class="text-white/70"><span class="font-semibold text-white/90">{strings.day.states[m.state] ?? m.state}.</span> {m.note}</span>
              </li>
            {/each}
          </ul>
        </div>

        <div class="bg-card p-3 lg:p-4">
          <h3 class={LABEL}>{strings.day.ahead}</h3>
          <ul class="mt-2 flex flex-col divide-y divide-line">
            {#each ahead as day, i (day.date)}
              <li class="cal-rise grid grid-cols-[5.5rem_minmax(0,1fr)_auto] items-baseline gap-x-2 py-1.5 text-xs tv:grid-cols-[8rem_minmax(0,1fr)_auto] tv:text-base" style:--i={i}>
                <span class="font-mono font-bold text-white tabular-nums">{dateLabel(day.date)}</span>
                <span class="min-w-0 truncate text-white/75" title={day.label}>{day.label}</span>
                <span class="text-[0.62rem] font-bold tracking-wider uppercase {day.session === SESSION_CLOSED ? 'text-muted' : day.session === SESSION_HALF ? 'text-warn' : 'text-ok'}">{strings.day.states[day.session] ?? day.session}</span>
              </li>
            {/each}
          </ul>
        </div>
      </div>
    {/if}
  {:else}
    <!-- No calendar yet (still connecting, an older runner, a recording without one): the stripe keeps its place. -->
    <p class="flex min-w-0 flex-1 items-center gap-2 px-3 text-xs text-muted lg:px-4 tv:text-base">
      <CalendarDays class="size-3.5 shrink-0 tv:size-5" aria-hidden="true" />
      <span class="font-mono font-bold text-white/80 tabular-nums">{new Date(now).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}</span>
      <span class="truncate">{strings.day.unknown}</span>
    </p>
  {/if}
</section>
