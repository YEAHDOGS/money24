/**
 * Light / dark / auto for every Money 24 page. Auto is dark from sunset to sunrise, worked out on this
 * device from the date and a position: the device's own if the visitor shared it, else a guess from the
 * time zone. Nothing is sent anywhere. The page's colors live in src/styles/theme.css under
 * html[data-theme]; the inline script in each page's <head> applies the last answer before first paint
 * (it reads STORE_MODE and STORE_LAST), this module keeps that answer true afterwards.
 */

export const MODES = ['auto', 'light', 'dark'];
export const THEME_EVENT = 'm24-theme';

const STORE_MODE = 'm24.theme';
const STORE_LAST = 'm24.theme.last';
const STORE_SPOT = 'm24.theme.spot';
const DEFAULT_MODE = 'auto';
const FALLBACK_LATITUDE = 40;
const DEGREES_PER_HOUR = 15;
const MS_PER_HOUR = 3_600_000;
const MS_PER_DAY = 86_400_000;
const MIN_RECHECK_MS = 60_000;
const POLAR_RECHECK_MS = 6 * MS_PER_HOUR;
const GEO_TIMEOUT_MS = 10_000;
const GEO_MAX_AGE_MS = 7 * MS_PER_DAY;
/** The browser's address-bar color per theme (--color-app in src/styles/theme.css). Keep in step. */
const META_COLORS = { dark: '#050508', light: '#f4f4f7' };

// Sunrise equation (Meeus/NOAA): the sun's disc touches the horizon at -0.833 degrees.
const JULIAN_UNIX_EPOCH = 2440587.5;
const JULIAN_J2000 = 2451545.0;
const HORIZON_SIN = Math.sin((-0.833 * Math.PI) / 180);
const AXIAL_TILT_SIN = Math.sin((23.4397 * Math.PI) / 180);
const RAD = Math.PI / 180;

/** @type {string} */
let mode = DEFAULT_MODE;
/** @type {ReturnType<typeof setTimeout> | undefined} */
let timer;

/** Storage can be missing or throw (private tabs, blocked cookies): the theme still works, it just is not remembered. */
function read(/** @type {string} */ key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
function write(/** @type {string} */ key, /** @type {string} */ value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Not remembered on this device.
  }
}

/** Where the visitor is, as far as this device knows: shared coordinates, else the time zone's meridian. */
function spot() {
  const saved = JSON.parse(read(STORE_SPOT) ?? 'null');
  if (saved && Date.now() - saved.at < GEO_MAX_AGE_MS) return { lat: saved.lat, lon: saved.lon, shared: true };
  return { lat: FALLBACK_LATITUDE, lon: (-new Date().getTimezoneOffset() / 60) * DEGREES_PER_HOUR, shared: false };
}

/**
 * Sunrise and sunset (ms since epoch) of the solar day nearest `day`, or why there are none.
 * @returns {{ rise: number, set: number } | 'polar-day' | 'polar-night'}
 */
function sunOfDay(/** @type {number} */ day, /** @type {number} */ lat, /** @type {number} */ lon) {
  const julian = day / MS_PER_DAY + JULIAN_UNIX_EPOCH;
  const n = Math.ceil(julian - JULIAN_J2000 - 0.0009 + lon / 360);
  const solar = n - lon / 360;
  const anomaly = (357.5291 + 0.98560028 * solar) % 360;
  const center = 1.9148 * Math.sin(anomaly * RAD) + 0.02 * Math.sin(2 * anomaly * RAD) + 0.0003 * Math.sin(3 * anomaly * RAD);
  const eclipticLon = (anomaly + center + 180 + 102.9372) % 360;
  const transit = JULIAN_J2000 + solar + 0.0053 * Math.sin(anomaly * RAD) - 0.0069 * Math.sin(2 * eclipticLon * RAD);
  const sinDecl = Math.sin(eclipticLon * RAD) * AXIAL_TILT_SIN;
  const cosDecl = Math.cos(Math.asin(sinDecl));
  const cosHour = (HORIZON_SIN - Math.sin(lat * RAD) * sinDecl) / (Math.cos(lat * RAD) * cosDecl);
  if (cosHour > 1) return 'polar-night';
  if (cosHour < -1) return 'polar-day';
  const half = Math.acos(cosHour) / (2 * Math.PI);
  return { rise: (transit - half - JULIAN_UNIX_EPOCH) * MS_PER_DAY, set: (transit + half - JULIAN_UNIX_EPOCH) * MS_PER_DAY };
}

