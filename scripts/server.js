// Money 24 as a server: the spare desktop at home today, a Linux box (EC2) later. One install, then it
// runs for good and keeps itself on the newest signed release (scripts/updater.js, docs/SERVER.md).
//   node scripts/server.js install [--from FILE] [--no-start]   pin the release key, build the newest signed
//                                     release, start at boot (Windows logon task, or a systemd user unit), start now
//   node scripts/server.js run        the launcher the boot entry runs: keeps the supervisor running, starts it
//                                     again after an update, and rolls back when a new one cannot even start
//   node scripts/server.js status     the release, the one before, and every service
//   node scripts/server.js update     check for a signed release now instead of in a few minutes
//   node scripts/server.js rollback   go back to the release before
//   node scripts/server.js pack       on the old machine, services stopped: .env, the data and key files in
//                                     one file to carry to the new machine (it holds secrets: keep it private)
// deploy/install.ps1 (Windows) and deploy/install.sh (Linux) install the tools, then run install.
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { connect } from 'node:net';
import { homedir, tmpdir } from 'node:os';
import { basename, isAbsolute, join, relative } from 'node:path';
import { BINARIES, CONFIG_DIR, REPO_SIGNERS, ROOT, SIGNERS_FILE, newestRelease, npm, readRelease, rollback, stage, swapIn } from './updater.js';

