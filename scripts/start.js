// Starts all of Money 24 on this machine: builds the two Go services, then runs them beside the
// web pages and prints where to look. Ctrl+C stops everything.
//   node scripts/start.js           (or: npm start)
//   node scripts/start.js --open    also opens the trading desk in the browser
// Needs Node and Go on PATH. Settings are read from .env when there is one (see .env.example).
import { execFileSync, spawn } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { connect } from 'node:net';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const ENGINE_DIR = join(ROOT, 'engine');
const RUN_DIR = join(ROOT, '.run'); // git-ignored: built binaries live here
const ENV_FILE = join(ROOT, '.env');
const VITE_BIN = join(ROOT, 'node_modules', 'vite', 'bin', 'vite.js');
const EXE = process.platform === 'win32' ? '.exe' : '';
const HOST = '127.0.0.1';
const WEB_PORT = 5173;
const OPEN_FLAG = '--open';
const PORT_CHECK_MS = 800;
const READY_POLL_MS = 400;
const READY_TRIES = 60;
const LABEL_WIDTH = 6;
const ENV_LINE = /^\s*([A-Z0-9_]+)\s*=\s*(.*?)\s*$/i;
const SERVICES = [
  { name: 'newsd', port: 8024, about: 'news and calendar' },
  { name: 'm24d', port: 8025, about: 'paper trading, market data, source records' },
];
const PAGES = [
  ['Trading desk', 'trading.html'],
  ['Markets', 'markets.html'],
  ['News and calendar', 'feed.html'],
];
const OPENERS = { win32: ['cmd', ['/c', 'start', '']], darwin: ['open', []] };
const DEFAULT_OPENER = ['xdg-open', []];

/** @type {import('node:child_process').ChildProcess[]} */
const children = [];
let stopping = false;

function fail(/** @type {string} */ message) {
  console.error(`\n${message}\n`);
  stop(1);
}

function stop(/** @type {number} */ code) {
  stopping = true;
  for (const child of children) child.kill();
  process.exit(code);
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

/** Runs one long-lived process, with each of its output lines labelled. */
function run(/** @type {string} */ label, /** @type {string} */ command, /** @type {string[]} */ args, /** @type {NodeJS.ProcessEnv} */ env) {
  const child = spawn(command, args, { cwd: ROOT, env, stdio: ['ignore', 'pipe', 'pipe'] });
  children.push(child);
  for (const stream of [child.stdout, child.stderr]) {
    let rest = '';
    stream.on('data', (chunk) => {
      const lines = (rest + chunk).split(/\r?\n/);
      rest = lines.pop() ?? '';
      for (const line of lines) if (line.trim()) console.log(`${label.padEnd(LABEL_WIDTH)}| ${line}`);
    });
  }
  child.on('error', (err) => fail(`${label} could not start: ${err.message}`));
  child.on('exit', (code) => { if (!stopping) fail(`${label} stopped (exit code ${code}). Everything else was stopped too.`); });
}

if (!existsSync(VITE_BIN)) fail('Packages are not installed yet. Run: npm install');
try {
  execFileSync('go', ['version'], { stdio: 'ignore' });
} catch {
  fail('Go is not installed or not on PATH. Get it from https://go.dev/dl/ then run this again.');
}
for (const { name, port } of [...SERVICES, { name: 'the web pages', port: WEB_PORT }]) {
  if (await portBusy(port)) fail(`Port ${port} is already in use, so ${name} cannot start. Money 24 may already be running: close it first.`);
}

mkdirSync(RUN_DIR, { recursive: true });
for (const { name } of SERVICES) {
  console.log(`Building ${name}...`);
  try {
    execFileSync('go', ['build', '-buildvcs=false', '-o', join(RUN_DIR, name + EXE), `./cmd/${name}`], {
      cwd: ENGINE_DIR, stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' },
    });
  } catch {
    fail(`${name} did not build. The error is above.`);
  }
}

const env = { ...process.env, ...readEnv() };
for (const { name } of SERVICES) run(name, join(RUN_DIR, name + EXE), [], env);
run('web', process.execPath, [VITE_BIN], env);

process.on('SIGINT', () => stop(0));
process.on('SIGTERM', () => stop(0));

for (let tries = 0; tries < READY_TRIES && !(await portBusy(WEB_PORT)); tries++) {
  await new Promise((wait) => setTimeout(wait, READY_POLL_MS));
}
const base = `http://localhost:${WEB_PORT}`;
console.log('\nMoney 24 is running. Paper only: nothing here can place a real order.\n');
for (const [title, page] of PAGES) console.log(`  ${title.padEnd(20)}${base}/${page}`);
for (const { name, port, about } of SERVICES) console.log(`  ${name.padEnd(20)}http://${HOST}:${port}  (${about})`);
console.log('\nPress Ctrl+C to stop everything.\n');
if (process.argv.includes(OPEN_FLAG)) {
  const [command, args] = OPENERS[process.platform] ?? DEFAULT_OPENER;
  spawn(command, [...args, `${base}/${PAGES[0][1]}`], { stdio: 'ignore', detached: true }).on('error', () => {}).unref();
}
