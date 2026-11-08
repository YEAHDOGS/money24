// Builds the Go news engine (engine/cmd/newsd).
//   node scripts/build-engine.js          -> Tauri sidecar for this machine: src/tauri/binaries/newsd-<target triple>[.exe]
//   node scripts/build-engine.js --linux  -> headless server binary: engine/dist/newsd-linux-amd64
// Tauri sets TAURI_ENV_TARGET_TRIPLE when it runs this as a before-command, so cross-builds pick the right target.
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const ENGINE_DIR = join(ROOT, 'engine');
const SIDECAR_DIR = join(ROOT, 'src', 'tauri', 'binaries');
const SERVER_OUT = join(ENGINE_DIR, 'dist', 'newsd-linux-amd64');
const PACKAGE = './cmd/newsd';
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

function build(goos, goarch, out) {
  mkdirSync(dirname(out), { recursive: true });
  execFileSync('go', ['build', '-buildvcs=false', '-trimpath', '-ldflags=-s -w', '-o', out, PACKAGE], {
    cwd: ENGINE_DIR,
    stdio: 'inherit',
    env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: '0' },
  });
  console.log(`newsd -> ${out}`);
}

if (process.argv.includes(LINUX_FLAG)) {
  build('linux', 'amd64', SERVER_OUT);
} else {
  const triple = process.env.TAURI_ENV_TARGET_TRIPLE || hostTriple();
  const target = GO_TARGETS[triple];
  if (!target) throw new Error(`No Go target for Rust triple "${triple}"`);
  const suffix = target[0] === 'windows' ? WINDOWS_EXE : '';
  build(target[0], target[1], join(SIDECAR_DIR, `newsd-${triple}${suffix}`));
}
