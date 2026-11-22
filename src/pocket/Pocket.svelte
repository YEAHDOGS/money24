<script>
  /**
   * The pocket: Money 24 at a glance, in the small window over the tray (src/tauri/src/pocket.rs).
   * A verdict at the top, then the problems in plain words (only when there are any), the services,
   * the recorders, the desks and the open positions soonest to close, read every ten seconds
   * (pocket-client.svelte.js). At the bottom, the button that opens the full desk in the app's window.
   * The tray's "Stop Money 24…" opens it on the stop question instead (#stop).
   */
  import strings from './strings.json';
  import { fill, pnl } from '../plays/play-format.js';
  import { PocketClient } from './pocket-client.svelte.js';
  import { TONE } from './pocket-model.js';
  import { TONE_CLASSES } from './tones.js';
  import { COMMAND, native } from './native.js';
  import Section from './Section.svelte';
  import StatusRow from './StatusRow.svelte';
  import PlayRow from './PlayRow.svelte';
  import StopPanel from './StopPanel.svelte';
  import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
  import { onMount } from 'svelte';

  const STOP_HASH = '#stop';
  const ESCAPE = 'Escape';
  const EMPTY = ' ';
  const DESK_PAGE = `${import.meta.env.BASE_URL}home.html`;
  const CLOCK = new Intl.DateTimeFormat('en-US', { hour: 'numeric', minute: '2-digit', second: '2-digit' });

  const client = new PocketClient();
  let confirming = $state(location.hash === STOP_HASH);
  /** @type {{ attached: boolean, supervisor_known: boolean } | null} what the app says about itself */
  let info = $state(null);

  const mode = $derived(!native ? strings.mode.browser : info?.attached === false ? strings.mode.own : strings.mode.attached);

  onMount(() => {
    const onHash = () => (confirming = location.hash === STOP_HASH);
    const onKey = (/** @type {KeyboardEvent} */ event) => {
      if (event.key !== ESCAPE) return;
      if (confirming) cancel();
      else native?.invoke(COMMAND.hide);
    };
    window.addEventListener('hashchange', onHash);
    window.addEventListener('keydown', onKey);
    native?.invoke(COMMAND.info).then((answer) => (info = answer), () => {});
    client.start();
    return () => {
      window.removeEventListener('hashchange', onHash);
      window.removeEventListener('keydown', onKey);
      client.stop();
    };
  });

  function cancel() {
    history.replaceState(null, '', location.pathname);
    confirming = false;
    native?.invoke(COMMAND.hide);
  }

  function openDesk(/** @type {MouseEvent} */ event) {
    if (!native) return; // a plain link in a browser
    event.preventDefault();
    native.invoke(COMMAND.openDesk);
  }

  const money = (/** @type {number | null} */ n) => (n === null ? 'text-muted' : n < 0 ? 'text-bad' : 'text-ok');
</script>

<main class="flex h-dvh flex-col overflow-hidden bg-app text-white">
  <header class="pocket-in flex shrink-0 items-center gap-2 border-b border-line px-3 py-2 tv:py-4">
    <span class="text-base leading-none font-extrabold tracking-tight whitespace-nowrap tv:text-3xl">{strings.brand}<span class="text-neon-red">{strings.brandAccent}</span></span>
    <span class="rounded-sm border border-paper px-1 py-px text-[0.54rem] font-bold tracking-wider text-paper uppercase tv:text-xs" title={strings.paperNote}>{strings.paper}</span>
    <p class="ml-auto flex min-w-0 items-center gap-1.5 text-[0.72rem] font-semibold tv:text-lg {TONE_CLASSES[client.overall.tone].text}" role="status">
      <span class="relative size-1.5 shrink-0 rounded-full {TONE_CLASSES[client.overall.tone].dot} {client.overall.tone === TONE.ok ? 'live-dot' : ''}" aria-hidden="true"></span>
      <span class="truncate">{client.overall.text}</span>
    </p>
  </header>

  {#if confirming}
    <StopPanel onCancel={cancel} />
  {:else}
    <div class="list-scroll min-h-0 flex-1 overflow-y-auto overscroll-contain">
      {#key client.shown}
        {#if !client.snap}
          <p class="px-3 py-4 text-sm text-muted">{strings.loading}</p>
        {:else}
          <div class="sm:grid sm:grid-cols-2 sm:items-start xl:grid-cols-3">
            {#if client.problems.length}
              <div class="sm:col-span-full">
                <Section id="pocket-problems" title={strings.sections.problems} count={client.problems.length} tone="text-bad">
                  {#each client.problems as problem, i (problem.id)}
                    <li class="cal-rise flex gap-2 px-3 py-1 text-[0.74rem] leading-snug text-white/90 tv:text-base" style:--i={i}>
                      <span class="mt-[0.45em] size-1.5 shrink-0 rounded-full bg-bad" aria-hidden="true"></span>
                      <span class="min-w-0 break-words">{problem.text}</span>
                    </li>
                  {/each}
                </Section>
              </div>
            {/if}
            <Section id="pocket-services" title={strings.sections.services}>
              {#each client.services as row, i (row.id)}<StatusRow {row} index={i} />{/each}
            </Section>
            <Section id="pocket-recorders" title={strings.sections.recorders}>
              {#each client.recorders as row, i (row.id)}<StatusRow {row} index={i} />{/each}
            </Section>
            <Section id="pocket-desks" title={strings.sections.desks}>
              {#each client.desks as row, i (row.id)}
                <StatusRow {row} index={i}>
                  {#snippet aside()}
                    <span class="text-right font-mono tabular-nums">
                      <span class="block text-[0.78rem] font-bold tv:text-lg {money(row.pnl)}">{row.pnl === null ? EMPTY : pnl(row.pnl)}</span>
                      <span class="block text-[0.58rem] text-muted tv:text-xs">{row.pnlNote || EMPTY}</span>
                    </span>
                  {/snippet}
                </StatusRow>
              {/each}
            </Section>
            <Section id="pocket-plays" title={strings.sections.plays} count={client.plays.length}>
              {#each client.plays as play, i (play.id)}
                <PlayRow {play} index={i} />
              {:else}
                <li class="px-3 py-1.5 text-[0.74rem] text-muted tv:text-base">{strings.plays.empty}</li>
              {/each}
            </Section>
          </div>
        {/if}
      {/key}
    </div>
  {/if}

  <footer class="pocket-in flex shrink-0 items-center gap-2 border-t border-line px-3 py-2 tv:py-4">
    <p class="min-w-0 flex-1 font-mono text-[0.6rem] leading-snug text-muted tv:text-sm">
      <span class="block truncate" title={mode}>{mode}</span>
      <span class="block truncate tabular-nums">{client.snap ? fill(strings.updated, { time: CLOCK.format(client.now) }) : EMPTY}</span>
    </p>
    <a href={DESK_PAGE} onclick={openDesk}
      class="spring flex shrink-0 items-center gap-1 rounded-sm border border-neon-red bg-neon-red/10 px-2.5 py-1.5 text-[0.7rem] font-bold tracking-wider text-white uppercase hover:bg-neon-red/20 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:text-base">
      {strings.openDesk}<ArrowUpRight class="size-3.5 tv:size-5" aria-hidden="true" />
    </a>
  </footer>
</main>
