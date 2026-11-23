<script>
  /** The editor for one card, in a dialog. Closing it (Escape, Cancel, the backdrop) saves nothing. */
  import { untrack } from 'svelte';
  import strings from './strings.json';
  import { KINDS } from './deck-store.svelte.js';

  /** @type {{ card: any, onSave: (card: any) => void, onClose: () => void }} */
  let { card, onSave, onClose } = $props();

  const E = strings.editor;
  const FIELD = 'spring w-full rounded-sm border border-line bg-card px-2 py-1.5 text-sm text-white focus-visible:outline focus-visible:outline-neon-red';
  const LABEL = 'mt-3 block text-[0.65rem] font-bold tracking-[0.18em] text-muted uppercase';

  let draft = $state(untrack(() => ({ ...card })));
  /** @type {HTMLDialogElement | undefined} */
  let dialog = $state();

  $effect(() => {
    dialog?.showModal();
  });

  function submit(/** @type {SubmitEvent} */ e) {
    e.preventDefault();
    onSave({ ...draft });
  }
</script>

<dialog bind:this={dialog} onclose={onClose} onclick={(e) => e.target === dialog && onClose()} aria-labelledby="card-editor-title"
  class="m-auto w-[min(34rem,calc(100vw-1rem))] max-h-[calc(100dvh-1rem)] overflow-y-auto rounded-md border border-line bg-app p-0 text-white backdrop:bg-app/80">
  <form onsubmit={submit} class="p-4">
    <h2 id="card-editor-title" class="text-lg font-bold">{card.name ? E.editTitle : E.newTitle}</h2>
    <label class={LABEL}>{E.name}<input class="{FIELD} mt-1" bind:value={draft.name} maxlength="80" /></label>
    <div class="grid grid-cols-2 gap-3">
      <label class={LABEL}>{E.kind}
        <select class="{FIELD} mt-1" bind:value={draft.kind}>{#each KINDS as k (k)}<option value={k}>{strings.kinds[k]}</option>{/each}</select>
      </label>
      <label class={LABEL}>{E.asset}<input class="{FIELD} mt-1 uppercase" bind:value={draft.asset} maxlength="24" /></label>
    </div>
    <label class={LABEL}>{E.when}<textarea class="{FIELD} mt-1" rows="3" bind:value={draft.when} maxlength="600"></textarea></label>
    <label class={LABEL}>{E.then}<textarea class="{FIELD} mt-1" rows="3" bind:value={draft.then} maxlength="600"></textarea></label>
    <label class={LABEL}>{E.notes}<textarea class="{FIELD} mt-1" rows="2" bind:value={draft.notes} maxlength="600"></textarea></label>
    <div class="mt-4 flex justify-end gap-2">
      <button type="button" onclick={onClose} class="spring rounded-sm border border-line px-3 py-1.5 text-xs font-bold tracking-wider uppercase hover:bg-white/5 focus-visible:outline focus-visible:outline-neon-red">{E.cancel}</button>
      <button type="submit" class="spring rounded-sm bg-neon-red px-3.5 py-1.5 text-xs font-bold tracking-wider text-app uppercase hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{E.save}</button>
    </div>
  </form>
</dialog>
