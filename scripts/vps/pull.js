// pull-backup: copies new and changed data from the cloud machine to this PC. It is the
// free alternative to R2. Every byte leaves Google, so a run stays inside the egress room
// the machine's guard reports and under PULL_MAX_BYTES.
//
// The news database is never copied. The machine writes each finished New York day once into
// news/days/YYYY-MM-DD.jsonl.gz (a big day continues in YYYY-MM-DD.002.jsonl.gz and so on) and
// the open day plus small tables into news/state.jsonl.gz; those are what the PC pulls. A day
// file never changes, so it is pulled once. Rebuild a database from them with
// m24backup --rebuild-news (see docs/CLOUD.md).
import { existsSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { dirname, join } from 'node:path';

export const PULL_MAX_BYTES = 50_000_000;
const MANIFEST = '.money24-pull.json';
const SAFE_PATH = /^[A-Za-z0-9._/-]+$/;
const STAGING = /^\/tmp\/money24\.[A-Za-z0-9]{10}$/;
const TAB = '\t';
const NEWS_DB = /^news\/news\.db(-wal|-shm|-journal)?$/;
const NEWS_STATE = 'news/state.jsonl.gz';
const NEWS_DAYS = 'news/days/';
// Priorities when a night is tight: trading data first, then the news state, then news days.
const PRIORITY_DATA = 0;
const PRIORITY_NEWS_STATE = 1;
const PRIORITY_NEWS_DAY = 2;
// What the tar archive adds to the files it holds, so a run never lands over its room.
const TAR_ENTRY_OVERHEAD = 1024;
const TAR_END_OVERHEAD = 10240;

/**
 * Parses pull-manifest output: a ROOM line, then one F line per file.
 * @returns {{room: number, files: Map<string, {size: number, mtime: string}>}}
 */
export function parseManifest(text) {
  let room = 0;
  const files = new Map();
  for (const line of text.split(/\r?\n/)) {
    const parts = line.split(TAB);
    if (parts[0] === 'ROOM') room = Number(parts[1]) || 0;
    if (parts[0] !== 'F' || parts.length !== 4) continue;
    const [, size, mtime, path] = parts;
    if (!SAFE_PATH.test(path) || path.includes('..') || NEWS_DB.test(path)) continue;
    files.set(path, { size: Number(size), mtime });
  }
  return { room, files };
}

function priority(path) {
  if (path === NEWS_STATE) return PRIORITY_NEWS_STATE;
  return path.startsWith(NEWS_DAYS) ? PRIORITY_NEWS_DAY : PRIORITY_DATA;
}

/**
 * Picks changed files in priority order whose size, with the archive's own overhead, fits the budget:
 * trading data (newest first), then the news state, then news days oldest first. Data that does not
 * fit is skipped so smaller files still go; news days stop at the first one that does not fit, so
 * the PC copy never has a hole in its history. Compression only helps.
 * @returns {{picked: string[], waiting: number}}
 */
export function choose(remote, local, budget) {
  const changed = [...remote.entries()]
    .filter(([path, file]) => local[path]?.size !== file.size || local[path]?.mtime !== file.mtime)
    .sort(([pathA, a], [pathB, b]) => {
      const byPriority = priority(pathA) - priority(pathB);
      if (byPriority) return byPriority;
      return priority(pathA) === PRIORITY_NEWS_DAY ? pathA.localeCompare(pathB) : Number(b.mtime) - Number(a.mtime);
    });
  const picked = [];
  let total = TAR_END_OVERHEAD;
  let newsFull = false;
  for (const [path, file] of changed) {
    const isDay = priority(path) === PRIORITY_NEWS_DAY;
    if (isDay && newsFull) continue;
    if (total + file.size + TAR_ENTRY_OVERHEAD > budget) {
      newsFull = newsFull || isDay;
      continue;
    }
    picked.push(path);
    total += file.size + TAR_ENTRY_OVERHEAD;
  }
  return { picked, waiting: changed.length - picked.length };
}

function readLocal(folder) {
  const file = join(folder, MANIFEST);
  return existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : {};
}

/** Runs the pull. vm is the kit's VM helper; runDir holds the temporary download. */
export function pullBackup(vm, folder, runDir) {
  if (vm.dry) {
    vm.admin('pull-manifest', { show: false });
    console.log(`  [dry-run] compare with ${join(folder, MANIFEST)}; pick changed files (trading data, the news state,`);
    console.log(`            then news days oldest first) within the guard's egress room and ${PULL_MAX_BYTES / 1e6} MB`);
    vm.admin('pull-pack', { show: false, pullList: ['<changed files>'] });
    vm.call(['compute', 'scp', `${vm.instance}:/tmp/money24.XXXXXXXXXX/pull.tar.gz`, '.run/vps/pull-XXXX/pull.tar.gz'], undefined, true);
    vm.admin('pull-clean', { args: ['/tmp/money24.XXXXXXXXXX'], show: false });
    console.log(`  [dry-run] extract into ${folder} and record what arrived`);
    return;
  }
  mkdirSync(folder, { recursive: true });
  const { room, files } = parseManifest(vm.admin('pull-manifest', { show: false }));
  const local = readLocal(folder);
  const budget = Math.min(PULL_MAX_BYTES, room);
  const { picked, waiting } = choose(files, local, budget);
  if (!picked.length) {
    console.log(waiting ? `Nothing fits tonight's egress room (${room} bytes); ${waiting} file(s) wait.` : 'The PC copy is up to date.');
    return;
  }
  const packed = vm.admin('pull-pack', { show: false, pullList: picked }).trim().split(TAB);
  const [tag, staging, size] = packed;
  if (tag !== 'PACK' || !STAGING.test(staging)) throw new Error('The machine did not return a pull archive.');
  mkdirSync(runDir, { recursive: true });
  const scratch = mkdtempSync(join(runDir, 'pull-'));
  try {
    if (Number(size) > budget) throw new Error(`The archive is ${size} bytes, over the ${budget}-byte room. Nothing was sent.`);
    const archive = join(scratch, 'pull.tar.gz');
    vm.call(['compute', 'scp', `${vm.instance}:${staging}/pull.tar.gz`, vm.relative(archive)], undefined, false);
    const tree = join(scratch, 'tree');
    mkdirSync(tree);
    const untar = spawnSync('tar', ['-xzf', archive, '-C', tree], { stdio: 'inherit' });
    if (untar.status !== 0) throw new Error('The pull archive did not extract.');
    for (const path of picked) {
      if (!existsSync(join(tree, path))) continue; // removed on the machine since the manifest
      const target = join(folder, path);
      mkdirSync(dirname(target), { recursive: true });
      renameSync(join(tree, path), target);
      local[path] = files.get(path);
    }
    writeFileSync(join(folder, `${MANIFEST}.tmp`), JSON.stringify(local));
    renameSync(join(folder, `${MANIFEST}.tmp`), join(folder, MANIFEST));
    console.log(`Pulled ${picked.length} file(s), ${size} bytes, into ${folder}.${waiting ? ` ${waiting} wait for a later run.` : ''}`);
  } finally {
    vm.admin('pull-clean', { args: [staging], show: false });
    rmSync(scratch, { recursive: true, force: true });
  }
}
