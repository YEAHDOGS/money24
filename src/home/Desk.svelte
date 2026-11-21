<script>
  /**
   * The desk at work, for watching it run on its own: whether the crypto desk and the stock finder
   * are looking right now and when they look next, then either everything the desk did (each buy,
   * sale and refusal from the journal, newest first, with what it made or lost) or what its last
   * scan decided about every coin and why. Nothing the desk decides is hidden. A row that belongs
   * to a play opens that play. The rows come from desk.js; this only draws.
   */
  import strings from './strings.json';
  import { fill, clock, pnl } from '../plays/play-format.js';
  import { TONE, activity, until } from './desk.js';
  import { PLAYS_PAGE, PLAY_PARAM, pageLink } from './home-links.js';

  const ACTIVITY_MAX = 40; // rows shown; the trading desk page has the whole journal
  const TAB_ACTIVITY = 'activity';
  const TAB_SCAN = 'scan';
  const TABS = [TAB_ACTIVITY, TAB_SCAN];
  const PERCENT = 100;
  const STATE_LOOKING = 'looking';
  const OUTCOME_SKIPPED = 'skipped';
  const STATUS_OFFLINE = 'offline';
  const STATUS_LIVE = 'live';
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase tv:text-sm';
  const PAPER_CHIP = 'rounded-sm border border-paper px-1.5 py-px text-[0.6rem] font-bold tracking-wider text-paper uppercase tv:text-sm';
  const ROW = 'grid grid-cols-[auto_minmax(0,1fr)_auto] items-baseline gap-x-2.5 px-3 py-2 lg:px-4 tv:py-3';
  /** A row's mark: money in is the desk color; a result is green or red, and always carries its sign in the number. */
  const TONE_DOT = { [TONE.buy]: 'bg-paper', [TONE.win]: 'bg-ok', [TONE.loss]: 'bg-bad', [TONE.skip]: 'bg-muted/50' };

  /**
   * @type {{ journal: any[], crypto: any, finder: any, status: string, now: number }}
   * crypto, finder: each desk's last pass, null from a runner that has none. status: the runner's connection.
   */
  let { journal, crypto, finder, status, now } = $props();

  let tab = $state(TAB_ACTIVITY);

  const rows = $derived(activity(journal, ACTIVITY_MAX));
  const notes = $derived(crypto?.notes ?? []);
  const tabIndex = $derived(TABS.indexOf(tab));
  /** The two desks, for the header: a name, a state, and a countdown while it is looking. */
  const desks = $derived([
    { id: 'coins', name: strings.desk.coins, pass: crypto },
    { id: 'stocks', name: strings.desk.stocks, pass: finder },
  ].filter((d) => d.pass));

  function next(/** @type {any} */ pass) {
    const time = until(pass.next_at, now);
    return time ? fill(strings.desk.nextScan, { time }) : strings.desk.scanning;
  }
</script>

