// Live connection to the m24d trading runner: loads the recent journal and the source catalog, then
// holds one Server-Sent Events stream of new journal entries. EventSource reconnects on its own and
// sends Last-Event-ID, so the runner backfills anything missed while asleep or offline.

const DEFAULT_API = 'http://127.0.0.1:8025';
const API_BASE = import.meta.env.VITE_TRADING_API || DEFAULT_API;
const INITIAL_LIMIT = 300;
const MAX_ITEMS = 1000;
const RETRY_MS = 3000;
const STATUS = { connecting: 'connecting', live: 'live', reconnecting: 'reconnecting', offline: 'offline' };

/** @typedef {{qty:number, price:number}} Target */
/** @typedef {{source_id:string, kind:string, symbol?:string, right?:string, strike?:number, expiry?:string,
 *   price?:number, confidence:number, seen_at:string, raw:string}} Signal */
/** @typedef {{qty:number, entry:number, stop:number, targets:Target[], runner:number, cost_usd:number,
 *   exit_by?:string, time_stop_at?:string, time_stop_above?:number}} Plan */
/** @typedef {{id:number, at:string, action:string, signal:Signal, plan?:Plan, rejected?:string, fresh?:boolean}} Entry */

export class TradingClient {
  /** @type {Entry[]} newest first */
  items = $state([]);
  /** @type {any[]} */
  sources = $state([]);
  status = $state(STATUS.connecting);

  #es = null;
  #lastId = 0;
  #retry = 0;

  async start() {
    try {
      const [journal, catalog] = await Promise.all([
        fetch(`${API_BASE}/api/journal?limit=${INITIAL_LIMIT}`).then((r) => (r.ok ? r.json() : Promise.reject(r.status))),
        fetch(`${API_BASE}/api/sources`).then((r) => (r.ok ? r.json() : Promise.reject(r.status))),
      ]);
      this.sources = catalog.sources ?? [];
      this.items = [...journal].reverse();
      this.#lastId = journal.at(-1)?.id ?? 0;
      this.#connect();
    } catch {
      this.status = STATUS.offline;
      this.#retry = setTimeout(() => this.start(), RETRY_MS);
    }
  }

  stop() {
    clearTimeout(this.#retry);
    this.#es?.close();
  }

  #connect() {
    const es = new EventSource(`${API_BASE}/api/stream?after=${this.#lastId}`);
    this.#es = es;
    es.onopen = () => { this.status = STATUS.live; };
    es.onerror = () => {
      if (es.readyState !== EventSource.CLOSED) {
        this.status = STATUS.reconnecting;
        return;
      }
      this.status = STATUS.offline;
      this.#retry = setTimeout(() => this.start(), RETRY_MS);
    };
    es.onmessage = (msg) => {
      /** @type {Entry} */
      const e = JSON.parse(msg.data);
      if (e.id <= this.#lastId) return;
      this.#lastId = e.id;
      e.fresh = true;
      this.items = [e, ...this.items].slice(0, MAX_ITEMS);
    };
  }
}

/** Journal entries are bucketed by what happened: a skipped entry shows as "rejected". */
export function bucket(/** @type {Entry} */ e) {
  return e.rejected ? 'rejected' : e.action;
}

/** "SPY 600C 11/21" for options, "SPY" otherwise. */
export function contractLabel(/** @type {Signal} */ s) {
  if (!s.symbol) return '—';
  if (!s.right) return s.symbol;
  const [, mm, dd] = (s.expiry ?? '').split('-');
  return `${s.symbol} ${s.strike}${s.right} ${Number(mm)}/${Number(dd)}`;
}
