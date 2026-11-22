// The pocket's rows, from what the services answered: each service, recorder and desk in a line, the
// open positions across the desks soonest to close first, and every problem in plain words. Pure
// functions over one snapshot: Pocket.svelte only draws. Missing answers (a service down, an older
// runner without a route) read as "not reported", never as an error of the page's own.
import strings from './strings.json';
import { contractName } from '../home/desk.js';
import { fill, usd } from '../plays/play-format.js';

const MS_PER_SECOND = 1000;
const SECONDS_PER_MINUTE = 60;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;
const PERCENT = 100;
const MINT_SHOWN = 4; // characters kept from each end of a token's address
const NO_TIME = Number.POSITIVE_INFINITY; // a play with no close time sorts last
const SUPERVISOR_RUNNING = 'running';
const SUPERVISOR_STALE = 'stale';
const KIND_TELEGRAM = 'telegram';
const SCOUT_LOOKING = 'looking';
const SCOUT_ERROR = 'error';
const SERVICE_ORDER = ['newsd', 'm24d', 'tunnel'];
const DESK = { predict: 'predict', memedesk: 'memedesk', scout: 'scout' };
// Kalshi questions shortened for one line: "Will the temp in Miami be above 74.99° on Nov 22, 2026?" reads
// "Temp in Miami be above 74.99°". The full question stays in the row's tooltip.
const QUESTION_START = /^will\s+(?:the\s+)?/i;
const QUESTION_DATE = /\s+(?:on|at|for)\s+[A-Z][a-z]{2,8}\s+\d{1,2},\s+\d{4}(?:\s+at\s+\d{1,2}(?::\d{2})?\s*[ap]m(?:\s+[A-Z]{2,4})?)?/g;
const QUESTION_END = /\?(?=\s|$)/g;
/** How a row reads at a glance: its dot and its state word. */
export const TONE = { ok: 'ok', warn: 'warn', bad: 'bad', idle: 'idle' };

/**
 * @typedef {{ runner: boolean, news: boolean, supervisor: any, follow: any[] | null, kalshi: any, meme: any,
 *   tape: any, predict: any, memedesk: any, scout: any, ai: any }} Snapshot
 * Each route's answer, null when it did not answer. runner and news: their health checks answered.
 * @typedef {{ id: string, name: string, tone: string, state: string, detail: string }} Row
 * @typedef {Row & { pnl: number | null, pnlNote: string }} DeskRow
 * @typedef {{ id: string, desk: string, title: string, full: string, closeAt: number, closes: string, soon: string, mark: string, pnl: number | null }} PlayRow
 */

const t = strings;

/** "45s", "12m", "2h 3m", "3d 4h" */
export function span(/** @type {number} */ ms) {
  const seconds = Math.max(0, Math.round(ms / MS_PER_SECOND));
  if (seconds < SECONDS_PER_MINUTE) return `${seconds}s`;
  const minutes = Math.floor(seconds / SECONDS_PER_MINUTE);
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  if (hours < HOURS_PER_DAY) return `${hours}h ${minutes % MINUTES_PER_HOUR}m`;
  return `${Math.floor(hours / HOURS_PER_DAY)}d ${hours % HOURS_PER_DAY}h`;
}

/** Time since an ISO time or epoch ms, or '' when there is none. */
function since(/** @type {string | number | undefined} */ when, /** @type {number} */ now) {
  const at = typeof when === 'number' ? when : Date.parse(when ?? '');
  return Number.isFinite(at) && at > 0 ? span(now - at) : '';
}

function restarts(/** @type {number} */ n) {
  if (!n) return t.service.noRestarts;
  return n === 1 ? t.service.restart : fill(t.service.restarts, { n });
}

