// Pan and zoom for the world map, in the map's own units (its SVG viewBox). The map can never be
// dragged off its frame or zoomed out past the whole world, so nothing on it becomes unreachable.

const MIN_SCALE = 1;
const MAX_SCALE = 10;

export class MapView {
  scale = $state(MIN_SCALE);
  x = $state(0);
  y = $state(0);

  #width;
  #height;

  /** @param {number} width @param {number} height the map's viewBox size */
  constructor(width, height) {
    this.#width = width;
    this.#height = height;
  }

  get zoomed() {
    return this.scale > MIN_SCALE;
  }

  get canZoomIn() {
    return this.scale < MAX_SCALE;
  }

  /** The SVG transform for the layer that holds the countries. */
  get transform() {
    return `translate(${this.x} ${this.y}) scale(${this.scale})`;
  }

  /** Keeps the frame covered: the map's edges never come inside it. */
  #settle(scale, x, y) {
    this.scale = scale;
    this.x = Math.min(0, Math.max(this.#width * (1 - scale), x));
    this.y = Math.min(0, Math.max(this.#height * (1 - scale), y));
  }

  /**
   * Zooms by a factor, keeping the point under (cx, cy) where it is.
   * @param {number} factor above 1 zooms in @param {number} cx @param {number} cy map units; the middle when left out
   */
  zoomAt(factor, cx = this.#width / 2, cy = this.#height / 2) {
    const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, this.scale * factor));
    const ratio = scale / this.scale;
    this.#settle(scale, cx - (cx - this.x) * ratio, cy - (cy - this.y) * ratio);
  }

  /** @param {number} dx @param {number} dy map units */
  pan(dx, dy) {
    this.#settle(this.scale, this.x + dx, this.y + dy);
  }

  reset() {
    this.#settle(MIN_SCALE, 0, 0);
  }
}
