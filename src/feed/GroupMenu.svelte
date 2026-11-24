<script>
  /**
   * Puts the open story into a storyline of your own: pick one you made, or start a new one by naming it.
   * A story already in a storyline shows where, with a way out. The grouping is in storylines.svelte.js.
   */
  import { storylines } from './storylines.svelte.js';
  import strings from './stories.json';

  /** @type {{ storyId: number }} */
  let { storyId } = $props();

  const L = strings.lines;
  const NEW = '__new__';
  const FIELD = 'spring min-w-0 rounded-sm border border-line bg-card px-1.5 py-1 text-xs text-white focus-visible:outline focus-visible:outline-neon-red';

  let choice = $state('');
  let title = $state('');
  const line = $derived(storylines.of(storyId));

  function pick(/** @type {string} */ value) {
    choice = value;
    if (value && value !== NEW) {
      storylines.add(value, storyId);
      choice = '';
    }
  }

  function create(/** @type {SubmitEvent} */ e) {
    e.preventDefault();
    storylines.create(title, storyId);
    title = '';
    choice = '';
  }
</script>

<div class="flex flex-wrap items-center gap-2 px-4 py-2 text-xs lg:px-6">
  {#if line}
    <span class="text-white/80">{L.inLine.replace('{title}', line.title)}</span>
    <button type="button" onclick={() => storylines.ungroup(storyId)}
      class="spring rounded-sm border border-line px-1.5 py-0.5 font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:outline focus-visible:outline-neon-red">{L.ungroup}</button>
  {:else if choice === NEW}
    <form onsubmit={create} class="flex min-w-0 flex-1 items-center gap-1.5">
      <input bind:value={title} placeholder={L.titlePlaceholder} aria-label={L.titleLabel} maxlength="80" class="{FIELD} flex-1" />
      <button type="submit" class="spring rounded-sm bg-neon-red px-2 py-1 font-bold tracking-wider text-app uppercase hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{L.create}</button>
    </form>
  {:else}
    <select class={FIELD} value={choice} onchange={(e) => pick(e.currentTarget.value)} aria-label={L.group}>
      <option value="">{L.groupInto}</option>
      <option value={NEW}>{L.newLine}</option>
      {#each storylines.list as l (l.id)}<option value={l.id}>{l.title}</option>{/each}
    </select>
  {/if}
</div>
