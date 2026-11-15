// Records the plays board's demo from a running m24d: every play with its report, the link
// history, the rulebook, the AI providers (names only, never keys) and the stake dial at every
// size, into public/demo/. The public site opens on this snapshot when no runner is set up.
//   npm start            (in another terminal: a runner with the plays you want to show)
//   node scripts/demo-snapshot.js [runner URL]
import { mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const DEMO_DIR = join(ROOT, 'public', 'demo');
const PREVIEW_DIR = join(DEMO_DIR, 'preview');
const DEFAULT_RUNNER = 'http://127.0.0.1:8025';
const SNAPSHOT_FILE = 'plays.json';
const PREVIEW_BATCH = 8;

const runner = (process.argv[2] ?? DEFAULT_RUNNER).replace(/\/+$/, '');

async function get(/** @type {string} */ path) {
  const res = await fetch(runner + path);
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return res.json();
}

/** The dial's view at each size, keeping only what a resize changes (the page merges it onto the play). */
async function previews(/** @type {{play: any, report: any}} */ item) {
  const { min_units: min, slider_max: max } = item.report.sizing;
  const top = Math.max(max, min + 1);
  /** @type {Record<number, any>} */
  const sizes = {};
  for (let u = min; u <= top; u += PREVIEW_BATCH) {
    const batch = [];
    for (let k = u; k < u + PREVIEW_BATCH && k <= top; k++) batch.push(k);
    const got = await Promise.all(batch.map((k) => get(`/api/plays/${encodeURIComponent(item.play.id)}/preview?units=${k}`)));
    got.forEach((p, i) => {
      sizes[batch[i]] = { play: { qty: p.play.qty, ladder: p.play.ladder, runner: p.play.runner }, report: p.report };
    });
  }
  return sizes;
}

let items;
try {
  items = await get('/api/plays');
} catch (err) {
  console.error(`\nNo runner at ${runner} (${err instanceof Error ? err.message : err}). Start one with npm start.\n`);
  process.exit(1);
}
if (!items.length) {
  console.error('\nThe runner has no plays to record. Paste a few links on the plays board first.\n');
  process.exit(1);
}
const [links, rules, providers] = await Promise.all([get('/api/plays/links'), get('/api/plays/rules'), get('/api/plays/ai')]);

mkdirSync(PREVIEW_DIR, { recursive: true });
for (const f of readdirSync(PREVIEW_DIR)) rmSync(join(PREVIEW_DIR, f));
const takenAt = new Date().toISOString();
writeFileSync(join(DEMO_DIR, SNAPSHOT_FILE), JSON.stringify({ taken_at: takenAt, items, links, rules, providers }));
for (const item of items) {
  writeFileSync(join(PREVIEW_DIR, `${item.play.id}.json`), JSON.stringify(await previews(item)));
}
console.log(`Recorded ${items.length} plays into public/demo/ (taken ${takenAt}).`);
