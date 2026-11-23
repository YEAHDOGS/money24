<script>
  /**
   * An icon button at the end of the header and the panel it opens (the account menu, the Pit). The
   * panel floats, so opening it moves nothing; Escape and a press outside close it. Same size on every page.
   */
  /** @type {{ icon: any, label: string, tour?: string, badge?: number, onopen?: () => void, children: import('svelte').Snippet, width?: string }} */
  let { icon: Icon, label, tour = '', badge = 0, onopen, children, width = 'w-72' } = $props();

  const panelId = $props.id();
  let open = $state(false);
  /** @type {HTMLElement | undefined} */
  let root = $state();
  /** @type {HTMLButtonElement | undefined} */
  let button = $state();

  function toggle() {
    open = !open;
    if (open) onopen?.();
  }

  function close(/** @type {boolean} */ refocus = false) {
    open = false;
    if (refocus) button?.focus();
  }

  $effect(() => {
    if (!open) return;
    const away = (/** @type {PointerEvent} */ e) => {
      if (!root?.contains(/** @type {Node} */ (e.target))) close();
    };
    document.addEventListener('pointerdown', away);
    return () => document.removeEventListener('pointerdown', away);
  });
</script>

<svelte:window onkeydown={(e) => open && e.key === 'Escape' && close(true)} />

<div bind:this={root} data-tour={tour || undefined} class="relative flex h-8 shrink-0 items-center short:h-6">
  <button bind:this={button} type="button" aria-expanded={open} aria-controls={panelId} aria-label={badge ? `${label} (${badge})` : label} title={label}
    onclick={toggle}
    class="spring relative grid size-8 place-items-center rounded-sm text-white/70 hover:bg-white/5 hover:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red short:size-6 tv:size-12 {open ? 'bg-white/5 text-white' : ''}">
    <Icon class="size-5 tv:size-8" aria-hidden="true" />
    {#if badge}
      <span class="badge absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full bg-neon-red px-1 font-mono text-[0.6rem] leading-4 font-bold text-app tabular-nums" aria-hidden="true">{badge > 9 ? '9+' : badge}</span>
    {/if}
  </button>

  <!-- The panel floats over the page: opacity and transform only, so nothing under it moves. -->
  <div id={panelId} role="group" aria-label={label} inert={!open}
    class="panel absolute top-full right-0 z-50 mt-1.5 {width} max-w-[calc(100vw-1.5rem)] origin-top-right rounded-sm border border-line bg-card shadow-lg tv:w-[28rem] {open ? 'is-open' : ''}">
    {@render children()}
  </div>
</div>

<style>
  .panel {
    opacity: 0;
    transform: translateY(-4px) scale(0.97);
    visibility: hidden;
    transition: opacity 150ms ease, transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), visibility 0s linear 150ms;
  }
  .panel.is-open {
    opacity: 1;
    transform: none;
    visibility: visible;
    transition-delay: 0s;
  }
  /* A new count pops in; nothing around it moves. */
  .badge {
    animation: badge-pop 300ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }
  @keyframes badge-pop {
    from {
      transform: scale(0.4);
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .panel,
    .badge {
      transition: none;
      animation: none;
    }
  }
</style>