/** One service the supervisor runs. */
function supervisedRow(/** @type {any} */ s, /** @type {number} */ now) {
  const name = t.names[s.name] ?? s.name;
  if (!s.up) {
    const detail = [s.last_exit ? fill(t.service.exited, { how: s.last_exit }) : '', restarts(s.restarts)].filter(Boolean).join(' · ');
    return { id: s.name, name, tone: TONE.bad, state: t.states.down, detail };
  }
  const missed = s.health_ok === false && s.misses > 0;
  const detail = [fill(t.service.up, { span: since(s.started_at, now) }), restarts(s.restarts), missed ? fill(t.service.missed, { n: s.misses }) : ''];
  return { id: s.name, name, tone: missed ? TONE.warn : TONE.ok, state: t.states.running, detail: detail.filter(Boolean).join(' · ') };
}

/** A service checked directly, when no supervisor reports it. */
function directRow(/** @type {string} */ id, /** @type {boolean} */ answers) {
  return { id, name: t.names[id], tone: answers ? TONE.ok : TONE.bad, state: answers ? t.states.answering : t.states.silent, detail: answers ? t.service.direct : '' };
}

/** @returns {Row[]} the supervisor, then news, runner and tunnel */
export function serviceRows(/** @type {Snapshot} */ snap, /** @type {number} */ now) {
  const sup = snap.supervisor;
  const known = sup && (sup.state === SUPERVISOR_RUNNING || sup.state === SUPERVISOR_STALE);
  if (!known) {
    const head = { id: 'supervisor', name: t.names.supervisor, tone: TONE.idle, state: t.states.unknown, detail: snap.runner ? t.service.noSupervisor : '' };
    return [head, directRow('newsd', snap.news), directRow('m24d', snap.runner)];
  }
  const stale = sup.state === SUPERVISOR_STALE;
  const head = {
    id: 'supervisor', name: t.names.supervisor, tone: stale ? TONE.warn : TONE.ok, state: stale ? t.states.stale : t.states.running,
    detail: stale ? fill(t.service.beat, { ago: since(sup.beat_at, now) }) : fill(t.service.up, { span: since(sup.started_at, now) }),
  };
  const services = [...sup.services].sort((a, b) => SERVICE_ORDER.indexOf(a.name) - SERVICE_ORDER.indexOf(b.name));
  return [head, ...services.map((s) => supervisedRow(s, now))];
}

function telegramRow(/** @type {any[] | null} */ follow, /** @type {number} */ now) {
  const base = { id: KIND_TELEGRAM, name: t.names.telegram };
  const tg = follow?.find((k) => k.kind === KIND_TELEGRAM);
  if (!tg) return { ...base, tone: TONE.idle, state: t.states.unknown, detail: '' };
  if (!tg.sources) return { ...base, tone: TONE.idle, state: t.states.off, detail: t.recorder.telegramNone };
  const last = since(tg.last_at, now);
  const detail = [fill(t.recorder.telegram, { today: tg.today, sources: tg.sources }), tg.signed_in ? t.recorder.signedIn : t.recorder.public];
  return { ...base, tone: tg.failing?.length ? TONE.warn : TONE.ok, state: last ? fill(t.recorder.telegramLast, { ago: last }) : t.states.quiet, detail: detail.join(' · ') };
}

/** @returns {Row[]} the Kalshi tape, the meme recorder, the options tape and the Telegram follower */
export function recorderRows(/** @type {Snapshot} */ snap, /** @type {number} */ now) {
  const { kalshi, meme, tape } = snap;
  const rows = [];
  rows.push(!kalshi ? unreported('kalshi') : {
    id: 'kalshi', name: t.names.kalshi, tone: kalshi.last_error ? TONE.warn : kalshi.recording ? TONE.ok : TONE.idle,
    state: since(kalshi.last_pass_at, now) ? fill(t.recorder.kalshiLast, { ago: since(kalshi.last_pass_at, now) }) : t.states.idle,
    detail: fill(t.recorder.kalshi, { snapshots: kalshi.snapshots_today ?? 0, results: kalshi.results_today ?? 0 }),
  });
  rows.push(!meme ? unreported('meme') : {
    id: 'meme', name: t.names.meme, tone: meme.last_error ? TONE.warn : meme.running && meme.connected ? TONE.ok : meme.running ? TONE.warn : TONE.idle,
    state: meme.running ? (meme.connected ? t.states.recording : t.states.connecting) : t.states.off,
    detail: fill(t.recorder.meme, { seen: meme.seen_today ?? 0, snapshots: meme.snapshots_today ?? 0, tracked: meme.tracked_now ?? 0 }),
  });
  const tapeLast = tape ? since(tape.last_at, now) : '';
  rows.push(!tape ? unreported('tape') : {
    id: 'tape', name: t.names.tape, tone: tape.last_error ? TONE.warn : tape.recording ? TONE.ok : TONE.idle,
    state: tape.recording ? t.states.recording : t.states.idle,
    detail: [fill(t.recorder.tape, { today: tape.today ?? 0 }), tapeLast ? fill(t.recorder.tapeLast, { ago: tapeLast }) : ''].filter(Boolean).join(' · '),
  });
  rows.push(telegramRow(snap.follow, now));
  return rows;
}

