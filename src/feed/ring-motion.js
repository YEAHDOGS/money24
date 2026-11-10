// Poll ring choreography. The ring fills clockwise in step with the engine's clock, spins while the
// engine checks its sources, then pops when the round's report lands and starts filling again.
// Everything runs on the Web Animations API with transforms, opacity and stroke offset only.

export const RING_SIZE = 36;
export const RING_RADIUS = 15;
export const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;
const NEARLY_FULL = 0.95;
const SPIN_MS = 650;
const POP_MS = 560;
const BURST_MS = 700;
const BURST_SCALE = 2.3;
const REPORT_WAIT_MS = 16000; // never hold a full ring longer than the engine's longest report wait
const SPRING = 'cubic-bezier(0.34, 1.56, 0.64, 1)';
const TURN = 'rotate(360deg)';
const NO_TURN = 'rotate(0deg)';

/**
 * Drives one ring.
 * @param {{ root: SVGElement, fill: SVGCircleElement, head: SVGGElement, burst: SVGCircleElement, calm: boolean }} parts
 */
export class RingMotion {
  #parts;
  #fill = null;
  #head = null;
  #spin = null;
  #queued = null;
  #holdTimer = 0;

  constructor(parts) {
    this.#parts = parts;
  }

  /** A new plan from the engine. If the ring is (nearly) full, hold it until the report pops it. */
  plan(plan) {
    if (this.#isFull()) {
      this.#queued = plan;
      this.#startSpin();
      clearTimeout(this.#holdTimer);
      this.#holdTimer = setTimeout(() => this.#release(), REPORT_WAIT_MS);
      return;
    }
    this.#run(plan);
  }

  /** The round's report arrived: pop, then carry on with the held plan. */
  report() {
    this.#pop();
    this.#release();
  }

  stop() {
    clearTimeout(this.#holdTimer);
    for (const a of [this.#fill, this.#head, this.#spin]) a?.cancel();
  }

  #release() {
    clearTimeout(this.#holdTimer);
    this.#spin?.cancel();
    this.#spin = null;
    if (!this.#queued) return;
    const plan = this.#queued;
    this.#queued = null;
    this.#run(plan);
  }

  #isFull() {
    if (this.#queued) return true;
    if (!this.#fill) return false;
    const duration = Number(this.#fill.effect?.getTiming().duration) || 0;
    return this.#fill.playState === 'finished' || (duration > 0 && Number(this.#fill.currentTime) / duration >= NEARLY_FULL);
  }

  #run(plan) {
    const total = Date.parse(plan.next_at) - Date.parse(plan.last_at);
    if (!(total > 0)) return;
    const elapsed = Math.min(total, Math.max(0, Date.now() + plan.offsetMs - Date.parse(plan.last_at)));
    this.#fill?.cancel();
    this.#head?.cancel();
    const timing = { duration: total, easing: 'linear', fill: 'forwards' };
    this.#fill = this.#parts.fill.animate([{ strokeDashoffset: RING_CIRCUMFERENCE }, { strokeDashoffset: 0 }], timing);
    this.#head = this.#parts.head.animate([{ transform: NO_TURN }, { transform: TURN }], timing);
    this.#fill.currentTime = elapsed;
    this.#head.currentTime = elapsed;
    // Full before the engine answers: keep turning so it reads as "checking".
    this.#fill.onfinish = () => this.#startSpin();
  }

  #startSpin() {
    if (this.#spin || this.#parts.calm) return;
    this.#spin = this.#parts.head.animate([{ transform: NO_TURN }, { transform: TURN }], { duration: SPIN_MS, iterations: Infinity });
  }

  #pop() {
    if (this.#parts.calm) return;
    this.#parts.root.animate(
      [{ transform: 'scale(1)' }, { transform: 'scale(1.28)' }, { transform: 'scale(0.94)' }, { transform: 'scale(1)' }],
      { duration: POP_MS, easing: SPRING },
    );
    this.#parts.burst.animate(
      [{ transform: 'scale(1)', opacity: 0.85 }, { transform: `scale(${BURST_SCALE})`, opacity: 0 }],
      { duration: BURST_MS, easing: 'ease-out' },
    );
  }
}
