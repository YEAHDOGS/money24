// Number and time formatting shared by the Markets page components.

const UNITS = [[1e12, 'T'], [1e9, 'B'], [1e6, 'M'], [1e3, 'K']];
const DASH = '—';
const MINUS = '−';
const PLUS = '+';
const SMALL_PRICE = 1;
const DECIMALS = 2;
const SMALL_DECIMALS = 4; // most decimals for prices between a cent and one unit
const TINY_PRICE = 0.01;
const TINY_DIGITS = 4; // significant digits for prices under a cent
const MS_PER_SECOND = 1000;
const MS_PER_MINUTE = 60000;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;

/** "168.80", "73,582.90", "0.00001234". A missing or zero value reads as a dash. */
export function price(/** @type {number|undefined} */ n) {
  if (!n) return DASH;
  if (Math.abs(n) < TINY_PRICE) return n.toPrecision(TINY_DIGITS);
  const most = Math.abs(n) < SMALL_PRICE ? SMALL_DECIMALS : DECIMALS;
  return n.toLocaleString('en-US', { minimumFractionDigits: DECIMALS, maximumFractionDigits: most });
}

/** "4.07T", "424.95M". */
export function compact(/** @type {number|undefined} */ n) {
  if (!n) return DASH;
  const unit = UNITS.find(([size]) => Math.abs(n) >= size);
  return unit ? `${(n / unit[0]).toFixed(DECIMALS)}${unit[1]}` : n.toFixed(0);
}

/** "+1.09" or "−9.87": a move with its sign. */
export function signed(/** @type {number} */ n) {
  return `${n < 0 ? MINUS : PLUS}${price(Math.abs(n)) === DASH ? '0.00' : price(Math.abs(n))}`;
}

/** "5.53%", without a sign: the arrow beside it carries the direction. */
export function percent(/** @type {number|undefined} */ n) {
  return n || n === 0 ? `${Math.abs(n).toFixed(DECIMALS)}%` : DASH;
}

/** "Nov 10, 4:00 PM EST" in the exchange's own zone. */
export function marketTime(/** @type {number} */ seconds, /** @type {string} */ zone) {
  if (!seconds) return '';
  return new Date(seconds * MS_PER_SECOND).toLocaleString('en-US', {
    timeZone: zone || undefined, month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short',
  });
}

/** "12m", "11h", "3d": how long ago a story ran. */
export function age(/** @type {number} */ seconds, /** @type {number} */ now) {
  const minutes = Math.max(0, Math.round((now - seconds * MS_PER_SECOND) / MS_PER_MINUTE));
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  return hours < HOURS_PER_DAY ? `${hours}h` : `${Math.floor(hours / HOURS_PER_DAY)}d`;
}
