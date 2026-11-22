// Moves the Money 24 runner onto Google's free e2-micro and keeps it free. See docs/CLOUD.md.
//   node scripts/vps.js <command> [--dry-run]
// Every gcloud command passes the free-tier allowlist in scripts/vps/freetier.js, in a dry run too.
import { randomBytes } from 'node:crypto';
import {
  closeSync, copyFileSync, existsSync, lstatSync, mkdirSync, mkdtempSync,
  openSync, readFileSync, readdirSync, renameSync, rmSync,
} from 'node:fs';
import { connect } from 'node:net';
import { homedir } from 'node:os';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { DESK_PATH, WEB_ENV } from './desk-build.js';
import { Gcloud } from './vps/gcloud.js';
import { AUDIT_LISTS, auditProblems, freeZone, instanceProblems, machinePlan } from './vps/freetier.js';
import { assertKillswitchVerified, setupKillswitch, testKillswitch } from './vps/killswitch.js';
import { pullBackup } from './vps/pull.js';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const RUN_DIR = join(ROOT, '.run', 'vps');
const RELEASE = join(RUN_DIR, 'release');
const DEPLOY_DIR = join(ROOT, 'engine', 'deploy', 'vps');
const VITE = join(ROOT, 'node_modules', 'vite', 'bin', 'vite.js');
const ENV_FILE = join(ROOT, '.env');
const BINS = ['m24d', 'newsd', 'm24backup', 'm24guard'];
const UNITS = ['money24-gate.service', 'm24guard.service', 'm24guard.timer', 'newsd.service', 'm24d.service',
  'money24-cloudflared.service', 'm24backup.service', 'm24backup.timer'];
const IS_WINDOWS = process.platform === 'win32';
const IS_MAC = process.platform === 'darwin';
const INSTANCE = /^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const PROJECT = /^[a-z][a-z0-9-]{4,28}[a-z0-9]$/;
const BILLING_ACCOUNT = /^[A-F0-9]{6}-[A-F0-9]{6}-[A-F0-9]{6}$/;
const BUDGET = /^\d{1,2}(\.\d{1,2})?$/;
const MAX_BUDGET_USD = 10;
const DEFAULT_BUDGET_USD = '1';
const DEFAULT_INSTANCE = 'money24';
const MAX_GUARD_BYTES = 950_000_000; // the guard's own ceiling, under Google's free 1 GB
const MAX_R2_CAP_BYTES = 8_000_000_000;
const MAX_R2_NIGHT_BYTES = 50_000_000;
const PLACEHOLDERS = { VPS_PROJECT: 'your-project-id', VPS_ZONE: 'us-west1-b', GCP_BILLING_ACCOUNT: '000000-000000-000000' };
const STAGING = /^\/tmp\/money24\.[a-zA-Z0-9]{10}$/;
const DRY_STAGING = '/tmp/money24.XXXXXXXXXX';
const ENV_LINE = /^\s*([A-Z0-9_]+)\s*=\s*(.*?)\s*$/;
const DATA_DIR = 'Money24';
const PULL_DIR = 'Money24-cloud-backup';
const LOCAL_PORTS = [8024, 8025, 8026];
const PORT_TIMEOUT_MS = 800;
const SERVER_SETTINGS = [
  'USER_AGENT', 'NEWSD_ADMIN_TOKEN', 'NEWSD_ORIGINS', 'NEWSD_HOSTS',
  'M24D_WEBHOOK_SECRET', 'M24D_ORIGINS', 'M24D_HOSTS', 'M24D_NEWSD',
  'M24D_PUBLIC_FOLLOW', 'M24D_PREDICT',
  'WEBULL_APP_KEY', 'WEBULL_APP_SECRET', 'WEBULL_ENV', 'WEBULL_ACCESS_TOKEN',
  'ANTHROPIC_API_KEY', 'OPENAI_API_KEY', 'XAI_API_KEY', 'GEMINI_API_KEY',
  'M24D_ANTHROPIC_MODEL', 'M24D_OPENAI_MODEL', 'M24D_XAI_MODEL', 'M24D_GEMINI_MODEL',
  'ALPACA_KEY_ID', 'ALPACA_SECRET_KEY', 'ALPACA_DATA_FEED',
  'TELEGRAM_API_ID', 'TELEGRAM_API_HASH', 'DISCORD_BOT_TOKEN',
  'R2_ACCOUNT_ID', 'R2_ACCESS_KEY_ID', 'R2_SECRET_ACCESS_KEY', 'R2_BUCKET', 'R2_BACKUP_MAX_BYTES', 'R2_BACKUP_NIGHT_BYTES',
];
const R2_SETTINGS = ['R2_ACCOUNT_ID', 'R2_ACCESS_KEY_ID', 'R2_SECRET_ACCESS_KEY', 'R2_BUCKET'];
const SECRET_KEY = /TOKEN|SECRET|KEY|HASH|PHONE/;
const EXTRA_DISCORD_TOKEN = /^DISCORD_[A-Z0-9_]+_BOT_TOKEN$/;
const DESK_PORT = 8025;
const PC_STOP = 'node scripts/forever.js --stop';
const PC_UNINSTALL = 'node scripts/forever.js --uninstall';
const COMMANDS = ['killswitch', 'build', 'setup', 'warp', 'netcheck', 'env', 'deploy', 'move-data', 'status', 'logs',
  'rollback', 'pull-backup'];
