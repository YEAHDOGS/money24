// Demo mode, shared by every app page. With no runner set up (the public site, or ?demo on any
// address) a page shows a recorded snapshot from public/demo/ instead of asking a local service.
// The snapshot is written by scripts/demo-snapshot.js. A demo only reads: nothing in it is live.

export const STORE_RUNNER = 'm24.trading.runner';
export const DEMO_PARAM = 'demo';
export const DEMO_STATUS = 'demo';
const DEMO_BASE = `${import.meta.env.BASE_URL}demo/`;
const LOCAL_HOST = /(^|\.)localhost$|^127\.|^\[::1\]$/;

/** Browser storage can be missing or throw (private mode, some WebViews): fall back to defaults. */
export function stored(/** @type {string} */ key, /** @type {string} */ fallback) {
  try {
    return localStorage.getItem(key) ?? fallback;
  } catch {
    return fallback;
  }
}

/**
 * Whether the page opens on the demo without asking a runner: ?demo, or a public website where the
 * visitor never set a runner up (asking 127.0.0.1 from there only triggers a local-network prompt).
 */
export function demoFirst() {
  const url = new URL(location.href);
  if (url.searchParams.has(DEMO_PARAM)) return true;
  return url.protocol.startsWith('http') && !LOCAL_HOST.test(url.hostname) && stored(STORE_RUNNER, '') === '';
}

/** A link to another app page that stays in the demo when this page was opened with ?demo. */
export function demoHref(/** @type {string} */ href) {
  return new URLSearchParams(location.search).has(DEMO_PARAM) ? `${href}?${DEMO_PARAM}` : href;
}

/** One recorded file from public/demo/. Rejects when it is missing. @returns {Promise<any>} */
export async function loadDemo(/** @type {string} */ file) {
  const res = await fetch(DEMO_BASE + file);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}
