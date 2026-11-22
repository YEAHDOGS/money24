// Runs Money 24 around the clock on this machine: builds the two Go services and the pages, then
// keeps the services running. A service that exits or stops answering is restarted on its own,
// with a growing wait between tries. The runner serves the pages itself, so there is one address
// to open, and one address for a tunnel to a phone.
//   node scripts/forever.js              (or: npm run forever) build, run and supervise; Ctrl+C stops
//   node scripts/forever.js --no-build   run what was built last time
//   node scripts/forever.js --status     what is running, since when, and how healthy
//   node scripts/forever.js --stop       stop the supervisor and both services
//   node scripts/forever.js --install    start at logon from now on, and start now (--dry-run only prints)
//   node scripts/forever.js --uninstall  no longer start at logon
// Needs Node and Go on PATH. Settings are read from .env when there is one (see .env.example).
// Nothing here can place a real order.
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, readFileSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { connect } from 'node:net';
import { userInfo } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT = fileURLToPath(import.meta.url);
const ROOT = resolve(dirname(SCRIPT), '..');
const ENGINE_DIR = join(ROOT, 'engine');
const RUN_DIR = join(ROOT, '.run'); // git-ignored: builds, logs and the supervisor's own files
const LOG_DIR = join(RUN_DIR, 'logs');
const WEB_DIR = join(RUN_DIR, 'web'); // the pages, built to call the runner that serves them
const WEB_NEXT_DIR = join(RUN_DIR, 'web.next'); // a build lands here first, so a failed one leaves the last good pages
const WEB_START_PAGE = 'home.html';
const PID_FILE = join(RUN_DIR, 'forever.pid');
const STATE_FILE = join(RUN_DIR, 'forever.json');
const STOP_FILE = join(RUN_DIR, 'forever.stop'); // --stop leaves this; the supervisor sees it and shuts down cleanly
const TASK_FILE = join(RUN_DIR, 'money24-task.xml');
const ENV_FILE = join(ROOT, '.env');
const VITE_BIN = join(ROOT, 'node_modules', 'vite', 'bin', 'vite.js');
const IS_WINDOWS = process.platform === 'win32';
const EXE = IS_WINDOWS ? '.exe' : '';
const HOST = '127.0.0.1';
const RUNNER_PORT = 8025;
const SERVICES = [
  { name: 'newsd', port: 8024 },
  { name: 'm24d', port: RUNNER_PORT },
];
const RUNNER = 'm24d';
const SUPERVISOR = 'forever'; // the supervisor's own log
const FLAG = { noBuild: '--no-build', stop: '--stop', status: '--status', install: '--install', uninstall: '--uninstall', dryRun: '--dry-run', noTray: '--no-tray' };
// Both services exit on their own when this pipe closes, so a supervisor that dies leaves nothing behind.
const EXIT_WITH_STDIN = '--exit-with-stdin';
// The desk lives under this path, here (http://127.0.0.1:8025/desk/) and on money24.dogs.red/desk/.
const DESK_PATH = '/desk';
const RUNNER_FLAGS = ['--web', WEB_DIR, '--base', DESK_PATH, '--keep-awake'];
const ENV_RUNNER_ARGS = 'M24D_ARGS'; // replaces RUNNER_FLAGS, space-separated
const ENV_PUBLIC_HOST = 'M24D_PUBLIC_HOST'; // a tunnel's hostname, added to the hosts the runner answers to
const ENV_HOSTS = 'M24D_HOSTS';
const ENV_SUPERVISOR_STATE = 'M24D_SUPERVISOR_STATE'; // the state file, which the runner serves at /api/supervisor
// The Cloudflare Tunnel runs as a third service when .env holds its token (docs/RUNNING.md). cloudflared
// reads the token from TUNNEL_TOKEN, so it never shows on a command line.
const TUNNEL = 'tunnel';
const ENV_TUNNEL_TOKEN = 'M24D_TUNNEL_TOKEN';
const ENV_CLOUDFLARED = 'M24D_CLOUDFLARED'; // where cloudflared is, when it is not in the usual place
const CLOUDFLARED_TOKEN_VAR = 'TUNNEL_TOKEN';
const CLOUDFLARED_DEFAULT = IS_WINDOWS ? 'C:/Program Files (x86)/cloudflared/cloudflared.exe' : 'cloudflared';
const TUNNEL_PORT = 8026; // cloudflared's own status page: /ready answers 200 once it is connected to Cloudflare
const TUNNEL_HEALTH_PATH = '/ready';
const TUNNEL_ARGS = ['tunnel', '--no-autoupdate', '--metrics', `${HOST}:${TUNNEL_PORT}`, 'run'];
const LOCAL_HOSTS = [`${HOST}:${RUNNER_PORT}`, `localhost:${RUNNER_PORT}`];
// The pages call their own origin: the runner for trading, the runner's /news/ for news, and never the demo.
const WEB_ENV = { M24_BASE: `${DESK_PATH}/`, VITE_TRADING_API: DESK_PATH, VITE_NEWS_API: `${DESK_PATH}/news`, VITE_SELF_HOSTED: '1' };
const BACKOFF_START_MS = 1000;
const BACKOFF_MAX_MS = 60000;
const HEALTHY_AFTER_MS = 600000; // a run this long resets the wait before the next restart
const HEALTH_EVERY_MS = 60000;
const HEALTH_TIMEOUT_MS = 5000;
const HEALTH_MISSES_MAX = 3; // in a row: a hung process, or a laptop back from sleep with dead sockets
const HEALTH_PATH = '/healthz';
const STALE_AFTER_MS = 3 * HEALTH_EVERY_MS; // a pid file whose supervisor has not written in this long is left over
const LOG_MAX_BYTES = 5 * 1024 * 1024; // then the file becomes <name>.log.1 and a new one starts
const STOP_POLL_MS = 1000;
const SERVICE_GRACE_MS = 8000; // how long a service gets to close its files before it is killed
const STOP_WAIT_MS = 20000; // how long --stop waits for a clean shutdown before forcing it
const START_WAIT_MS = 180000; // how long --install waits for the first build and start
const PORT_CHECK_MS = 800;
const WAIT_STEP_MS = 250;
const TASK_NAME = 'Money24';
const RUN_KEY = 'HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run'; // the fallback when Task Scheduler refuses
const HEADLESS = '--headless'; // conhost runs a console program with no window at all
// On Windows the supervisor always shows an icon in the system tray while it runs, so nothing of Money 24
// runs out of sight. When the desktop app is built (src/tauri), the app is the tray: it attaches to these
// services, its icon opens a status window, and it opens the desk in its own window. Otherwise, and after
// the app's Quit, the icon is scripts/tray.ps1 (Windows PowerShell is built in; nothing to install).
const TRAY_SCRIPT = join(ROOT, 'scripts', 'tray.ps1');
const TRAY_ICON = join(ROOT, 'public', 'favicon.ico');
const TRAY_SHELL = 'powershell.exe';
const TRAY_RESPAWN_MS = 5000; // a tray that dies while the supervisor runs comes back after this long
const TRAY = 'tray'; // the tray's own log name
// The desktop app, newest first of: the path in M24_DESKTOP_APP, an installed copy, the release build.
const ENV_DESKTOP_APP = 'M24_DESKTOP_APP';
const APP_INSTALL_DIR = join(process.env.LOCALAPPDATA ?? '', 'Money 24');
const APP_CANDIDATES = [join(APP_INSTALL_DIR, 'money24.exe'), join(APP_INSTALL_DIR, 'Money 24.exe'), join(ROOT, 'src', 'tauri', 'target', 'release', 'money24.exe')];
// The app's tray id (src/tauri/src/tray.rs), found in every build that can be the tray. An older build lacks it:
// launched with --tray it would open a window and try to start its own services, so it is passed over.
const APP_TRAY_MARK = Buffer.from('m24-desktop-tray-1');
const APP_TRAY_FLAG = '--tray'; // only the tray icon, no window
const APP_FLAGS = ['--attach', '--root', ROOT]; // use these services, never its own; and where Restart and Stop are
const APP_QUIT_CODE = 0; // the app's Quit: closed on purpose, so it is not brought back
const TRAY_OPEN_APP_CODE = 10; // tray.ps1's "Open the desk" with the app built: the supervisor opens the app
const APP_FAILS_MAX = 3; // quick crashes in a row before the supervisor falls back to tray.ps1
const APP_QUICK_MS = 30000; // an app that dies sooner than this counts as a quick crash
const ENV_LINE = /^\s*([A-Z0-9_]+)\s*=\s*(.*?)\s*$/i;
const MS_PER_SECOND = 1000;
const SECONDS_PER_MINUTE = 60;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;

