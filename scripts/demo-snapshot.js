// Records the public demo from the two running services into public/demo/: what every app page
// shows when no runner is set up (src/lib/demo.js). One moment in time, read once, never live.
//
//   plays.json, preview/   every play with its report, the link history, the rulebook, the AI
//                          providers (names only, never keys) and the stake dial at every size
//   trading.json           the paper journal and the source catalog
//   news.json              recent headlines, the calendar, the poll setting and the network meter
//   markets/               a handful of symbols: quote, news, related tickers, a chart per range
//
//   npm start            (in another terminal: the services, with the plays you want to show)
//   node scripts/demo-snapshot.js [runner URL] [news URL]
//
// Left out on purpose: source trust records and stored posts (other people's posts and a score
// on a named account do not belong on a public page), source notes, keys and the webhook secret.
import { mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const DEMO_DIR = join(ROOT, 'public', 'demo');
const PREVIEW_DIR = join(DEMO_DIR, 'preview');
const MARKETS_DIR = join(DEMO_DIR, 'markets');
const DEFAULT_RUNNER = 'http://127.0.0.1:8025';
const DEFAULT_NEWS = 'http://127.0.0.1:8024';
const PLAYS_FILE = 'plays.json';
const TRADING_FILE = 'trading.json';
const NEWS_FILE = 'news.json';
const MARKETS_INDEX = 'index.json';
const PREVIEW_BATCH = 8;
const JOURNAL_LIMIT = 300;
const NEWS_LIMIT = 400;
// The network page draws rates from the gap between meter readings, so a few are recorded in a row.
const METER_LISTEN_MS = 16000;
const METER_READINGS_MAX = 8;
/** The symbols the markets page can open on the demo. The first is the one it opens on. */
const MARKET_SYMBOLS = ['NVDA', 'SPY', 'BTC-USD', 'AAL', 'DAL', 'UAL'];
const MARKET_FOLLOWING = ['NVDA', 'SPY', 'BTC-USD'];
/** Each range at the candle size the page opens it in. Mirrors INTERVALS in src/markets/market-client.svelte.js. */
const MARKET_RANGES = { '1d': '1m', '5d': '5m', '1mo': '30m', '6mo': '1d', ytd: '1d', '1y': '1d', '5y': '1wk', max: '1mo' };
const PRICE_DECIMALS = 4; // candles keep four decimals: a third of the bytes, nothing a chart can show
const OWNER_NOTE = /owner/i; // a catalog source described as the owner's own stays out of the public demo
const SSE_EVENT = /^event: (.*)$/m;
const SSE_DATA = /^data: (.*)$/m;

const runner = (process.argv[2] ?? DEFAULT_RUNNER).replace(/\/+$/, '');
const news = (process.argv[3] ?? DEFAULT_NEWS).replace(/\/+$/, '');

async function get(/** @type {string} */ base, /** @type {string} */ path) {
  const res = await fetch(base + path);
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return res.json();
}

function write(/** @type {string} */ dir, /** @type {string} */ file, /** @type {any} */ data) {
  writeFileSync(join(dir, file), JSON.stringify(data));
}

/** An empty folder, so a symbol or play dropped from the demo leaves no stale file behind. */
function emptyDir(/** @type {string} */ dir) {
  mkdirSync(dir, { recursive: true });
  for (const f of readdirSync(dir)) rmSync(join(dir, f));
}

/** The dial's view at each size, keeping only what a resize changes (the page merges it onto the play). */
async function previews(/** @type {{play: any, report: any}} */ item) {
  const { min_units: min, slider_max: max } = item.report.sizing;
  const top = Math.max(max, min + 1);
  /** @type {Record<number, any>} */
  const sizes = {};
  for (let u = min; u <= top; u += PREVIEW_BATCH) {
    const batch = [];
    for (let k = u; k < u + PREVIEW_BATCH && k <= top; k++) batch.push(k);
    const got = await Promise.all(batch.map((k) => get(runner, `/api/plays/${encodeURIComponent(item.play.id)}/preview?units=${k}`)));
    got.forEach((p, i) => {
      sizes[batch[i]] = { play: { qty: p.play.qty, ladder: p.play.ladder, runner: p.play.runner }, report: p.report };
    });
  }
  return sizes;
}

async function recordPlays(/** @type {string} */ takenAt) {
  const items = await get(runner, '/api/plays');
  if (!items.length) throw new Error('the runner has no plays to record. Paste a few links on the plays board first');
  const [links, rules, providers] = await Promise.all([get(runner, '/api/plays/links'), get(runner, '/api/plays/rules'), get(runner, '/api/plays/ai')]);
  emptyDir(PREVIEW_DIR);
  write(DEMO_DIR, PLAYS_FILE, { taken_at: takenAt, items, links, rules, providers });
  for (const item of items) write(PREVIEW_DIR, `${item.play.id}.json`, await previews(item));
  return `${items.length} plays`;
}

async function recordTrading(/** @type {string} */ takenAt) {
  const [lines, catalog] = await Promise.all([get(runner, `/api/journal?limit=${JOURNAL_LIMIT}`), get(runner, '/api/sources')]);
  // Local times become UTC: the machine's time zone is nobody's business.
  const journal = JSON.parse(JSON.stringify(lines), (key, value) => (key === 'seen_at' && value ? new Date(value).toISOString() : value));
  const sources = (catalog.sources ?? [])
    .filter((/** @type {any} */ s) => !OWNER_NOTE.test(s.note ?? ''))
    .map(({ note: _note, ...rest }) => rest);
  write(DEMO_DIR, TRADING_FILE, { taken_at: takenAt, journal, sources });
  return `${journal.length} journal lines, ${sources.length} sources`;
}

/** Listens to the news stream for a while: the meter's readings in order, and each round's report. */
async function meter() {
  const stats = [];
  const cycles = [];
  const stop = new AbortController();
  const timer = setTimeout(() => stop.abort(), METER_LISTEN_MS);
  try {
    // "after" far past the newest post: only the meter, the plan and the reports are wanted here.
    const res = await fetch(`${news}/api/stream?after=${Number.MAX_SAFE_INTEGER}`, { signal: stop.signal });
    let buffer = '';
    for await (const chunk of res.body.pipeThrough(new TextDecoderStream())) {
      buffer += chunk;
      const messages = buffer.split('\n\n');
      buffer = messages.pop() ?? '';
      for (const message of messages) {
        const name = message.match(SSE_EVENT)?.[1];
        const data = message.match(SSE_DATA)?.[1];
        if (!data) continue;
        if (name === 'stats') stats.push(JSON.parse(data));
        if (name === 'cycle') cycles.unshift(JSON.parse(data));
      }
      if (stats.length >= METER_READINGS_MAX) stop.abort();
    }
  } catch (err) {
    if (!stop.signal.aborted) throw err;
  } finally {
    clearTimeout(timer);
  }
  return { stats, cycles };
}

async function recordNews(/** @type {string} */ takenAt) {
  const [events, calendar, settings, { stats, cycles }] = await Promise.all([
    get(news, `/api/events?limit=${NEWS_LIMIT}`), get(news, '/api/calendar'), get(news, '/api/settings'), meter(),
  ]);
  write(DEMO_DIR, NEWS_FILE, { taken_at: takenAt, events, calendar, settings, stats, cycles });
  return `${events.length} headlines, ${calendar.length} calendar events, ${stats.length} meter readings`;
}

/** A chart with its candles trimmed to four decimals. */
function slim(/** @type {any} */ chart) {
  const round = (/** @type {number} */ n) => Number(n.toFixed(PRICE_DECIMALS));
  return { ...chart, candles: chart.candles.map((/** @type {any} */ c) => ({ t: c.t, o: round(c.o), h: round(c.h), l: round(c.l), c: round(c.c), v: c.v })) };
}

async function recordMarkets(/** @type {string} */ takenAt) {
  emptyDir(MARKETS_DIR);
  const symbols = [];
  /** @type {Record<string, any>} */
  const quotes = {};
  for (const symbol of MARKET_SYMBOLS) {
    const q = { symbol };
    const [summary, search, related, ...charts] = await Promise.all([
      get(runner, `/api/market/summary?${new URLSearchParams(q)}`),
      get(runner, `/api/market/search?${new URLSearchParams({ q: symbol })}`),
      get(runner, `/api/market/related?${new URLSearchParams(q)}`).catch(() => []),
      ...Object.entries(MARKET_RANGES).map(([range, interval]) => get(runner, `/api/market/chart?${new URLSearchParams({ symbol, range, interval })}`)),
    ]);
    const byRange = Object.fromEntries(Object.keys(MARKET_RANGES).map((range, i) => [range, slim(charts[i])]));
    write(MARKETS_DIR, `${encodeURIComponent(symbol)}.json`, { summary, news: search.news ?? [], related, charts: byRange });
    symbols.push({ symbol, name: summary.name, exchange: summary.exchange, type: summary.type ?? '' });
    quotes[symbol] = { symbol, name: summary.name, price: summary.price, change_pct: summary.change_pct, currency: summary.currency };
  }
  write(MARKETS_DIR, MARKETS_INDEX, { taken_at: takenAt, symbols, following: MARKET_FOLLOWING, quotes });
  return `${symbols.length} symbols`;
}

const takenAt = new Date().toISOString();
mkdirSync(DEMO_DIR, { recursive: true });
const parts = [['plays', recordPlays], ['trading', recordTrading], ['news', recordNews], ['markets', recordMarkets]];
let failed = false;
for (const [name, record] of parts) {
  try {
    console.log(`${name.padEnd(8)} ${await record(takenAt)}`);
  } catch (err) {
    failed = true;
    console.error(`${name.padEnd(8)} not recorded: ${err instanceof Error ? err.message : err}. Start the services with npm start.`);
  }
}
if (failed) process.exit(1);
console.log(`\nRecorded the demo into public/demo/ (taken ${takenAt}).`);
