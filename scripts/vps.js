import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import {
  closeSync, copyFileSync, existsSync, lstatSync, mkdirSync, mkdtempSync,
  openSync, readFileSync, readdirSync, renameSync, rmSync,
} from 'node:fs';
import { connect } from 'node:net';
import { homedir } from 'node:os';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { DESK_PATH, WEB_ENV } from './desk-build.js';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const RUN_DIR = join(ROOT, '.run', 'vps');
const RELEASE = join(RUN_DIR, 'release');
const DEPLOY_DIR = join(ROOT, 'engine', 'deploy', 'vps');
const VITE = join(ROOT, 'node_modules', 'vite', 'bin', 'vite.js');
const ENV_FILE = join(ROOT, '.env');
const BINS = ['m24d', 'newsd', 'm24backup'];
const UNITS = ['newsd.service', 'm24d.service', 'money24-cloudflared.service', 'm24backup.service', 'm24backup.timer'];
const IS_WINDOWS = process.platform === 'win32';
const IS_MAC = process.platform === 'darwin';
const INSTANCE = /^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const ZONE = /^(us-central1|us-east1|us-west1)-[a-z]$/;
const PROJECT = /^[a-z][a-z0-9-]{4,28}[a-z0-9]$/;
const STAGING = /^\/tmp\/money24\.[a-zA-Z0-9]{10}$/;
const ENV_LINE = /^\s*([A-Z0-9_]+)\s*=\s*(.*?)\s*$/;
const DATA_DIR = 'Money24';
const LOCAL_PORTS = [8024, 8025, 8026];
const PORT_TIMEOUT_MS = 800;
const MAX_COMMAND_OUTPUT = 4 * 1024 * 1024;
const SERVER_SETTINGS = [
  'USER_AGENT', 'NEWSD_ADMIN_TOKEN', 'NEWSD_ORIGINS', 'NEWSD_HOSTS',
  'M24D_WEBHOOK_SECRET', 'M24D_ORIGINS', 'M24D_HOSTS', 'M24D_NEWSD',
  'M24D_PUBLIC_FOLLOW', 'M24D_PREDICT',
  'WEBULL_APP_KEY', 'WEBULL_APP_SECRET', 'WEBULL_ENV', 'WEBULL_ACCESS_TOKEN',
  'ANTHROPIC_API_KEY', 'OPENAI_API_KEY', 'XAI_API_KEY', 'GEMINI_API_KEY',
  'M24D_ANTHROPIC_MODEL', 'M24D_OPENAI_MODEL', 'M24D_XAI_MODEL', 'M24D_GEMINI_MODEL',
  'ALPACA_KEY_ID', 'ALPACA_SECRET_KEY', 'ALPACA_DATA_FEED',
  'TELEGRAM_API_ID', 'TELEGRAM_API_HASH', 'DISCORD_BOT_TOKEN',
  'R2_ACCOUNT_ID', 'R2_ACCESS_KEY_ID', 'R2_SECRET_ACCESS_KEY', 'R2_BUCKET', 'R2_BACKUP_MAX_BYTES',
];
const REQUIRED_SETTINGS = ['M24D_TUNNEL_TOKEN', 'R2_ACCOUNT_ID', 'R2_ACCESS_KEY_ID', 'R2_SECRET_ACCESS_KEY', 'R2_BUCKET'];
const SECRET_KEY = /TOKEN|SECRET|KEY|HASH|PHONE/;
const EXTRA_DISCORD_TOKEN = /^DISCORD_[A-Z0-9_]+_BOT_TOKEN$/;
const DESK_PORT = 8025;
const PC_STOP = 'node scripts/forever.js --stop';
const PC_UNINSTALL = 'node scripts/forever.js --uninstall';
const DRY_RUN = {
  build: `Cross-build CGO-free Linux amd64 m24d, newsd and m24backup. Build Vite pages for ${DESK_PATH}/. Assemble .run/vps/release/.`,
  setup: 'Use gcloud SSH over IAP to install Cloudflare’s Debian package, SQLite and vnStat. Configure 1 GiB swap, the money24 user, private data and bounded journald. Enable systemd services and the nightly timer for boot, gated on data cutover.',
  env: 'Send allowlisted settings from .env over gcloud SSH stdin to root-owned /etc/money24.env (0600). Map the tunnel token to TUNNEL_TOKEN and enforce Alpaca paper mode. Values are omitted.',
  deploy: 'Upload .run/vps/release/ via gcloud SCP. Atomically select the release and keep the previous build. Restart and check active VPS services, or stage the first release with services stopped.',
  'move-data': `Require the PC services to be stopped (${PC_UNINSTALL}, then ${PC_STOP}). Upload the full local data tree privately via gcloud SCP. Refuse newer/server-only data unless --force. Activate VPS services after import. PC autostart command: ${PC_UNINSTALL}`,
  status: 'Use gcloud SSH to report systemd state, memory, disk, data size, journal usage and monthly interface TX if available.',
  logs: 'Use gcloud SSH to show the last 100 journal lines from the runner, news, tunnel and backup units.',
  rollback: 'Switch the VPS to the previous release. Restart and check services if cutover is complete.',
};

