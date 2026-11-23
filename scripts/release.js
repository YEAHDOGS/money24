// Makes a Money 24 release: a git tag on master, signed with the owner's SSH release key, which every
// server checks before it installs anything (scripts/updater.js, docs/SERVER.md).
//   npm run release              sign and push a release tag for master as it is on GitHub
//   npm run release -- --setup   make the release key (once, on the owner's PC) and write deploy/release-signers
//   npm run release -- --key     show the release key's fingerprint, to compare with a server's
// The private key never leaves this PC: not to GitHub, not to CI. Losing it means making a new one and
// pinning it again on each server by hand, which is the point.
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const KEY = process.env.M24_RELEASE_KEY || join(homedir(), '.ssh', 'money24_release');
const SIGNERS = join(ROOT, 'deploy', 'release-signers');
const SIGNERS_PATH_IN_REPO = 'deploy/release-signers';
const PRINCIPAL = 'money24-release';
const BRANCH = 'master';
const REMOTE = 'origin';
const PREFIX = 'release-';
const FLAG = { setup: '--setup', key: '--key' };
const GOOD_SIGNATURE = /^Good "git" signature for \S+ with /m;
const PAD = 2;

function fail(/** @type {string} */ message) {
  console.error(`\n${message}\n`);
  process.exit(1);
}

/** Runs a command; its trimmed output, or a failure with what it printed. */
function sh(/** @type {string} */ command, /** @type {string[]} */ args, quiet = false) {
  const done = spawnSync(command, args, { cwd: ROOT, encoding: 'utf8', windowsHide: true });
  const out = `${done.stdout ?? ''}${done.stderr ?? ''}`.trim();
  if (done.status !== 0 && !quiet) fail(`${command} ${args.join(' ')} failed:\n${out || done.error?.message}`);
  return done.status === 0 ? out : '';
}

/** The allowed-signers line for the release key: who signs, for git only, with which key. */
function signerLine() {
  const pub = readFileSync(`${KEY}.pub`, 'utf8').trim().split(/\s+/).slice(0, 2).join(' ');
  return `${PRINCIPAL} namespaces="git" ${pub}`;
}

function setup() {
  if (!existsSync(KEY)) {
    mkdirSync(dirname(KEY), { recursive: true });
    sh('ssh-keygen', ['-q', '-t', 'ed25519', '-N', '', '-C', PRINCIPAL, '-f', KEY]);
    console.log(`Made the release key ${KEY}. Keep it on this PC; back it up somewhere offline if you like.`);
  }
  writeFileSync(SIGNERS, `${signerLine()}\n`);
  console.log(`Wrote ${SIGNERS_PATH_IN_REPO}. Commit and push it; a server pins it the first time it installs.`);
  console.log(sh('ssh-keygen', ['-l', '-f', `${KEY}.pub`]));
}

function release() {
  if (!existsSync(KEY)) fail(`No release key at ${KEY}. Make it once: npm run release -- ${FLAG.setup}`);
  if (sh('git', ['rev-parse', '--abbrev-ref', 'HEAD']) !== BRANCH) fail(`Releases are made from ${BRANCH}: check it out first.`);
  if (sh('git', ['status', '--porcelain', '--untracked-files=no'])) fail('There are uncommitted changes. A release is what is on GitHub: commit and push first.');
  sh('git', ['fetch', '--quiet', REMOTE, BRANCH]);
  const head = sh('git', ['rev-parse', 'HEAD']);
  if (head !== sh('git', ['rev-parse', `${REMOTE}/${BRANCH}`])) fail(`This ${BRANCH} is not the one on GitHub. Push or pull until they match, then release.`);
  const signers = sh('git', ['show', `HEAD:${SIGNERS_PATH_IN_REPO}`], true);
  if (!signers.includes(signerLine().split(' ').at(-1) ?? '')) fail(`${SIGNERS_PATH_IN_REPO} on ${BRANCH} does not hold this release key. Run --setup, commit and push it.`);
  const already = sh('git', ['tag', '--points-at', 'HEAD', '--list', `${PREFIX}*`]);
  if (already) fail(`${BRANCH} is already released as ${already.split(/\r?\n/)[0]}.`);
  const now = new Date();
  const two = (/** @type {number} */ n) => String(n).padStart(PAD, '0');
  const tag = `${PREFIX}${now.getUTCFullYear()}.${two(now.getUTCMonth() + 1)}.${two(now.getUTCDate())}-${two(now.getUTCHours())}${two(now.getUTCMinutes())}`;
  const subject = sh('git', ['log', '-1', '--format=%s']);
  sh('git', ['-c', 'gpg.format=ssh', '-c', `user.signingkey=${KEY}`, 'tag', '-s', tag, '-m', `Money 24 ${tag}\n\n${subject}`]);
  // Checked here the way a server checks it, before anything is pushed.
  const dir = mkdtempSync(join(tmpdir(), 'm24-release-'));
  writeFileSync(join(dir, 'signers'), `${signerLine()}\n`);
  const check = spawnSync('git', ['-c', `gpg.ssh.allowedSignersFile=${join(dir, 'signers')}`, 'verify-tag', '--raw', tag], { cwd: ROOT, encoding: 'utf8' });
  rmSync(dir, { recursive: true, force: true });
  if (check.status !== 0 || !GOOD_SIGNATURE.test(`${check.stdout}${check.stderr}`)) {
    sh('git', ['tag', '-d', tag], true);
    fail(`The new tag did not verify, so it was deleted:\n${check.stdout}${check.stderr}`);
  }
  sh('git', ['push', '--quiet', REMOTE, `refs/tags/${tag}`]);
  console.log(`Released ${tag} (${head.slice(0, 7)}: ${subject}).\nServers pick it up within a few minutes, or at once with: node scripts/server.js update`);
}

if (process.argv.includes(FLAG.setup)) setup();
else if (process.argv.includes(FLAG.key)) console.log(existsSync(KEY) ? sh('ssh-keygen', ['-l', '-f', `${KEY}.pub`]) : `No release key at ${KEY}.`);
else release();
