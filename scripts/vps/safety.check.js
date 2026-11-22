// node --test scripts/vps/safety.check.js   (node:test, not vitest: named so vitest skips it)
// Billing safety checks: the gcloud allowlist, the machine checks, the kill switch's decision.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { assertAllowed, auditProblems, freeZone, instanceProblems, machinePlan } from './freetier.js';
import { testMessage } from './killswitch.js';
import { PULL_MAX_BYTES, choose, parseManifest } from './pull.js';
import { decide, parseEvent, stopBilling } from '../../engine/deploy/gcp/killswitch/index.js';

const SETTINGS = { project: 'money24-test', zone: 'us-east1-b', instance: 'money24' };
const createArgs = () => machinePlan(SETTINGS).at(-1).create;

test('the kit plan passes its own allowlist', () => {
  for (const step of machinePlan(SETTINGS)) {
    assert.doesNotThrow(() => assertAllowed(step.check));
    assert.doesNotThrow(() => assertAllowed(step.create));
  }
});

test('billable resources are refused', () => {
  const refused = [
    ['compute', 'snapshots', 'create', 'snap'],
    ['compute', 'disks', 'snapshot', 'money24'],
    ['compute', 'disks', 'create', 'extra'],
    ['compute', 'disks', 'resize', 'money24', '--size=50GB'],
    ['compute', 'addresses', 'create', 'static-ip'],
    ['compute', 'forwarding-rules', 'create', 'lb'],
    ['compute', 'backend-services', 'create', 'lb'],
    ['compute', 'routers', 'create', 'nat'],
    ['compute', 'instances', 'attach-disk', 'money24'],
    ['compute', 'instances', 'add-access-config', 'money24'],
    ['compute', 'instances', 'set-machine-type', 'money24', '--machine-type=e2-small'],
    ['compute', 'images', 'create', 'img'],
    ['sql', 'instances', 'create', 'db'],
  ];
  for (const args of refused) assert.throws(() => assertAllowed(args), undefined, args.join(' '));
  assert.doesNotThrow(() => assertAllowed(['compute', 'snapshots', 'list']));
  assert.doesNotThrow(() => assertAllowed(['compute', 'disks', 'describe', 'money24', '--zone=us-east1-b']));
});

test('an instance create must be the free one', () => {
  const swap = (flag, value) => createArgs().map((arg) => (arg.startsWith(`${flag}=`) ? `${flag}=${value}` : arg));
  const bad = {
    'machine type': swap('--machine-type', 'e2-small'),
    zone: swap('--zone', 'us-east4-a'),
    'disk type': swap('--boot-disk-type', 'pd-balanced'),
    'disk size': swap('--boot-disk-size', '31GB'),
    spot: swap('--provisioning-model', 'SPOT'),
    ipv4: swap('--network-interface', 'subnet=money24-subnet,stack-type=IPV4_IPV6'),
    'static ip': swap('--network-interface', 'subnet=money24-subnet,no-address,address=1.2.3.4'),
    'extra disk': [...createArgs(), '--create-disk=size=10GB'],
    'local ssd': [...createArgs(), '--local-ssd=interface=NVME'],
    gpu: [...createArgs(), '--accelerator=type=nvidia-l4,count=1'],
    preemptible: [...createArgs(), '--preemptible'],
  };
  for (const [what, args] of Object.entries(bad)) assert.throws(() => assertAllowed(args), /Refusing/, what);
  assert.ok(freeZone('us-west1-a') && !freeZone('us-west2-a') && !freeZone('us-west1') && !freeZone('europe-west1-b'));
});

test('a described instance is checked for every billable part', () => {
  const free = {
    machineType: '.../machineTypes/e2-micro', zone: '.../zones/us-west1-b', scheduling: { provisioningModel: 'STANDARD' },
    networkInterfaces: [{ ipv6AccessConfigs: [{}] }], disks: [{ source: '.../disks/money24' }],
  };
  const disk = { name: 'money24', type: '.../diskTypes/pd-standard', sizeGb: '30' };
  assert.deepEqual(instanceProblems(free, [disk]), []);
  const priced = { ...free, machineType: 'e2-small', networkInterfaces: [{ accessConfigs: [{ natIP: '1.2.3.4' }] }],
    disks: [{}, {}], scheduling: { provisioningModel: 'SPOT' } };
  const problems = instanceProblems(priced, [{ ...disk, type: 'pd-ssd', sizeGb: '40' }]).join('; ');
  for (const word of ['e2-small', 'IPv4', '2 disks', 'pd-ssd', '40 GB', 'Spot']) assert.match(problems, new RegExp(word));
});

test('the audit finds billable leftovers', () => {
  const inventory = { instances: [{ name: 'money24' }], disks: [{ name: 'money24' }], snapshots: [{ name: 'old' }],
    addresses: [{ name: 'ip' }], forwardingRules: [], routers: [{ name: 'nat' }], images: [] };
  assert.deepEqual(auditProblems(inventory, 'money24'), ['snapshots: old', 'static or reserved IP addresses: ip', 'Cloud Routers (NAT): nat']);
  assert.deepEqual(auditProblems({ instances: [{ name: 'money24' }], disks: [{ name: 'money24' }] }, 'money24'), []);
});

