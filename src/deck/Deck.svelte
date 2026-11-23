<script>
  /**
   * The strategy deck: every way Money 24 can play, as cards. Filter by kind, search, add, edit, bench, copy,
   * remove, and import or export the deck as JSON. Filed under the Plays tab. Cards live in deck-store.svelte.js.
   */
  import AppHeader from '../lib/AppHeader.svelte';
  import AppFooter from '../lib/AppFooter.svelte';
  import { demoFirst, DEMO_STATUS } from '../lib/demo.js';
  import DeckCard from './DeckCard.svelte';
  import CardEditor from './CardEditor.svelte';
  import { deck, KINDS } from './deck-store.svelte.js';
  import strings from './strings.json';
  import Search from '@lucide/svelte/icons/search';
  import Plus from '@lucide/svelte/icons/plus';
  import Download from '@lucide/svelte/icons/download';
  import Upload from '@lucide/svelte/icons/upload';

  const PAGE = 'deck';
  const ALL = 'all';
  const STAGGER_MAX = 12;
  const CHIP = 'spring rounded-sm border px-2 py-0.5 text-[0.68rem] font-bold tracking-wider uppercase focus-visible:outline-none focus-visible:border-neon-red';
  const TOOL = 'spring flex items-center gap-1 rounded-sm border border-line px-2 py-1 text-[0.68rem] font-bold tracking-wider text-white/70 uppercase hover:text-white focus-visible:border-neon-red focus-visible:outline-none';

  let kind = $state(ALL);
  let query = $state('');
  /** The card in the editor: null closed, a card to edit it, an empty one for a new card. @type {any} */
  let editing = $state(null);
  let message = $state('');

  const wanted = $derived(query.trim().toLowerCase());
  const shown = $derived(deck.cards.filter((c) =>
    (kind === ALL || c.kind === kind) && (!wanted || `${c.name} ${c.asset} ${c.when} ${c.then} ${c.notes}`.toLowerCase().includes(wanted))));

  function save(/** @type {any} */ card) {
    deck.put(card);
    editing = null;
  }

  function download() {
    const url = URL.createObjectURL(new Blob([deck.exportJson()], { type: 'application/json' }));
    Object.assign(document.createElement('a'), { href: url, download: 'money24-deck.json' }).click();
    URL.revokeObjectURL(url);
  }

  async function upload(/** @type {Event} */ e) {
    const input = /** @type {HTMLInputElement} */ (e.target);
    const n = deck.importJson(await (input.files?.[0]?.text() ?? ''));
    message = n < 0 ? strings.importBad : strings.imported.replace('{n}', String(n));
    input.value = '';
  }
</script>

<div class="flex h-dvh min-w-0 flex-col">
  <AppHeader page={PAGE} status={demoFirst() ? DEMO_STATUS : ''} />
  <main class="mx-auto flex min-h-0 w-full max-w-7xl flex-1 flex-col xl:border-x xl:border-line ultrawide:max-w-none">
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-line px-4 py-3 lg:px-6">
      <div class="min-w-0">
        <h1 class="text-lg font-bold tv:text-3xl">{strings.title}</h1>
        <p class="text-xs text-muted tv:text-base">{strings.sub}</p>
      </div>
      <div class="ml-auto flex flex-wrap items-center gap-1.5">
        <label class="spring flex items-center gap-1.5 rounded-sm border border-line bg-card px-2 py-1 focus-within:border-neon-red">
          <Search class="size-3.5 text-muted" aria-hidden="true" />
          <input type="search" bind:value={query} placeholder={strings.search} aria-label={strings.search} class="w-36 bg-transparent text-xs text-white placeholder:text-muted focus:outline-none" />
        </label>
        <button type="button" class={TOOL} onclick={download}><Download class="size-3" aria-hidden="true" />{strings.export}</button>
        <label class="{TOOL} cursor-pointer focus-within:border-neon-red"><Upload class="size-3" aria-hidden="true" />{strings.import}<input type="file" accept="application/json,.json" class="sr-only" onchange={upload} /></label>
        <button type="button" onclick={() => { editing = { name: '', kind: 'rule', asset: '', when: '', then: '', notes: '', by: 'you', enabled: true, id: '' }; }}
          class="spring flex items-center gap-1 rounded-sm bg-neon-red px-2.5 py-1 text-[0.68rem] font-bold tracking-wider text-app uppercase hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red"><Plus class="size-3" aria-hidden="true" />{strings.add}</button>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-1.5 border-b border-line px-4 py-2 lg:px-6" role="group" aria-label={strings.title}>
      {#each [ALL, ...KINDS] as k (k)}
        <button type="button" aria-pressed={kind === k} onclick={() => { kind = k; }} class="{CHIP} {kind === k ? 'border-neon-red bg-white/5 text-white' : 'border-line text-white/55 hover:text-white'}">
          {k === ALL ? strings.all : strings.kinds[k]} <span class="font-mono tabular-nums text-muted">{k === ALL ? deck.cards.length : deck.cards.filter((c) => c.kind === k).length}</span>
        </button>
      {/each}
      {#if message}<p class="ml-auto text-xs text-muted" role="status">{message}</p>{/if}
    </div>

    <!-- The deck scrolls inside its own box; the page never does. -->
    <div class="list-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto p-4 lg:p-6">
      {#if shown.length}
        <ul class="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-3 tv:grid-cols-[repeat(auto-fill,minmax(22rem,1fr))] tv:gap-5">
          {#each shown as card, i (card.id)}
            <li class="rise flex" style="--i:{Math.min(i, STAGGER_MAX)}">
              <div class="flex-1">
                <DeckCard {card} onEdit={() => { editing = card; }} onCopy={() => deck.copy(card.id)}
                  onRemove={() => confirm(strings.removeConfirm) && deck.remove(card.id)} onToggle={() => deck.toggle(card.id)} />
              </div>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="text-sm text-muted">{strings.empty}</p>
      {/if}
    </div>
  </main>
  <AppFooter />
</div>

{#if editing}<CardEditor card={editing} onSave={save} onClose={() => { editing = null; }} />{/if}

<style>
  /* Cards deal in with a short stagger; the card inside stays free to tilt. */
  .rise {
    animation: rise 360ms cubic-bezier(0.22, 1, 0.36, 1) both;
    animation-delay: calc(var(--i, 0) * 45ms);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .rise {
      animation: none;
    }
  }
</style>
