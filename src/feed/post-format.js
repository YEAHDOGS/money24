// Display helpers for feed posts: relative times and category colors.

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const JUST_NOW = 10 * SECOND;

/** Full class strings so Tailwind can see them. One entry per kind in strings.json. */
export const KIND_CLASSES = {
  news: { text: 'text-kind-news', border: 'border-kind-news/40', bg: 'bg-kind-news' },
  filing: { text: 'text-kind-filing', border: 'border-kind-filing/40', bg: 'bg-kind-filing' },
  insider: { text: 'text-kind-insider', border: 'border-kind-insider/40', bg: 'bg-kind-insider' },
  central_bank: { text: 'text-kind-central-bank', border: 'border-kind-central-bank/40', bg: 'bg-kind-central-bank' },
  macro: { text: 'text-kind-macro', border: 'border-kind-macro/40', bg: 'bg-kind-macro' },
};
export const FALLBACK_KIND = { text: 'text-muted', border: 'border-line', bg: 'bg-muted' };

/**
 * Short age like "now", "42s", "7m", "3h", "2d".
 * @param {string|null} iso
 * @param {number} now epoch ms
 */
export function age(iso, now) {
  if (!iso) return '';
  const ms = Math.max(0, now - Date.parse(iso));
  if (ms < JUST_NOW) return 'now';
  if (ms < MINUTE) return `${Math.floor(ms / SECOND)}s`;
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)}m`;
  if (ms < DAY) return `${Math.floor(ms / HOUR)}h`;
  return `${Math.floor(ms / DAY)}d`;
}
