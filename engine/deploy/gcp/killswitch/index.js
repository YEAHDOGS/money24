// Money 24 billing kill switch: a Cloud Run function subscribed to the budget's Pub/Sub topic.
// When the month's cost reaches the budget it removes the project's billing account, which
// stops every paid service. This follows Google's "Disable billing usage with notifications":
// https://docs.cloud.google.com/billing/docs/how-to/disable-billing-with-notifications
// No npm packages: the Cloud Billing REST API with the function's own token.

const METADATA_TOKEN_URL = 'http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token';
const BILLING_API = 'https://cloudbilling.googleapis.com/v1/projects/';
const PROJECTS_API = 'https://cloudresourcemanager.googleapis.com/v1/projects/';
const UNLINK_PERMISSION = 'resourcemanager.projects.deleteBillingAssignment';
const PROJECT_ENV = 'KILL_PROJECT';
const KILL_AT_ENV = 'KILL_AT_USD';
// Only `node scripts/vps.js killswitch test` sets these. Google's budget messages never carry them.
const DRY_RUN_ATTRIBUTE = 'money24DryRun';
const TEST_ID_ATTRIBUTE = 'money24TestId';
const LOG_TAG = 'money24-killswitch';
const PROJECT_ID = /^[a-z][a-z0-9-]{4,28}[a-z0-9]$/;

/**
 * Decides whether a budget notification must cut billing.
 * @param {{costAmount?: unknown, budgetAmount?: unknown}} budget
 * @param {number} killAt the deployed ceiling in USD; the lower of it and the budget wins
 * @returns {{fire: boolean, reason: string}}
 */
export function decide(budget, killAt) {
  const cost = Number(budget?.costAmount);
  const amount = Number(budget?.budgetAmount);
  if (!Number.isFinite(cost)) return { fire: false, reason: 'not a budget notification: no costAmount' };
  const limits = [amount, killAt].filter((value) => Number.isFinite(value) && value > 0);
  if (!limits.length) return { fire: false, reason: 'no budget amount and no KILL_AT_USD' };
  const limit = Math.min(...limits);
  if (cost < limit) return { fire: false, reason: `cost ${cost} is under ${limit}` };
  return { fire: true, reason: `cost ${cost} reached ${limit}` };
}

/**
 * Reads a Pub/Sub event in either shape Cloud Run functions deliver: a CloudEvent
 * (data.message) or a background event (data, attributes).
 * @returns {{budget: object, attributes: Record<string, string>}}
 */
export function parseEvent(event) {
  const message = event?.data?.message ?? event?.message ?? event ?? {};
  const encoded = typeof message.data === 'string' ? message.data : '';
  let budget = {};
  try {
    budget = JSON.parse(Buffer.from(encoded, 'base64').toString('utf8'));
  } catch {
    budget = {};
  }
  return { budget, attributes: message.attributes ?? event?.attributes ?? {} };
}

function log(severity, message, fields) {
  console.log(JSON.stringify({ severity, message: `${LOG_TAG}: ${message}`, tag: LOG_TAG, ...fields }));
}

async function token() {
  const response = await fetch(METADATA_TOKEN_URL, { headers: { 'Metadata-Flavor': 'Google' } });
  if (!response.ok) throw new Error(`metadata token: HTTP ${response.status}`);
  return (await response.json()).access_token;
}

async function call(method, url, bearer, body) {
  const response = await fetch(url, {
    method,
    headers: { authorization: `Bearer ${bearer}`, 'content-type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) throw new Error(`${method} ${new URL(url).host}: HTTP ${response.status}`);
  return response.json();
}

/** Entry point. Throws on API failure, so the error is logged and Pub/Sub's next notification retries. */
export async function stopBilling(event) {
  const { budget, attributes } = parseEvent(event);
  const project = process.env[PROJECT_ENV] ?? '';
  const dryRun = attributes[DRY_RUN_ATTRIBUTE] === '1';
  const fields = {
    testId: attributes[TEST_ID_ATTRIBUTE] ?? '', dryRun, project,
    costAmount: budget.costAmount, budgetAmount: budget.budgetAmount,
  };
  if (!PROJECT_ID.test(project)) {
    log('ERROR', `${PROJECT_ENV} is not a project ID`, { ...fields, decision: 'misconfigured' });
    return;
  }
  const verdict = decide(budget, Number(process.env[KILL_AT_ENV]));
  if (!verdict.fire) {
    log('INFO', verdict.reason, { ...fields, decision: 'none' });
    return;
  }
  const bearer = await token();
  const info = await call('GET', `${BILLING_API}${project}/billingInfo`, bearer);
  if (dryRun) {
    // Prove the grant without using it: the account can read billing and holds the unlink permission.
    const held = await call('POST', `${PROJECTS_API}${project}:testIamPermissions`, bearer, { permissions: [UNLINK_PERMISSION] });
    const canUnlink = (held.permissions ?? []).includes(UNLINK_PERMISSION);
    log('NOTICE', `dry run: would disable billing (${verdict.reason})`,
      { ...fields, decision: 'would-disable', billingEnabled: Boolean(info.billingEnabled), canUnlink });
    return;
  }
  if (!info.billingEnabled) {
    log('INFO', 'billing is already disabled', { ...fields, decision: 'already-disabled' });
    return;
  }
  await call('PUT', `${BILLING_API}${project}/billingInfo`, bearer, { billingAccountName: '' });
  log('ALERT', `billing disabled (${verdict.reason})`, { ...fields, decision: 'disabled' });
}
