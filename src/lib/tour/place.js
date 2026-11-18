// Where a tour bubble goes, given the box it points at. Pure arithmetic, so Blurb.svelte only draws.
//
//   place(rect, size, side, at, bounds, taken)
//     rect    the target's box: { left, top, right, bottom, width, height }
//     size    { w, h } of the bubble, once drawn
//     side    the side of the target the bubble prefers: 'top' | 'bottom' | 'left' | 'right'
//     at      where along the target's edge the tail lands, 0 to 1
//     bounds  { w, h } of the window
//     taken   [{ left, top, w, h }] where other bubbles already are
//   returns { left, top, side, tx, ty }: the bubble's corner, the side it ended up on, and the
//   tail's point inside the bubble.
//
// The preferred side is tried first. When the bubble does not fit there (a phone is narrow) or
// would cover another bubble, the other sides are tried in turn: the first that fits clear of the
// others wins, then the first that fits at all. If none does, the preferred side is kept and the
// bubble is pushed inside the window.

const GAP = 14; // room between target and bubble: the tail lives here
const MARGIN = 8; // the bubble never comes closer than this to the window's edge
const TAIL_INSET = 16; // the tail stays this far from the bubble's corners
const TAIL_ALONG = 0.3; // the tail sits a third of the way along the bubble, not dead centre
const SIDES = ['bottom', 'top', 'right', 'left'];
const OPPOSITE = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' };

const clamp = (/** @type {number} */ v, /** @type {number} */ lo, /** @type {number} */ hi) => Math.min(Math.max(v, lo), Math.max(lo, hi));

/** The point on the target's edge the tail comes from, on a given side. */
function anchor(rect, side, at) {
  if (side === 'top') return { x: rect.left + rect.width * at, y: rect.top };
  if (side === 'bottom') return { x: rect.left + rect.width * at, y: rect.bottom };
  if (side === 'left') return { x: rect.left, y: rect.top + rect.height * at };
  return { x: rect.right, y: rect.top + rect.height * at };
}

function attempt(rect, size, side, at, bounds) {
  const a = anchor(rect, side, at);
  const upright = side === 'top' || side === 'bottom';
  const left = upright
    ? clamp(a.x - size.w * TAIL_ALONG, MARGIN, bounds.w - size.w - MARGIN)
    : side === 'right' ? a.x + GAP : a.x - GAP - size.w;
  const top = upright
    ? side === 'bottom' ? a.y + GAP : a.y - GAP - size.h
    : clamp(a.y - size.h * TAIL_ALONG, MARGIN, bounds.h - size.h - MARGIN);
  const fits = left >= MARGIN && left + size.w <= bounds.w - MARGIN && top >= MARGIN && top + size.h <= bounds.h - MARGIN;
  return { left, top, side, fits, tx: clamp(a.x - left, TAIL_INSET, size.w - TAIL_INSET), ty: clamp(a.y - top, TAIL_INSET, size.h - TAIL_INSET) };
}

const overlaps = (a, b) => a.left < b.left + b.w && a.left + a.w > b.left && a.top < b.top + b.h && a.top + a.h > b.top;

export function place(rect, size, side, at, bounds, taken = []) {
  const order = [side, OPPOSITE[side], ...SIDES].filter((s, i, all) => all.indexOf(s) === i);
  const tries = order.map((s) => attempt(rect, size, s, at, bounds));
  const fitting = tries.filter((p) => p.fits);
  const clear = fitting.find((p) => !taken.some((t) => overlaps({ left: p.left, top: p.top, w: size.w, h: size.h }, t)));
  const found = clear ?? fitting[0];
  if (found) return found;
  // Nowhere fits (a very small window): keep the preferred side, inside the window at least.
  const first = tries[0];
  first.left = clamp(first.left, MARGIN, bounds.w - size.w - MARGIN);
  first.top = clamp(first.top, MARGIN, bounds.h - size.h - MARGIN);
  return first;
}

/** Whether a target is on screen enough to point at: it has a size and some of it is inside the window. */
export function onScreen(rect, bounds) {
  if (!rect || rect.width === 0 || rect.height === 0) return false;
  return rect.bottom > MARGIN && rect.top < bounds.h - MARGIN && rect.right > MARGIN && rect.left < bounds.w - MARGIN;
}
