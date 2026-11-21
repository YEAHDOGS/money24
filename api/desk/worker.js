// money24.dogs.red/desk/: the owner's live desk, the runner on the owner's own machine, reached
// through a Cloudflare Tunnel with Workers VPC. Nothing else in the project points at the machine.
//
// Locks, in order:
//   1. Cloudflare Access covers money24.dogs.red/desk: one-time PIN to the owner's emails only.
//   2. This Worker checks the Access token itself (signature, audience, issuer, expiry) and refuses
//      anything without a valid one, so a mistake in the Access setup still lets nobody through.
//   3. The runner only listens on 127.0.0.1; the tunnel publishes no hostname at all. The only way
//      in is this Worker's VPC binding.
// Writes (pasting a link, trading a play, editing rules) carry the runner's webhook secret, which
// only this Worker holds, so the phone can run the desk once the owner has logged in.
//
// Bindings (api/desk/wrangler.jsonc): DESK (VPC Service: the tunnel to 127.0.0.1:8025),
// ACCESS_AUD (the Access application's audience tag), DESK_SECRET (the runner's webhook secret).

const DESK_PATH = '/desk';
const RUNNER = 'http://127.0.0.1:8025';
const TEAM = 'https://icecream-dogs-pages.cloudflareaccess.com';
const CERTS_URL = `${TEAM}/cdn-cgi/access/certs`;
const CERTS_TTL_MS = 60 * 60 * 1000; // Access rotates its keys on a schedule; an hour's cache is plenty
const ACCESS_HEADER = 'Cf-Access-Jwt-Assertion';
const SAFE_METHODS = ['GET', 'HEAD'];
// Headers that stay at the edge: the runner needs none of them, and some it would refuse.
const DROPPED_HEADERS = ['Origin', 'Referer', 'Cookie', ACCESS_HEADER, 'Host'];
const SIGNATURE_ALGORITHM = { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' };
const JWT_ALG = 'RS256';
const MS_PER_SECOND = 1000;
const NOT_FOUND = 'Not found';
const FORBIDDEN = 'Forbidden: log in through Cloudflare Access.';
const DESK_DOWN = 'The desk is not answering: the machine running Money 24 is off, asleep or offline.';
const BAD_GATEWAY = 502;

/** @type {{ at: number, keys: Record<string, CryptoKey> }} */
let certs = { at: 0, keys: {} };

/** base64url to bytes. */
function bytes(/** @type {string} */ text) {
  const base64 = text.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(text.length / 4) * 4, '=');
  return Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
}

/** Access's signing keys by key id, fetched at most once an hour. */
async function signingKeys() {
  if (Date.now() - certs.at < CERTS_TTL_MS) return certs.keys;
  const res = await fetch(CERTS_URL);
  if (!res.ok) return certs.keys;
  /** @type {{ keys: (JsonWebKey & { kid: string })[] }} */
  const body = await res.json();
  /** @type {Record<string, CryptoKey>} */
  const keys = {};
  for (const jwk of body.keys) keys[jwk.kid] = await crypto.subtle.importKey('jwk', jwk, SIGNATURE_ALGORITHM, false, ['verify']);
  certs = { at: Date.now(), keys };
  return keys;
}

/** True when the token is a live Access token for this application, signed by Access. */
async function validToken(/** @type {string | null} */ token, /** @type {string} */ audience) {
  const parts = token?.split('.') ?? [];
  if (parts.length !== 3 || !audience) return false;
  const [head, body, signature] = parts;
  let header, claims;
  try {
    header = JSON.parse(new TextDecoder().decode(bytes(head)));
    claims = JSON.parse(new TextDecoder().decode(bytes(body)));
  } catch {
    return false;
  }
  const key = header.alg === JWT_ALG ? (await signingKeys())[header.kid] : undefined;
  if (!key) return false;
  const signed = await crypto.subtle.verify(SIGNATURE_ALGORITHM, key, bytes(signature), new TextEncoder().encode(`${head}.${body}`));
  const audiences = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  return signed && claims.iss === TEAM && audiences.includes(audience) && claims.exp * MS_PER_SECOND > Date.now();
}

export default {
  /** @param {Request} request @param {{ DESK: Fetcher, ACCESS_AUD: string, DESK_SECRET: string }} env */
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname !== DESK_PATH && !url.pathname.startsWith(`${DESK_PATH}/`)) return new Response(NOT_FOUND, { status: 404 });
    if (!(await validToken(request.headers.get(ACCESS_HEADER), env.ACCESS_AUD))) return new Response(FORBIDDEN, { status: 403 });

    const headers = new Headers(request.headers);
    for (const name of DROPPED_HEADERS) headers.delete(name);
    const safe = SAFE_METHODS.includes(request.method);
    if (!safe) headers.set('Authorization', `Bearer ${env.DESK_SECRET}`);
    try {
      return await env.DESK.fetch(`${RUNNER}${url.pathname}${url.search}`, {
        method: request.method, headers, body: safe ? undefined : request.body, redirect: 'manual',
      });
    } catch {
      return new Response(DESK_DOWN, { status: BAD_GATEWAY });
    }
  },
};
