<script>
  /**
   * The search box: type a ticker or a name, pick a match with the pointer, or with the arrow keys
   * and Enter. Enter with nothing highlighted opens exactly what was typed.
   */
  import Search from '@lucide/svelte/icons/search';
  import strings from './strings.json';

  const LIST_ID = 'symbol-matches';

  /** @type {{ matches: any[], onSearch: (q: string) => void, onPick: (symbol: string) => void }} */
  let { matches, onSearch, onPick } = $props();

  let query = $state('');
  let active = $state(-1);
  let open = $state(false);

  function pick(/** @type {string} */ symbol) {
    if (!symbol) return;
    onPick(symbol);
    query = '';
    open = false;
    active = -1;
  }

  function onKey(/** @type {KeyboardEvent} */ event) {
    if (event.key === 'Escape') {
      open = false;
      return;
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      pick(matches[active]?.symbol ?? query.trim());
      return;
    }
    const step = { ArrowDown: 1, ArrowUp: -1 }[event.key];
    if (!step || !matches.length) return;
    event.preventDefault();
    active = (active + step + matches.length) % matches.length;
  }
</script>

<div class="relative min-w-0 flex-1" role="search">
  <label class="flex items-center gap-2 rounded-sm border border-line bg-card px-2.5 focus-within:border-neon-red">
    <Search class="size-4 shrink-0 text-muted" aria-hidden="true" />
    <span class="sr-only">{strings.searchLabel}</span>
    <input
      bind:value={query}
      oninput={() => { open = true; active = -1; onSearch(query); }}
      onfocus={() => { open = true; }}
      onblur={() => { open = false; }}
      onkeydown={onKey}
      placeholder={strings.searchPlaceholder}
      role="combobox"
      aria-expanded={open && matches.length > 0}
      aria-controls={LIST_ID}
      aria-autocomplete="list"
      autocomplete="off"
      spellcheck="false"
      class="min-w-0 flex-1 bg-transparent py-1.5 text-sm text-white placeholder:text-muted focus:outline-none short:py-1"
    />
  </label>
  <ul id={LIST_ID} role="listbox" class="list-scroll absolute top-full right-0 left-0 z-30 mt-1 max-h-[60dvh] overflow-y-auto rounded-sm border border-line bg-card shadow-lg {open && query.trim() ? '' : 'hidden'}">
    {#each matches as m, i (m.symbol)}
      <li role="option" aria-selected={active === i}>
        <!-- pointerdown, not click: it lands before the input's blur closes the list -->
        <button type="button" tabindex="-1" onpointerdown={(e) => { e.preventDefault(); pick(m.symbol); }}
          class="spring flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-white/5 {active === i ? 'bg-white/8' : ''}">
          <span class="w-20 shrink-0 truncate font-mono text-sm font-bold text-white">{m.symbol}</span>
          <span class="min-w-0 flex-1 truncate text-sm text-white/70">{m.name}</span>
          <span class="shrink-0 text-[0.62rem] font-bold uppercase tracking-wider text-muted max-sm:hidden">{m.type} · {m.exchange}</span>
        </button>
      </li>
    {:else}
      <li class="px-3 py-2 text-sm text-muted">{strings.searchEmpty}</li>
    {/each}
  </ul>
</div>
