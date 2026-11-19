/**
 * The Money 24 waitlist: sends an address to the DOGS API Worker (api.dogs.red), which stores it
 * in D1. Nothing is emailed yet. Cloudflare Turnstile proves a person sent it, in invisible mode
 * (no box, no puzzle); its script loads only when the form is submitted, never before.
 */

const WAITLIST_URL = "https://api.dogs.red/waitlist";
const SITE = "money24";
const TURNSTILE_SCRIPT = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
const TURNSTILE_SITE_KEY = "0x4AAAAAAFn0k2xvNoVQuLjj";
const TURNSTILE_ACTION = "waitlist";
const TOKEN_TIMEOUT_MS = 15000;
// Invisible widgets draw nothing, but still need an element in the page.
const HOST_STYLE = "position:fixed;left:0;bottom:0;width:0;height:0;overflow:hidden";
const STORAGE_KEY = "m24-waitlist-joined";
const JOINED_VALUE = "yes";
const HTTP_TOO_MANY = 429;
const HTTP_UNAVAILABLE = 503;
const ERROR_MESSAGE = "Could not join. Please try again.";
const RATE_MESSAGE = "Too many tries. Wait a minute.";
const REST_MESSAGE = "Please try again tomorrow.";

/** @type {Promise<any> | null} */
let scriptLoad = null;

/** Loads the Turnstile script once; a failed load can be retried. */
function loadTurnstile() {
  if (scriptLoad) return scriptLoad;
  scriptLoad = new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = TURNSTILE_SCRIPT;
    script.async = true;
    script.onload = () => resolve(window.turnstile);
    script.onerror = () => {
      scriptLoad = null;
      reject(new Error(ERROR_MESSAGE));
    };
    document.head.appendChild(script);
  });
  return scriptLoad;
}

/** @returns {Promise<string>} a single-use token for the waitlist action */
async function turnstileToken() {
  const turnstile = await loadTurnstile();
  return new Promise((resolve, reject) => {
    const host = document.createElement("div");
    host.style.cssText = HOST_STYLE;
    document.body.appendChild(host);
    let widgetId = null;
    const finish = (settle, value) => {
      clearTimeout(timer);
      if (widgetId !== null) turnstile.remove(widgetId);
      host.remove();
      settle(value);
    };
    const timer = setTimeout(() => finish(reject, new Error(ERROR_MESSAGE)), TOKEN_TIMEOUT_MS);
    widgetId = turnstile.render(host, {
      sitekey: TURNSTILE_SITE_KEY,
      action: TURNSTILE_ACTION,
      callback: (token) => finish(resolve, token),
      // Returning true tells Turnstile the error was handled, so it does not log it.
      "error-callback": () => {
        finish(reject, new Error(ERROR_MESSAGE));
        return true;
      },
    });
  });
}

/** @returns {boolean} whether this browser already joined (the address itself is never kept here) */
export function hasJoined() {
  try { return localStorage.getItem(STORAGE_KEY) === JOINED_VALUE; }
  catch { return false; }
}

/**
 * @param {string} email
 * @returns {Promise<void>} resolves once the Worker has stored it; rejects with a message to show
 */
export async function joinWaitlist(email) {
  const token = await turnstileToken();
  const res = await fetch(WAITLIST_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: email.trim().toLowerCase(), site: SITE, token }),
  }).catch(() => { throw new Error(ERROR_MESSAGE); });
  if (res.status === HTTP_TOO_MANY) throw new Error(RATE_MESSAGE);
  if (res.status === HTTP_UNAVAILABLE) throw new Error(REST_MESSAGE);
  if (!res.ok) throw new Error(ERROR_MESSAGE);
  try { localStorage.setItem(STORAGE_KEY, JOINED_VALUE); } catch { /* Storage is optional. */ }
}
