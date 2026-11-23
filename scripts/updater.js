// Signed releases for a Money 24 server: find the newest release tag, check its signature against
// the pinned release key, build it beside the running release, switch to it, and roll back.
//
// A release is a git tag named release-YYYY.MM.DD-HHMM, signed with the owner's SSH release key
// (scripts/release.js). The server pins that key once, at install, in its config folder: outside the
// repo, so nothing fetched from the repo can change which key it trusts. A tag counts only when:
//   - git verifies its signature against the pinned key (an unsigned tag, or one signed by any other
//     key, is ignored, so a spoofed DNS answer or a hijacked GitHub account cannot push code);
//   - the name inside the signed tag matches the tag's name (an old release can't come back under a
//     new name), and it is newer than the release running now (no downgrades).
// npm ci and go build check every package against the hashes in package-lock.json and go.sum, which
// are part of the signed commit.
//
// Layout, all in .run/ (git-ignored): the running release's newsd, m24d and web/; stage/ is a git
// worktree of the release being built and stage-out/ what it built; prev/ is the release before, kept
// for a rollback; release.json says which release runs, the one before, and the ones rolled back.
import { spawn } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { WEB_ENV } from './desk-build.js';

export const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const RUN_DIR = join(ROOT, '.run');
export const RELEASE_FILE = join(RUN_DIR, 'release.json');
const STAGE_DIR = join(RUN_DIR, 'stage');
const STAGE_OUT = join(RUN_DIR, 'stage-out');
const PREV_DIR = join(RUN_DIR, 'prev');
const IS_WINDOWS = process.platform === 'win32';
const EXE = IS_WINDOWS ? '.exe' : '';
const SERVICE_NAMES = ['newsd', 'm24d'];
export const BINARIES = SERVICE_NAMES.map((name) => name + EXE);
const WEB = 'web';
const SWAPPED = [...BINARIES, WEB];
export const RELEASE_PREFIX = 'release-';
// Go's os.UserConfigDir: where newsd and m24d keep their data, beside the pinned key.
export const CONFIG_DIR = IS_WINDOWS
  ? join(process.env.APPDATA ?? join(homedir(), 'AppData', 'Roaming'), 'Money24')
  : join(process.env.XDG_CONFIG_HOME ?? join(homedir(), '.config'), 'Money24');
// M24_SIGNERS_FILE moves the pinned key file (tests use it); whoever can set it already owns the machine.
export const SIGNERS_FILE = process.env.M24_SIGNERS_FILE || join(CONFIG_DIR, 'release-signers');
export const REPO_SIGNERS = join(ROOT, 'deploy', 'release-signers');
const GOOD_SIGNATURE = /^Good "git" signature for \S+ with /m;
const KEEP_BAD = 20;
const STEP_TIMEOUT_MS = 20 * 60 * 1000; // a first npm ci on a slow line can take a while
const OUTPUT_KEEP = 4000; // characters of a step's output kept for its error message
const ERROR_LINES = 6;
const MS_PER_SECOND = 1000;
const VITE = ['node_modules', 'vite', 'bin', 'vite.js'];

/**
 * Runs a command to the end without blocking the event loop (the supervisor keeps health-checking
 * meanwhile). npm on Windows is a .cmd, which only starts through a shell; its arguments are constants.
 * @returns {Promise<{ ok: boolean, out: string }>}
 */
