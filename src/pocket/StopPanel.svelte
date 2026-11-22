<script>
  /**
   * The question the tray's "Stop Money 24…" asks first, in the pocket: what stops, then Stop or
   * Cancel. Stop asks the app to run forever.js --stop; the supervisor then closes the app as well.
   */
  import strings from './strings.json';
  import { fill } from '../plays/play-format.js';
  import { COMMAND, native } from './native.js';
  import Power from '@lucide/svelte/icons/power';

  /** @type {{ onCancel: () => void }} */
  let { onCancel } = $props();

  const t = strings.stop;
  let stopping = $state(false);
  let error = $state('');
  /** @type {HTMLButtonElement | undefined} */
  let cancelButton = $state();

  // Cancel takes the focus, so Enter or Space never stops everything by accident.
  $effect(() => cancelButton?.focus());

  async function stop() {
    if (!native || stopping) return;
    stopping = true;
    error = '';
    try {
      await native.invoke(COMMAND.stop);
    } catch (err) {
      stopping = false;
      error = fill(t.failed, { error: String(err) });
    }
  }
</script>

<div class="stop-in flex min-h-0 flex-1 flex-col justify-center gap-3 overflow-y-auto px-5 py-4" role="alertdialog" aria-labelledby="stop-title" aria-describedby="stop-body">
  <span class="grid size-9 shrink-0 place-items-center rounded-full border border-bad/60 text-bad" aria-hidden="true"><Power class="size-4" /></span>
  <h2 id="stop-title" class="text-lg font-bold text-white tv:text-2xl">{t.title}</h2>
  <p id="stop-body" class="text-[0.78rem] leading-relaxed text-white/75 tv:text-base">{t.body}</p>
  <p class="min-h-[1.2em] font-mono text-[0.66rem] text-bad" role="status">{error}</p>
  <div class="flex flex-wrap gap-2">
    <button type="button" onclick={stop} disabled={!native || stopping}
      class="spring rounded-sm border border-bad bg-bad/15 px-3 py-1.5 text-xs font-bold tracking-wider text-bad uppercase hover:bg-bad/25 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red disabled:opacity-50 tv:text-base">
      {stopping ? t.stopping : t.yes}
    </button>
    <button type="button" onclick={onCancel} bind:this={cancelButton}
      class="spring rounded-sm border border-line px-3 py-1.5 text-xs font-bold tracking-wider text-white/80 uppercase hover:bg-white/5 hover:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:text-base">
      {t.no}
    </button>
  </div>
</div>
