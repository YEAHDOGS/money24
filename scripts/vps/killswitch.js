// The Google billing kill switch: a budget, its Pub/Sub topic, and a function that
// removes the project's billing account when the month's cost reaches the budget.
// https://docs.cloud.google.com/billing/docs/how-to/disable-billing-with-notifications
import { randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const TOPIC = 'money24-billing';
const FUNCTION = 'money24-killswitch';
const BUDGET_NAME = 'money24-killswitch';
const KILL_ACCOUNT = 'money24-killswitch';
const BUILD_ACCOUNT = 'money24-build';
const SOURCE = 'engine/deploy/gcp/killswitch';
const CLEANUP_POLICY = 'engine/deploy/gcp/artifact-cleanup.json';
const ARTIFACT_REPO = 'gcf-artifacts'; // where Cloud Run functions keep the built image
const RUNTIME = 'nodejs24';
const APIS = [
  'cloudbilling.googleapis.com', 'billingbudgets.googleapis.com', 'pubsub.googleapis.com',
  'cloudfunctions.googleapis.com', 'run.googleapis.com', 'cloudbuild.googleapis.com',
  'artifactregistry.googleapis.com', 'eventarc.googleapis.com', 'cloudresourcemanager.googleapis.com',
  'logging.googleapis.com', 'iam.googleapis.com', 'compute.googleapis.com', 'iap.googleapis.com',
];
// Least privilege, from Google's "Disable billing for a project" permissions table:
// Project Billing Manager + Project Browser on the project, Billing Account Viewer on the account.
const KILL_PROJECT_ROLES = ['roles/billing.projectManager', 'roles/browser'];
const KILL_ACCOUNT_ROLE = 'roles/billing.viewer';
const BUILD_ROLE = 'roles/cloudbuild.builds.builder';
// Count cost after the free tier but before promotional credits, so a Free Trial credit
// cannot hide real spending from the switch.
const COUNTED_CREDITS = 'FREE_TIER,SUSTAINED_USAGE_DISCOUNT';
const THRESHOLDS = ['0.5', '0.9', '1.0'];
const TEST_COST_FACTOR = 2;
const LOG_WAIT_MS = 10000;
const LOG_ATTEMPTS = 18; // three minutes: a cold function and log ingestion take a while
const PUBSUB_API = 'https://pubsub.googleapis.com/v1/';
const LOGGING_API = 'https://logging.googleapis.com/v2/entries:list';
const VERIFIED_FILE = 'killswitch-verified.json';

const accountEmail = (name, project) => `${name}@${project}.iam.gserviceaccount.com`;

/** Creates or updates every piece. Safe to run again. */
export function setupKillswitch(gcloud, s) {
  const kill = accountEmail(KILL_ACCOUNT, s.project);
  const build = accountEmail(BUILD_ACCOUNT, s.project);
  gcloud.must(['services', 'enable', ...APIS], { note: 'APIs are free to enable' });
  gcloud.ensure({ what: 'budget topic', check: ['pubsub', 'topics', 'describe', TOPIC], create: ['pubsub', 'topics', 'create', TOPIC] });
  gcloud.ensure({
    what: 'kill switch service account',
    check: ['iam', 'service-accounts', 'describe', kill],
    create: ['iam', 'service-accounts', 'create', KILL_ACCOUNT, '--display-name=Money 24 billing kill switch'],
    grants: [
      ...KILL_PROJECT_ROLES.map((role) => ['projects', 'add-iam-policy-binding', s.project,
        `--member=serviceAccount:${kill}`, `--role=${role}`, '--condition=None']),
      ['billing', 'accounts', 'add-iam-policy-binding', s.billingAccount, `--member=serviceAccount:${kill}`,
        `--role=${KILL_ACCOUNT_ROLE}`],
    ],
  });
  gcloud.ensure({
    what: 'build service account',
    check: ['iam', 'service-accounts', 'describe', build],
    create: ['iam', 'service-accounts', 'create', BUILD_ACCOUNT, '--display-name=Money 24 function builds'],
    grants: [['projects', 'add-iam-policy-binding', s.project, `--member=serviceAccount:${build}`,
      `--role=${BUILD_ROLE}`, '--condition=None']],
  });
  gcloud.must(['functions', 'deploy', FUNCTION, '--gen2', `--runtime=${RUNTIME}`, `--region=${s.region}`,
    `--source=${SOURCE}`, '--entry-point=stopBilling', `--trigger-topic=${TOPIC}`,
    `--service-account=${kill}`, `--trigger-service-account=${kill}`,
    `--build-service-account=projects/${s.project}/serviceAccounts/${build}`,
    `--set-env-vars=KILL_PROJECT=${s.project},KILL_AT_USD=${s.budgetUsd}`,
    '--memory=128Mi', '--max-instances=1', '--min-instances=0', '--timeout=60s',
    '--ingress-settings=internal-only', '--no-allow-unauthenticated'], { note: 'one Cloud Build run, about a minute' });
  gcloud.must(['functions', 'add-invoker-policy-binding', FUNCTION, `--region=${s.region}`, `--member=serviceAccount:${kill}`],
    { note: 'lets the Pub/Sub trigger call the function' });
  gcloud.must(['artifacts', 'repositories', 'set-cleanup-policies', ARTIFACT_REPO, `--location=${s.region}`,
    `--policy=${CLEANUP_POLICY}`, '--no-dry-run'], { note: 'keep two images, under the free 0.5 GB' });
  const budget = [`--display-name=${BUDGET_NAME}`, `--budget-amount=${s.budgetUsd}USD`, '--calendar-period=month',
    `--filter-projects=projects/${s.project}`, '--credit-types-treatment=include-specified-credits',
    `--filter-credit-types=${COUNTED_CREDITS}`, ...THRESHOLDS.map((value) => `--threshold-rule=percent=${value}`),
    `--notifications-rule-pubsub-topic=projects/${s.project}/topics/${TOPIC}`];
  const budgets = gcloud.json(['billing', 'budgets', 'list', `--billing-account=${s.billingAccount}`], []);
  const existing = budgets.find((item) => item.displayName === BUDGET_NAME);
  if (existing) {
    gcloud.must(['billing', 'budgets', 'update', existing.name.split('/').pop(), `--billing-account=${s.billingAccount}`,
      `--budget-amount=${s.budgetUsd}USD`, `--notifications-rule-pubsub-topic=projects/${s.project}/topics/${TOPIC}`],
    { note: 'budget already exists: set its amount and topic' });
  } else {
    gcloud.must(['billing', 'budgets', 'create', `--billing-account=${s.billingAccount}`, ...budget]);
  }
  console.log(`Kill switch set: billing is removed from ${s.project} at $${s.budgetUsd}. Prove it with: node scripts/vps.js killswitch test`);
}

/** The fake budget message the test publishes: over the budget, marked as a dry run. */
export function testMessage(budgetUsd, testId) {
  const cost = Number(budgetUsd) * TEST_COST_FACTOR;
  const data = { budgetDisplayName: BUDGET_NAME, costAmount: cost, budgetAmount: Number(budgetUsd),
    budgetAmountType: 'SPECIFIED_AMOUNT', costIntervalStart: new Date().toISOString(), currencyCode: 'USD' };
  return {
    data: Buffer.from(JSON.stringify(data)).toString('base64'),
    attributes: { money24DryRun: '1', money24TestId: testId },
  };
}

/** Publishes a fake over-budget message and waits for the function's dry-run log line. */
export async function testKillswitch(gcloud, s, runDir) {
  const testId = randomBytes(8).toString('hex');
  await gcloud.api('POST', `${PUBSUB_API}projects/${s.project}/topics/${TOPIC}:publish`,
    { messages: [testMessage(s.budgetUsd, testId)] });
  console.log(`Published a fake $${Number(s.budgetUsd) * TEST_COST_FACTOR} budget message (dry run, test ${testId}). Waiting for the function...`);
  if (gcloud.dry) {
    await gcloud.api('POST', LOGGING_API, { note: 'poll for the function log line with this test id' });
    console.log('  [dry-run] would require decision=would-disable and canUnlink=true, then record the pass.');
    return;
  }
  const filter = `jsonPayload.tag="${FUNCTION}" AND jsonPayload.testId="${testId}"`;
  for (let attempt = 0; attempt < LOG_ATTEMPTS; attempt++) {
    await new Promise((done) => setTimeout(done, LOG_WAIT_MS));
    const page = await gcloud.api('POST', LOGGING_API, { resourceNames: [`projects/${s.project}`], filter, pageSize: 5 });
    const entry = (page.entries ?? []).map((item) => item.jsonPayload ?? {}).find((item) => item.decision);
    if (!entry) continue;
    if (entry.decision !== 'would-disable' || !entry.canUnlink) {
      throw new Error(`The kill switch answered ${entry.decision}, canUnlink=${entry.canUnlink}: ${entry.message}`);
    }
    mkdirSync(runDir, { recursive: true });
    writeFileSync(join(runDir, VERIFIED_FILE), JSON.stringify({ project: s.project, budgetUsd: s.budgetUsd, testId, at: new Date().toISOString() }));
    console.log(`Kill switch PASSED: it would remove billing at $${s.budgetUsd}, and holds the permission to. Nothing was disabled.`);
    return;
  }
  throw new Error('No answer from the kill switch in three minutes. Check its logs in the console under Cloud Run.');
}

/** Setup refuses to build a machine until the kill switch has passed its test for this project and budget. */
export function assertKillswitchVerified(runDir, s, dry) {
  const file = join(runDir, VERIFIED_FILE);
  const record = existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : null;
  if (record?.project === s.project && String(record.budgetUsd) === String(s.budgetUsd)) return;
  const message = 'Set up and test the kill switch first: node scripts/vps.js killswitch, then killswitch test.';
  if (dry) {
    console.log(`  [dry-run] ${message} (not verified yet; a real setup stops here)`);
    return;
  }
  throw new Error(message);
}