/**
 * @typedef {{ name: string, port: number, args: string[], exe?: string, healthPath?: string, env?: Record<string, string>,
 *   child: import('node:child_process').ChildProcess | null,
 *   startedAt: number, restarts: number, backoff: number, misses: number, timer: NodeJS.Timeout | undefined,
 *   health: { ok: boolean, at: number } | null, lastExit: { at: number, how: string } | null }} Service
 */

/** @type {Service[]} */
let services = [];
/** @type {NodeJS.ProcessEnv} */
let serviceEnv = process.env;
let startedAt = 0;
let stopping = false;
/** @type {import('node:child_process').ChildProcess | null} the tray: the desktop app or tray.ps1 */
let trayChild = null;
let appQuit = false; // the app was closed with Quit: tray.ps1 shows the icon until the app is asked for again
let appFails = 0;
/** @type {Map<string, boolean>} by path and modified time: whether that build can be the tray */
const appCanTray = new Map();
/** @type {Record<string, number>} bytes in each log file, so rotating needs no stat per line */
const logSizes = {};

/** Appends one timestamped line to a log, rotating the file once it is full. */
function log(/** @type {string} */ name, /** @type {string} */ text) {
  const file = join(LOG_DIR, `${name}.log`);
  const line = `${new Date().toISOString()} ${text}\n`;
  logSizes[file] ??= existsSync(file) ? statSync(file).size : 0;
  if (logSizes[file] + line.length > LOG_MAX_BYTES) {
    renameSync(file, `${file}.1`);
    logSizes[file] = 0;
  }
  appendFileSync(file, line);
  logSizes[file] += Buffer.byteLength(line);
}

