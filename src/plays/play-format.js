// Formatting and the plain-English "what must happen" lines for a play. Numbers come from the
// engine's report; nothing here recomputes the math, it only says it in words.
import strings from './strings.json';

const MARKET_TZ = 'America/New_York';
const SMALL_PRICE_DIGITS = 4;
const CENT_FLOOR = 0.01; // a cent or more reads in cents; below that, in significant digits (tokens)
const PERCENT = 100;
const MINUS = '−'; // a real minus sign: tabular and readable
const PLUS = '+';
const UP = '▲';
const DOWN = '▼';
const RIGHT_PUT = 'P';
const LEVEL_STOP = 'stop';
const LEVEL_BREAKEVEN = 'breakeven';
const LEVEL_RUNNER = 'runner';
const RUNG_PREFIX = 'T';
const SCENARIO_RUNGS = 'rungs:';
const MS_PER_MINUTE = 60000;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;
const THOUSAND = 1e3;
const MILLION = 1e6;
const BILLION = 1e9;

const WHEN_FORMAT = new Intl.DateTimeFormat('en-US', { timeZone: MARKET_TZ, weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
const SHORT_WHEN_FORMAT = new Intl.DateTimeFormat('en-US', { timeZone: MARKET_TZ, weekday: 'short', hour: 'numeric', minute: '2-digit' });
const CLOCK_FORMAT = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });

/** Fills "{name}" placeholders in a template string. */
export function fill(/** @type {string} */ template, /** @type {Record<string, string | number>} */ values) {
  return template.replace(/\{(\w+)\}/g, (_, k) => String(values[k] ?? ''));
}

/** Cents for prices of a cent or more; four significant digits below that, so $0.00000531 survives. */
export function price(/** @type {number} */ n) {
  if (!Number.isFinite(n)) return '—';
  return n >= CENT_FLOOR || n <= 0 ? n.toFixed(2) : n.toPrecision(SMALL_PRICE_DIGITS);
}

