// The news banner's state: which story is showing, whether it changes by itself, and the gestures
// that change it (arrow keys, a sideways swipe). NewsCarousel.svelte only draws and calls in.
// It changes by itself unless the reader stopped it, is pointing at it or has focus inside it,
// asked for reduced motion, or the tab is in the background.

const ADVANCE_MS = 7000;
const SWIPE_PX = 40; // a sideways move at least this long is a swipe
const KEY_BACK = 'ArrowLeft';
const KEY_FORWARD = 'ArrowRight';
export const BACK = -1;
export const FORWARD = 1;
const REDUCED_QUERY = '(prefers-reduced-motion: reduce)';
const SLIDE = 'article';
/** On the banner's arrow buttons: which way each one steps. */
export const STEP_ATTR = 'data-step';
export const STEP_BACK = 'previous';
export const STEP_FORWARD = 'next';
export const SIDE_HERE = 'here';
export const SIDE_BEFORE = 'before';
export const SIDE_AFTER = 'after';

export class Carousel {
  /** The slide showing, from zero. */
  index = $state(0);
  /** How many slides there are. */
  count = $state(0);
  /** The reader pressed pause. */
  stopped = $state(false);
  pointed = $state(false);
  focused = $state(false);
  reduced = $state(matchMedia(REDUCED_QUERY).matches);
  /** True while the banner changes by itself. */
  playing = $derived(this.count > 1 && !this.stopped && !this.pointed && !this.focused && !this.reduced);

  /** How many slides there are now. The one showing stays, unless it is gone. */
  size(/** @type {number} */ count) {
    this.count = count;
    if (this.index >= count) this.index = 0;
  }

  /** Shows a slide. Past either end it goes round to the other. */
  go(/** @type {number} */ index) {
    if (!this.count) return;
    this.index = ((index % this.count) + this.count) % this.count;
  }

  /**
   * Which side of the frame a slide waits on: the one showing is here; the half after it waits on
   * the right and the half before on the left, so going round the end keeps moving the same way.
   */
  side(/** @type {number} */ i) {
    const ahead = (i - this.index + this.count) % this.count;
    if (!ahead) return SIDE_HERE;
    return ahead <= this.count / 2 ? SIDE_AFTER : SIDE_BEFORE;
  }

  /** Changes the story on a timer while playing. Returns the function that stops it. */
  run() {
    const timer = setInterval(() => {
      if (!document.hidden) this.go(this.index + 1);
    }, ADVANCE_MS);
    return () => clearInterval(timer);
  }
}

/**
 * Svelte action for the whole banner: left and right arrow keys, a sideways swipe or drag, and
 * pausing while it is pointed at or holds focus. A swipe that ends on a link does not open it.
 * @param {HTMLElement} node  @param {Carousel} carousel
 */
export function gestures(node, carousel) {
  let startX = 0;
  let pressed = false;
  let swiped = false;
  const media = matchMedia(REDUCED_QUERY);

  const onKey = (/** @type {KeyboardEvent} */ event) => {
    if (event.key !== KEY_BACK && event.key !== KEY_FORWARD) return;
    event.preventDefault();
    const step = event.key === KEY_BACK ? STEP_BACK : STEP_FORWARD;
    // A slide that holds focus is about to hide: its arrow button takes the focus, so the keys keep working.
    if (document.activeElement?.closest(SLIDE)) /** @type {HTMLElement | null} */ (node.querySelector(`[${STEP_ATTR}="${step}"]`))?.focus();
    carousel.go(carousel.index + (step === STEP_BACK ? BACK : FORWARD));
  };
  const onDown = (/** @type {PointerEvent} */ event) => {
    pressed = true;
    swiped = false;
    startX = event.clientX;
  };
  const onUp = (/** @type {PointerEvent} */ event) => {
    if (!pressed) return;
    pressed = false;
    const moved = event.clientX - startX;
    if (Math.abs(moved) < SWIPE_PX) return;
    swiped = true;
    carousel.go(carousel.index + (moved < 0 ? FORWARD : BACK));
  };
  const onClick = (/** @type {MouseEvent} */ event) => {
    if (!swiped) return;
    swiped = false;
    event.preventDefault();
    event.stopPropagation();
  };
  const onEnter = () => { carousel.pointed = true; };
  const onLeave = () => { carousel.pointed = false; pressed = false; };
  const onFocusIn = () => { carousel.focused = true; };
  const onFocusOut = (/** @type {FocusEvent} */ event) => { carousel.focused = node.contains(/** @type {Node | null} */ (event.relatedTarget)); };
  const onMotion = () => { carousel.reduced = media.matches; };

  /** @type {[EventTarget, string, EventListener, boolean?][]} */
  const listeners = [
    [node, 'keydown', /** @type {EventListener} */ (onKey)],
    [node, 'pointerdown', /** @type {EventListener} */ (onDown)],
    [node, 'pointerup', /** @type {EventListener} */ (onUp)],
    [node, 'click', /** @type {EventListener} */ (onClick), true],
    [node, 'pointerenter', onEnter],
    [node, 'pointerleave', onLeave],
    [node, 'focusin', onFocusIn],
    [node, 'focusout', /** @type {EventListener} */ (onFocusOut)],
    [media, 'change', onMotion],
  ];
  for (const [target, type, handler, capture] of listeners) target.addEventListener(type, handler, capture);
  return {
    destroy() {
      for (const [target, type, handler, capture] of listeners) target.removeEventListener(type, handler, capture);
    },
  };
}