export function run(/** @type {string} */ command, /** @type {string[]} */ args, /** @type {{ cwd?: string, env?: NodeJS.ProcessEnv, shell?: boolean }} */ opts = {}) {
  return new Promise((done) => {
    const child = spawn(command, args, { cwd: opts.cwd ?? ROOT, env: opts.env ?? process.env, shell: opts.shell ?? false, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
    let out = '';
    const keep = (/** @type {Buffer} */ chunk) => { out = (out + chunk).slice(-OUTPUT_KEEP); };
    child.stdout.on('data', keep);
    child.stderr.on('data', keep);
    const timer = setTimeout(() => child.kill(), STEP_TIMEOUT_MS);
    child.on('error', (err) => { clearTimeout(timer); done({ ok: false, out: err.message }); });
    child.on('close', (code) => { clearTimeout(timer); done({ ok: code === 0, out: out.trim() }); });
  });
}

/** The last few lines of a step's output, for a one-paragraph error. */
function tail(/** @type {string} */ out) {
  return out.split(/\r?\n/).filter(Boolean).slice(-ERROR_LINES).join(' | ');
}

/**
 * @typedef {{ tag: string, commit: string, at: number }} Release
 * @typedef {Release & { previous: Release | null, pending: number | null, bad: string[], rolled_back?: { tag: string, reason: string, at: number } }} ReleaseState
 */

/** What release.json says, or null on a machine that was never installed as a server. @returns {ReleaseState | null} */
export function readRelease() {
  try {
    return JSON.parse(readFileSync(RELEASE_FILE, 'utf8'));
  } catch {
    return null;
  }
}

export function writeRelease(/** @type {ReleaseState} */ state) {
  mkdirSync(RUN_DIR, { recursive: true });
  writeFileSync(`${RELEASE_FILE}.tmp`, JSON.stringify(state, null, 1));
  renameSync(`${RELEASE_FILE}.tmp`, RELEASE_FILE);
}

/**
 * Checks one tag: an annotated tag on a commit, its signed name the same as its ref name, and a
 * signature git verifies against the pinned key. @returns {Promise<{ ok: true } & Release | { ok: false, reason: string }>}
 */
export async function verifyTag(/** @type {string} */ tag) {
  const raw = await run('git', ['cat-file', 'tag', tag]);
  if (!raw.ok) return { ok: false, reason: 'not a signed release tag' };
  const name = raw.out.match(/^tag (.+)$/m)?.[1];
  const commit = raw.out.match(/^object ([0-9a-f]{40,64})$/m)?.[1];
  const stamp = raw.out.match(/^tagger .* (\d+) [+-]\d{4}$/m)?.[1];
  if (!/^type commit$/m.test(raw.out) || !commit || !stamp) return { ok: false, reason: 'not a release tag' };
  if (name !== tag) return { ok: false, reason: `its signed name is ${name}` };
  if (!existsSync(SIGNERS_FILE)) return { ok: false, reason: `no release key pinned at ${SIGNERS_FILE}` };
  const check = await run('git', ['-c', `gpg.ssh.allowedSignersFile=${SIGNERS_FILE}`, 'verify-tag', '--raw', tag]);
  if (!check.ok || !GOOD_SIGNATURE.test(check.out)) return { ok: false, reason: `not signed by the pinned release key (${tail(check.out) || 'no signature'})` };
  return { ok: true, tag, commit, at: Number(stamp) * MS_PER_SECOND };
}

/** Tags already reported as unsigned or forged, so each is logged once per supervisor run. */
const reported = new Set();

/**
 * Fetches the repository's tags and returns the newest signed release newer than the running one.
 * @returns {Promise<{ release?: Release, error?: string }>}
 */
export async function newestRelease(/** @type {(text: string) => void} */ say = () => {}) {
  const fetched = await run('git', ['fetch', '--quiet', '--force', '--tags', 'origin']);
  if (!fetched.ok) return { error: `could not fetch from the repository: ${tail(fetched.out)}` };
  const listed = await run('git', ['tag', '--list', `${RELEASE_PREFIX}*`, '--sort=-creatordate']);
  const state = readRelease();
  const bad = new Set(state?.bad ?? []);
  for (const tag of listed.out.split(/\r?\n/).filter(Boolean)) {
    if (tag === state?.tag) return {};
    if (bad.has(tag)) continue;
    const found = await verifyTag(tag);
    if (!found.ok) {
      if (!reported.has(tag)) say(`Release ${tag} is ignored: ${found.reason}`);
      reported.add(tag);
      continue;
    }
    if (found.at <= (state?.at ?? 0)) return {};
    return { release: { tag: found.tag, commit: found.commit, at: found.at } };
  }
  return {};
}

/** Removes the staging worktree and what it built. */
async function clearStage() {
  await run('git', ['worktree', 'remove', '--force', STAGE_DIR]);
  rmSync(STAGE_DIR, { recursive: true, force: true });
  await run('git', ['worktree', 'prune']);
  rmSync(STAGE_OUT, { recursive: true, force: true });
}

/**
 * Builds a release beside the running one: a fresh worktree of its commit, its exact packages, both
 * services and the pages into stage-out/. Nothing running changes. @returns {Promise<{ ok: boolean, reason?: string }>}
 */
export async function stage(/** @type {Release} */ release, /** @type {(text: string) => void} */ say = () => {}) {
  await clearStage();
  const goEnv = { ...process.env, CGO_ENABLED: '0' };
  /** @type {[string, string, string[], { cwd?: string, env?: NodeJS.ProcessEnv, shell?: boolean }][]} */
  const steps = [
    [`Checking out ${release.tag}`, 'git', ['worktree', 'add', '--detach', '--force', STAGE_DIR, release.commit], {}],
    // --ignore-scripts: no package runs its own install code; the build needs none.
    ['Installing packages', 'npm', ['ci', '--no-audit', '--no-fund', '--ignore-scripts'], { cwd: STAGE_DIR, shell: IS_WINDOWS }],
    ...SERVICE_NAMES.map((name) => /** @type {[string, string, string[], any]} */ ([`Building ${name}`, 'go',
      ['build', '-buildvcs=false', '-trimpath', '-o', join(STAGE_OUT, name + EXE), `./cmd/${name}`], { cwd: join(STAGE_DIR, 'engine'), env: goEnv }])),
    ['Building the pages', process.execPath, [join(STAGE_DIR, ...VITE), 'build', '--outDir', join(STAGE_OUT, WEB), '--emptyOutDir', '--logLevel', 'warn'],
      { cwd: STAGE_DIR, env: { ...process.env, ...WEB_ENV } }],
  ];
  for (const [what, command, args, opts] of steps) {
    say(`${what}...`);
    const done = await run(command, args, opts);
    if (!done.ok) return { ok: false, reason: `${what} failed: ${tail(done.out)}` };
  }
  return { ok: true };
}

/**
 * Switches to a staged release while the services are stopped: the running build moves to prev/, the
 * staged one takes its place, and the checkout moves to the release's commit so the supervisor's own
 * scripts follow. A first install (nothing running yet) keeps no previous release.
 */
export async function swapIn(/** @type {Release} */ release) {
  const state = readRelease();
  const hadBuild = BINARIES.every((name) => existsSync(join(RUN_DIR, name)));
  rmSync(PREV_DIR, { recursive: true, force: true });
  mkdirSync(PREV_DIR, { recursive: true });
  for (const name of SWAPPED) {
    if (existsSync(join(RUN_DIR, name))) renameSync(join(RUN_DIR, name), join(PREV_DIR, name));
    renameSync(join(STAGE_OUT, name), join(RUN_DIR, name));
  }
  const moved = await run('git', ['checkout', '--force', '--detach', release.commit]);
  if (!moved.ok) throw new Error(`could not check out ${release.tag}: ${tail(moved.out)}`);
  const previous = state && hadBuild ? { tag: state.tag, commit: state.commit, at: state.at } : null;
  writeRelease({ ...release, previous, pending: previous ? Date.now() : null, bad: state?.bad ?? [] });
  await clearStage();
}

/** The running release passed its first minutes: it stays, and is no longer on probation. */
export function confirm() {
  const state = readRelease();
  if (state?.pending) writeRelease({ ...state, pending: null });
}

/**
 * Goes back to the release before, while the services are stopped. The release rolled back from is
 * never tried again. @returns {Promise<boolean>} false when there is no earlier release to go back to.
 */
export async function rollback(/** @type {string} */ reason) {
  const state = readRelease();
  if (!state?.previous || !BINARIES.every((name) => existsSync(join(PREV_DIR, name)))) return false;
  for (const name of SWAPPED) {
    rmSync(join(RUN_DIR, name), { recursive: true, force: true });
    if (existsSync(join(PREV_DIR, name))) renameSync(join(PREV_DIR, name), join(RUN_DIR, name));
  }
  rmSync(PREV_DIR, { recursive: true, force: true });
  const moved = await run('git', ['checkout', '--force', '--detach', state.previous.commit]);
  if (!moved.ok) throw new Error(`could not check out ${state.previous.tag}: ${tail(moved.out)}`);
  writeRelease({ ...state.previous, previous: null, pending: null, bad: [...state.bad, state.tag].slice(-KEEP_BAD),
    rolled_back: { tag: state.tag, reason, at: Date.now() } });
  return true;
}
