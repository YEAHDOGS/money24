<script>
  /**
   * The world, shaded by how loud each country is. Drag to move, pinch or scroll to zoom, or use
   * the buttons. The loudest places pulse; an open story draws arcs from where it is broadcast to
   * the places it is about. The ranked list beside the map is its keyboard and screen reader twin.
   */
  import Plus from '@lucide/svelte/icons/plus';
  import Minus from '@lucide/svelte/icons/minus';
  import Maximize from '@lucide/svelte/icons/maximize';
  import strings from './stories.json';
  import { MapView } from './map-view.svelte.js';
  import { untrack } from 'svelte';

  const ZOOM_STEP = 1.6;
  const WHEEL_STEP = 1.0015; // per wheel pixel
  const DRAG_PX = 5; // a press that moves less than this is a click on a country
  const BASE_OPACITY = 0.1;
  const HEAT_OPACITY = 0.85;
  const DOT_RADIUS = 3;
  const PULSES = 4;
  const PULSE_RADIUS = 7;
  const ARC_LIFT = 0.22; // how far an arc bows, as a share of its length
  const BUTTON = 'spring grid size-8 place-items-center border border-line bg-app/90 text-white/80 hover:text-white focus-visible:outline focus-visible:outline-neon-red disabled:opacity-30';

  /**
   * @type {{
   *   world: { width: number, height: number, countries: { id: string, name: string, d: string, x: number, y: number }[] },
   *   heat: Record<string, number>,
   *   selected: string,
   *   arcs: [string, string][],
   *   onPick: (id: string) => void,
   *   onHover: (id: string) => void,
   * }} heat: 0 to 1 per country id. arcs: pairs of country ids to join.
   */
  let { world, heat, selected, arcs, onPick, onHover } = $props();

  // The map's viewBox size never changes, and the view keeps its pan and zoom: it is made once, from the first world.
  const view = untrack(() => new MapView(world.width, world.height));
  const points = $derived(Object.fromEntries(world.countries.map((c) => [c.id, c])));
  const loudest = $derived(Object.entries(heat).sort((a, b) => b[1] - a[1]).slice(0, PULSES).map(([id]) => points[id]).filter(Boolean));
  const curves = $derived(arcs.map(([from, to]) => [points[from], points[to]]).filter(([a, b]) => a && b && a !== b).map(([a, b]) => {
    const [mx, my, dx, dy] = [(a.x + b.x) / 2, (a.y + b.y) / 2, b.x - a.x, b.y - a.y];
    return { key: `${a.id}-${b.id}`, d: `M${a.x} ${a.y}Q${mx + dy * ARC_LIFT} ${my - Math.abs(dx) * ARC_LIFT} ${b.x} ${b.y}` };
  }));

  /** @type {SVGSVGElement | undefined} */
  let svg = $state();
  /** @type {Map<number, {x: number, y: number}>} fingers and the mouse, in map units */
  const pointers = new Map();
  let moved = 0;
  let dragging = $state(false);

  /** A screen point in map units. @param {{clientX: number, clientY: number}} e */
  function mapPoint(e) {
    const matrix = svg?.getScreenCTM();
    if (!matrix) return { x: 0, y: 0 };
    const p = new DOMPoint(e.clientX, e.clientY).matrixTransform(matrix.inverse());
    return { x: p.x, y: p.y };
  }

  /** @param {PointerEvent} e */
  function onDown(e) {
    pointers.set(e.pointerId, mapPoint(e));
    if (pointers.size === 1) moved = 0;
  }

  /** One pointer drags; two pinch around the point between them. @param {PointerEvent} e */
  function onMove(e) {
    const last = pointers.get(e.pointerId);
    if (!last) return;
    const now = mapPoint(e);
    if (pointers.size === 1) {
      moved += Math.hypot(e.movementX, e.movementY);
      if (moved < DRAG_PX) return;
      if (!dragging) {
        dragging = true;
        svg?.setPointerCapture(e.pointerId);
      }
      view.pan(now.x - last.x, now.y - last.y);
      pointers.set(e.pointerId, now);
      return;
    }
    const other = [...pointers.entries()].find(([id]) => id !== e.pointerId)?.[1];
    if (!other) return;
    dragging = true;
    view.zoomAt(Math.hypot(now.x - other.x, now.y - other.y) / Math.max(1, Math.hypot(last.x - other.x, last.y - other.y)), (now.x + other.x) / 2, (now.y + other.y) / 2);
    pointers.set(e.pointerId, now);
  }

  /** @param {PointerEvent} e */
  function onUp(e) {
    pointers.delete(e.pointerId);
    if (pointers.size) return;
    // The click that ends a drag belongs to the drag, not to the country under it.
    setTimeout(() => { dragging = false; });
  }

  /** The wheel zooms on the pointer. Registered by hand: it has to be able to stop the page scrolling. */
  function wheelZoom(/** @type {SVGSVGElement} */ node) {
    const onWheel = (/** @type {WheelEvent} */ e) => {
      e.preventDefault();
      const at = mapPoint(e);
      view.zoomAt(WHEEL_STEP ** -e.deltaY, at.x, at.y);
    };
    node.addEventListener('wheel', onWheel, { passive: false });
    return () => node.removeEventListener('wheel', onWheel);
  }

  /** @param {string} id */
  function pick(id) {
    if (!dragging) onPick(id);
  }
