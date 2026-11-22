<script>
  /**
   * The pocket's tabs: one marker slides under the chosen tab, the way the desk's tabs do. A tab can
   * carry a count (open plays) in its own tone (red for problems). Arrow keys, Home and End move
   * between tabs; the chosen one is the only one in the tab order, as a tab list should be.
   */
  const PERCENT = 100;
  const KEY_STEP = { ArrowRight: 1, ArrowLeft: -1 };
  const KEY_HOME = 'Home';
  const KEY_END = 'End';

  /**
   * @type {{ tabs: { id: string, label: string, count?: number, tone?: string }[], active: string,
   *   label: string, onSelect: (id: string) => void }}
   */
  let { tabs, active, label, onSelect } = $props();

  const index = $derived(Math.max(0, tabs.findIndex((t) => t.id === active)));
  /** @type {HTMLButtonElement[]} */
  const buttons = [];

  function onKey(/** @type {KeyboardEvent} */ event) {
    const step = KEY_STEP[/** @type {keyof typeof KEY_STEP} */ (event.key)];
    let next = -1;
    if (step) next = (index + step + tabs.length) % tabs.length;
    if (event.key === KEY_HOME) next = 0;
    if (event.key === KEY_END) next = tabs.length - 1;
    if (next < 0) return;
    event.preventDefault();
    onSelect(tabs[next].id);
    buttons[next]?.focus();
  }
</script>

<div role="tablist" aria-label={label} tabindex="-1" onkeydown={onKey}
  class="relative grid shrink-0 border-b border-line" style:grid-template-columns="repeat({tabs.length}, minmax(0, 1fr))">
  <span aria-hidden="true" class="pointer-events-none absolute inset-y-0 left-0 bg-white/6 transition-transform duration-300 ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-reduce:transition-none"
    style:width="{PERCENT / tabs.length}%" style:transform="translateX({index * PERCENT}%)">
    <span class="absolute inset-x-2 bottom-0 h-0.5 bg-neon-red"></span>
  </span>
  {#each tabs as tab, i (tab.id)}
    {@const selected = tab.id === active}
    <button bind:this={buttons[i]} type="button" role="tab" id="pocket-tab-{tab.id}" aria-selected={selected} aria-controls="pocket-panel"
      tabindex={selected ? 0 : -1} onclick={() => onSelect(tab.id)}
      class="spring relative flex min-w-0 items-center justify-center gap-1 px-1 py-2 text-[0.64rem] font-bold tracking-wider whitespace-nowrap uppercase focus-visible:outline focus-visible:-outline-offset-2 focus-visible:outline-neon-red tv:py-3 tv:text-base {selected ? 'text-white' : 'text-white/55 hover:text-white'}">
      <span class="truncate">{tab.label}</span>
      {#if tab.count}<span class="font-mono tabular-nums {tab.tone || 'text-white/70'}">{tab.count}</span>{/if}
    </button>
  {/each}
</div>
