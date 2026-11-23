<script>
  /**
   * The Pit: a bell in the header that counts, and a panel that lists, every new play and every change
   * of phase on the desk, newest first. The watcher is ./pit.svelte.js. Opening the panel reads them.
   */
  import strings from './pit.json';
  import { pit, describe } from './pit.svelte.js';
  import { demoFirst } from './demo.js';
  import HeaderPopover from './HeaderPopover.svelte';
  import Bell from '@lucide/svelte/icons/bell';

  const DEMO = demoFirst();
  const TIME_FORMAT = { hour: 'numeric', minute: '2-digit' };
  const SHOWN = 30;

  let permission = $state(typeof Notification === 'undefined' ? 'unsupported' : Notification.permission);

  $effect(() => {
    pit.start();
  });

  async function allow() {
    permission = await Notification.requestPermission();
  }
</script>

<HeaderPopover icon={Bell} label={strings.button} tour="pit" badge={pit.unread} onopen={() => pit.markRead()} width="w-80">
  <div class="flex items-center justify-between gap-2 border-b border-line px-3 py-2 tv:px-5 tv:py-3">
    <h2 class="text-[0.62rem] font-bold tracking-[0.2em] text-muted uppercase tv:text-sm">{strings.title}</h2>
    <span class="flex items-center gap-1.5 text-[0.62rem] font-bold tracking-wider uppercase {pit.live ? 'text-ok' : 'text-muted'}">
      <span class="size-1.5 rounded-full bg-current"></span>{strings.status[pit.live ? 'live' : DEMO ? 'demo' : 'offline']}
    </span>
  </div>

  {#if pit.events.length}
    <ul class="max-h-80 overflow-y-auto" aria-live="polite">
      {#each pit.events.slice(0, SHOWN) as e (e.id + e.at)}
        <li class="border-b border-line px-3 py-2 last:border-b-0 tv:px-5 tv:py-3">
          <p class="flex items-baseline justify-between gap-2 text-xs tv:text-base">
            <a href="plays.html?play={encodeURIComponent(e.play)}" class="truncate font-semibold text-white hover:underline focus-visible:underline focus-visible:outline-none">{e.title}</a>
            <time class="shrink-0 font-mono text-[0.65rem] text-muted tabular-nums" datetime={new Date(e.at).toISOString()}>{new Date(e.at).toLocaleTimeString([], TIME_FORMAT)}</time>
          </p>
          <p class="mt-0.5 text-xs text-white/70 tv:text-sm">{describe(e)}</p>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="px-3 py-4 text-xs leading-snug text-muted tv:px-5 tv:text-base">{DEMO ? strings.demo : strings.empty}</p>
  {/if}

  {#if !DEMO && permission !== 'unsupported'}
    <div class="border-t border-line px-3 py-2 text-xs tv:px-5 tv:text-sm">
      {#if permission === 'granted'}
        <p class="text-muted">{strings.alertsOn}</p>
      {:else if permission === 'denied'}
        <p class="text-muted">{strings.alertsBlocked}</p>
      {:else}
        <button type="button" onclick={allow}
          class="spring font-semibold text-white/80 underline decoration-neon-red/70 underline-offset-4 hover:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{strings.alertsOff}</button>
      {/if}
    </div>
  {/if}
</HeaderPopover>
