// Display helpers for the news archive: New York times, short counts and link labels.

const ARCHIVE_ZONE = 'America/New_York';
const THOUSAND = 1000;
const ONE_DECIMAL_BELOW = 10 * THOUSAND; // 8.2k, but 12k
const WWW_PREFIX = /^www\./;
const TRAILING_ZERO = /\.0$/;
const MIDNIGHT_UTC = 'T00:00:00Z';
const UTC = 'UTC';
/** Aggregators whose links redirect to the outlet: the article's own address is not in their feeds. */
const REDIRECT_HOSTS = ['news.google.com'];

const TIME_FORMAT = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', timeZone: ARCHIVE_ZONE });
const DAY_FORMAT = new Intl.DateTimeFormat(undefined, { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric', timeZone: UTC });

/** An archive time as New York wall-clock time. @param {string} iso */
export function etTime(iso) {
  return TIME_FORMAT.format(new Date(iso));
}

/** "Saturday, November 21, 2026" for a YYYY-MM-DD day key. @param {string} day */
export function longDay(day) {
  return DAY_FORMAT.format(new Date(day + MIDNIGHT_UTC));
}

/** 940, 8.2k, 12k. @param {number} n */
export function shortCount(n) {
  if (n < THOUSAND) return String(n);
  const k = n / THOUSAND;
  return `${n < ONE_DECIMAL_BELOW ? k.toFixed(1).replace(TRAILING_ZERO, '') : Math.round(k)}k`;
}

/** The host of a link without "www.", or '' for a bad one. @param {string|undefined} url */
export function hostOf(url) {
  if (!url) return '';
  try {
    return new URL(url).hostname.replace(WWW_PREFIX, '');
  } catch {
    return '';
  }
}

/** True when a link goes through an aggregator's redirect rather than straight to the article. @param {string} url */
export function isRedirect(url) {
  return REDIRECT_HOSTS.includes(hostOf(url));
}
