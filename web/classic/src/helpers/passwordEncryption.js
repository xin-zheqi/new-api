/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

const KEY_CACHE_TTL_MS = 5 * 60_000;

let cachedKey = null;
let cachedAt = 0;

export function clearPasswordEncryptionCache() {
  cachedKey = null;
  cachedAt = 0;
}

export async function buildPasswordLoginFields(
  password,
  encryptionEnabled,
  apiClient,
) {
  if (!encryptionEnabled) {
    return { password };
  }

  try {
    const key = await getPasswordEncryptionKey(apiClient);
    return {
      password_encrypted: await encryptPasswordWithPublicKey(
        password,
        key.public_key,
      ),
      encryption_key_id: key.kid,
    };
  } catch (error) {
    clearPasswordEncryptionCache();
    throw error;
  }
}

async function getPasswordEncryptionKey(apiClient) {
  const now = Date.now();
  if (cachedKey && now - cachedAt < KEY_CACHE_TTL_MS) {
    return cachedKey;
  }
  if (!apiClient || typeof apiClient.get !== 'function') {
    throw new Error('Password encryption API is unavailable');
  }

  const response = await apiClient.get('/api/user/login/encryption-key', {
    skipAuthRefresh: true,
  });
  const key = response.data?.data;
  if (
    !response.data?.success ||
    key?.enabled !== true ||
    typeof key.kid !== 'string' ||
    !key.kid ||
    typeof key.public_key !== 'string' ||
    !key.public_key
  ) {
    throw new Error('Password encryption key is unavailable');
  }
  cachedKey = key;
  cachedAt = now;
  return key;
}

export async function encryptPasswordWithPublicKey(password, publicKeyPEM) {
  if (typeof globalThis.crypto?.subtle !== 'undefined') {
    try {
      const publicKey = await globalThis.crypto.subtle.importKey(
        'spki',
        pemToDER(publicKeyPEM),
        { name: 'RSA-OAEP', hash: 'SHA-256' },
        false,
        ['encrypt'],
      );
      const ciphertext = await globalThis.crypto.subtle.encrypt(
        { name: 'RSA-OAEP' },
        publicKey,
        new TextEncoder().encode(password),
      );
      return arrayBufferToBase64(ciphertext);
    } catch {
      // Some HTTP browsers expose SubtleCrypto but reject RSA-OAEP operations.
    }
  }

  const forge = await import('node-forge');
  const publicKey = forge.pki.publicKeyFromPem(publicKeyPEM);
  const ciphertext = publicKey.encrypt(
    forge.util.encodeUtf8(password),
    'RSA-OAEP',
    { md: forge.md.sha256.create() },
  );
  return forge.util.encode64(ciphertext);
}

function pemToDER(pem) {
  const body = pem
    .replace('-----BEGIN PUBLIC KEY-----', '')
    .replace('-----END PUBLIC KEY-----', '')
    .replaceAll(/\s+/g, '');
  const binary = atob(body);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes.buffer;
}

function arrayBufferToBase64(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}
