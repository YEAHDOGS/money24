// Arithmetic for the quote ribbon. The ribbon is one list of quotes drawn several times in a row.
// Running, the row slides left by one copy's width and starts again. Held still, it is a row to
// scroll through, and moving between the two keeps every quote where it was.

const EXTRA_COPIES = 2; // copies past the ones that fill the view: one to slide over, one to step back into
const PX_PER_SECOND = 42; // how fast the ribbon runs
const STEP_SHARE = 0.6; // an arrow moves the row by this share of what is in view
const MS_PER_SECOND = 1000;

/** How many copies of the list it takes to run without a gap, however wide the screen. */
export function copiesFor(/** @type {number} */ viewWidth, /** @type {number} */ copyWidth) {
  if (!copyWidth) return EXTRA_COPIES;
  return Math.ceil(viewWidth / copyWidth) + EXTRA_COPIES;
}

/** Seconds one loop takes, so the speed is the same however many quotes there are. */
export function loopSeconds(/** @type {number} */ copyWidth) {
  return Math.max(1, copyWidth / PX_PER_SECOND);
}

/** The same place in the first copy: every copy shows the same quotes. */
export function wrap(/** @type {number} */ offset, /** @type {number} */ copyWidth) {
  if (!copyWidth) return 0;
  return ((offset % copyWidth) + copyWidth) % copyWidth;
}

/**
 * Where an arrow sends the row: from, where to jump first without anything moving on screen, and
 * to, where to glide. Stepping back from the start jumps one copy ahead first.
 * @param {number} offset  @param {number} direction -1 back, 1 forward
 * @param {number} viewWidth  @param {number} copyWidth
 */
export function stepTo(offset, direction, viewWidth, copyWidth) {
  const step = viewWidth * STEP_SHARE;
  const at = wrap(offset, copyWidth);
  const from = direction < 0 && at < step ? at + copyWidth : at;
  return { from, to: from + direction * step };
}

/** The animation delay that starts a loop part-way in, so the run picks up where the row was held. */
export function resumeDelay(/** @type {number} */ offset, /** @type {number} */ copyWidth) {
  if (!copyWidth) return '0ms';
  return `${-Math.round((wrap(offset, copyWidth) / copyWidth) * loopSeconds(copyWidth) * MS_PER_SECOND)}ms`;
}

const MOUSE = 'mouse';
const MAIN_BUTTON = 0;
const DRAG_SLOP_PX = 6; // a press that moves less than this is a click, not a drag

/**
 * Svelte action: dragging the row with a mouse. Touch scrolls it natively, so only a mouse is
 * followed. A drag that ends on a quote does not open it.
 * @param {HTMLElement} node
 * @param {{ grab: () => number, move: (offset: number) => void }} row grab: the drag begins, returns
 *   the row's offset; move: the offset the row should be at now.
 */
export function mouseDrag(node, row) {
  /** @type {{ x: number, offset: number, moved: boolean } | null} */
  let press = null;
  let dragged = false;

  const onDown = (/** @type {PointerEvent} */ event) => {
    dragged = false;
    if (event.pointerType !== MOUSE || event.button !== MAIN_BUTTON) return;
    press = { x: event.clientX, offset: 0, moved: false };
  };
  const onMove = (/** @type {PointerEvent} */ event) => {
    if (!press) return;
    const moved = event.clientX - press.x;
    if (press.moved) {
      row.move(press.offset - moved);
      return;
    }
    if (Math.abs(moved) < DRAG_SLOP_PX) return;
    press = { x: event.clientX, offset: row.grab(), moved: true };
    node.setPointerCapture(event.pointerId);
  };
  const onUp = () => {
    dragged = Boolean(press?.moved);
    press = null;
  };
  const onClick = (/** @type {MouseEvent} */ event) => {
    if (!dragged) return;
    dragged = false;
    event.preventDefault();
    event.stopPropagation();
  };

  node.addEventListener('pointerdown', onDown);
  node.addEventListener('pointermove', onMove);
  node.addEventListener('pointerup', onUp);
  node.addEventListener('pointercancel', onUp);
  node.addEventListener('click', onClick, true);
  return {
    destroy() {
      node.removeEventListener('pointerdown', onDown);
      node.removeEventListener('pointermove', onMove);
      node.removeEventListener('pointerup', onUp);
      node.removeEventListener('pointercancel', onUp);
      node.removeEventListener('click', onClick, true);
    },
  };
}
