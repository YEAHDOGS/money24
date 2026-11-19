// How a headline's category and angle look on the Home page: full class strings, so Tailwind can
// see them. Plain headlines take the news page's own kind colors; story categories stay neutral.
// An angle always carries a mark beside its color, so it never reads by color alone.
import { FALLBACK_KIND, KIND_CLASSES } from '../feed/post-format.js';
import { MODE_EVENTS } from './news-model.js';

const NEUTRAL = { text: 'text-white/80', border: 'border-white/40', bg: 'bg-white/60' };
const NO_ANGLE = { text: 'text-muted', border: 'border-line', mark: '' };
const ANGLES = {
  bullish: { text: 'text-ok', border: 'border-ok/50', mark: '▲' },
  bearish: { text: 'text-bad', border: 'border-bad/50', mark: '▼' },
  mixed: { text: 'text-warn', border: 'border-warn/50', mark: '◆' },
  unclear: { text: 'text-muted', border: 'border-line', mark: '?' },
};

/** @returns {{ text: string, border: string, bg: string }} */
export function categoryStyle(/** @type {string} */ mode, /** @type {string} */ id) {
  if (mode !== MODE_EVENTS) return NEUTRAL;
  return KIND_CLASSES[id] ?? FALLBACK_KIND;
}

/** @returns {{ text: string, border: string, mark: string }} */
export function angleStyle(/** @type {string} */ angle) {
  return ANGLES[angle] ?? NO_ANGLE;
}