/**
 * What the sun says at `now`: the theme, and when that next flips.
 * @returns {{ theme: 'light' | 'dark', until: number, sunrise: number | null, sunset: number | null }}
 */
export function sunTheme(/** @type {number} */ now, /** @type {{lat:number, lon:number}} */ { lat, lon }) {
  const days = [-1, 0, 1].map((offset) => sunOfDay(now + offset * MS_PER_DAY, lat, lon));
  const events = days
    .flatMap((d) => (typeof d === 'string' ? [] : [{ at: d.rise, theme: 'light' }, { at: d.set, theme: 'dark' }]))
    .sort((a, b) => a.at - b.at);
  const last = events.filter((e) => e.at <= now).pop();
  const next = events.find((e) => e.at > now);
  if (!last || !next) {
    const polar = days[1];
    return { theme: polar === 'polar-day' ? 'light' : 'dark', until: now + POLAR_RECHECK_MS, sunrise: null, sunset: null };
  }
  const today = days[1];
  const [sunrise, sunset] = typeof today === 'string' ? [null, null] : [today.rise, today.set];
  return { theme: /** @type {'light' | 'dark'} */ (last.theme), until: next.at, sunrise, sunset };
}

/** @returns {'auto' | 'light' | 'dark'} */
export function getMode() {
  return mode;
}

/** The theme in force right now, and for auto, the sun behind it. */
export function resolve(/** @type {number} */ now = Date.now()) {
  if (mode !== 'auto') return { theme: /** @type {'light' | 'dark'} */ (mode), until: Infinity, sunrise: null, sunset: null };
  return sunTheme(now, spot());
}

/** Paints the page in `theme`, tells the browser chrome, and wakes whoever draws with tokens (the chart). */
function paint(/** @type {'light' | 'dark'} */ theme) {
  const root = document.documentElement;
  const changed = root.dataset.theme !== theme;
  root.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', META_COLORS[theme]);
  if (changed) window.dispatchEvent(new CustomEvent(THEME_EVENT, { detail: theme }));
}

/** Applies the current answer and, in auto, sleeps until the sun next flips it. */
export function apply() {
  clearTimeout(timer);
  const { theme, until } = resolve();
  paint(theme);
  write(STORE_LAST, `${theme}|${Number.isFinite(until) ? until : 0}`);
  if (!Number.isFinite(until)) return;
  timer = setTimeout(apply, Math.max(until - Date.now(), MIN_RECHECK_MS));
}

export function setMode(/** @type {string} */ next) {
  if (!MODES.includes(next)) return;
  mode = next;
  write(STORE_MODE, next);
  apply();
}

/** Asks the device where it is, once, for a truer sunset. Resolves false when the visitor says no. */
export function shareLocation() {
  if (!navigator.geolocation) return Promise.resolve(false);
  return new Promise((done) => {
    navigator.geolocation.getCurrentPosition(
      ({ coords }) => {
        write(STORE_SPOT, JSON.stringify({ lat: coords.latitude, lon: coords.longitude, at: Date.now() }));
        apply();
        done(true);
      },
      () => done(false),
      { timeout: GEO_TIMEOUT_MS, maximumAge: GEO_MAX_AGE_MS },
    );
  });
}

/** True once a real position backs auto; false while it is guessing from the time zone. */
export function knowsLocation() {
  return spot().shared;
}

// Start-up: read the choice, apply it, and follow it from other tabs and back from sleep.
mode = MODES.includes(read(STORE_MODE) ?? '') ? /** @type {string} */ (read(STORE_MODE)) : DEFAULT_MODE;
apply();
window.addEventListener('storage', (e) => {
  if (e.key !== STORE_MODE && e.key !== STORE_SPOT) return;
  mode = MODES.includes(read(STORE_MODE) ?? '') ? /** @type {string} */ (read(STORE_MODE)) : DEFAULT_MODE;
  apply();
});
document.addEventListener('visibilitychange', () => {
  if (!document.hidden) apply();
});
