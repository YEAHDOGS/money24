import strings from './schedule.json';

const MINUTE_MS = 60000;
const HOUR_MS = 60 * MINUTE_MS;
const DEFAULT_LEAD_MS = 24 * HOUR_MS;
const MIN_LEAD_MS = 3 * HOUR_MS;
const LOCAL_LENGTH = 16;
const IMMEDIATE = 'immediate';
const PLANNED = 'planned';
const DATE_FORMAT = { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short' };

/** ISO time as a datetime-local value in the device time zone. */
export function localTime(iso) {
  const time = Date.parse(iso);
  if (!(time > 0)) return '';
  return new Date(time - new Date(time).getTimezoneOffset() * MINUTE_MS).toISOString().slice(0, LOCAL_LENGTH);
}

/** Human-readable planned time, including the device time zone. */
export function scheduleTime(iso) {
  const time = Date.parse(iso);
  if (!(time > 0)) return strings.notRecorded;
  return new Date(time).toLocaleString(undefined, DATE_FORMAT);
}

/** Editable clock defaults for a draft that predates scheduling. */
export function scheduleFields(play) {
  const entry = play.expected_entry_at || new Date(Date.now() + DEFAULT_LEAD_MS).toISOString();
  const exit = play.expected_exit_at || play.failsafes?.exit_by || new Date(Date.parse(entry) + DEFAULT_LEAD_MS).toISOString();
  return { entryAt: localTime(entry), exitBy: localTime(exit) };
}

/** Validates an edited clock while preserving the seconds of an unchanged plan. */
export function editedSchedule(play, entryInput, exitInput, now = Date.now()) {
  const changed = entryInput !== localTime(play.expected_entry_at);
  const entry = Date.parse(changed ? entryInput : play.expected_entry_at);
  const exit = Date.parse(exitInput === localTime(play.expected_exit_at) ? play.expected_exit_at : exitInput);
  if (!Number.isFinite(entry) || !Number.isFinite(exit) || exit <= entry) throw new Error(strings.invalid);
  if (changed && entry < now + MIN_LEAD_MS) throw new Error(strings.tooSoon);
  const mode = !changed && play.schedule_mode === IMMEDIATE ? IMMEDIATE : PLANNED;
  return { expected_entry_at: new Date(entry).toISOString(), expected_exit_at: new Date(exit).toISOString(), schedule_mode: mode };
}

/** Explains why an entry cannot start; mirrors the server's mandatory clock check. */
export function entryBlock(play, now) {
  const entry = Date.parse(play.expected_entry_at);
  const exit = Date.parse(play.expected_exit_at);
  if (!(entry > 0) || !(exit > entry)) return strings.legacy;
  if (now < entry) return strings.future;
  if (now >= exit) return strings.expired;
  return '';
}