test('the kill switch fires at the lower of the budget and its ceiling', () => {
  assert.equal(decide({ costAmount: 0.99, budgetAmount: 1 }, 1).fire, false);
  assert.equal(decide({ costAmount: 1, budgetAmount: 1 }, 1).fire, true);
  assert.equal(decide({ costAmount: 0.6, budgetAmount: 5 }, 0.5).fire, true, 'a raised budget cannot lift the deployed ceiling');
  assert.equal(decide({ costAmount: 'x', budgetAmount: 1 }, 1).fire, false);
  assert.equal(decide({ costAmount: 3 }, Number.NaN).fire, false);
});

test('the test message is a real budget shape that fires, marked dry run', async () => {
  const message = testMessage('1', 'abc');
  const cloudEvent = { data: { message } };
  const { budget, attributes } = parseEvent(cloudEvent);
  assert.equal(attributes.money24DryRun, '1');
  assert.equal(decide(budget, 1).fire, true);
  assert.deepEqual(parseEvent({ data: message.data, attributes: message.attributes }).budget, budget, 'background shape');

  const calls = [];
  const logs = [];
  const realFetch = globalThis.fetch;
  const realLog = console.log;
  process.env.KILL_PROJECT = 'money24-test';
  process.env.KILL_AT_USD = '1';
  globalThis.fetch = async (url, options = {}) => {
    calls.push(`${options.method ?? 'GET'} ${url}`);
    const target = String(url);
    const body = target.includes('token') ? { access_token: 't' } :
      target.includes('billingAccounts/') ? { permissions: ['billing.resourceAssociations.list'] } :
        target.includes('testIamPermissions') ? { permissions: ['resourcemanager.projects.deleteBillingAssignment'] } :
          { billingEnabled: true, billingAccountName: 'billingAccounts/000000-000000-000000' };
    return { ok: true, status: 200, json: async () => body };
  };
  console.log = (line) => logs.push(JSON.parse(line));
  try {
    await stopBilling(cloudEvent);
  } finally {
    globalThis.fetch = realFetch;
    console.log = realLog;
  }
  assert.equal(logs.at(-1).decision, 'would-disable');
  assert.equal(logs.at(-1).canUnlink, true);
  assert.ok(!calls.some((call) => call.startsWith('PUT')), 'a dry run never changes billing');
});

const TAR_FLOOR = 10240; // the archive's own end blocks, which every run pays
const TAR_ENTRY = 1024; // and each file's header and padding

test('pull-backup never lists the live news database or unsafe paths', () => {
  const lines = ['ROOM\t100', 'F\t5\t1.0\ttrading/a.jsonl', 'F\t50\t40.1\tnews/news.db', 'F\t5\t1.0\tnews/news.db-wal',
    'F\t10\t10.0\t../escape', 'F\t7\t2.0\tnews/days/2026-11-20.jsonl.gz'];
  const { room, files } = parseManifest(lines.join('\n'));
  assert.equal(room, 100);
  assert.deepEqual([...files.keys()], ['trading/a.jsonl', 'news/days/2026-11-20.jsonl.gz']);
});

test('pull-backup sends trading data, then the news state, then news days oldest first', () => {
  const day = (name, size) => [`news/days/${name}.jsonl.gz`, { size, mtime: '5.0' }];
  const remote = new Map([
    day('2026-11-20', 1000), day('2026-11-18', 1000), day('2026-11-19', 1000),
    ['news/state.jsonl.gz', { size: 500, mtime: '9.0' }],
    ['trading/journal.jsonl', { size: 200, mtime: '1.0' }],
    ['trading/same.jsonl', { size: 5, mtime: '1.0' }],
  ]);
  const local = { 'trading/same.jsonl': { size: 5, mtime: '1.0' } };
  const tight = choose(remote, local, TAR_FLOOR + 3 * TAR_ENTRY + 200 + 500 + 1000 + 1);
  assert.deepEqual(tight.picked, ['trading/journal.jsonl', 'news/state.jsonl.gz', 'news/days/2026-11-18.jsonl.gz']);
  assert.equal(tight.waiting, 2);
  const none = choose(remote, local, TAR_FLOOR);
  assert.deepEqual(none.picked, []);
  assert.equal(none.waiting, 5);
  const all = choose(remote, local, PULL_MAX_BYTES);
  assert.deepEqual(all.picked, ['trading/journal.jsonl', 'news/state.jsonl.gz', 'news/days/2026-11-18.jsonl.gz',
    'news/days/2026-11-19.jsonl.gz', 'news/days/2026-11-20.jsonl.gz']);
  assert.equal(all.waiting, 0);
});

test('pull-backup stops at the first news day that does not fit, so history has no hole', () => {
  const remote = new Map([
    ['news/days/2026-11-18.jsonl.gz', { size: 4000, mtime: '1.0' }],
    ['news/days/2026-11-19.jsonl.gz', { size: 100, mtime: '2.0' }],
    ['trading/small.jsonl', { size: 10, mtime: '3.0' }],
  ]);
  const { picked, waiting } = choose(remote, {}, TAR_FLOOR + 2 * TAR_ENTRY + 500);
  assert.deepEqual(picked, ['trading/small.jsonl']);
  assert.equal(waiting, 2);
});
