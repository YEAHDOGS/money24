<script>
  /**
   * Where the trading runner lives. On the desktop app and local dev the defaults just work.
   * From a phone, point it at your runner (VPS or home PC) and paste its webhook secret once.
   */
  import strings from './strings.json';

  /** @type {{ url: string, secret: string, onSave: (url: string, secret: string) => void | Promise<void> }} */
  let { url, secret, onSave } = $props();

  let open = $state(false);
  let draftUrl = $state('');
  let draftSecret = $state('');
  let saving = $state(false);
  let error = $state('');

  function toggle() {
    open = !open;
    draftUrl = url;
    draftSecret = secret;
    error = '';
  }

  async function save(/** @type {SubmitEvent} */ event) {
    event.preventDefault();
    if (saving) return;
    saving = true;
    error = '';
    try {
      await onSave(draftUrl.trim(), draftSecret.trim());
      draftSecret = '';
      open = false;
    } catch (reason) {
      error = reason.message;
    } finally {
      saving = false;
    }
  }
</script>

<div class="border-t border-line px-4 py-2">
  <button type="button" onclick={toggle} aria-expanded={open}
    class="spring flex w-full items-center justify-between text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted hover:text-white focus-visible:text-white focus-visible:outline-none">
    {strings.runnerSettings}
    <span class="font-mono normal-case tracking-normal">{url.replace(/^https?:\/\//, '')}</span>
  </button>
  {#if open}
    <form class="mt-2 grid gap-2 pb-1" onsubmit={save}>
      <label class="grid gap-1 text-xs text-muted">{strings.runnerUrl}
        <input bind:value={draftUrl} inputmode="url" autocomplete="off" spellcheck="false"
          class="rounded-sm border border-line bg-card px-2 py-1.5 font-mono text-xs text-white focus:border-neon-red focus:outline-none" />
      </label>
      <label class="grid gap-1 text-xs text-muted">{strings.runnerSecret}
        <input bind:value={draftSecret} type="password" autocomplete="off"
          class="rounded-sm border border-line bg-card px-2 py-1.5 font-mono text-xs text-white focus:border-neon-red focus:outline-none" />
      </label>
      <p class="text-[0.7rem] leading-snug text-muted">{strings.runnerHint}</p>
      {#if error}<p role="alert" class="text-xs text-bad">{error}</p>{/if}
      <button type="submit" disabled={saving} class="spring rounded-sm border border-neon-red px-2 py-1.5 text-xs font-bold uppercase tracking-wider text-neon-red hover:bg-neon-red/10 focus-visible:outline-none focus-visible:bg-neon-red/10 disabled:opacity-50">{strings.save}</button>
    </form>
  {/if}
</div>
