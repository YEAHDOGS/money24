import { createVault, passwordKey, encryptConnection, decryptConnection, parseVault } from './vault.js';

export const LOCK_EVENT = 'm24-app-lock';
export const STORE_VAULT = 'm24.mobile.vault';
const STORE_RUNNER = 'm24.trading.runner';
const STORE_SECRET = 'm24.trading.secret';
const GET_SESSION = 'mobile_lock_session';
const SET_SESSION = 'mobile_unlock_session';
const CLEAR_SESSION = 'mobile_lock';
const HTTPS_PROTOCOL = 'https:';
const LOCAL_HOSTS = ['localhost', '127.0.0.1', '[::1]'];
let connection = null;
let keyBytes = null;
let revision = 0;

/** Only native mobile builds inject this immutable flag before any page script. */
export function isMobileLock() {
  return typeof window !== 'undefined' && window.__M24_MOBILE_LOCK__ === true;
}

/** Connection secrets never fall through to plain storage in the Android shell. */
export function protectedSetting(key, fallback) {
  if (!isMobileLock() || ![STORE_RUNNER, STORE_SECRET].includes(key)) return undefined;
  if (!connection) return fallback;
  return (key === STORE_RUNNER ? connection.runner : connection.secret) || fallback;
}

/** Remote mobile connections must use TLS, including before the first request. */
function validateConnection(runner, secret) {
  const address = new URL(runner);
  if (address.protocol !== HTTPS_PROTOCOL && !LOCAL_HOSTS.includes(address.hostname)) throw new Error('Use an https:// runner address on this phone.');
  if (address.username || address.password || address.hash || address.search) throw new Error('Use a runner address without credentials, a query, or a fragment.');
  return { runner: runner.replace(/\/+$/, ''), secret };
}

/** Reuses a native, memory-only key across pages; a fresh app process has none. */
export async function restoreSession() {
  const current = revision;
  const saved = localStorage.getItem(STORE_VAULT);
  if (!saved) return false;
  const bytes = await window.__TAURI__.core.invoke(GET_SESSION);
  if (!bytes || current !== revision) return false;
  const opened = await decryptConnection(parseVault(saved), new Uint8Array(bytes));
  if (current !== revision || document.hidden) return false;
  connection = validateConnection(opened.runner, opened.secret);
  keyBytes = new Uint8Array(bytes);
  return true;
}

/** Creates or opens the vault, then makes the connection available to page clients. */
export async function unlockSession(password, firstConnection) {
  const current = revision;
  let vault;
  let key;
  let opened;
  if (firstConnection) {
    opened = validateConnection(firstConnection.runner, firstConnection.secret);
    ({ vault, key } = await createVault(password, opened));
  } else {
    vault = parseVault(localStorage.getItem(STORE_VAULT));
    key = await passwordKey(password, new Uint8Array(vault.salt));
    opened = await decryptConnection(vault, key);
    opened = validateConnection(opened.runner, opened.secret);
  }
  if (current !== revision || document.hidden) throw new Error('The app locked while unlocking. Try again.');
  if (firstConnection) localStorage.setItem(STORE_VAULT, JSON.stringify(vault));
  // Remove old unencrypted settings only after encryption and persistence succeed.
  localStorage.removeItem(STORE_SECRET);
  localStorage.removeItem(STORE_RUNNER);
  await window.__TAURI__.core.invoke(SET_SESSION, { key: Array.from(key) });
  if (current !== revision || document.hidden) {
    await window.__TAURI__.core.invoke(CLEAR_SESSION);
    throw new Error('The app locked while unlocking. Try again.');
  }
  connection = opened;
  keyBytes = key;
}

/** Settings writes use the unlocked key; a locked or failed vault never writes plaintext. */
export async function saveConnection(runner, secret) {
  if (!isMobileLock()) {
    localStorage.setItem(STORE_RUNNER, runner);
    localStorage.setItem(STORE_SECRET, secret);
    return;
  }
  if (!connection || !keyBytes) throw new Error('Unlock the app before changing its connection.');
  const current = revision;
  const next = validateConnection(runner, secret);
  const vault = parseVault(localStorage.getItem(STORE_VAULT));
  const sealed = await encryptConnection(next, keyBytes, new Uint8Array(vault.salt));
  if (current !== revision || document.hidden) throw new Error('The app locked before saving. Try again.');
  localStorage.setItem(STORE_VAULT, JSON.stringify(sealed));
  connection = next;
}

/** Clear page references immediately; the bootstrap then reloads to terminate every client. */
export function clearSession() {
  revision += 1;
  connection = null;
  keyBytes?.fill(0);
  keyBytes = null;
  return window.__TAURI__.core.invoke(CLEAR_SESSION);
}

/** Manual locking uses the same path as the native background event. */
export function lockApp() {
  window.dispatchEvent(new Event(LOCK_EVENT));
}