const IS_WINDOWS = process.platform === 'win32';
const RUN_DIR = join(ROOT, '.run');
const FOREVER = join(ROOT, 'scripts', 'forever.js');
const ENV_FILE = join(ROOT, '.env');
const UPDATE_NOW_FILE = join(RUN_DIR, 'update.now');
const ROLLBACK_NOW_FILE = join(RUN_DIR, 'rollback.now');
// The supervisor's exit codes (scripts/forever.js).
const EXIT_RESTART = 75;
const EXIT_BUSY = 3;
const EXIT_STOPPED = 0;
const RESTART_START_MS = 5000;
const RESTART_MAX_MS = 5 * 60 * 1000;
const HEALTHY_AFTER_MS = 10 * 60 * 1000; // a supervisor that ran this long resets the wait before a restart
const MIN_NODE = [20, 19];
const MIN_GO = [1, 26];
const SERVICE_PORTS = [8024, 8025];
const PORT_CHECK_MS = 800;
// The boot entry on Linux: a systemd user unit, kept running after logout by lingering.
const UNIT_NAME = 'money24.service';
const UNIT_DIR = join(process.env.XDG_CONFIG_HOME ?? join(homedir(), '.config'), 'systemd', 'user');
// Windows: keep the desktop awake on mains power (the runner also holds it awake while it runs).
const POWER_SETTINGS = [['standby-timeout-ac', '0'], ['hibernate-timeout-ac', '0']];
// pack: what moves to the new machine. The backups the compaction scripts left behind stay behind.
const PACK_NAME = 'money24-move.tar.gz';
const PACK_DATA = ['trading', 'news'];
const PACK_ENV = '.env';
const PACK_KEYS = 'keys'; // files .env points to (a Kalshi private key, say), restored into CONFIG_DIR/keys
const ENV_LINE = /^(\s*[A-Z0-9_]+\s*=\s*)(["']?)(.*?)\2\s*$/i;
const TAR = IS_WINDOWS ? join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'tar.exe') : 'tar';
const FLAG = { from: '--from', noStart: '--no-start' };

function fail(/** @type {string} */ message) {
  console.error(`\n${message}\n`);
  process.exit(1);
}

/** Runs a command and returns what it printed, or null when it failed or is missing. */
function output(/** @type {string} */ command, /** @type {string[]} */ args) {
  const done = spawnSync(command, args, { cwd: ROOT, encoding: 'utf8', windowsHide: true });
  return done.status === 0 ? `${done.stdout}${done.stderr}`.trim() : null;
}

/** Whether a "1.26.3" or "v22.12.0" is at least [major, minor]. */
function atLeast(/** @type {string} */ version, /** @type {number[]} */ [major, minor]) {
  const [a, b] = (version.match(/(\d+)\.(\d+)/) ?? []).slice(1).map(Number);
  return a > major || (a === major && b >= minor);
}

function portBusy(/** @type {number} */ port) {
  return new Promise((done) => {
    const socket = connect({ host: '127.0.0.1', port });
    socket.setTimeout(PORT_CHECK_MS);
    socket.once('connect', () => { socket.destroy(); done(true); });
    socket.once('timeout', () => { socket.destroy(); done(false); });
    socket.once('error', () => done(false));
  });
}

/** The SHA256 fingerprint ssh-keygen shows for each key in an allowed-signers file. */
function fingerprints(/** @type {string} */ text) {
  return text.split(/\r?\n/).map((line) => line.match(/\bssh-ed25519 ([A-Za-z0-9+/=]+)/)?.[1]).filter(Boolean)
    .map((key) => `SHA256:${createHash('sha256').update(Buffer.from(/** @type {string} */ (key), 'base64')).digest('base64').replace(/=+$/, '')}`);
}

/** The tools a build needs, at the versions it needs. */
function checkTools() {
  const missing = [];
  if (!atLeast(process.version, MIN_NODE)) missing.push(`Node ${MIN_NODE.join('.')} or newer (this is ${process.version})`);
  const go = output('go', ['version']);
  if (!go || !atLeast(go.replace(/^go version go/, ''), MIN_GO)) missing.push(`Go ${MIN_GO.join('.')} or newer (${go ?? 'not found'})`);
  if (!output('git', ['--version'])) missing.push('git');
  const npmVersion = npm(['--version']);
  if (spawnSync(npmVersion.command, npmVersion.args, { shell: npmVersion.shell, windowsHide: true }).status !== 0) missing.push('npm');
  if (spawnSync('ssh-keygen', ['-?'], { windowsHide: true }).error) missing.push('ssh-keygen (OpenSSH), which checks release signatures');
  if (missing.length) fail(`Missing: ${missing.join('; ')}.\nRun deploy/install.ps1 (Windows) or deploy/install.sh (Linux), which installs them.`);
}

/**
 * Pins the release key on first install, from the repo as cloned. From then on the pinned copy rules:
 * a different key in a later checkout is reported and ignored.
 */
function pinKey() {
  if (!existsSync(REPO_SIGNERS)) fail(`${REPO_SIGNERS} is missing: this checkout has no release key.`);
  const repo = readFileSync(REPO_SIGNERS, 'utf8');
  if (!existsSync(SIGNERS_FILE)) {
    mkdirSync(CONFIG_DIR, { recursive: true });
    copyFileSync(REPO_SIGNERS, SIGNERS_FILE);
    console.log(`Pinned the release key ${fingerprints(repo).join(', ')} in ${SIGNERS_FILE}.\nOnly releases signed with it will ever be installed here. Compare it with the fingerprint on your own PC (npm run release -- --key).`);
    return;
  }
  const pinned = readFileSync(SIGNERS_FILE, 'utf8');
  if (pinned.trim() !== repo.trim()) console.log(`The repo's release key differs from the one pinned here, which stays: ${fingerprints(pinned).join(', ')}.\nTo trust the repo's key instead, delete ${SIGNERS_FILE} and install again.`);
}

/** pack's other half: puts .env, the data and the key files from a move file in place, refusing to overwrite data. */
function restore(/** @type {string} */ file) {
  if (!existsSync(file)) fail(`${file} does not exist.`);
  const existing = PACK_DATA.filter((dir) => existsSync(join(CONFIG_DIR, dir)) && readdirSync(join(CONFIG_DIR, dir)).length);
  if (existing.length) fail(`This machine already has Money 24 data in ${existing.map((d) => join(CONFIG_DIR, d)).join(' and ')}. Move it away first if the packed data should replace it.`);
  const tmp = mkdtempSync(join(tmpdir(), 'm24-move-'));
  const unpacked = spawnSync(TAR, ['-xzf', file, '-C', tmp], { encoding: 'utf8' });
  if (unpacked.status !== 0) fail(`Could not unpack ${file}: ${unpacked.stderr}`);
  mkdirSync(CONFIG_DIR, { recursive: true });
  const restored = PACK_DATA.filter((dir) => existsSync(join(tmp, dir)));
  for (const dir of restored) cpSync(join(tmp, dir), join(CONFIG_DIR, dir), { recursive: true });
  const keyDir = join(CONFIG_DIR, PACK_KEYS);
  const keys = existsSync(join(tmp, PACK_KEYS)) ? readdirSync(join(tmp, PACK_KEYS)) : [];
  for (const name of keys) {
    mkdirSync(keyDir, { recursive: true });
    copyFileSync(join(tmp, PACK_KEYS, name), join(keyDir, name));
  }
  // A path in .env to a key file now points at its copy here.
  const env = existsSync(join(tmp, PACK_ENV)) ? readFileSync(join(tmp, PACK_ENV), 'utf8').split(/\r?\n/).map((line) => {
    const m = line.startsWith('#') ? null : line.match(ENV_LINE);
    const name = m?.[3].split(/[\\/]/).pop() ?? '';
    return m && m[3] && keys.includes(name) ? `${m[1]}${join(keyDir, name).replace(/\\/g, '/')}` : line;
  }).join('\n') : '';
  if (env && existsSync(ENV_FILE)) {
    writeFileSync(`${ENV_FILE}.moved`, env);
    console.log(`.env already exists here and was kept; the packed one is .env.moved.`);
  } else if (env) writeFileSync(ENV_FILE, env);
  rmSync(tmp, { recursive: true, force: true });
  console.log(`Restored ${[env && '.env', ...restored, ...keys].filter(Boolean).join(', ')} (data into ${CONFIG_DIR}).`);
}

/** Starts at boot from now on. Windows: the Money24 logon task, through forever.js. Linux: a systemd user unit. */
function registerBoot(/** @type {boolean} */ startNow) {
  if (IS_WINDOWS) {
    for (const [setting, value] of POWER_SETTINGS) spawnSync('powercfg', ['/change', setting, value], { stdio: 'ignore' });
    console.log('Set this PC to never sleep or hibernate on mains power.');
    if (!startNow) {
      console.log('Not registering the logon task (--no-start). Later: node scripts/forever.js --install --launcher');
      return;
    }
    const done = spawnSync(process.execPath, [FOREVER, '--install', '--launcher'], { stdio: 'inherit' });
    if (done.status !== 0) fail('Registering the logon task failed (the error is above).');
    return;
  }
  const systemctl = (/** @type {string[]} */ args) => spawnSync('systemctl', ['--user', ...args], { stdio: 'inherit' }).status === 0;
  mkdirSync(UNIT_DIR, { recursive: true });
  writeFileSync(join(UNIT_DIR, UNIT_NAME), `[Unit]
Description=Money 24 server: news service, trading runner and tunnel, kept on the newest signed release
After=network-online.target
Wants=network-online.target

[Service]
WorkingDirectory=${ROOT}
ExecStart=${process.execPath} ${join(ROOT, 'scripts', 'server.js')} run
# The PATH install ran with: go, npm, git, cloudflared and the AI apps are found as they were then.
Environment=PATH=${process.env.PATH}
Restart=on-failure
RestartSec=10
KillMode=mixed
TimeoutStopSec=40

[Install]
WantedBy=default.target
`);
  console.log(`Wrote ${join(UNIT_DIR, UNIT_NAME)}.`);
  if (!systemctl(['daemon-reload'])) fail('systemctl --user did not answer. Is systemd running for this user? (On WSL, turn systemd on in /etc/wsl.conf.)');
  if (!systemctl(['enable', ...(startNow ? ['--now'] : []), UNIT_NAME])) fail('Could not enable the unit (the error is above).');
  const linger = spawnSync('loginctl', ['enable-linger', process.env.USER ?? ''], { stdio: 'ignore' }).status === 0;
  console.log(linger ? 'Lingering is on: it starts at boot and keeps running with nobody logged in.'
    : `Could not turn on lingering, so it only runs while you are logged in. Run once: sudo loginctl enable-linger ${process.env.USER}`);
}

async function install() {
  const args = process.argv.slice(3);
  const from = args.includes(FLAG.from) ? args[args.indexOf(FLAG.from) + 1] : '';
  const startNow = !args.includes(FLAG.noStart);
  checkTools();
  if (output('git', ['rev-parse', '--is-inside-work-tree']) !== 'true') fail(`${ROOT} is not a git checkout. Clone it: git clone https://github.com/YEAHDOGS/money24`);
  const dirty = output('git', ['status', '--porcelain', '--untracked-files=no']);
  if (dirty) fail(`This checkout has local changes, and updates replace the checkout:\n${dirty}\nUse a clean clone for the server.`);
  const busy = (await Promise.all(SERVICE_PORTS.map(portBusy))).some(Boolean);
  if (busy) fail('Money 24 is already running on this machine. Stop it first: node scripts/forever.js --stop');
  if (from) restore(from);
  pinKey();
  console.log('Looking for the newest signed release...');
  const found = await newestRelease((text) => console.log(text));
  const state = readRelease();
  if (found.error) fail(found.error);
  if (!found.release && !(state && BINARIES.every((name) => existsSync(join(RUN_DIR, name))))) fail('No release signed with the pinned key was found. Make one on your PC: npm run release');
  if (found.release) {
    console.log(`Building ${found.release.tag} (the first time takes a few minutes)...`);
    const built = await stage(found.release, (text) => console.log(`  ${text}`));
    if (!built.ok) fail(built.reason ?? 'The build failed.');
    await swapIn(found.release);
    console.log(`Installed ${found.release.tag}.`);
  }
  if (!existsSync(ENV_FILE)) console.log(`No .env here: the services run with defaults, and no tunnel. Copy .env from your old machine, or install with ${FLAG.from} ${PACK_NAME}.`);
  registerBoot(startNow);
  console.log(`\nMoney 24 is installed as a server. It checks for signed releases every few minutes.
  Status     node scripts/server.js status
  Desk       http://127.0.0.1:8025/desk/ here, or your tunnel's address
  Docs       docs/SERVER.md`);
}

/**
 * The launcher: runs the supervisor and starts it again after an update (EXIT_RESTART). A supervisor
 * that crashes while a new release is on probation is rolled back before the restart; other crashes
 * restart after a wait that doubles. A clean stop (forever.js --stop) or a second copy ends it.
 */
async function launch() {
  let wait = RESTART_START_MS;
  /** @type {import('node:child_process').ChildProcess | null} */
  let child = null;
  let ending = false;
  for (const signal of /** @type {NodeJS.Signals[]} */ (['SIGTERM', 'SIGINT'])) {
    process.on(signal, () => { ending = true; child?.kill(signal); });
  }
  for (;;) {
    const started = Date.now();
    child = spawn(process.execPath, [FOREVER, '--no-build'], { cwd: ROOT, stdio: 'inherit', windowsHide: true });
    const code = await new Promise((done) => child?.on('exit', (exitCode) => done(exitCode ?? 1)));
    if (ending || code === EXIT_STOPPED || code === EXIT_BUSY) process.exit(code === EXIT_BUSY ? EXIT_BUSY : 0);
    if (code === EXIT_RESTART) {
      wait = RESTART_START_MS;
      continue;
    }
    if (readRelease()?.pending && (await rollback(`the supervisor exited with code ${code} while the release was on probation`))) {
      console.log('The new release could not keep the supervisor running: rolled back.');
      continue;
    }
    if (Date.now() - started > HEALTHY_AFTER_MS) wait = RESTART_START_MS;
    console.log(`The supervisor exited with code ${code}: starting it again in ${wait / 1000}s.`);
    await new Promise((done) => setTimeout(done, wait));
    wait = Math.min(wait * 2, RESTART_MAX_MS);
  }
}

function status() {
  const state = readRelease();
  if (!state) fail('This machine is not installed as a server (node scripts/server.js install).');
  const when = (/** @type {number} */ ms) => new Date(ms).toLocaleString();
  console.log(`Release:          ${state.tag}, signed ${when(state.at)}${state.pending ? ', ON PROBATION' : ''}`);
  console.log(`Rolls back to:    ${state.previous ? state.previous.tag : 'nothing (no earlier release here)'}`);
  if (state.rolled_back) console.log(`Last rollback:    from ${state.rolled_back.tag}, ${when(state.rolled_back.at)}: ${state.rolled_back.reason}`);
  if (state.bad.length) console.log(`Never retried:    ${state.bad.join(', ')}`);
  console.log(`Pinned key:       ${existsSync(SIGNERS_FILE) ? fingerprints(readFileSync(SIGNERS_FILE, 'utf8')).join(', ') : 'NONE'}`);
  spawnSync(process.execPath, [FOREVER, '--status'], { stdio: 'inherit' });
}

/** Leaves a request file for the running supervisor, which looks for it every second. */
function ask(/** @type {string} */ file, /** @type {string} */ what) {
  if (!readRelease()) fail('This machine is not installed as a server (node scripts/server.js install).');
  mkdirSync(RUN_DIR, { recursive: true });
  writeFileSync(file, String(Date.now()));
  console.log(`Asked the supervisor to ${what}. Follow it with: node scripts/server.js status (and .run/logs/forever.log)`);
}

/** pack: .env, the data folders and the files .env points to, in one file for the new machine. */
async function pack() {
  if ((await Promise.all(SERVICE_PORTS.map(portBusy))).some(Boolean)) fail('Stop Money 24 first, so the data is not being written while it is packed: npm run forever -- --stop');
  const tmp = mkdtempSync(join(tmpdir(), 'm24-pack-'));
  const items = [];
  if (existsSync(ENV_FILE)) {
    copyFileSync(ENV_FILE, join(tmp, PACK_ENV));
    items.push({ dir: tmp, name: PACK_ENV });
    // Absolute file paths in .env outside the repo (a private key, say) travel too.
    for (const line of readFileSync(ENV_FILE, 'utf8').split(/\r?\n/)) {
      const value = line.startsWith('#') ? '' : line.match(ENV_LINE)?.[3] ?? '';
      if (!value || !isAbsolute(value) || !existsSync(value) || !relative(ROOT, value).startsWith('..')) continue;
      mkdirSync(join(tmp, PACK_KEYS), { recursive: true });
      copyFileSync(value, join(tmp, PACK_KEYS, basename(value)));
      console.log(`Packing ${value}, which .env points to.`);
    }
    if (existsSync(join(tmp, PACK_KEYS))) items.push({ dir: tmp, name: PACK_KEYS });
  }
  for (const dir of PACK_DATA) if (existsSync(join(CONFIG_DIR, dir))) items.push({ dir: CONFIG_DIR, name: dir });
  const out = join(process.cwd(), PACK_NAME);
  const args = ['-czf', out, ...items.flatMap((item) => ['-C', item.dir, item.name])];
  const done = spawnSync(TAR, args, { stdio: 'inherit' });
  rmSync(tmp, { recursive: true, force: true });
  if (done.status !== 0) fail('Packing failed (the error is above).');
  console.log(`\nWrote ${out}: ${items.map((i) => i.name).join(', ')}.\nIt holds your secrets (tunnel token, broker keys, sessions): carry it privately and delete it once it is restored.\nOn the new machine: deploy/install.ps1 -From <path to it>   or   deploy/install.sh --from <path to it>`);
}

const COMMANDS = { install, run: launch, status, pack, update: () => ask(UPDATE_NOW_FILE, 'check for a signed release now'),
  rollback: () => ask(ROLLBACK_NOW_FILE, 'roll back to the release before') };
const command = /** @type {keyof typeof COMMANDS} */ (process.argv[2]);
if (!COMMANDS[command]) fail(`Usage: node scripts/server.js ${Object.keys(COMMANDS).join(' | ')}\nSee docs/SERVER.md.`);
await COMMANDS[command]();
