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

import assert from 'node:assert/strict';
import { constants, generateKeyPairSync, privateDecrypt } from 'node:crypto';
import { afterEach, describe, test } from 'node:test';

import {
  buildPasswordLoginFields,
  clearPasswordEncryptionCache,
} from './passwordEncryption.js';

describe('classic password login encryption', () => {
  afterEach(() => clearPasswordEncryptionCache());

  test('sends only the RSA-OAEP SHA-256 fields accepted by the backend when enabled', async () => {
    const { privateKey, publicKey } = generateKeyPairSync('rsa', {
      modulusLength: 2048,
    });
    const publicKeyPEM = publicKey.export({ type: 'spki', format: 'pem' });
    const apiClient = {
      get: async () => ({
        data: {
          success: true,
          data: {
            enabled: true,
            kid: 'login-key-1',
            public_key: publicKeyPEM,
          },
        },
      }),
    };

    const fields = await buildPasswordLoginFields(
      'ClassicLogin!2026',
      true,
      apiClient,
    );
    const plaintext = privateDecrypt(
      {
        key: privateKey,
        padding: constants.RSA_PKCS1_OAEP_PADDING,
        oaepHash: 'sha256',
      },
      Buffer.from(fields.password_encrypted, 'base64'),
    );

    assert.equal(fields.encryption_key_id, 'login-key-1');
    assert.equal('password' in fields, false);
    assert.equal(plaintext.toString('utf8'), 'ClassicLogin!2026');
  });

  test('preserves the existing plaintext request field when encryption is disabled', async () => {
    const fields = await buildPasswordLoginFields('ClassicLogin!2026', false);

    assert.deepEqual(fields, { password: 'ClassicLogin!2026' });
  });
});
