import strings from './scribe.json';

const SCRIBE_PATH = '/api/plays/scribe';
const PAPER_PATH = '/paper';
const POLL_MS = 4000;
const REQUEST_TIMEOUT_MS = 15000;
const HOUR_MS = 3600000;
const DEFAULT_LEAD_HOURS = 24;
const MIN_LEAD_HOURS = 3;
const LOCAL_DATE_LENGTH = 16;
const JSON_HEADERS = { 'Content-Type': 'application/json' };
const DATE_OPTIONS = { dateStyle: 'medium', timeStyle: 'short' };
const SAFE_PROTOCOLS = ['https:', 'http:'];

/** A datetime-local input value, preserving the device's timezone. */
export function localTime(at) {
  const date = new Date(at);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, LOCAL_DATE_LENGTH);
}

/** Day-ahead research window; the server validates it again. */
export function defaultWindow() {
  const entry = Date.now() + DEFAULT_LEAD_HOURS * HOUR_MS;
  return { entry: localTime(entry), exit: localTime(entry + DEFAULT_LEAD_HOURS * HOUR_MS) };
}

/** Convert user-local times into an unambiguous UTC request. */
export function requestWindow(entry, exit) {
  const start = new Date(entry).getTime();
  const end = new Date(exit).getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || start < Date.now() + MIN_LEAD_HOURS * HOUR_MS || end <= start) {
    throw new Error(strings.badTime);
  }
  return { expected_entry_at: new Date(start).toISOString(), expected_exit_at: new Date(end).toISOString() };
}

/** Display an absolute date so a proposed window cannot be mistaken for today. */
export function displayTime(value) {
  if (!value || !Number.isFinite(Date.parse(value))) return '—';
  return new Date(value).toLocaleString(undefined, DATE_OPTIONS);
}

/** Model-generated links never become executable URLs. */
export function sourceURL(value) {
  try {
    const url = new URL(value);
    return SAFE_PROTOCOLS.includes(url.protocol) ? url.href : '';
  } catch {
    return '';
  }
}

/** Why a proposal's contracts cannot start on paper yet; empty when the planned entry is due. */
export function startBlock(record, now) {
  if (now < Date.parse(record.expected_entry_at)) return strings.paperEarly;
  if (now >= Date.parse(record.expected_exit_at)) return strings.paperLate;
  return '';
}

/** Private research stays on the runner; no tips or credentials are cached here. */
export class ScribeClient {
  records = $state([]);
  error = $state('');
  busy = $state(false);
  /** The clock of the last poll, so a due entry unlocks without a reload. */
  now = $state(Date.now());
  #base;
  #secret;
  #timer;
  #stopped = false;
  #requests = new Set();

  constructor(base, secret) {
    this.#base = base.replace(/\/+$/, '');
    this.#secret = secret;
  }

  async #request(method, input, path = '') {
    const controller = new AbortController();
    this.#requests.add(controller);
    const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
    try {
      const headers = { ...JSON_HEADERS };
      if (this.#secret) headers.Authorization = `Bearer ${this.#secret}`;
      const response = await fetch(`${this.#base}${SCRIBE_PATH}${path}`, {
        method, headers, body: input ? JSON.stringify(input) : undefined, signal: controller.signal,
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || strings.failed);
      return data;
    } finally {
      clearTimeout(timeout);
      this.#requests.delete(controller);
    }
  }

  async refresh() {
    if (this.#stopped) return;
    clearTimeout(this.#timer);
    try {
      const records = await this.#request('GET');
      if (this.#stopped) return;
      this.records = records ?? [];
      this.now = Date.now();
      this.error = '';
    } catch (error) {
      if (this.#stopped) return;
      this.error = error instanceof Error ? error.message : strings.failed;
    }
    if (!this.#stopped) this.#timer = setTimeout(() => this.refresh(), POLL_MS);
  }

  async submit(input) {
    if (this.busy || this.#stopped) return;
    this.busy = true;
    try {
      const record = await this.#request('POST', input);
      if (this.#stopped) return;
      this.records = [record, ...this.records.filter((item) => item.id !== record.id)];
      this.error = '';
      return record;
    } finally {
      this.busy = false;
    }
  }

  /** Starts one exchange-checked candidate on the prediction desk's paper book. */
  async paper(id, ticker) {
    if (this.busy || this.#stopped) return;
    this.busy = true;
    try {
      const record = await this.#request('POST', { id, ticker }, PAPER_PATH);
      if (this.#stopped) return;
      this.records = this.records.map((item) => (item.id === record.id ? record : item));
      return record;
    } finally {
      this.busy = false;
    }
  }

  stop() {
    this.#stopped = true;
    clearTimeout(this.#timer);
    for (const request of this.#requests) request.abort();
    this.#requests.clear();
    this.records = [];
    this.#secret = '';
  }
}
