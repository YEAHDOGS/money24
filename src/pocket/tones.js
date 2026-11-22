// Each row tone as Tailwind classes: the dot's fill and the state word's color. Shared by every pocket row.
import { TONE } from './pocket-model.js';

export const TONE_CLASSES = {
  [TONE.ok]: { dot: 'bg-ok', text: 'text-ok' },
  [TONE.warn]: { dot: 'bg-warn', text: 'text-warn' },
  [TONE.bad]: { dot: 'bg-bad', text: 'text-bad' },
  [TONE.idle]: { dot: 'bg-muted/50', text: 'text-muted' },
};