function unreported(/** @type {string} */ id) {
  return { id, name: t.names[id], tone: TONE.idle, state: t.states.unknown, detail: '' };
}

/** @returns {DeskRow[]} the prediction desk, the meme desk and the options scout */
export function deskRows(/** @type {Snapshot} */ snap) {
  const { predict, memedesk, scout } = snap;
  const rows = [];
  if (!predict) rows.push({ ...unreported(DESK.predict), pnl: null, pnlNote: '' });
  else {
    const a = predict.account ?? {};
    rows.push({
      id: DESK.predict, name: t.names.predict, tone: predict.on ? TONE.ok : TONE.idle, state: predict.on ? t.states.on : t.states.off,
      detail: `${fill(t.desk.predict, { open: a.open ?? 0, won: a.won ?? 0, settled: a.settled ?? 0 })} · ${fill(t.desk.predictToday, { lost: usd(predict.lost_today_usd ?? 0), limit: usd(predict.daily_loss_usd ?? 0) })}`,
      pnl: (a.realized_usd ?? 0) + (a.mark_usd ?? 0) - (a.open_usd ?? 0), pnlNote: t.desk.total,
    });
  }
  if (!memedesk) rows.push({ ...unreported(DESK.memedesk), pnl: null, pnlNote: '' });
  else {
    const open = (memedesk.strategies ?? []).reduce((/** @type {number} */ n, /** @type {any} */ s) => n + (s.open_positions?.length ?? 0), 0);
    const stopped = memedesk.daily_loss_stop;
    rows.push({
      id: DESK.memedesk, name: t.names.memedesk, tone: stopped ? TONE.warn : TONE.ok, state: stopped ? t.states.stopped : t.states.on,
      detail: fill(t.desk.memedesk, { open, cash: usd(memedesk.cash_usd ?? 0) }),
      pnl: memedesk.day_realized_pnl_usd ?? 0, pnlNote: t.desk.today,
    });
  }
  if (!scout) rows.push({ ...unreported(DESK.scout), pnl: null, pnlNote: '' });
  else {
    const tone = scout.state === SCOUT_ERROR ? TONE.bad : scout.state === SCOUT_LOOKING ? TONE.ok : TONE.idle;
    const detail = [fill(t.desk.scout, { open: scout.open?.length ?? 0, closed: scout.closed?.length ?? 0 }), scout.why ?? ''].filter(Boolean).join(' · ');
    rows.push({ id: DESK.scout, name: t.names.scout, tone, state: t.states[scout.state] ?? scout.state ?? t.states.unknown, detail, pnl: null, pnlNote: '' });
  }
  return rows;
}

/** "93.7¢" */
const cents = (/** @type {number} */ n) => `${Number((n * PERCENT).toFixed(1))}¢`;

/** "2h 3m" until a close, "closing now" once it is past. */
function closes(/** @type {number} */ at, /** @type {number} */ now) {
  if (!Number.isFinite(at)) return '';
  return at > now ? fill(t.plays.closesIn, { time: span(at - now) }) : t.plays.closing;
}

/** A Kalshi question in a few words, for one line. */
function shortQuestion(/** @type {string} */ question) {
  const s = question.replace(QUESTION_START, '').replace(QUESTION_DATE, '').replace(QUESTION_END, '').trim();
  return s ? s[0].toUpperCase() + s.slice(1) : question;
}

