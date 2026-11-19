// Calendar logic: which day an event belongs to, the week grid, filters, grouping and countdowns.

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const DAYS_IN_WEEK = 7;
const MONDAY_OFFSET = 6; // days since Monday, for JS weekdays where Sunday is 0
const DATE_KEY_LENGTH = 'YYYY-MM-DD'.length;
const CATEGORY_ALL = 'all';
const CATEGORY_HOLIDAY = 'holiday';
const GROUP_MIN = 2; // a day needs this many matching events before they fold into one row
/** Titles the Forex Factory feed gives Federal Reserve events: "FOMC Statement", "Federal Funds Rate",
 * "Fed Chair Powell Speaks". "Fed" must open the title, so regional surveys ("Philly Fed Manufacturing
 * Index") and the Treasury's "Federal Budget Balance" stay rows of their own. */
const FED_TITLES = [/\bFOMC\b/i, /\bFederal Funds\b/i, /\bFederal Reserve\b/i, /^Fed\b/i, /\bBeige Book\b/i, /\bJackson Hole\b/i];
/** What folds into one row per day; the first rule an event meets wins. `kind` names the group in
 * strings.json, `name` is the field the folded row previews, `preview` how many of them, and
 * `byTitle` breaks impact ties by title instead of by time. */
const GROUP_RULES = [
  { kind: 'earnings', category: 'earnings', name: 'symbol', preview: 5, byTitle: true },
  { kind: 'fed', category: 'macro', region: 'USD', titles: FED_TITLES, name: 'title', preview: 2 },
];

/** Impact as a number, so filters and sorting can compare it. */
export const IMPACT_RANK = { '': 0, low: 1, medium: 2, high: 3 };
export const IMPACT_LEVELS = [1, 2, 3];

/** Full class strings so Tailwind can see them. One entry per category in strings.json. */
export const CATEGORY_CLASSES = {
  macro: { text: 'text-kind-macro', border: 'border-kind-macro/40', bg: 'bg-kind-macro' },
  earnings: { text: 'text-kind-filing', border: 'border-kind-filing/40', bg: 'bg-kind-filing' },
  auction: { text: 'text-kind-central-bank', border: 'border-kind-central-bank/40', bg: 'bg-kind-central-bank' },
  holiday: { text: 'text-kind-insider', border: 'border-kind-insider/40', bg: 'bg-kind-insider' },
  other: { text: 'text-kind-news', border: 'border-kind-news/40', bg: 'bg-kind-news' },
};
export const IMPACT_CLASSES = { 1: 'bg-muted', 2: 'bg-warn', 3: 'bg-neon-red' };

const TIME_FORMAT = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });
const WEEKDAY_FORMAT = new Intl.DateTimeFormat(undefined, { weekday: 'short' });
const DAY_FORMAT = new Intl.DateTimeFormat(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
const RANGE_FORMAT = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' });

/** @typedef {{uid:string, source:string, category:string, starts_at:string, all_day:boolean, title:string,
 *   region?:string, symbol?:string, impact?:string, forecast?:string, previous?:string, actual?:string,
 *   detail?:string, url?:string}} CalEvent */

/** Several events of one kind on one day, folded into a single row. `events` keep the day's order,
 * `names` are the biggest ones (highest impact first), `more` is how many the preview leaves out.
 * @typedef {{kind:string, category:string, events:CalEvent[], names:string[], more:number, impact:string}} CalGroup */
/** One agenda row. `event` is the event itself, or a group's earliest member, which places the row.
 * @typedef {{key:string, event:CalEvent, group?:CalGroup}} CalRow */

/** @param {Date} d local date as YYYY-MM-DD */
export function dateKey(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** The day an event sits under: all-day events keep their own date, timed ones use the reader's clock.
 * @param {CalEvent} e */
export function eventDay(e) {
  return e.all_day ? e.starts_at.slice(0, DATE_KEY_LENGTH) : dateKey(new Date(e.starts_at));
}

/** When an event starts, as local epoch ms. All-day events start at local midnight of their date.
 * @param {CalEvent} e */
export function startMs(e) {
  if (!e.all_day) return Date.parse(e.starts_at);
  const [y, m, d] = e.starts_at.slice(0, DATE_KEY_LENGTH).split('-').map(Number);
  return new Date(y, m - 1, d).getTime();
}

/** @param {CalEvent} e */
export function timeLabel(e) {
  return TIME_FORMAT.format(new Date(e.starts_at));
}

/** The seven days (Monday first) of the week `offset` weeks from the one holding `now`.
 * @param {number} now epoch ms @param {number} offset */
export function weekDays(now, offset) {
  const today = new Date(now);
  const monday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - ((today.getDay() + MONDAY_OFFSET) % DAYS_IN_WEEK) + offset * DAYS_IN_WEEK);
  return Array.from({ length: DAYS_IN_WEEK }, (_, i) => {
    const date = new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + i);
    return { key: dateKey(date), date, weekday: WEEKDAY_FORMAT.format(date), number: date.getDate(), long: DAY_FORMAT.format(date) };
  });
}

