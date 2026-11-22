// Runs gcloud for the cloud kit. Every command passes the free-tier allowlist first,
// in --dry-run too, and secrets only ever travel over stdin.
import { spawnSync } from 'node:child_process';
import { assertAllowed } from './freetier.js';

const IS_WINDOWS = process.platform === 'win32';
const MAX_COMMAND_OUTPUT = 4 * 1024 * 1024;
// Characters cmd.exe leaves alone inside double quotes. Arguments are fixed or validated names.
const WINDOWS_SAFE_ARG = /^[a-zA-Z0-9_:/ .,=@+-]+$/;
const BARE_ARG = /^[a-zA-Z0-9_:/.,=@+-]+$/;
// Google REST APIs the kit calls with the signed-in user's token.
const API_HOSTS = new Set(['pubsub.googleapis.com', 'logging.googleapis.com']);
const DRY_PREFIX = '  [dry-run] gcloud';

/** Shows a command the way a person would type it; values never include secrets. */
export function display(args) {
  return args.map((arg) => (BARE_ARG.test(arg) ? arg : `"${arg}"`)).join(' ');
}

function spawn(args, input) {
  const options = { input, encoding: 'utf8', maxBuffer: MAX_COMMAND_OUTPUT, windowsHide: true };
  if (!IS_WINDOWS) return spawnSync('gcloud', args, options);
  // The Windows SDK entry point is a .cmd file, so the command line goes through cmd.exe.
  if (args.some((arg) => !WINDOWS_SAFE_ARG.test(arg))) throw new Error('Invalid gcloud argument.');
  const command = args.map((arg) => `"${arg}"`).join(' ');
  return spawnSync('cmd.exe', ['/d', '/s', '/c', `"gcloud.cmd ${command}"`], { ...options, windowsVerbatimArguments: true });
}

export class Gcloud {
  /** @param {{dry: boolean, project?: string}} options */
  constructor({ dry, project }) {
    this.dry = dry;
    this.project = project;
    this.token = '';
  }

  /** Checks that gcloud is installed and signed in. A dry run checks nothing. */
  ready() {
    if (this.dry) return;
    const version = spawn(['--version']);
    if (version.error || version.status !== 0) throw new Error('Install Google Cloud CLI (gcloud) and add it to PATH.');
    const auth = spawn(['auth', 'print-access-token', '--quiet']);
    if (auth.error || auth.status !== 0 || !auth.stdout?.trim()) {
      throw new Error('Sign in first with gcloud auth login.');
    }
    this.token = auth.stdout.trim();
  }

  /**
   * Runs one command, or prints it in a dry run. A dry run answers "not found" to every check,
   * so the whole plan prints.
   * @param {string[]} args
   * @param {{input?: string, note?: string, quiet?: boolean, project?: boolean}} [options]
   */
  run(args, { input, note, quiet = false, project = true } = {}) {
    const full = project && this.project ? [...args, `--project=${this.project}`] : args;
    assertAllowed(full);
    if (this.dry) {
      console.log(`${DRY_PREFIX} ${display(full)}${note ? `   # ${note}` : ''}`);
      return { ok: false, stdout: '', stderr: '' };
    }
    const result = spawn([...full, '--quiet'], input);
    if (result.error) throw new Error(`gcloud ${args.slice(0, 3).join(' ')} could not start.`);
    if (!quiet && result.stderr) process.stderr.write(result.stderr);
    return { ok: result.status === 0, stdout: result.stdout ?? '', stderr: result.stderr ?? '' };
  }

  /** Runs a command that must succeed. */
  must(args, options) {
    const result = this.run(args, options);
    if (!this.dry && !result.ok) throw new Error(`gcloud ${args.slice(0, 3).join(' ')} failed.`);
    return result;
  }

  /** Runs a command whose JSON output the kit reads. A dry run gets the fallback. */
  json(args, fallback) {
    const result = this.must([...args, '--format=json'], { quiet: true });
    if (this.dry) return fallback;
    try {
      return JSON.parse(result.stdout || 'null') ?? fallback;
    } catch {
      throw new Error(`gcloud ${args.slice(0, 3).join(' ')} did not return JSON.`);
    }
  }

  /** Creates a resource only when its check command finds nothing. */
  ensure({ what, check, create, grants = [] }) {
    const found = this.run(check, { quiet: true, note: `does the ${what} exist?` }).ok;
    if (found) {
      console.log(`Exists: ${what}.`);
    } else {
      this.must(create, { note: `create ${what}` });
    }
    for (const grant of grants) this.must(grant, { project: false, note: 'least-privilege grant' });
  }

  /** Calls a Google REST API with the signed-in user's token. The token is never printed. */
  async api(method, url, body) {
    const host = new URL(url).host;
    if (!API_HOSTS.has(host)) throw new Error(`Refusing a call to ${host}.`);
    if (this.dry) {
      console.log(`  [dry-run] ${method} ${url}`);
      return {};
    }
    const response = await fetch(url, {
      method,
      headers: { authorization: `Bearer ${this.token}`, 'content-type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!response.ok) throw new Error(`${host} answered HTTP ${response.status}.`);
    return response.json();
  }
}