/** A supervisor event: to its log and to the terminal, when there is one. */
function say(/** @type {string} */ text) {
  log(SUPERVISOR, text);
  console.log(text);
}

function fail(/** @type {string} */ message) {
  console.error(`\n${message}\n`);
  process.exit(1);
}

/** "45s", "12m", "2h 3m", "3d 4h" */
function span(/** @type {number} */ ms) {
  const seconds = Math.max(0, Math.round(ms / MS_PER_SECOND));
  if (seconds < SECONDS_PER_MINUTE) return `${seconds}s`;
  const minutes = Math.floor(seconds / SECONDS_PER_MINUTE);
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  if (hours < HOURS_PER_DAY) return `${hours}h ${minutes % MINUTES_PER_HOUR}m`;
  return `${Math.floor(hours / HOURS_PER_DAY)}d ${hours % HOURS_PER_DAY}h`;
}

/** KEY=VALUE lines from .env, without the surrounding quotes. Missing file: nothing. */
function readEnv() {
  if (!existsSync(ENV_FILE)) return {};
  /** @type {Record<string, string>} */
  const env = {};
  for (const line of readFileSync(ENV_FILE, 'utf8').split(/\r?\n/)) {
    const m = line.startsWith('#') ? null : line.match(ENV_LINE);
    if (m && m[2]) env[m[1]] = m[2].replace(/^(["'])(.*)\1$/, '$2');
  }
  return env;
}

/** Resolves true when something already answers on the port. */
function portBusy(/** @type {number} */ port) {
  return new Promise((done) => {
    const socket = connect({ host: HOST, port });
    socket.setTimeout(PORT_CHECK_MS);
    socket.once('connect', () => { socket.destroy(); done(true); });
    socket.once('timeout', () => { socket.destroy(); done(false); });
    socket.once('error', () => done(false));
  });
}

/** The ports of SERVICES that something is listening on. @returns {Promise<number[]>} */
async function busyPorts() {
  const busy = await Promise.all(SERVICES.map((s) => portBusy(s.port)));
  return SERVICES.filter((_, i) => busy[i]).map((s) => s.port);
}

/** Waits until check() is true or the time is up, and says which. */
async function waitFor(/** @type {() => boolean | Promise<boolean>} */ check, /** @type {number} */ ms) {
  for (const end = Date.now() + ms; Date.now() < end; ) {
    if (await check()) return true;
    await new Promise((wait) => setTimeout(wait, WAIT_STEP_MS));
  }
  return check();
}

function pidAlive(/** @type {number} */ pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (err) {
    return /** @type {NodeJS.ErrnoException} */ (err).code === 'EPERM';
  }
}

/** The state file the supervisor rewrites, or null when there is none to read. @returns {any} */
function readState() {
  try {
    return JSON.parse(readFileSync(STATE_FILE, 'utf8'));
  } catch {
    return null;
  }
}

/**
 * The pid of a supervisor that is really running, or 0. A pid file alone proves little (the pid may
 * belong to something else by now), so the supervisor must also have written its state lately, or
 * a service port must be answering.
 */
async function supervisorPid() {
  const pid = existsSync(PID_FILE) ? Number(readFileSync(PID_FILE, 'utf8').trim()) : 0;
  const state = readState();
  if (!pid || !pidAlive(pid) || state?.pid !== pid) return 0;
  if (Date.now() - state.beat_at < STALE_AFTER_MS) return pid;
  return (await busyPorts()).length ? pid : 0;
}

/** Writes what is running, for --status and for the next start to check. */
function saveState() {
  const state = {
    pid: process.pid,
    started_at: startedAt,
    beat_at: Date.now(),
    url: `http://${HOST}:${RUNNER_PORT}${DESK_PATH}/`,
    services: services.map((s) => ({
      name: s.name, port: s.port, pid: s.child?.pid ?? 0, started_at: s.startedAt, restarts: s.restarts,
      misses: s.misses, health: s.health, last_exit: s.lastExit,
    })),
  };
  writeFileSync(`${STATE_FILE}.tmp`, JSON.stringify(state, null, 1));
  renameSync(`${STATE_FILE}.tmp`, STATE_FILE);
}

/** Runs one build step and shows what it printed, warnings included; on failure that also goes to the log. */
function buildStep(/** @type {string} */ command, /** @type {string[]} */ args, /** @type {import('node:child_process').SpawnSyncOptions} */ opts) {
  const run = spawnSync(command, args, { ...opts, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8' });
  const output = [run.stdout, run.stderr, run.error?.message].filter(Boolean).join('\n').trim();
  if (run.status === 0) {
    if (output) console.log(output);
    return true;
  }
  say(output || `${command} exited with code ${run.status}`);
  return false;
}

/** A step that did not build: carry on with the earlier build when there is one, otherwise give up. */
function keepEarlier(/** @type {string} */ what, /** @type {string} */ artifact) {
  if (!existsSync(artifact)) fail(`${what} did not build and there is no earlier build to run. The error is above.`);
  say(`${what} did not build (the error is above): running the earlier build`);
}

/** Builds both services the way scripts/start.js does, then the pages for the runner to serve. */
function build() {
  for (const { name } of SERVICES) {
    const binary = join(RUN_DIR, name + EXE);
    console.log(`Building ${name}...`);
    const built = buildStep('go', ['build', '-buildvcs=false', '-o', binary, `./cmd/${name}`], { cwd: ENGINE_DIR, env: { ...process.env, CGO_ENABLED: '0' } });
    if (!built) keepEarlier(name, binary);
  }
  console.log('Building the pages...');
  const built = buildStep(process.execPath, [VITE_BIN, 'build', '--outDir', WEB_NEXT_DIR, '--emptyOutDir'], { cwd: ROOT, env: { ...process.env, ...WEB_ENV } });
  if (!built) {
    keepEarlier('The pages', join(WEB_DIR, WEB_START_PAGE));
    return;
  }
  rmSync(WEB_DIR, { recursive: true, force: true });
  renameSync(WEB_NEXT_DIR, WEB_DIR);
}

/** The environment the services run in: this shell's, then .env, then the full host list for a tunnel. */
function buildServiceEnv() {
  const env = { ...process.env, ...readEnv(), [ENV_SUPERVISOR_STATE]: STATE_FILE };
  const publicHost = (env[ENV_PUBLIC_HOST] ?? '').replace(/^https?:\/\//, '').replace(/\/.*$/, '');
  // --hosts replaces the runner's localhost defaults, so they are listed again beside the tunnel's host.
  if (publicHost && !env[ENV_HOSTS]) env[ENV_HOSTS] = [...LOCAL_HOSTS, publicHost].join(',');
  return env;
}

/** Starts one service and labels each of its output lines into its log. */
function start(/** @type {Service} */ svc) {
  // Our own services exit when the supervisor's pipe closes; cloudflared does not, and is killed on stop instead.
  const command = svc.exe ?? join(RUN_DIR, svc.name + EXE);
  const args = svc.exe ? svc.args : [...svc.args, EXIT_WITH_STDIN];
  const child = spawn(command, args, { cwd: ROOT, env: { ...serviceEnv, ...svc.env }, stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
  Object.assign(svc, { child, startedAt: Date.now(), misses: 0, health: null });
  for (const stream of [child.stdout, child.stderr]) {
    let rest = '';
    stream.on('data', (chunk) => {
      const lines = (rest + chunk).split(/\r?\n/);
      rest = lines.pop() ?? '';
      for (const line of lines) if (line.trim()) log(svc.name, line);
    });
  }
  child.stdin.on('error', () => {}); // the pipe breaks when the service exits first
  child.on('error', (err) => gone(svc, child, `could not start: ${err.message}`));
  child.on('exit', (code, signal) => gone(svc, child, signal ? `killed, ${signal}` : `exit code ${code}`));
  say(`${svc.name} started (pid ${child.pid})`);
  saveState();
}

/** A service ended. Unless everything is stopping, it is started again after a wait that doubles each time. */
function gone(/** @type {Service} */ svc, /** @type {import('node:child_process').ChildProcess} */ child, /** @type {string} */ how) {
  if (svc.child !== child) return;
  const ran = Date.now() - svc.startedAt;
  Object.assign(svc, { child: null, lastExit: { at: Date.now(), how } });
  if (stopping) return;
  if (ran >= HEALTHY_AFTER_MS) svc.backoff = BACKOFF_START_MS;
  const wait = svc.backoff;
  svc.backoff = Math.min(svc.backoff * 2, BACKOFF_MAX_MS);
  svc.restarts += 1;
  say(`${svc.name} stopped (${how}) after ${span(ran)}: restart ${svc.restarts} in ${span(wait)}`);
  saveState();
  svc.timer = setTimeout(() => start(svc), wait);
}

/** Asks one service whether it is alive. Too many silences in a row and it is killed, which restarts it. */
async function checkHealth(/** @type {Service} */ svc) {
  const child = svc.child;
  if (!child) return;
  const ok = await fetch(`http://${HOST}:${svc.port}${svc.healthPath ?? HEALTH_PATH}`, { signal: AbortSignal.timeout(HEALTH_TIMEOUT_MS) }).then((res) => res.ok, () => false);
  if (svc.child !== child) return; // it was restarted while we asked
  Object.assign(svc, { misses: ok ? 0 : svc.misses + 1, health: { ok, at: Date.now() } });
  if (ok) return;
  if (svc.misses < HEALTH_MISSES_MAX) {
    say(`${svc.name} did not answer its health check (${svc.misses} of ${HEALTH_MISSES_MAX})`);
    return;
  }
  say(`${svc.name} did not answer ${HEALTH_MISSES_MAX} health checks in a row: restarting it`);
  child.kill();
}

/** Whether a build of the app can be the tray: it carries the tray's mark. Read once per build. */
function canTray(/** @type {string} */ path) {
  const key = `${path}|${statSync(path).mtimeMs}`;
  if (!appCanTray.has(key)) appCanTray.set(key, readFileSync(path).includes(APP_TRAY_MARK));
  return appCanTray.get(key);
}

/** The desktop app to run as the tray: the newest build that can be one, or '' when there is none. */
function desktopApp() {
  const found = [serviceEnv[ENV_DESKTOP_APP], ...APP_CANDIDATES].filter((path) => path && existsSync(path) && canTray(path));
  return found.sort((a, b) => statSync(b).mtimeMs - statSync(a).mtimeMs)[0] ?? '';
}

/**
 * Shows the tray icon (Windows), and brings it back if it dies while the supervisor runs. The desktop
 * app when it is built, unless it was quit or keeps crashing; tray.ps1 otherwise. openDesk: the app
 * opens the desk at once (tray.ps1's "Open the desk" asked for it).
 */
function startTray(openDesk = false) {
  if (!IS_WINDOWS || stopping || process.argv.includes(FLAG.noTray)) return;
  const app = desktopApp();
  if (app && !appQuit && appFails < APP_FAILS_MAX) {
    startApp(app, openDesk);
    return;
  }
  if (existsSync(TRAY_SCRIPT)) startPowerShellTray(app);
}

/** The desktop app as the tray. Quit is respected; a crash brings it back, and crashes in a row give up on it. */
function startApp(/** @type {string} */ app, /** @type {boolean} */ openDesk) {
  const started = Date.now();
  const args = openDesk ? APP_FLAGS : [APP_TRAY_FLAG, ...APP_FLAGS];
  const child = spawn(app, args, { cwd: ROOT, stdio: ['ignore', 'ignore', 'pipe'], windowsHide: true });
  trayChild = child;
  log(SUPERVISOR, `tray: the desktop app ${app} (pid ${child.pid})`);
  child.stderr?.on('data', (chunk) => log(TRAY, String(chunk).trimEnd()));
  child.on('error', (err) => log(SUPERVISOR, `tray: the desktop app could not start: ${err.message}`));
  child.on('exit', (code) => {
    if (trayChild === child) trayChild = null;
    if (stopping) return;
    if (code === APP_QUIT_CODE) {
      appQuit = true;
      appFails = 0;
      log(SUPERVISOR, 'tray: the desktop app was closed with Quit (or handed to a copy already open): tray.ps1 takes over');
      startTray();
      return;
    }
    appFails = Date.now() - started < APP_QUICK_MS ? appFails + 1 : 0;
    log(SUPERVISOR, `tray: the desktop app exited (${code})${appFails >= APP_FAILS_MAX ? ': it keeps failing, so tray.ps1 takes over' : ': starting it again'}`);
    setTimeout(startTray, TRAY_RESPAWN_MS);
  });
}

/** tray.ps1 as the tray. Its "Open the desk" hands back to the desktop app when there is one. */
function startPowerShellTray(/** @type {string} */ app) {
  const args = ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-STA', '-WindowStyle', 'Hidden', '-File', TRAY_SCRIPT,
    '-Supervisor', String(process.pid), '-Root', ROOT, '-Desk', `http://${HOST}:${RUNNER_PORT}${DESK_PATH}/`,
    '-Health', `http://${HOST}:${RUNNER_PORT}${HEALTH_PATH}`, '-Logs', LOG_DIR, '-Icon', TRAY_ICON, '-Task', TASK_NAME,
    ...(app && appFails < APP_FAILS_MAX ? ['-App', app] : [])];
  const child = spawn(TRAY_SHELL, args, { cwd: ROOT, stdio: ['ignore', 'ignore', 'pipe'], windowsHide: true });
  trayChild = child;
  child.stderr?.on('data', (chunk) => log(TRAY, String(chunk).trimEnd()));
  child.on('exit', (code) => {
    if (trayChild === child) trayChild = null;
    if (stopping) return;
    if (code === TRAY_OPEN_APP_CODE) {
      appQuit = false;
      log(SUPERVISOR, 'tray: tray.ps1 asked for the desktop app');
      startTray(true);
      return;
    }
    log(SUPERVISOR, `tray icon exited (${code}): showing it again`);
    setTimeout(startTray, TRAY_RESPAWN_MS);
  });
}

/** Stops both services (cleanly first, then by force), removes the supervisor's files and exits. */
async function shutdown(/** @type {number} */ code) {
  if (stopping) return;
  stopping = true;
  say('Stopping...');
  for (const svc of services) {
    clearTimeout(svc.timer);
    svc.child?.stdin?.end();
  }
  await waitFor(() => services.every((s) => !s.child), SERVICE_GRACE_MS);
  for (const svc of services) {
    if (!svc.child) continue;
    say(`${svc.name} did not stop in time: killing it`);
    svc.child.kill();
  }
  trayChild?.kill();
  for (const file of [PID_FILE, STOP_FILE, STATE_FILE]) rmSync(file, { force: true });
  say('Stopped.');
  process.exit(code);
}

/** The default mode: build, start both services, and watch them until told to stop. */
async function supervise() {
  mkdirSync(LOG_DIR, { recursive: true });
  const running = await supervisorPid();
  if (running) fail(`Money 24 is already running around the clock (supervisor pid ${running}).\nSee it with: npm run forever -- ${FLAG.status}\nStop it with: npm run forever -- ${FLAG.stop}`);
  const busy = await busyPorts();
  if (busy.length) fail(`Port ${busy.join(' and ')} is already in use, so the services cannot start.\nMoney 24 may be running another way (npm start, or the desktop app): close it first.`);

  if (!process.argv.includes(FLAG.noBuild)) {
    if (!existsSync(VITE_BIN)) fail('Packages are not installed yet. Run: npm install');
    build();
  }
  for (const { name } of SERVICES) {
    if (!existsSync(join(RUN_DIR, name + EXE))) fail(`${name} has not been built yet. Run without ${FLAG.noBuild}.`);
  }

  serviceEnv = buildServiceEnv();
  const runnerArgs = serviceEnv[ENV_RUNNER_ARGS] === undefined ? RUNNER_FLAGS : serviceEnv[ENV_RUNNER_ARGS].split(/\s+/).filter(Boolean);
  services = SERVICES.map((s) => ({
    ...s, args: s.name === RUNNER ? runnerArgs : [], child: null, startedAt: 0, restarts: 0, backoff: BACKOFF_START_MS,
    misses: 0, timer: undefined, health: null, lastExit: null,
  }));
  const token = serviceEnv[ENV_TUNNEL_TOKEN];
  if (token) {
    services.push({ name: TUNNEL, port: TUNNEL_PORT, args: TUNNEL_ARGS, exe: serviceEnv[ENV_CLOUDFLARED] || CLOUDFLARED_DEFAULT,
      healthPath: TUNNEL_HEALTH_PATH, env: { [CLOUDFLARED_TOKEN_VAR]: token }, child: null, startedAt: 0, restarts: 0,
      backoff: BACKOFF_START_MS, misses: 0, timer: undefined, health: null, lastExit: null });
  }
  startedAt = Date.now();
  rmSync(STOP_FILE, { force: true });
  writeFileSync(PID_FILE, String(process.pid));
  say(`Supervisor started (pid ${process.pid})`);
  for (const svc of services) start(svc);
  startTray();

  process.on('SIGINT', () => shutdown(0));
  process.on('SIGTERM', () => shutdown(0));
  process.on('uncaughtException', (err) => {
    say(`Supervisor error: ${err.stack ?? err}`);
    shutdown(1);
  });
  setInterval(() => { if (existsSync(STOP_FILE)) shutdown(0); }, STOP_POLL_MS);
  setInterval(async () => {
    await Promise.all(services.map(checkHealth));
    if (!stopping) saveState();
  }, HEALTH_EVERY_MS);

  console.log(`\nMoney 24 is running around the clock. Nothing here can place a real order.\n`);
  console.log(`  Open                http://${HOST}:${RUNNER_PORT}${DESK_PATH}/`);
  console.log(`  Logs                ${LOG_DIR}`);
  console.log(`\nPress Ctrl+C to stop everything, or from another terminal: npm run forever -- ${FLAG.stop}\n`);
}

/** Kills a process and, on Windows, everything it started. Already gone is fine. */
function forceKill(/** @type {number} */ pid) {
  try {
    if (IS_WINDOWS) execFileSync('taskkill', ['/PID', String(pid), '/T', '/F'], { stdio: 'ignore' });
    else process.kill(pid, 'SIGKILL');
  } catch {
    // It had already exited.
  }
}

/** --stop: asks the supervisor to shut down, forces it when it does not, and checks nothing is left listening. */
async function stop() {
  const pid = await supervisorPid();
  const state = readState();
  if (!pid) {
    for (const file of [PID_FILE, STOP_FILE]) rmSync(file, { force: true });
    const busy = await busyPorts();
    if (busy.length) fail(`No supervisor is running, but port ${busy.join(' and ')} is in use by something it did not start (npm start, or the desktop app).`);
    console.log('Money 24 is not running.');
    return;
  }
  mkdirSync(RUN_DIR, { recursive: true });
  writeFileSync(STOP_FILE, String(Date.now()));
  console.log(`Stopping the supervisor (pid ${pid})...`);
  if (!(await waitFor(() => !pidAlive(pid), STOP_WAIT_MS))) {
    console.log('It did not stop in time: forcing it.');
    forceKill(pid);
  }
  // The services exit when the supervisor's pipe closes. One still holding its port is killed by pid.
  if (!(await waitFor(async () => (await busyPorts()).length === 0, SERVICE_GRACE_MS))) {
    for (const s of state?.services ?? []) if (s.pid && pidAlive(s.pid)) forceKill(s.pid);
  }
  for (const file of [PID_FILE, STOP_FILE, STATE_FILE]) rmSync(file, { force: true });
  const busy = await waitFor(async () => (await busyPorts()).length === 0, SERVICE_GRACE_MS) ? [] : await busyPorts();
  if (busy.length) fail(`Stopped the supervisor, but port ${busy.join(' and ')} is still in use by another program.`);
  console.log('Stopped. Nothing is listening on the service ports.');
}

/** Whether a logon start is registered on Windows, and how. Empty when it is not. */
function logonStart() {
  const has = (/** @type {string} */ command, /** @type {string[]} */ args) => {
    try {
      execFileSync(command, args, { stdio: 'ignore' });
      return true;
    } catch {
      return false;
    }
  };
  if (!IS_WINDOWS) return '';
  if (has('schtasks', ['/Query', '/TN', TASK_NAME])) return `scheduled task ${TASK_NAME}`;
  return has('reg', ['query', RUN_KEY, '/v', TASK_NAME]) ? `registry Run entry ${TASK_NAME}` : '';
}

/** --status: what is running. Exits 0 when the supervisor is alive, 1 when it is not. */
async function status() {
  const pid = await supervisorPid();
  const state = readState();
  const now = Date.now();
  if (IS_WINDOWS) console.log(`Starts at logon:  ${logonStart() || `no (npm run forever -- ${FLAG.install})`}`);
  if (!pid) {
    const busy = await busyPorts();
    console.log(`Supervisor:       not running${busy.length ? ` (but port ${busy.join(' and ')} is in use by something else)` : ''}`);
    process.exit(1);
  }
  console.log(`Supervisor:       running, pid ${pid}, up ${span(now - state.started_at)}`);
  for (const s of state.services) {
    const health = !s.health ? 'not checked yet' : `${s.health.ok ? 'ok' : `NOT ANSWERING (${s.misses} in a row)`}, ${span(now - s.health.at)} ago`;
    const run = s.pid ? `pid ${s.pid}, up ${span(now - s.started_at)}` : `down (${s.last_exit?.how ?? 'starting'})`;
    console.log(`  ${s.name.padEnd(16)}${run}, restarts ${s.restarts}, health ${health}`);
  }
  console.log(`Open:             ${state.url}`);
  console.log(`Logs:             ${LOG_DIR}`);
}

/** One command-line argument, in quotes when it has a space in it. */
function quoted(/** @type {string} */ arg) {
  return /\s/.test(arg) ? `"${arg}"` : arg;
}

function xmlText(/** @type {string} */ text) {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/** How the supervisor is started with no window: conhost's headless mode, then Node, then this script. */
function hiddenStart() {
  const command = join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'conhost.exe');
  return { command, args: [HEADLESS, process.execPath, SCRIPT] };
}

/**
 * The Task Scheduler task as XML. The plain schtasks switches cannot say three things a laptop
 * needs: run on battery, keep running when the charger is pulled, and no three-day time limit.
 */
function taskXml() {
  const { command, args } = hiddenStart();
  const user = xmlText([process.env.USERDOMAIN, userInfo().username].filter(Boolean).join('\\'));
  return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Money 24: the trading runner, the news service and the pages, kept running. No real orders.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>${user}</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>${user}</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <StartWhenAvailable>true</StartWhenAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>${xmlText(command)}</Command>
      <Arguments>${xmlText(args.map(quoted).join(' '))}</Arguments>
      <WorkingDirectory>${xmlText(ROOT)}</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`;
}

/** What to do on systems with no Task Scheduler: the unit or agent to add by hand. */
function printUnixInstall() {
  if (process.platform === 'darwin') {
    console.log(`macOS: add a LaunchAgent (~/Library/LaunchAgents/red.dogs.money24.plist) with RunAtLoad and KeepAlive, whose\nProgramArguments are:  /usr/bin/caffeinate -i ${process.execPath} ${SCRIPT}\nthen: launchctl load ~/Library/LaunchAgents/red.dogs.money24.plist`);
    return;
  }
  console.log(`Save this as ~/.config/systemd/user/money24.service:

[Unit]
Description=Money 24 (trading runner, news service and pages)
After=network-online.target

[Service]
WorkingDirectory=${ROOT}
ExecStart=${process.execPath} ${SCRIPT}
Restart=always
RestartSec=5

[Install]
WantedBy=default.target

then:  systemctl --user enable --now money24  &&  loginctl enable-linger "$USER"
On a server, engine/deploy/*.service run the two services under systemd directly instead.`);
}

/** --install: start at logon from now on, and start now when nothing is running. */
async function install() {
  if (!IS_WINDOWS) {
    printUnixInstall();
    return;
  }
  const { command, args } = hiddenStart();
  const create = ['/Create', '/TN', TASK_NAME, '/XML', TASK_FILE, '/F'];
  const runLine = [command, ...args].map(quoted).join(' ');
  if (process.argv.includes(FLAG.dryRun)) {
    console.log(`Would write ${TASK_FILE} and run:\n  schtasks ${create.join(' ')}\n\nThe task runs, at logon, with no window:\n  ${runLine}\n\nIf Task Scheduler refuses, the fallback is:\n  reg add ${RUN_KEY} /v ${TASK_NAME} /t REG_SZ /d ${JSON.stringify(runLine)} /f\n\n${taskXml()}`);
    return;
  }
  mkdirSync(RUN_DIR, { recursive: true });
  writeFileSync(TASK_FILE, `\ufeff${taskXml()}`, 'utf16le');
  let task = true;
  try {
    execFileSync('schtasks', create, { stdio: ['ignore', 'ignore', 'pipe'], encoding: 'utf8' });
    console.log(`Registered the scheduled task ${TASK_NAME}: Money 24 starts when you log on.`);
  } catch (err) {
    task = false;
    console.log(`Task Scheduler refused (${String(/** @type {any} */ (err).stderr ?? err).trim()}).\nUsing your account's Run entry instead.`);
    execFileSync('reg', ['add', RUN_KEY, '/v', TASK_NAME, '/t', 'REG_SZ', '/d', runLine, '/f'], { stdio: 'ignore' });
    console.log(`Added the Run entry ${TASK_NAME}: Money 24 starts when you log on.`);
  }
  if (await supervisorPid()) {
    console.log('It is already running now.');
    return;
  }
  const busy = await busyPorts();
  if (busy.length) fail(`Not started now: port ${busy.join(' and ')} is in use (npm start, or the desktop app). Close it, then: npm run forever -- ${FLAG.install}`);
  console.log('Starting it now, in the background. The first build takes a minute or two...');
  if (task) execFileSync('schtasks', ['/Run', '/TN', TASK_NAME], { stdio: 'ignore' });
  else spawn(command, args, { cwd: ROOT, detached: true, stdio: 'ignore', windowsHide: true }).unref();
  const up = await waitFor(async () => (await busyPorts()).length === SERVICES.length, START_WAIT_MS);
  if (!up) fail(`It has not come up yet. Look at ${join(LOG_DIR, `${SUPERVISOR}.log`)}, and check again with: npm run forever -- ${FLAG.status}`);
  console.log(`Running. Open http://${HOST}:${RUNNER_PORT}${DESK_PATH}/`);
}

/** --uninstall: no longer start at logon. What is running now is left running. */
function uninstall() {
  if (!IS_WINDOWS) {
    console.log('Remove the unit or agent you added: systemctl --user disable --now money24 (Linux), launchctl unload (macOS).');
    return;
  }
  for (const [command, args] of /** @type {[string, string[]][]} */ ([['schtasks', ['/Delete', '/TN', TASK_NAME, '/F']], ['reg', ['delete', RUN_KEY, '/v', TASK_NAME, '/f']]])) {
    try {
      execFileSync(command, args, { stdio: 'ignore' });
    } catch {
      // That kind of logon start was not registered.
    }
  }
  rmSync(TASK_FILE, { force: true });
  console.log(`Money 24 no longer starts at logon. To stop what is running now: npm run forever -- ${FLAG.stop}`);
}

const MODES = { [FLAG.stop]: stop, [FLAG.status]: status, [FLAG.install]: install, [FLAG.uninstall]: uninstall };
await (MODES[process.argv.find((arg) => arg in MODES) ?? ''] ?? supervise)();