/** "2h 3m" until a close, or "now" once it is past: the short form for a one-line row. */
function soon(/** @type {number} */ at, /** @type {number} */ now) {
  if (!Number.isFinite(at)) return '';
  return at > now ? span(at - now) : t.plays.now;
}

/** @returns {PlayRow[]} every open position across the desks, soonest to close first */
export function playRows(/** @type {Snapshot} */ snap, /** @type {number} */ now) {
  /** @type {PlayRow[]} */
  const rows = [];
  for (const p of snap.predict?.open ?? []) {
    const closeAt = Date.parse(p.buy?.close_at ?? '');
    const pnlNow = (p.mark - p.buy.price) * p.buy.qty;
    rows.push({ id: `p-${p.buy.id}`, desk: t.plays.desk.predict, title: shortQuestion(p.buy.title), full: p.buy.title, closeAt, closes: closes(closeAt, now),
      soon: soon(closeAt, now), mark: cents(p.mark), pnl: Number.isFinite(pnlNow) ? pnlNow : null });
  }
  for (const s of snap.memedesk?.strategies ?? []) {
    const hold = (s.config?.exit?.hold_seconds ?? 0) * MS_PER_SECOND;
    for (const p of s.open_positions ?? []) {
      const closeAt = hold ? Date.parse(p.buy?.at ?? '') + hold : NaN;
      const value = (p.mark_usd ?? 0) + (p.proceeds_usd ?? 0) - (p.buy?.net_usd ?? 0);
      rows.push({ id: `m-${p.mint}`, desk: t.plays.desk.memedesk, title: `${s.config?.id ?? ''} · ${p.mint.slice(0, MINT_SHOWN)}…${p.mint.slice(-MINT_SHOWN)}`, closeAt, closes: closes(closeAt, now), soon: soon(closeAt, now), full: '', mark: usd(p.mark_usd ?? 0), pnl: value });
    }
  }
  for (const c of snap.scout?.open ?? []) {
    const closeAt = Date.parse(c.exit_by ?? '');
    rows.push({ id: `s-${c.id}`, desk: t.plays.desk.scout, title: contractName(c.contract), full: '', closeAt, closes: closes(closeAt, now), soon: soon(closeAt, now), mark: c.mark ? `$${c.mark.toFixed(2)}` : '—', pnl: null });
  }
  return rows.sort((a, b) => (Number.isFinite(a.closeAt) ? a.closeAt : NO_TIME) - (Number.isFinite(b.closeAt) ? b.closeAt : NO_TIME));
}

/** @returns {{ id: string, text: string }[]} every last error and every service that is down, in plain words */
export function problems(/** @type {Snapshot} */ snap, /** @type {number} */ now) {
  const out = [];
  const add = (/** @type {string} */ id, /** @type {string} */ text) => out.push({ id, text });
  if (!snap.runner) add('runner', t.problems.runner);
  if (!snap.news) add('news', t.problems.news);
  const sup = snap.supervisor;
  if (sup?.state === SUPERVISOR_STALE) add('stale', fill(t.problems.stale, { ago: since(sup.beat_at, now) }));
  for (const s of sup?.state === SUPERVISOR_RUNNING ? sup.services : []) {
    const name = t.names[s.name] ?? s.name;
    if (!s.up) add(`down-${s.name}`, fill(t.problems.down, { name, how: s.last_exit || t.states.down }));
    else if (s.health_ok === false && s.misses > 0) add(`missed-${s.name}`, fill(t.problems.missed, { name, n: s.misses }));
  }
  for (const [id, status] of /** @type {[string, any][]} */ ([['kalshi', snap.kalshi], ['meme', snap.meme], ['tape', snap.tape]])) {
    if (status?.last_error) add(`error-${id}`, fill(t.problems.error, { name: t.names[id], error: status.last_error }));
  }
  if (snap.meme?.running && !snap.meme.connected) add('meme-connect', t.problems.notConnected);
  for (const f of snap.follow?.find((k) => k.kind === KIND_TELEGRAM)?.failing ?? []) {
    add(`follow-${f.source}`, fill(t.problems.follow, { source: f.source, error: f.error }));
  }
  for (const [id, desk] of /** @type {[string, any][]} */ ([[DESK.predict, snap.predict], [DESK.memedesk, snap.memedesk]])) {
    if (desk?.rulebook_error) add(`rulebook-${id}`, fill(t.problems.rulebook, { name: t.names[id], error: desk.rulebook_error }));
  }
  for (const s of snap.predict?.strategies ?? []) {
    if (s.last_error) add(`strategy-${s.config.id}`, fill(t.problems.strategy, { name: t.names.predict, strategy: s.config.name, error: s.last_error }));
  }
  if (snap.scout?.state === SCOUT_ERROR) add('scout', fill(t.problems.error, { name: t.names.scout, error: snap.scout.why ?? '' }));
  const asked = (snap.ai?.apps ?? []).filter((/** @type {any} */ a) => a.state);
  const answering = asked.filter((/** @type {any} */ a) => a.state === AI_STATE_OK).length;
  if (asked.length && answering < snap.ai.need) add('ai', fill(t.problems.aiShort, { n: answering, need: snap.ai.need }));
  return out;
}

