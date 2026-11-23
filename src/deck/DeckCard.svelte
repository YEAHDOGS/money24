<script>
  /**
   * One strategy as a card: a colored top band with its kind, the name, the asset, WHEN and THEN, who wrote it,
   * and whether it is in play. A card tilts toward the pointer (transform only, so the grid never reflows).
   */
  import strings from './strings.json';
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import Activity from '@lucide/svelte/icons/activity';
  import Zap from '@lucide/svelte/icons/zap';
  import Layers from '@lucide/svelte/icons/layers';
  import Target from '@lucide/svelte/icons/target';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Copy from '@lucide/svelte/icons/copy';
  import Trash2 from '@lucide/svelte/icons/trash-2';

  /** @type {{ card: import('./deck-store.svelte.js').Card, onEdit: () => void, onCopy: () => void, onRemove: () => void, onToggle: () => void }} */
  let { card, onEdit, onCopy, onRemove, onToggle } = $props();

  const KIND = {
    ai: { icon: Sparkles, band: 'bg-kind-insider/20 text-kind-insider' },
    rule: { icon: Activity, band: 'bg-kind-news/20 text-kind-news' },
    catalyst: { icon: Zap, band: 'bg-kind-macro/20 text-kind-macro' },
    spread: { icon: Layers, band: 'bg-kind-central-bank/20 text-kind-central-bank' },
    prediction: { icon: Target, band: 'bg-kind-filing/20 text-kind-filing' },
  };
  const TILT_DEG = 5;
  const ACTION = 'spring grid size-7 place-items-center rounded-sm text-white/60 hover:bg-white/5 hover:text-white focus-visible:outline focus-visible:outline-neon-red';

  const kind = $derived(KIND[card.kind] ?? KIND.rule);
  const Icon = $derived(kind.icon);
  /** @type {HTMLElement | undefined} */
  let face = $state();

  function tilt(/** @type {PointerEvent} */ e) {
    if (!face || e.pointerType !== 'mouse') return;
    const r = face.getBoundingClientRect();
    const x = (e.clientX - r.left) / r.width - 0.5;
    const y = (e.clientY - r.top) / r.height - 0.5;
    face.style.transform = `perspective(700px) rotateY(${x * TILT_DEG * 2}deg) rotateX(${-y * TILT_DEG * 2}deg)`;
  }
  function level() {
    if (face) face.style.transform = '';
  }
</script>

<article bind:this={face} onpointermove={tilt} onpointerleave={level} aria-label={card.name}
  class="card flex h-full min-h-72 flex-col overflow-hidden rounded-md border bg-card transition-[transform,opacity,border-color] duration-150 ease-out {card.enabled ? 'border-line' : 'border-dashed border-line opacity-60'}">
  <div class="flex items-center gap-2 px-3 py-2 {kind.band}">
    <Icon class="size-4 shrink-0" aria-hidden="true" />
    <span class="text-[0.65rem] font-bold tracking-[0.18em] uppercase">{strings.kinds[card.kind]}</span>
    <span class="ml-auto font-mono text-xs font-bold">{card.asset}</span>
  </div>

  <div class="flex min-h-0 flex-1 flex-col gap-2 px-3 py-3">
    <h3 class="text-base leading-tight font-bold">{card.name}</h3>
    <p class="text-xs leading-snug"><span class="font-bold tracking-wider text-neon-red uppercase">{strings.editor.when}</span> <span class="text-white/85">{card.when}</span></p>
    <p class="text-xs leading-snug"><span class="font-bold tracking-wider text-ok uppercase">{strings.editor.then}</span> <span class="text-white/85">{card.then}</span></p>
    {#if card.notes}<p class="line-clamp-3 text-[0.7rem] leading-snug text-muted">{card.notes}</p>{/if}
    <p class="mt-auto pt-2 text-[0.65rem] leading-snug text-muted">{strings.ungraded}</p>
  </div>

  <div class="flex items-center gap-1 border-t border-line px-2 py-1.5">
    <button type="button" role="switch" aria-checked={card.enabled} onclick={onToggle}
      class="spring rounded-sm px-2 py-1 text-[0.65rem] font-bold tracking-wider uppercase focus-visible:outline focus-visible:outline-neon-red {card.enabled ? 'bg-ok/15 text-ok' : 'text-muted hover:text-white'}">{card.enabled ? strings.on : strings.off}</button>
    <span class="ml-1 text-[0.65rem] tracking-wider text-muted uppercase">{strings.by[card.by]}</span>
    <span class="ml-auto flex">
      <button type="button" class={ACTION} title={strings.edit} aria-label="{strings.edit} {card.name}" onclick={onEdit}><Pencil class="size-3.5" aria-hidden="true" /></button>
      <button type="button" class={ACTION} title={strings.copy} aria-label="{strings.copy} {card.name}" onclick={onCopy}><Copy class="size-3.5" aria-hidden="true" /></button>
      <button type="button" class={ACTION} title={strings.remove} aria-label="{strings.remove} {card.name}" onclick={onRemove}><Trash2 class="size-3.5" aria-hidden="true" /></button>
    </span>
  </div>
</article>

<style>
  .card {
    transform-style: preserve-3d;
  }
  @media (prefers-reduced-motion: reduce) {
    .card {
      transition: none;
    }
  }
</style>