/** "$1,234.50" with no sign. */
export function usd(/** @type {number} */ n) {
  if (!Number.isFinite(n)) return '—';
  return `$${Math.abs(n).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

/** "+$52.00" / "−$66.00": P&L always carries its sign, never color alone. */
export function pnl(/** @type {number} */ n) {
  if (!Number.isFinite(n)) return '—';
  return `${n < 0 ? MINUS : PLUS}${usd(n)}`;
}

/** "▲ 4.2%" / "▼ 6.6%": a move always carries its arrow. */
export function move(/** @type {number} */ pct) {
  if (!Number.isFinite(pct)) return '—';
  return `${pct < 0 ? DOWN : UP} ${Math.abs(pct).toFixed(1)}%`;
}

/** "+133%" / "−46%" */
export function signedPct(/** @type {number} */ pct) {
  if (!Number.isFinite(pct)) return '—';
  return `${pct < 0 ? MINUS : PLUS}${Math.abs(Math.round(pct))}%`;
}

/** "25%" for chances; 0.5 decimals dropped. */
export function pct(/** @type {number} */ n) {
  if (!Number.isFinite(n)) return '—';
  return `${n >= 10 || n === 0 ? Math.round(n) : n.toFixed(1)}%`;
}

/** 54305947 → "54.3M" */
export function compact(/** @type {number} */ n) {
  if (!Number.isFinite(n)) return '—';
  if (n >= BILLION) return `${(n / BILLION).toFixed(1)}B`;
  if (n >= MILLION) return `${(n / MILLION).toFixed(1)}M`;
  if (n >= THOUSAND) return `${(n / THOUSAND).toFixed(1)}K`;
  return String(Math.round(n));
}

/** "Thu, Nov 19, 3:00 PM" in New York time, where expiries and exits are clocked. */
export function when(/** @type {string} */ iso) {
  const t = Date.parse(iso);
  return t > 0 ? WHEN_FORMAT.format(t) : '—';
}

/** "Thu 3:00 PM" in New York time, for tight rows. */
export function shortWhen(/** @type {string} */ iso) {
  const t = Date.parse(iso);
  return t > 0 ? SHORT_WHEN_FORMAT.format(t) : '—';
}

/** Local clock time for history rows. */
export function clock(/** @type {string} */ iso) {
  const t = Date.parse(iso);
  return t > 0 ? CLOCK_FORMAT.format(t) : '—';
}

/** "4m", "2h 10m", "3d" since a moment. */
export function age(/** @type {string} */ iso, /** @type {number} */ now) {
  const m = Math.max(0, Math.round((now - Date.parse(iso)) / MS_PER_MINUTE));
  if (m < MINUTES_PER_HOUR) return `${m}m`;
  const h = Math.floor(m / MINUTES_PER_HOUR);
  if (h >= HOURS_PER_DAY * 2) return `${Math.floor(h / HOURS_PER_DAY)}d`;
  return `${h}h ${m % MINUTES_PER_HOUR}m`;
}

/** True for puts: the stock has to fall. */
export function isBearish(/** @type {any} */ play) {
  return play.contract?.right === RIGHT_PUT;
}

/** A level's display name: T1, Runner, Stop, Breakeven. */
export function levelName(/** @type {string} */ name) {
  return strings.levels[name] ?? name;
}

/** Rungs and the runner: the levels that sell part of the position at a profit. */
export function isExitLevel(/** @type {any} */ l) {
  return l.name.startsWith(RUNG_PREFIX) || l.name === LEVEL_RUNNER;
}

/** The first exit level (T1, or the runner when there is no ladder). */
export function firstExit(/** @type {any} */ report) {
  return report?.levels?.find(isExitLevel);
}

/** A scenario's label: "Stopped out", "2 rung(s), rest stopped", "Every exit", "Total calamity". */
export function scenarioName(/** @type {string} */ name) {
  if (name.startsWith(SCENARIO_RUNGS)) return fill(strings.scenarios.rungs, { n: name.slice(SCENARIO_RUNGS.length) });
  return strings.scenarios[name] ?? name;
}

/**
 * The plain-English lines: what the stock has to do, by when, for each exit to pay, then the
 * breakeven, the stop (and what time decay does to it) and the calamity case.
 * @param {any} play  @param {any} report
 * @param {boolean} [short] a phone: the same facts as symbols ("AAL ≤ $12.49 by Thu 3:00 PM"), not sentences
 * @returns {{ kind: string, text: string }[]}
 */
export function mustHappen(play, report, short = false) {
  if (!play || !report) return [];
  const s = short ? strings.sentenceShort : strings.sentence;
  const sym = play.contract.symbol;
  const bearish = isBearish(play);
  const by = (short ? shortWhen : when)(report.payoff.exit_at);
  const out = [];
  if (report.payoff.vol_source === 'none') out.push({ kind: 'warn', text: s.noVol });

  for (const l of report.levels) {
    if (!isExitLevel(l)) continue;
    const values = { sym, name: levelName(l.name), qty: l.qty, price: `$${price(l.price)}`, pnl: pnl(l.pnl_usd), spot: `$${price(l.spot_by_exit)}`, when: by, move: move(l.move_pct) };
    if (!l.reachable) {
      out.push({ kind: 'bad', text: fill(s.unreachable, values) });
      continue;
    }
    const base = l.already_past ? s.rungPast : bearish ? s.rungDown : s.rungUp;
    out.push({ kind: 'gain', text: `${fill(base, values)} ${fill(s.odds, { pct: pct(l.chance_pct) })}` });
  }

  const be = report.levels.find((l) => l.name === LEVEL_BREAKEVEN);
  if (be?.reachable) {
    out.push({ kind: 'neutral', text: fill(bearish ? s.breakevenDown : s.breakevenUp, { sym, spot: `$${price(be.spot_by_exit)}`, when: by, move: move(be.move_pct) }) });
  }

  const stop = report.levels.find((l) => l.name === LEVEL_STOP);
  if (stop) {
    const values = { sym, now: `$${price(stop.spot_now)}`, exit: `$${price(stop.spot_by_exit)}`, pnl: pnl(stop.pnl_usd) };
    let text = fill(bearish ? s.stopDown : s.stopUp, values);
    if (stop.already_past) text += ` ${fill(bearish ? s.decayDown : s.decayUp, values)}`;
    else if (stop.chance_pct > 0) text += ` ${fill(s.stopOdds, { pct: pct(stop.chance_pct) })}`;
    out.push({ kind: 'loss', text });
  }
  out.push({ kind: 'loss', text: fill(s.calamity, { pnl: pnl(-report.payoff.worst_loss_usd) }) });
  return out;
}

/** "needs AAL ≤ $12.30 by Thu 3:00 PM" for a list row, from the first exit. */
export function needsLine(/** @type {any} */ play, /** @type {any} */ report) {
  const l = firstExit(report);
  if (!l || !l.reachable) return '';
  const op = isBearish(play) ? '≤' : '≥';
  return `${strings.card.needs} ${play.contract.symbol} ${op} $${price(l.spot_by_exit)} ${strings.card.by} ${shortWhen(report.payoff.exit_at)}`;
}

/** How many checks are failing now: blocks first, then warnings. */
export function failing(/** @type {any} */ report) {
  const checks = report?.checks ?? [];
  return { blocks: checks.filter((c) => c.status === 'fail').length, warns: checks.filter((c) => c.status === 'warn').length };
}

/** A play's P&L once closed, or the sum of what it sold so far. */
export function realized(/** @type {any} */ play) {
  if (play.result) return play.result.pnl_usd;
  return (play.fills ?? []).reduce((sum, f) => sum + f.pnl_usd, 0);
}

/** A number input's value, or 0 when empty or not a number. */
export function num(/** @type {string | number} */ v) {
  const n = typeof v === 'number' ? v : parseFloat(v);
  return Number.isFinite(n) ? n : 0;
}

/** Whole-number percent of a ratio for bars: 0..100. */
export function barPct(/** @type {number} */ part, /** @type {number} */ whole) {
  if (!whole) return 0;
  return Math.max(0, Math.min(PERCENT, (Math.abs(part) / Math.abs(whole)) * PERCENT));
}