// Each AI app state from the runner (engine/aicli) and how it reads in the pocket.
const AI_STATE = {
  ok: { tone: TONE.ok, word: 'ok' },
  'sign in': { tone: TONE.warn, word: 'signIn' },
  'out of usage': { tone: TONE.warn, word: 'limit' },
  'not installed': { tone: TONE.idle, word: 'missing' },
  'too slow': { tone: TONE.warn, word: 'slow' },
  failed: { tone: TONE.bad, word: 'failed' },
};
const AI_STATE_OK = 'ok';
const AI_STATE_SIGN_IN = 'sign in';

/** Seconds to one decimal: '3.1s'. */
const seconds = (/** @type {number} */ ms) => `${(ms / MS_PER_SECOND).toFixed(1)}s`;

/** One row per AI app: whether it answers, how fast, and the one step that fixes it when it does not. */
export function aiRows(/** @type {Snapshot} */ snap) {
  return (snap.ai?.apps ?? []).map((/** @type {any} */ a) => {
    const known = AI_STATE[/** @type {keyof typeof AI_STATE} */ (a.state)];
    const name = a.name;
    if (!known) return { id: a.id, name, tone: TONE.idle, state: t.ai.waiting, detail: a.plan };
    const detail = a.state === AI_STATE_OK ? fill(t.ai.okDetail, { time: seconds(a.ms), via: a.via, plan: a.plan })
      : !a.installed ? fill(t.ai.missingDetail, { via: a.via })
      : a.state === AI_STATE_SIGN_IN ? fill(t.ai.signInDetail, { step: a.sign_in }) : a.error ?? '';
    return { id: a.id, name, tone: known.tone, state: t.ai[/** @type {keyof typeof t.ai} */ (known.word)], detail };
  });
}

/** The line under the AI rows: the agreement rule and the last story put to them. */
export function aiSummary(/** @type {Snapshot} */ snap, /** @type {number} */ now) {
  const ai = snap.ai;
  if (!ai) return [];
  const last = ai.impacts?.[0];
  const lines = [last ? fill(t.ai.rule, { need: ai.need, ago: since(last.at, now) }) : fill(t.ai.ruleNever, { need: ai.need })];
  if (last?.agreed) lines.push(fill(t.ai.lastAgreed, { direction: last.consensus.direction, tickers: last.consensus.tickers.join(' '), time: seconds(last.decided_ms) }));
  else if (last) lines.push(fill(t.ai.lastSplit, { need: ai.need }));
  return lines;
}

/** The header's one-line verdict. */
export function overall(/** @type {Snapshot | null} */ snap, /** @type {number} */ problemCount) {
  if (!snap) return { tone: TONE.idle, text: t.overall.loading };
  if (!snap.runner) return { tone: TONE.bad, text: t.overall.down };
  if (!problemCount) return { tone: TONE.ok, text: t.overall.ok };
  return { tone: TONE.warn, text: problemCount === 1 ? t.overall.one : fill(t.overall.many, { n: problemCount }) };
}

