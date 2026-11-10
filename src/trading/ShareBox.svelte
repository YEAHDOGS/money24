<script>
  /**
   * Paper-trade a post right now: paste (or share from the X app) a status link, or paste the callout text.
   * The runner reads the post, plans it with the source's rules as of its post time, and the plan opens.
   */
  import strings from './strings.json';

  /** @type {{ onShare: (input: string) => Promise<string>, initial?: string }} */
  let { onShare, initial = '' } = $props();

  let input = $state(initial);
  let busy = $state(false);
  let message = $state('');
  let failed = $state(false);

  async function submit(/** @type {SubmitEvent} */ event) {
    event.preventDefault();
    const value = input.trim();
    if (!value || busy) return;
    busy = true;
    message = '';
    try {
      message = await onShare(value);
      failed = false;
      input = '';
    } catch (err) {
      message = err instanceof Error ? err.message : String(err);
      failed = true;
    } finally {
      busy = false;
    }
  }
</script>

<form class="border-b border-line px-4 py-3 short:py-2 lg:px-6" onsubmit={submit}>
  <label for="share-input" class="text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.shareLabel}</label>
  <div class="mt-1.5 flex gap-2">
    <input
      id="share-input"
      bind:value={input}
      placeholder={strings.sharePlaceholder}
      autocomplete="off"
      spellcheck="false"
      class="min-w-0 flex-1 rounded-sm border border-line bg-card px-3 py-2 font-mono text-sm text-white placeholder:text-muted focus:border-neon-red focus:outline-none"
    />
    <button
      type="submit"
      disabled={busy || !input.trim()}
      class="spring shrink-0 rounded-sm bg-neon-red px-3 py-2 text-xs font-bold uppercase tracking-wider text-app hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-white disabled:opacity-40"
    >{busy ? strings.shareBusy : strings.shareButton}</button>
  </div>
  <p class="mt-1.5 min-h-4 text-xs {failed ? 'text-bad' : 'text-muted'}" role="status">{message}</p>
</form>
