// Network monitor math: byte formatting, monthly projections at the current poll interval, and cost.
// Cloud providers bill a GB as 2^30 bytes.

const BYTES_PER_GB = 1024 ** 3;
const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];
const BYTE_STEP = 1024;
const MS_PER_MONTH = 30 * 24 * 60 * 60 * 1000;
const MS_PER_MINUTE = 60 * 1000;
const SECOND_MS = 1000;
const TINY_COST = 0.000001;
const SMALL_COST = 0.01;

/** @param {number} bytes */
export function formatBytes(bytes) {
  if (!bytes || bytes < 1) return '0 B';
  let value = bytes;
  let unit = 0;
  while (value >= BYTE_STEP && unit < BYTE_UNITS.length - 1) {
    value /= BYTE_STEP;
    unit += 1;
  }
  const digits = value >= 100 || unit === 0 ? 0 : 1;
  return `${value.toFixed(digits)} ${BYTE_UNITS[unit]}`;
}

/** Dollar amounts from fractions of a cent up to whole dollars. @param {number} dollars */
export function formatCost(dollars) {
  if (!dollars) return '$0';
  if (dollars < TINY_COST) return '<$0.000001';
  if (dollars < SMALL_COST) return `$${dollars.toPrecision(2)}`;
  return `$${dollars.toFixed(2)}`;
}

/** @param {number} ms */
export function formatInterval(ms) {
  if (ms < MS_PER_MINUTE) return `${Math.round(ms / SECOND_MS)}s`;
  return `${Math.round(ms / MS_PER_MINUTE)}m`;
}


/**
 * Projects one source's monthly bytes from its average request size and interval.
 * @param {{avg_in:number, avg_out:number, requests:number, min_every_ms:number}} source
 * @param {number} everyMs
 */
export function projectSource(source, everyMs) {
  if (!source.requests) return { requests: 0, in: 0, out: 0 };
  // A source polls at the global interval, or its politeness floor if that is longer.
  const requests = MS_PER_MONTH / Math.max(everyMs, source.min_every_ms || 0);
  return { requests, in: source.avg_in * requests, out: source.avg_out * requests };
}

/**
 * Sums projections over every source, plus what we stream to the UI (egress on a server).
 * @param {object[]} sources
 * @param {number} everyMs
 * @param {number} servedPerMs bytes per millisecond sent to clients this session
 */
export function projectMonth(sources, everyMs, servedPerMs) {
  const total = { requests: 0, in: 0, out: 0, served: servedPerMs * MS_PER_MONTH };
  for (const s of sources) {
    const p = projectSource(s, everyMs);
    total.requests += p.requests;
    total.in += p.in;
    total.out += p.out;
  }
  return total;
}

/** Marginal cost of bytes, ignoring free allowances. Worst case bills inbound at the egress rate. */
export function marginalCost(bytesIn, bytesOut, provider, worstCase) {
  const inRate = worstCase ? provider.egress_per_gb : provider.ingress_per_gb;
  return (bytesIn * inRate + bytesOut * provider.egress_per_gb) / BYTES_PER_GB;
}

/** Monthly bill: billable egress above the included allowance, plus any inbound charge. */
export function monthlyCost(month, provider, worstCase) {
  const egressGb = (month.out + month.served + (worstCase ? month.in : 0)) / BYTES_PER_GB;
  const billable = Math.max(0, egressGb - provider.included_gb);
  const inbound = worstCase ? 0 : (month.in / BYTES_PER_GB) * provider.ingress_per_gb;
  return billable * provider.egress_per_gb + inbound;
}
