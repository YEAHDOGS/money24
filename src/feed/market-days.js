// US stock market holidays and half days as calendar events, worked out from the exchanges' own rules (NYSE
// Rule 7.2), so no list needs updating each year. The same rules as engine/calendar/sessions.go: keep the two in
// step. The news service's calendar carries economic releases and earnings but not market closures, so the
// pages add these themselves.

const FIXED = [
  { id: 'new_year', month: 0, day: 1 },
  { id: 'juneteenth', month: 5, day: 19, since: 2022 },
  { id: 'independence', month: 6, day: 4 },
  { id: 'christmas', month: 11, day: 25 },
];
/** nth is 1 for the first such weekday of the month, or LAST for the last. */
const LAST = -1;
const FLOATING = [
  { id: 'mlk', month: 0, weekday: 1, nth: 3 },
  { id: 'washington', month: 1, weekday: 1, nth: 3 },
  { id: 'memorial', month: 4, weekday: 1, nth: LAST },
  { id: 'labor', month: 8, weekday: 1, nth: 1 },
  { id: 'thanksgiving', month: 10, weekday: 4, nth: 4 },
];
const NAMES = {
  new_year: "New Year's Day", mlk: 'Martin Luther King Jr. Day', washington: "Washington's Birthday", good_friday: 'Good Friday',
  memorial: 'Memorial Day', juneteenth: 'Juneteenth', independence: 'Independence Day', labor: 'Labor Day', thanksgiving: 'Thanksgiving', christmas: 'Christmas',
};
const GOOD_FRIDAY_OFFSET = -2; // days from Easter Sunday
const SUNDAY = 0;
const SATURDAY = 6;
const DAYS_PER_WEEK = 7;
const DAY_MS = 86_400_000;
const CHRISTMAS_EVE = 24;
const JULY_THIRD = 3;
const FRIDAY_AFTER = 1; // days after Thanksgiving
const YEARS_BEHIND = 1;
const YEARS_AHEAD = 2;
const SOURCE = 'm24-calendar';
const REGION = 'USD';

/** A day as a UTC date, so plain date arithmetic never meets a clock change. */
const utc = (/** @type {number} */ y, /** @type {number} */ m, /** @type {number} */ d) => new Date(Date.UTC(y, m, d));
const key = (/** @type {Date} */ d) => d.toISOString().slice(0, 10);
const addDays = (/** @type {Date} */ d, /** @type {number} */ n) => new Date(d.getTime() + n * DAY_MS);

function nthWeekday(/** @type {number} */ year, /** @type {number} */ month, /** @type {number} */ weekday, /** @type {number} */ nth) {
  if (nth === LAST) {
    const last = utc(year, month + 1, 0);
    return addDays(last, -((last.getUTCDay() - weekday + DAYS_PER_WEEK) % DAYS_PER_WEEK));
  }
  const first = utc(year, month, 1);
  return addDays(first, ((weekday - first.getUTCDay() + DAYS_PER_WEEK) % DAYS_PER_WEEK) + (nth - 1) * DAYS_PER_WEEK);
}

/** Easter Sunday of a Gregorian year (the anonymous Gregorian computus). */
function easter(/** @type {number} */ y) {
  const a = y % 19, b = Math.floor(y / 100), c = y % 100;
  const d = Math.floor(b / 4), e = b % 4, f = Math.floor((b + 8) / 25), g = Math.floor((b - f + 1) / 3);
  const h = (19 * a + b - d - g + 15) % 30, i = Math.floor(c / 4), k = c % 4;
  const l = (32 + 2 * e + 2 * i - h - k) % 7, m = Math.floor((a + 11 * h + 22 * l) / 451);
  const n = h + l - 7 * m + 114;
  return utc(y, Math.floor(n / 31) - 1, (n % 31) + 1);
}

/** A fixed-date holiday on a weekend is kept on the nearest weekday; New Year's Day on a Saturday is not kept. */
function observed(/** @type {string} */ id, /** @type {Date} */ day) {
  if (day.getUTCDay() === SUNDAY) return addDays(day, 1);
  if (day.getUTCDay() === SATURDAY) return id === 'new_year' ? null : addDays(day, -1);
  return day;
}

/**
 * The market closures and half days of one year, as calendar events.
 * @returns {import('./calendar-format.js').CalEvent[]}
 */
function yearEvents(/** @type {number} */ year) {
  /** @type {{ id: string, on: Date }[]} */
  const closed = [];
  for (const h of FIXED) {
    const on = h.since && year < h.since ? null : observed(h.id, utc(year, h.month, h.day));
    if (on) closed.push({ id: h.id, on });
  }
  for (const h of FLOATING) closed.push({ id: h.id, on: nthWeekday(year, h.month, h.weekday, h.nth) });
  closed.push({ id: 'good_friday', on: addDays(easter(year), GOOD_FRIDAY_OFFSET) });

  const thanksgiving = nthWeekday(year, 10, 4, 4);
  const shut = new Set(closed.map((c) => key(c.on)));
  const isTradingDay = (/** @type {Date} */ d) => ![SUNDAY, SATURDAY].includes(d.getUTCDay()) && !shut.has(key(d));
  const half = [
    { name: 'Day after Thanksgiving', on: addDays(thanksgiving, FRIDAY_AFTER) },
    { name: 'Christmas Eve', on: utc(year, 11, CHRISTMAS_EVE) },
    { name: 'Eve of Independence Day', on: utc(year, 6, JULY_THIRD) },
  ].filter((h) => isTradingDay(h.on));

  const event = (/** @type {string} */ uid, /** @type {Date} */ on, /** @type {string} */ title, /** @type {string} */ detail, /** @type {string} */ impact) => ({
    uid: `${SOURCE}|${uid}|${key(on)}`, source: SOURCE, category: 'holiday', starts_at: `${key(on)}T00:00:00Z`, all_day: true, title, region: REGION, impact, detail,
  });
  return [
    ...closed.map((c) => event(c.id, c.on, `${NAMES[c.id]}: US stock markets closed`, 'Stocks and options are shut all day. Crypto and prediction markets keep trading.', 'high')),
    ...half.map((h) => event(h.name, h.on, `${h.name}: stocks close at 1 PM ET`, 'A shortened session: thin volume, wide spreads, early close.', 'medium')),
  ];
}

/** The calendar plus every market holiday and half day around today. Events already there keep their place (same uid wins). */
export function withMarketDays(/** @type {import('./calendar-format.js').CalEvent[]} */ events, /** @type {Date} */ today = new Date()) {
  const have = new Set(events.map((e) => e.uid));
  const year = today.getUTCFullYear();
  const added = [];
  for (let y = year - YEARS_BEHIND; y <= year + YEARS_AHEAD; y++) added.push(...yearEvents(y).filter((e) => !have.has(e.uid)));
  return [...events, ...added].sort((a, b) => a.starts_at.localeCompare(b.starts_at));
}