function readEnv() {
  if (!existsSync(ENV_FILE)) return {};
  const values = {};
  for (const line of readFileSync(ENV_FILE, 'utf8').split(/\r?\n/)) {
    const match = line.match(ENV_LINE);
    if (match?.[2]) values[match[1]] = match[2].replace(/^(["'])(.*)\1$/, '$2');
  }
  return values;
}

function local(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: ROOT, stdio: 'inherit', ...options });
  if (result.error || result.status !== 0) throw new Error(`${command === process.execPath ? 'Node build step' : command} failed.`);
}

function build() {
  if (!existsSync(VITE)) throw new Error('Run npm ci in the repo before building.');
  mkdirSync(RUN_DIR, { recursive: true });
  const scratch = mkdtempSync(join(RUN_DIR, 'build-'));
  const old = join(scratch, 'previous');
  const next = join(scratch, 'release');
  try {
    mkdirSync(next);
    local(process.execPath, ['scripts/build-engine.js', '--linux', '--backup']);
    for (const bin of BINS) {
      copyFileSync(join(ROOT, 'engine', 'dist', `${bin}-linux-amd64`), join(next, bin));
    }
    local(process.execPath, [VITE, 'build', '--outDir', join(next, 'web'), '--emptyOutDir'], {
      env: { ...process.env, ...WEB_ENV },
    });
    if (!existsSync(join(next, 'web', 'home.html'))) throw new Error('The build did not produce the desk home page.');
    if (existsSync(RELEASE)) renameSync(RELEASE, old);
    try {
      renameSync(next, RELEASE);
    } catch (error) {
      if (existsSync(old)) renameSync(old, RELEASE);
      throw error;
    }
    console.log('Release built at .run/vps/release/.');
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

/** Runs gcloud with static commands and validated names. Secrets travel only over stdin. */
function gcloud(args, input) {
  const options = { cwd: ROOT, input, encoding: 'utf8', maxBuffer: MAX_COMMAND_OUTPUT, windowsHide: true };
  if (!IS_WINDOWS) return spawnSync('gcloud', args, options);
  // The Windows SDK entry point is a .cmd file. All caller-supplied arguments are validated or generated.
  if (args.some((arg) => !/^[a-zA-Z0-9_:/ .-]+$/.test(arg))) throw new Error('Invalid gcloud argument.');
  const command = args.map((arg) => arg.includes(' ') ? `"${arg}"` : arg).join(' ');
  return spawnSync('cmd.exe', ['/d', '/s', '/c', `"gcloud.cmd ${command}"`], { ...options, windowsVerbatimArguments: true });
}

class VM {
  /** Validates the target and checks that the local Google Cloud CLI has active credentials. */
  constructor(settings) {
    if (!INSTANCE.test(settings.VPS_INSTANCE ?? '') || !ZONE.test(settings.VPS_ZONE ?? '')) {
      throw new Error('Set VPS_INSTANCE and VPS_ZONE in .env. Use a zone in us-central1, us-east1 or us-west1.');
    }
    this.instance = settings.VPS_INSTANCE;
    this.flags = ['--zone', settings.VPS_ZONE, '--tunnel-through-iap', '--quiet'];
    if (settings.VPS_PROJECT) {
      if (!PROJECT.test(settings.VPS_PROJECT)) throw new Error('VPS_PROJECT must be a Google Cloud project ID.');
      this.flags.push('--project', settings.VPS_PROJECT);
    }
    this.secrets = Object.entries(settings)
      .filter(([key, value]) => SECRET_KEY.test(key) && value)
      .map(([, value]) => value)
      .sort((a, b) => b.length - a.length);
    const version = gcloud(['--version']);
    if (version.error || version.status !== 0) throw new Error('Install Google Cloud CLI (gcloud) and add it to PATH.');
    const auth = gcloud(['auth', 'print-access-token', '--quiet']);
    if (auth.error || auth.status !== 0 || !auth.stdout?.trim()) {
      throw new Error('Sign in first with gcloud auth login and select the project with gcloud config set project.');
    }
  }

  call(args, input, show = true) {
    const result = gcloud([...args, ...this.flags], input);
    if (show) {
      for (const [stream, raw] of [[process.stdout, result.stdout], [process.stderr, result.stderr]]) {
        if (!raw) continue;
        let output = raw;
        for (const secret of this.secrets) output = output.replaceAll(secret, '[redacted]');
        stream.write(output);
      }
    }
    if (result.error || result.status !== 0) {
      throw new Error(`gcloud compute ${args[1]} failed.${show ? '' : ' Check IAP SSH access and noninteractive sudo.'}`);
    }
    return result.stdout;
  }

  ssh(command, input, show) {
    return this.call(['compute', 'ssh', this.instance, '--command', command], input, show);
  }

  admin(action, { args = [], units = false, settings, show = true } = {}) {
    let script = readFileSync(join(DEPLOY_DIR, 'admin.sh'), 'utf8').replaceAll('\r\n', '\n');
    if (units) {
      const install = UNITS.map((unit) => {
        const content = readFileSync(join(DEPLOY_DIR, unit), 'utf8').replaceAll('\r\n', '\n').trimEnd();
        return `cat > "$UNIT_DIR/${unit}" <<'MONEY24_UNIT'\n${content}\nMONEY24_UNIT\nchmod 0644 "$UNIT_DIR/${unit}"`;
      });
      script = script.replace('# INSTALL_UNITS', () => install.join('\n'));
    }
    if (settings) {
      const delimiter = `MONEY24_ENV_${randomBytes(16).toString('hex')}`;
      const install = [
        'temp=$(mktemp /etc/.money24.env.XXXXXXXXXX)',
        'trap \'rm -f -- "$temp"\' EXIT',
        `cat > "$temp" <<'${delimiter}'\n${settings}\n${delimiter}`,
        'mv -Tf -- "$temp" "$ENV_FILE"',
      ].join('\n');
      script = script.replace('# INSTALL_ENV', () => install);
    }
    return this.ssh(`sudo -n bash -s -- ${[action, ...args].join(' ')}`, script, show);
  }

  /** Uploads via a private remote staging directory and always attempts cleanup. */
  transfer(action, source, force = false) {
    mkdirSync(RUN_DIR, { recursive: true });
    const scratch = mkdtempSync(join(RUN_DIR, 'upload-'));
    const archive = join(scratch, 'payload.tar');
    let staging;
    try {
      closeSync(openSync(archive, 'wx', 0o600));
      local('tar', ['-cf', archive, '-C', source, '.'], { stdio: 'ignore' });
      const remote = this.ssh('mktemp -d /tmp/money24.XXXXXXXXXX', undefined, false).trim();
      if (!STAGING.test(remote)) throw new Error('The VPS did not return a private staging directory.');
      staging = remote;
      const rel = relative(ROOT, archive).split(sep).join('/');
      this.call(['compute', 'scp', rel, `${this.instance}:${staging}/payload.tar`], undefined, action !== 'move-data');
      return { staging, args: force ? [staging, 'force'] : [staging] };
    } catch (error) {
      if (staging) this.clean(staging);
      throw error;
    } finally {
      rmSync(scratch, { recursive: true, force: true });
    }
  }

  clean(staging) {
    try {
      this.ssh('sudo -n bash -s', `rm -rf -- '${staging}'\n`, false);
    } catch {
      console.error('The private upload could not be removed from the VPS.');
    }
  }
}

function serverEnv(settings) {
  for (const name of REQUIRED_SETTINGS) {
    if (!settings[name]) throw new Error(`Set ${name} in .env before installing server settings.`);
  }
  if (settings.ALPACA_ENV && settings.ALPACA_ENV.toLowerCase() !== 'paper') {
    throw new Error('The VPS kit only accepts ALPACA_ENV=paper.');
  }
  if (settings.R2_BACKUP_MAX_BYTES &&
      (!/^\d+$/.test(settings.R2_BACKUP_MAX_BYTES) || Number(settings.R2_BACKUP_MAX_BYTES) <= 0 ||
       Number(settings.R2_BACKUP_MAX_BYTES) >= 10_000_000_000)) {
    throw new Error('R2_BACKUP_MAX_BYTES must be positive and below 10000000000.');
  }
  const extra = Object.keys(settings).filter((name) => EXTRA_DISCORD_TOKEN.test(name));
  const values = Object.fromEntries([...SERVER_SETTINGS, ...extra].filter((name) => settings[name])
    .map((name) => [name, settings[name]]));
  values.TUNNEL_TOKEN = settings.M24D_TUNNEL_TOKEN;
  values.ALPACA_ENV = 'paper';
  if (!values.M24D_HOSTS && settings.M24D_PUBLIC_HOST) {
    const host = settings.M24D_PUBLIC_HOST.replace(/^https?:\/\//, '').split('/')[0];
    values.M24D_HOSTS = `127.0.0.1:${DESK_PORT},localhost:${DESK_PORT},${host}`;
  }
  return Object.entries(values).map(([name, value]) => {
    if (/[\x00-\x1f\x7f]/.test(value)) throw new Error(`Unsupported control character in ${name}.`);
    return `${name}="${value.replace(/[\\"$`]/g, '\\$&')}"`;
  }).join('\n');
}

function dataFolder(settings) {
  const base = IS_WINDOWS ? process.env.APPDATA :
    IS_MAC ? join(homedir(), 'Library', 'Application Support') :
      process.env.XDG_CONFIG_HOME || join(homedir(), '.config');
  if (!settings.VPS_DATA_DIR && !base) throw new Error('Set VPS_DATA_DIR in .env to the local Money24 data folder.');
  const folder = settings.VPS_DATA_DIR ? resolve(ROOT, settings.VPS_DATA_DIR) : join(base, DATA_DIR);
  if (!existsSync(folder) || !lstatSync(folder).isDirectory()) throw new Error('Local Money24 data folder is missing or is a symlink.');
  return folder;
}

function checkDataTree(folder) {
  for (const entry of readdirSync(folder, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      checkDataTree(join(folder, entry.name));
    } else if (!entry.isFile()) {
      throw new Error('Data migration requires regular files and directories. Remove links or special files first.');
    }
  }
}

async function assertPCStopped() {
  const ports = await Promise.all(LOCAL_PORTS.map((port) => new Promise((done) => {
    const socket = connect({ host: '127.0.0.1', port });
    socket.setTimeout(PORT_TIMEOUT_MS);
    socket.on('connect', () => { socket.destroy(); done(true); });
    socket.on('timeout', () => { socket.destroy(); done(true); });
    socket.on('error', (error) => done(error.code !== 'ECONNREFUSED'));
  })));
  if (ports.some(Boolean)) throw new Error(`Stop the PC services before cutover. Run ${PC_UNINSTALL} and then ${PC_STOP}.`);
}

async function main() {
  const [action, ...flags] = process.argv.slice(2);
  const dry = flags.includes('--dry-run');
  const force = flags.includes('--force');
  if (!Object.hasOwn(DRY_RUN, action) || flags.some((flag) => !['--dry-run', '--force'].includes(flag)) ||
      (force && action !== 'move-data')) {
    throw new Error('Usage: node scripts/vps.js <build|setup|env|deploy|move-data|status|logs|rollback> [--dry-run] [--force for move-data]');
  }
  if (dry) {
    console.log(`[dry-run] ${action} — ${DRY_RUN[action]}`);
    return;
  }
  if (action === 'build') {
    build();
    return;
  }
  const settings = readEnv();
  const vm = new VM(settings);
  switch (action) {
    case 'setup':
      vm.admin(action, { units: true });
      break;
    case 'env':
      vm.admin(action, { settings: serverEnv(settings), show: false });
      console.log('Server settings installed at /etc/money24.env (0600).');
      break;
    case 'deploy':
    case 'move-data': {
      const move = action === 'move-data';
      const source = move ? dataFolder(settings) : RELEASE;
      if (move) {
        await assertPCStopped();
        checkDataTree(source);
      } else if (!existsSync(join(RELEASE, 'web', 'home.html'))) {
        throw new Error('Run node scripts/vps.js build first.');
      }
      const { staging, args } = vm.transfer(action, source, force);
      try {
        if (move) await assertPCStopped();
        vm.admin(action, { args });
      } finally {
        vm.clean(staging);
      }
      if (move) console.log(`Keep PC autostart disabled: ${PC_UNINSTALL}`);
      break;
    }
    default:
      vm.admin(action);
  }
}

main().catch((error) => {
  console.error(`VPS kit: ${error.message}`);
  process.exitCode = 1;
});