const USAGE = `Usage: node scripts/vps.js <${COMMANDS.join('|')}> [--dry-run]
       node scripts/vps.js killswitch test [--dry-run]
       node scripts/vps.js move-data [--force] [--dry-run]`;

function readEnv() {
  if (!existsSync(ENV_FILE)) return {};
  const values = {};
  for (const line of readFileSync(ENV_FILE, 'utf8').split(/\r?\n/)) {
    const match = line.match(ENV_LINE);
    if (match?.[2]) values[match[1]] = match[2].replace(/^(["'])(.*)\1$/, '$2');
  }
  return values;
}

/** The cloud settings from .env, validated. A dry run fills gaps with placeholders and says so. */
function cloudSettings(env, dry) {
  const missing = [];
  const pick = (name) => {
    if (env[name]) return env[name];
    if (!dry) return '';
    missing.push(name);
    return PLACEHOLDERS[name];
  };
  const s = {
    project: pick('VPS_PROJECT'), zone: pick('VPS_ZONE'), billingAccount: pick('GCP_BILLING_ACCOUNT'),
    instance: env.VPS_INSTANCE || DEFAULT_INSTANCE, budgetUsd: env.GCP_BUDGET_USD || DEFAULT_BUDGET_USD,
  };
  if (missing.length) console.log(`[dry-run] placeholders for unset .env values: ${missing.join(', ')}`);
  if (!PROJECT.test(s.project)) throw new Error('Set VPS_PROJECT in .env to the Google Cloud project ID.');
  if (!freeZone(s.zone)) throw new Error('Set VPS_ZONE in .env to a zone in us-west1, us-central1 or us-east1, such as us-west1-b.');
  if (!INSTANCE.test(s.instance)) throw new Error('VPS_INSTANCE must be a lowercase instance name.');
  if (!BUDGET.test(s.budgetUsd) || Number(s.budgetUsd) <= 0 || Number(s.budgetUsd) > MAX_BUDGET_USD) {
    throw new Error(`GCP_BUDGET_USD must be a dollar amount above 0 and at most ${MAX_BUDGET_USD}.`);
  }
  s.region = s.zone.slice(0, s.zone.lastIndexOf('-'));
  return s;
}

function local(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: ROOT, stdio: 'inherit', ...options });
  if (result.error || result.status !== 0) throw new Error(`${command === process.execPath ? 'Node build step' : command} failed.`);
}

function build(dry) {
  if (dry) {
    console.log(`[dry-run] node scripts/build-engine.js --linux --cloud   # CGO-free linux/amd64 ${BINS.join(', ')}`);
    console.log(`[dry-run] vite build for ${DESK_PATH}/, then assemble .run/vps/release/`);
    return;
  }
  if (!existsSync(VITE)) throw new Error('Run npm ci in the repo before building.');
  mkdirSync(RUN_DIR, { recursive: true });
  const scratch = mkdtempSync(join(RUN_DIR, 'build-'));
  const old = join(scratch, 'previous');
  const next = join(scratch, 'release');
  try {
    mkdirSync(next);
    local(process.execPath, ['scripts/build-engine.js', '--linux', '--cloud']);
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

class VM {
  /** @param {Gcloud} gcloud @param {{instance: string, zone: string}} s @param {Record<string, string>} env */
  constructor(gcloud, s, env) {
    this.gcloud = gcloud;
    this.dry = gcloud.dry;
    this.instance = s.instance;
    this.flags = [`--zone=${s.zone}`, '--tunnel-through-iap'];
    this.secrets = Object.entries(env)
      .filter(([key, value]) => SECRET_KEY.test(key) && value)
      .map(([, value]) => value)
      .sort((a, b) => b.length - a.length);
  }

  relative(file) {
    return relative(ROOT, file).split(sep).join('/');
  }

  call(args, input, show = true, note) {
    const result = this.gcloud.run([...args, ...this.flags], { input, quiet: true, note });
    if (this.dry) return '';
    if (show) {
      for (const [stream, raw] of [[process.stdout, result.stdout], [process.stderr, result.stderr]]) {
        if (!raw) continue;
        let output = raw;
        for (const secret of this.secrets) output = output.replaceAll(secret, '[redacted]');
        stream.write(output);
      }
    }
    if (!result.ok) throw new Error(`gcloud compute ${args[1]} failed.${show ? '' : ' Check IAP SSH access and noninteractive sudo.'}`);
    return result.stdout;
  }

  /** Sends admin.sh over SSH stdin and runs one action as root. */
  admin(action, { args = [], units = false, settings, show = true, pullList } = {}) {
    let script = readFileSync(join(DEPLOY_DIR, 'admin.sh'), 'utf8').replaceAll('\r\n', '\n');
    const parts = [];
    if (units) {
      script = script.replace('# INSTALL_UNITS', () => UNITS.map((unit) => {
        const content = readFileSync(join(DEPLOY_DIR, unit), 'utf8').replaceAll('\r\n', '\n').trimEnd();
        return `cat > "$UNIT_DIR/${unit}" <<'MONEY24_UNIT'\n${content}\nMONEY24_UNIT\nchmod 0644 "$UNIT_DIR/${unit}"`;
      }).join('\n'));
      parts.push(`${UNITS.length} units`);
    }
    if (settings) {
      script = script.replace('# INSTALL_ENV', () => installEnv(settings));
      parts.push('settings, values withheld');
    }
    if (pullList) {
      script = script.replace('# PULL_LIST', () => `cat <<'MONEY24_LIST'\n${pullList.join('\n')}\nMONEY24_LIST`);
      parts.push(`${pullList.length} paths`);
    }
    const note = `stdin: engine/deploy/vps/admin.sh${parts.length ? ` with ${parts.join(', ')}` : ''}`;
    return this.call(['compute', 'ssh', this.instance, '--command', `sudo -n bash -s -- ${[action, ...args].join(' ')}`],
      script, show, note);
  }

  /** Uploads via a private remote staging directory and always attempts cleanup. */
  transfer(action, source, force = false) {
    const tail = force ? ['force'] : [];
    if (this.dry) {
      console.log(`  [dry-run] tar -cf .run/vps/upload-XXXX/payload.tar -C ${this.relative(source) || '.'} .`);
      this.call(['compute', 'scp', '.run/vps/upload-XXXX/payload.tar', `${this.instance}:${DRY_STAGING}/payload.tar`]);
      return { staging: DRY_STAGING, args: [DRY_STAGING, ...tail] };
    }
    mkdirSync(RUN_DIR, { recursive: true });
    const scratch = mkdtempSync(join(RUN_DIR, 'upload-'));
    const archive = join(scratch, 'payload.tar');
    let staging;
    try {
      closeSync(openSync(archive, 'wx', 0o600));
      local('tar', ['-cf', archive, '-C', source, '.'], { stdio: 'ignore' });
      const remote = this.call(['compute', 'ssh', this.instance, '--command', 'mktemp -d /tmp/money24.XXXXXXXXXX'], undefined, false).trim();
      if (!STAGING.test(remote)) throw new Error('The VPS did not return a private staging directory.');
      staging = remote;
      this.call(['compute', 'scp', this.relative(archive), `${this.instance}:${staging}/payload.tar`], undefined, action !== 'move-data');
      return { staging, args: [staging, ...tail] };
    } catch (error) {
      if (staging) this.clean(staging);
      throw error;
    } finally {
      rmSync(scratch, { recursive: true, force: true });
    }
  }

  clean(staging) {
    try {
      this.call(['compute', 'ssh', this.instance, '--command', 'sudo -n bash -s'], `rm -rf -- '${staging}'\n`, false);
    } catch {
      console.error('The private upload could not be removed from the VPS.');
    }
  }
}

/** The root-only files env installs: service settings, guard limit, and whether R2 backup is on. */
function installEnv({ service, guard, r2 }) {
  const write = (target, content) => {
    const delimiter = `MONEY24_ENV_${randomBytes(16).toString('hex')}`;
    return [`temp=$(mktemp /etc/.money24.env.XXXXXXXXXX)`, `cat > "$temp" <<'${delimiter}'\n${content}\n${delimiter}`,
      `trap 'rm -f -- "$temp"' EXIT`, `chmod 0600 "$temp"`, `mv -Tf -- "$temp" "${target}"`].join('\n');
  };
  return [write('$ENV_FILE', service), write('/etc/money24-guard.env', guard),
    r2 ? 'touch /etc/money24-r2.enabled' : 'rm -f /etc/money24-r2.enabled'].join('\n');
}

function byteLimit(env, name, most) {
  const raw = env[name];
  if (raw && (!/^\d+$/.test(raw) || Number(raw) <= 0 || Number(raw) > most)) {
    throw new Error(`${name} must be a whole number of bytes from 1 to ${most}.`);
  }
  return raw;
}

function serverEnv(settings) {
  if (!settings.M24D_TUNNEL_TOKEN) throw new Error('Set M24D_TUNNEL_TOKEN in .env before installing server settings.');
  const r2Count = R2_SETTINGS.filter((name) => settings[name]).length;
  if (r2Count && r2Count !== R2_SETTINGS.length) throw new Error(`Set all of ${R2_SETTINGS.join(', ')}, or none to back up with pull-backup.`);
  if (settings.ALPACA_ENV && settings.ALPACA_ENV.toLowerCase() !== 'paper') {
    throw new Error('The VPS kit only accepts ALPACA_ENV=paper.');
  }
  byteLimit(settings, 'R2_BACKUP_MAX_BYTES', MAX_R2_CAP_BYTES);
  byteLimit(settings, 'R2_BACKUP_NIGHT_BYTES', MAX_R2_NIGHT_BYTES);
  const guardBytes = byteLimit(settings, 'VPS_MONTH_TX_BYTES', MAX_GUARD_BYTES);
  const extra = Object.keys(settings).filter((name) => EXTRA_DISCORD_TOKEN.test(name));
  const values = Object.fromEntries([...SERVER_SETTINGS, ...extra].filter((name) => settings[name])
    .map((name) => [name, settings[name]]));
  values.TUNNEL_TOKEN = settings.M24D_TUNNEL_TOKEN;
  values.ALPACA_ENV = 'paper';
  if (!values.M24D_HOSTS && settings.M24D_PUBLIC_HOST) {
    const host = settings.M24D_PUBLIC_HOST.replace(/^https?:\/\//, '').split('/')[0];
    values.M24D_HOSTS = `127.0.0.1:${DESK_PORT},localhost:${DESK_PORT},${host}`;
  }
  const service = Object.entries(values).map(([name, value]) => {
    if (/[\x00-\x1f\x7f]/.test(value)) throw new Error(`Unsupported control character in ${name}.`);
    return `${name}="${value.replace(/[\\"$`]/g, '\\$&')}"`;
  }).join('\n');
  return { service, guard: guardBytes ? `M24GUARD_MONTH_TX_BYTES=${guardBytes}` : '', r2: r2Count > 0 };
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

function pullFolder(settings) {
  if (settings.VPS_PULL_DIR) return resolve(ROOT, settings.VPS_PULL_DIR);
  const base = IS_WINDOWS ? process.env.APPDATA : homedir();
  return join(base, PULL_DIR);
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

/** Refuses to go on while the project holds anything billable besides the kit's own machine. */
function audit(gcloud, s) {
  const inventory = {};
  for (const [key, args] of Object.entries(AUDIT_LISTS)) inventory[key] = gcloud.json(args, []);
  const problems = auditProblems(inventory, s.instance);
  if (problems.length) {
    throw new Error(`The project holds resources Google bills. Delete them in the console first:\n  ${problems.join('\n  ')}`);
  }
}

/** Confirms the running machine is the free one: e2-micro, free zone, one standard disk of 30 GB or less, no IPv4. */
function verifyMachine(gcloud, s) {
  const instance = gcloud.json(['compute', 'instances', 'describe', s.instance, `--zone=${s.zone}`], null);
  if (gcloud.dry) {
    console.log('  [dry-run] then check machine type, zone, provisioning, external IPv4 and every disk');
    return;
  }
  const disks = (instance.disks ?? []).map((disk) => gcloud.json(['compute', 'disks', 'describe',
    disk.source.split('/').pop(), `--zone=${s.zone}`], {}));
  const problems = instanceProblems(instance, disks);
  if (problems.length) throw new Error(`The machine is not free: ${problems.join('; ')}. Fix it in the console before going on.`);
  console.log(`Verified free tier: ${s.instance} is an e2-micro in ${s.zone} with one standard disk and no external IPv4.`);
}

function setup(gcloud, vm, s) {
  assertKillswitchVerified(RUN_DIR, s, gcloud.dry);
  audit(gcloud, s);
  for (const step of machinePlan(s)) gcloud.ensure(step);
  verifyMachine(gcloud, s);
  vm.admin('setup', { units: true });
}

async function moveOrDeploy(vm, action, env, force) {
  const move = action === 'move-data';
  const source = move ? dataFolder(env) : RELEASE;
  if (move && !vm.dry) {
    await assertPCStopped();
    checkDataTree(source);
  } else if (!move && !vm.dry && !existsSync(join(RELEASE, 'web', 'home.html'))) {
    throw new Error('Run node scripts/vps.js build first.');
  }
  if (move && vm.dry) console.log(`  [dry-run] require the PC services stopped: ${PC_UNINSTALL}, then ${PC_STOP}`);
  const { staging, args } = vm.transfer(action, source, force);
  try {
    if (move && !vm.dry) await assertPCStopped();
    vm.admin(action, { args });
  } finally {
    if (!vm.dry) vm.clean(staging);
  }
  if (move) console.log(`Keep PC autostart disabled: ${PC_UNINSTALL}`);
}

async function main() {
  const [action, ...rest] = process.argv.slice(2);
  const dry = rest.includes('--dry-run');
  const force = rest.includes('--force');
  const words = rest.filter((arg) => !arg.startsWith('--'));
  const testing = action === 'killswitch' && words[0] === 'test';
  const flagsOk = rest.every((arg) => ['--dry-run', '--force'].includes(arg) || (testing && arg === 'test'));
  if (!COMMANDS.includes(action) || !flagsOk || (force && action !== 'move-data') || (words.length && !testing)) {
    throw new Error(USAGE);
  }
  if (action === 'build') {
    build(dry);
    return;
  }
  const env = readEnv();
  const s = cloudSettings(env, dry);
  const gcloud = new Gcloud({ dry, project: s.project });
  gcloud.ready();
  const vm = new VM(gcloud, s, env);
  switch (action) {
    case 'killswitch':
      if (testing) {
        await testKillswitch(gcloud, s, RUN_DIR);
      } else {
        setupKillswitch(gcloud, s);
      }
      break;
    case 'setup':
      setup(gcloud, vm, s);
      break;
    case 'warp':
      vm.admin('warp');
      // A fresh SSH session proves IAP still reaches the machine; it cancels the dead-man timer.
      vm.admin('warp-confirm');
      break;
    case 'env':
      vm.admin(action, { settings: dry ? { service: '', guard: '', r2: false } : serverEnv(env), show: false });
      console.log('Server settings installed at /etc/money24.env (0600), the guard limit at /etc/money24-guard.env.');
      break;
    case 'deploy':
    case 'move-data':
      await moveOrDeploy(vm, action, env, force);
      break;
    case 'pull-backup':
      pullBackup(vm, pullFolder(env), RUN_DIR);
      break;
    default:
      vm.admin(action);
  }
}

main().catch((error) => {
  console.error(`VPS kit: ${error.message}`);
  process.exitCode = 1;
});
