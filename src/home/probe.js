// Svelte action for the equity chart: reports where the pointer is over it, so the chart can read
// out that moment. A mouse hovers; a finger presses and slides.

/**
 * @param {HTMLElement} node
 * @param {(x: number | null) => void} report pixels from the node's left edge, or null once the pointer is gone
 */
export function probe(node, report) {
  const onMove = (/** @type {PointerEvent} */ event) => report(event.clientX - node.getBoundingClientRect().left);
  const onGone = () => report(null);

  node.addEventListener('pointerdown', onMove);
  node.addEventListener('pointermove', onMove);
  node.addEventListener('pointerleave', onGone);
  node.addEventListener('pointercancel', onGone);
  return {
    destroy() {
      node.removeEventListener('pointerdown', onMove);
      node.removeEventListener('pointermove', onMove);
      node.removeEventListener('pointerleave', onGone);
      node.removeEventListener('pointercancel', onGone);
    },
  };
}