/** "Nov 9 – Nov 15" for a week from weekDays. @param {{date: Date}[]} days */
export function weekRange(days) {
  return `${RANGE_FORMAT.format(days[0].date)} – ${RANGE_FORMAT.format(days[DAYS_IN_WEEK - 1].date)}`;
}

/** Holidays have no impact rating but always matter (markets close), so they pass every impact filter.
 * @param {CalEvent} e @param {{category:string, minImpact:number, region:string}} filter */
export function matches(e, filter) {
  if (filter.category !== CATEGORY_ALL && e.category !== filter.category) return false;
  if (filter.region && e.region !== filter.region) return false;
  return e.category === CATEGORY_HOLIDAY || (IMPACT_RANK[e.impact ?? ''] ?? 0) >= filter.minImpact;
}

/** Events per day key. Within a day: timed events by time, then all-day ones, biggest impact first.
 * @param {CalEvent[]} events @returns {Record<string, CalEvent[]>} */
export function groupByDay(events) {
  /** @type {Record<string, CalEvent[]>} */
  const days = {};
  for (const e of events) (days[eventDay(e)] ??= []).push(e);
  for (const list of Object.values(days)) {
    list.sort((a, b) =>
      Number(a.all_day) - Number(b.all_day)
      || (a.all_day ? (IMPACT_RANK[b.impact ?? ''] ?? 0) - (IMPACT_RANK[a.impact ?? ''] ?? 0) : startMs(a) - startMs(b))
      || a.title.localeCompare(b.title));
  }
  return days;
}

/** A day's agenda rows: earnings fold into one row and Federal Reserve events into another, once the day
 * has GROUP_MIN of them. The day's order is kept; a group sits where its earliest member would.
 * @param {CalEvent[]} events one day from groupByDay @param {string} day its key @returns {CalRow[]} */
export function dayRows(events, day) {
  const rules = events.map((e) => GROUP_RULES.find((r) =>
    r.category === e.category && (!r.region || r.region === e.region) && (!r.titles || r.titles.some((t) => t.test(e.title)))));
  /** @type {Record<string, CalEvent[]>} */
  const members = {};
  events.forEach((e, i) => { if (rules[i]) (members[rules[i].kind] ??= []).push(e); });

  /** @type {CalRow[]} */
  const rows = [];
  events.forEach((event, i) => {
    const rule = rules[i];
    const group = rule ? members[rule.kind] : [];
    if (group.length < GROUP_MIN) { rows.push({ key: event.uid, event }); return; }
    if (group[0] !== event) return; // folded into the row its earliest member opened
    const ranked = [...group].sort((a, b) =>
      (IMPACT_RANK[b.impact ?? ''] ?? 0) - (IMPACT_RANK[a.impact ?? ''] ?? 0) || (rule.byTitle ? a.title.localeCompare(b.title) : 0));
    const names = ranked.slice(0, rule.preview).map((e) => e[rule.name] || e.title);
    rows.push({
      key: `${day}-${rule.kind}`,
      event,
      group: { kind: rule.kind, category: rule.category, events: group, names, more: group.length - names.length, impact: ranked[0].impact ?? '' },
    });
  });
  return rows;
}

/** Time left as two units at most: "3d 4h", "2h 14m", "14m 05s", "42s". @param {number} ms */
export function countdown(ms) {
  const left = Math.max(0, ms);
  const pad = (n) => String(n).padStart(2, '0');
  if (left >= DAY) return `${Math.floor(left / DAY)}d ${Math.floor((left % DAY) / HOUR)}h`;
  if (left >= HOUR) return `${Math.floor(left / HOUR)}h ${pad(Math.floor((left % HOUR) / MINUTE))}m`;
  if (left >= MINUTE) return `${Math.floor(left / MINUTE)}m ${pad(Math.floor((left % MINUTE) / SECOND))}s`;
  return `${Math.floor(left / SECOND)}s`;
}

/** "Tue 14:30" for lists that span several days. @param {CalEvent} e */
export function whenLabel(e) {
  const at = new Date(e.starts_at);
  return `${WEEKDAY_FORMAT.format(at)} ${TIME_FORMAT.format(at)}`;
}
