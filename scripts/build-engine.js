// Builds the Go engine commands: newsd (news) and m24d (paper trading runner).
//   node scripts/build-engine.js          -> Tauri sidecars for this machine: src/tauri/binaries/<cmd>-<target triple>[.exe]
//   node scripts/build-engine.js --linux  -> headless server binaries: engine/dist/<cmd>-linux-amd64
// Tauri sets TAURI_ENV_TARGET_TRIPLE when it runs this as a before-command, so cross-builds pick the right target.
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const ENGINE_DIR = join(ROOT, 'engine');
const SIDECAR_DIR = join(ROOT, 'src', 'tauri', 'binaries');
const SERVER_DIR = join(ENGINE_DIR, 'dist');
const COMMANDS = ['newsd', 'm24d'];
const LINUX_SUFFIX = '-linux-amd64';
const LINUX_FLAG = '--linux';
const WINDOWS_EXE = '.exe';
// Rust target triple -> Go GOOS/GOARCH. Every desktop target Tauri ships.
const GO_TARGETS = {
  'x86_64-pc-windows-msvc': ['windows', 'amd64'],
  'aarch64-pc-windows-msvc': ['windows', 'arm64'],
  'x86_64-unknown-linux-gnu': ['linux', 'amd64'],
  'aarch64-unknown-linux-gnu': ['linux', 'arm64'],
  'x86_64-apple-darwin': ['darwin', 'amd64'],
  'aarch64-apple-darwin': ['darwin', 'arm64'],
};

function hostTriple() {
  const info = execFileSync('rustc', ['-vV'], { encoding: 'utf8' });
  return info.match(/^host: (\S+)$/m)?.[1];
}

function build(cmd, goos, goarch, out) {
  mkdirSync(dirname(out), { recursive: true });
  execFileSync('go', ['build', '-buildvcs=false', '-trimpath', '-ldflags=-s -w', '-o', out, `./cmd/${cmd}`], {
    cwd: ENGINE_DIR,
    stdio: 'inherit',
    env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' },
  });
  console.log(`${cmd} -> ${out}`);
}

if (process.argv.includes(LINUX_FLAG)) {
  for (const cmd of COMMANDS) build(cmd, 'linux', 'amd64', join(SERVER_DIR, `${cmd}${LINUX_SUFFIX}`));
} else {
  const triple = process.env.TAURI_ENV_TARGET_TRIPLE || hostTriple();
  const target = GO_TARGETS[triple];
  if (!target) throw new Error(`No Go target for Rust triple "${triple}"`);
  const suffix = target[0] === 'windows' ? WINDOWS_EXE : '';
  for (const cmd of COMMANDS) build(cmd, target[0], target[1], join(SIDECAR_DIR, `${cmd}-${triple}${suffix}`));
}