</script>

<!-- Countries are picked with a pointer here and with the keyboard in the ranked list beside the map. -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="relative h-full w-full overflow-hidden">
  <svg
    bind:this={svg}
    {@attach wheelZoom}
    viewBox="0 0 {world.width} {world.height}"
    preserveAspectRatio="xMidYMid meet"
    role="img"
    aria-label={strings.map.mapLabel}
    class="h-full w-full touch-none select-none {dragging ? 'cursor-grabbing' : 'cursor-grab'}"
    onpointerdown={onDown}
    onpointermove={onMove}
    onpointerup={onUp}
    onpointercancel={onUp}
    onpointerleave={() => onHover('')}
  >
    <g transform={view.transform}>
      {#each world.countries as c (c.id)}
        {@const h = heat[c.id] ?? 0}
        {#if c.d}
          <path
            d={c.d}
            class="map-land {h ? 'fill-neon-red' : 'fill-white'} {selected === c.id ? 'stroke-white' : 'stroke-line'}"
            style:fill-opacity={h ? BASE_OPACITY + h * HEAT_OPACITY : 0.05}
            stroke-width={selected === c.id ? 1.5 : 0.5}
            vector-effect="non-scaling-stroke"
            onclick={() => pick(c.id)}
            onpointerenter={() => onHover(c.id)}
          />
        {:else}
          <circle cx={c.x} cy={c.y} r={DOT_RADIUS / view.scale} class="map-land {h ? 'fill-neon-red' : 'fill-white'} {selected === c.id ? 'stroke-white' : 'stroke-line'}"
            style:fill-opacity={h ? BASE_OPACITY + h * HEAT_OPACITY : 0.2} stroke-width="0.5" vector-effect="non-scaling-stroke"
            onclick={() => pick(c.id)} onpointerenter={() => onHover(c.id)} />
        {/if}
      {/each}

      <!-- The loudest places pulse -->
      {#each loudest as c, i (c.id)}
        <circle cx={c.x} cy={c.y} r={PULSE_RADIUS / view.scale} class="map-pulse pointer-events-none fill-none stroke-white" stroke-width="1" vector-effect="non-scaling-stroke" style:--i={i} />
      {/each}

      <!-- An open story: from where it is broadcast to the places it is about -->
      {#each curves as curve (curve.key)}
        <path d={curve.d} class="map-arc pointer-events-none fill-none stroke-white" stroke-width="1.25" stroke-linecap="round" vector-effect="non-scaling-stroke" />
      {/each}
    </g>
  </svg>

  <div class="absolute right-2 bottom-2 grid gap-px" role="group" aria-label={strings.map.zoomIn}>
    <button type="button" class={BUTTON} onclick={() => view.zoomAt(ZOOM_STEP)} disabled={!view.canZoomIn} aria-label={strings.map.zoomIn}><Plus class="size-4" aria-hidden="true" /></button>
    <button type="button" class={BUTTON} onclick={() => view.zoomAt(1 / ZOOM_STEP)} disabled={!view.zoomed} aria-label={strings.map.zoomOut}><Minus class="size-4" aria-hidden="true" /></button>
    <button type="button" class={BUTTON} onclick={() => view.reset()} disabled={!view.zoomed} aria-label={strings.map.reset}><Maximize class="size-4" aria-hidden="true" /></button>
  </div>
</div>