<section aria-labelledby="desk-title" class="flex min-h-0 shrink-0 flex-col border-b border-line ultrawide:border-b-0">
  <header class="flex flex-wrap items-center gap-x-2.5 gap-y-1.5 border-b border-line px-3 py-2 lg:px-4">
    <h2 id="desk-title" class={LABEL}>{strings.desk.title}</h2>
    <span class={PAPER_CHIP} title={strings.paperNote}>{strings.paper}</span>

    <!-- Each desk's state keeps its place whether it is looking or not, so the header never jumps. -->
    {#each desks as desk (desk.id)}
      {@const looking = desk.pass.state === STATE_LOOKING}
      <span class="flex items-center gap-1.5 font-mono text-[0.68rem] tabular-nums tv:text-sm" title={desk.pass.why || undefined}>
        <span class="relative size-1.5 shrink-0 rounded-full {looking ? 'live-dot bg-ok text-ok' : 'bg-muted/60'}" aria-hidden="true"></span>
        <span class="font-sans font-bold text-white">{desk.name}</span>
        <span class={looking ? 'text-ok' : 'text-white/70'}>{strings.desk.states[desk.pass.state] ?? desk.pass.state}</span>
        {#if looking}<span class="text-muted max-[419px]:hidden">· {next(desk.pass)}</span>{/if}
      </span>
    {/each}

    <!-- TABS: one marker slides between the two. -->
    <div role="group" aria-label={strings.desk.tabsLabel} class="relative ml-auto grid grid-cols-2 rounded-sm border border-line">
      <span aria-hidden="true" class="pointer-events-none absolute inset-y-0 left-0 w-1/2 bg-white/8 transition-transform duration-300 ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-reduce:transition-none"
        style:transform="translateX({tabIndex * PERCENT}%)">
        <span class="absolute inset-x-1.5 bottom-0 h-0.5 bg-neon-red"></span>
      </span>
      {#each TABS as t (t)}
        <button type="button" aria-pressed={tab === t} onclick={() => { tab = t; }}
          class="spring relative px-2.5 py-1 text-[0.68rem] font-bold tracking-wider whitespace-nowrap uppercase focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:px-4 tv:text-base {tab === t ? 'text-white' : 'text-white/55 hover:text-white'}">
          {strings.desk.tabs[t]}
        </button>
      {/each}
    </div>
  </header>

  <!-- Both lists have the same height, so changing tabs never moves what is under them. -->
  <ul class="list-scroll h-[22dvh] min-h-28 divide-y divide-line overflow-x-hidden overflow-y-auto landscape-phone:h-36 ultrawide:h-auto ultrawide:flex-1">
    {#if tab === TAB_ACTIVITY}
      {#each rows as row, i (row.id)}
        <li class="overflow-hidden">
          <svelte:element this={row.playId ? 'a' : 'div'} href={row.playId ? pageLink(PLAYS_PAGE, { [PLAY_PARAM]: row.playId }) : undefined} title={row.playId ? strings.desk.openPlay : undefined}
            class="cal-rise {ROW} {row.playId ? 'spring hover:bg-white/3 focus-visible:bg-white/5 focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red' : ''}" style:--i={Math.min(i, 12)}>
            <span class="size-1.5 shrink-0 translate-y-[-0.15em] rounded-full {TONE_DOT[row.tone]}" aria-hidden="true"></span>
            <span class="min-w-0">
              <span class="block truncate text-sm font-semibold tv:text-lg {row.tone === TONE.skip ? 'text-white/60' : 'text-white'}">{row.title}</span>
              <span class="block truncate font-mono text-[0.68rem] text-muted tabular-nums tv:text-sm">{row.detail}</span>
            </span>
            <span class="text-right font-mono tabular-nums">
              {#if row.pnl !== null}<span class="block text-sm font-bold tv:text-lg {row.pnl < 0 ? 'text-bad' : 'text-ok'}">{pnl(row.pnl)}</span>{/if}
              <span class="block text-[0.62rem] text-muted tv:text-sm">{clock(row.at)}</span>
            </span>
          </svelte:element>
        </li>
      {:else}
        <li class="px-3 py-4 text-sm text-muted lg:px-4">{status === STATUS_OFFLINE ? strings.offline : strings.desk.empty}</li>
      {/each}
    {:else}
      {#each notes as note, i (note.symbol)}
        {@const skipped = note.outcome === OUTCOME_SKIPPED}
        <li class="overflow-hidden">
          <div class="cal-rise {ROW}" style:--i={Math.min(i, 12)}>
            <span class="size-1.5 shrink-0 translate-y-[-0.15em] rounded-full {skipped ? 'bg-muted/50' : 'bg-paper'}" aria-hidden="true"></span>
            <span class="min-w-0">
              <span class="block truncate font-mono text-sm font-bold tv:text-lg {skipped ? 'text-white/70' : 'text-white'}">{note.symbol}</span>
              <span class="block text-[0.68rem] leading-snug text-muted tv:text-sm">{note.why}</span>
            </span>
            <span class="text-[0.62rem] font-bold tracking-wider uppercase tv:text-sm {skipped ? 'text-muted' : 'text-paper'}">{strings.desk.outcomes[note.outcome] ?? note.outcome}</span>
          </div>
        </li>
      {:else}
        <li class="px-3 py-4 text-sm text-muted lg:px-4">
          {#if status === STATUS_OFFLINE}{strings.offline}
          {:else if status === STATUS_LIVE && !crypto}{strings.desk.scanOld}
          {:else}{crypto?.why || strings.desk.scanEmpty}{/if}
        </li>
      {/each}
    {/if}
  </ul>
</section>
