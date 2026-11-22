// pull-backup: copies new and changed data from the cloud machine to this PC. It is the
// free alternative to R2. Every byte leaves Google, so a run stays inside the egress room
// the machine's guard reports and under PULL_MAX_BYTES.
import { existsSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { dirname, join } from 'node:path';

export const PULL_MAX_BYTES = 50_000_000;
const MANIFEST = '.money24-pull.json';
const SAFE_PATH = /^[A-Za-z0-9._/-]+$/;
const STAGING = /^\/tmp\/money24\.[A-Za-z0-9]{10}$/;
const TAB = '\t';

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
    if (!SAFE_PATH.test(path) || path.includes('..')) continue;
    files.set(path, { size: Number(size), mtime });
  }
  return { room, files };
}

/** Picks changed files, newest first, whose raw size fits the budget. Compression only helps. */
export function choose(remote, local, budget) {
  const changed = [...remote.entries()]
    .filter(([path, file]) => local[path]?.size !== file.size || local[path]?.mtime !== file.mtime)
    .sort((a, b) => Number(b[1].mtime) - Number(a[1].mtime));
  const picked = [];
  let total = 0;
  for (const [path, file] of changed) {
    if (total + file.size > budget) continue;
    picked.push(path);
    total += file.size;
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
    console.log(`  [dry-run] compare with ${join(folder, MANIFEST)}; pick changed files, newest first, within`);
    console.log(`            the guard's egress room and ${PULL_MAX_BYTES / 1e6} MB`);
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
