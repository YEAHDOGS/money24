// Runs one loader again and again on a slow timer, and only while the page is on screen. A tab left
// in the background asks for nothing; when it comes back it catches up once, then keeps time again.

const VISIBILITY_EVENT = 'visibilitychange';

export class Poller {
  #load;
  #delay;
  #timer = 0;
  #stale = false;
  #stopped = true;

  /**
   * @param {() => Promise<void>} load never rejects: it records its own failure
   * @param {() => number} delay milliseconds until the next load; 0 means never again (a recording)
   */
  constructor(load, delay) {
    this.#load = load;
    this.#delay = delay;
  }

  async start() {
    this.#stopped = false;
    document.addEventListener(VISIBILITY_EVENT, this.#onVisible);
    await this.#run();
  }

  stop() {
    this.#stopped = true;
    clearTimeout(this.#timer);
    document.removeEventListener(VISIBILITY_EVENT, this.#onVisible);
  }

  async #run() {
    await this.#load();
    clearTimeout(this.#timer);
    const wait = this.#delay();
    if (this.#stopped || !wait) return;
    this.#timer = setTimeout(() => this.#tick(), wait);
  }

  #tick() {
    if (document.hidden) {
      this.#stale = true;
      return;
    }
    this.#run();
  }

  #onVisible = () => {
    if (document.hidden || !this.#stale) return;
    this.#stale = false;
    this.#run();
  };
}
