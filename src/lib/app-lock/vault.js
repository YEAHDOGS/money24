// Only the encrypted connection is persisted. A password-derived key stays in memory.
export const MIN_PASSWORD_LENGTH = 12;
const VERSION = 1;
const ITERATIONS = 600000;
const KEY_BITS = 256;
const SALT_BYTES = 16;
const IV_BYTES = 12;
const KDF = 'PBKDF2';
const HASH = 'SHA-256';
const CIPHER = 'AES-GCM';
const PURPOSE = 'money24.mobile.connection.v1';
const MAX_ENVELOPE_LENGTH = 32768;
const UTF8 = new TextEncoder();
const TEXT = new TextDecoder();

/** Password stretching and authenticated encryption use the platform's WebCrypto. */
export async function passwordKey(password, salt) {
  const source = await crypto.subtle.importKey('raw', UTF8.encode(password), KDF, false, ['deriveBits']);
  return new Uint8Array(await crypto.subtle.deriveBits({ name: KDF, hash: HASH, salt, iterations: ITERATIONS }, source, KEY_BITS));
}

/** Creates an encrypted vault, or rewrites it with a fresh IV after settings change. */
export async function encryptConnection(connection, keyBytes, salt) {
  const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES));
  const key = await crypto.subtle.importKey('raw', keyBytes, CIPHER, false, ['encrypt']);
  const sealed = await crypto.subtle.encrypt({ name: CIPHER, iv, additionalData: UTF8.encode(PURPOSE) }, key, UTF8.encode(JSON.stringify(connection)));
  return { version: VERSION, salt: Array.from(salt), iv: Array.from(iv), data: Array.from(new Uint8Array(sealed)) };
}

/** Rejects malformed or unsupported vaults before doing expensive password work. */
export function parseVault(serialized) {
  if (!serialized || serialized.length > MAX_ENVELOPE_LENGTH) throw new Error('The saved app lock cannot be read.');
  const vault = JSON.parse(serialized);
  for (const name of ['salt', 'iv', 'data']) {
    if (!Array.isArray(vault[name]) || !vault[name].every((n) => Number.isInteger(n) && n >= 0 && n <= 255)) {
      throw new Error('The saved app lock is damaged.');
    }
  }
  if (vault.version !== VERSION || vault.salt.length !== SALT_BYTES || vault.iv.length !== IV_BYTES || vault.data.length < IV_BYTES) {
    throw new Error('The saved app lock is unsupported.');
  }
  return vault;
}

/** Authentication fails for a wrong password, changed ciphertext or changed metadata. */
export async function decryptConnection(vault, keyBytes) {
  const key = await crypto.subtle.importKey('raw', keyBytes, CIPHER, false, ['decrypt']);
  const plain = await crypto.subtle.decrypt({ name: CIPHER, iv: new Uint8Array(vault.iv), additionalData: UTF8.encode(PURPOSE) }, key, new Uint8Array(vault.data));
  const connection = JSON.parse(TEXT.decode(plain));
  if (typeof connection.runner !== 'string' || typeof connection.secret !== 'string') throw new Error('The saved connection is damaged.');
  return connection;
}

/** A unique salt prevents passwords from sharing a derived key across installations. */
export async function createVault(password, connection) {
  if (password.length < MIN_PASSWORD_LENGTH) throw new Error('Use at least 12 characters.');
  const salt = crypto.getRandomValues(new Uint8Array(SALT_BYTES));
  const key = await passwordKey(password, salt);
  const vault = await encryptConnection(connection, key, salt);
  return { vault, key };
}
