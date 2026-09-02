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
import { afterEach, beforeEach, describe, test } from 'node:test';

import {
  acceptAuthData,
  applyAuthRotation,
  clearAuthentication,
  getAccessToken,
  getSessionId,
  isAuthBundle,
} from './authSession.js';

const bundle = {
  access_token: 'access-token',
  token_type: 'Bearer',
  access_expires_at: 4102444800,
  session: { sid: 'session-1', current: true },
  user: { id: 7, username: 'classic-user', role: 1 },
};

describe('classic dashboard authentication storage', () => {
  beforeEach(() => {
    const values = new Map();
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      value: {
        getItem: (key) => values.get(key) ?? null,
        setItem: (key, value) => values.set(key, String(value)),
        removeItem: (key) => values.delete(key),
      },
    });
    clearAuthentication();
  });

  afterEach(() => {
    clearAuthentication();
    Reflect.deleteProperty(globalThis, 'localStorage');
  });

  test('keeps access credentials in memory and persists only user data', () => {
    assert.equal(isAuthBundle(bundle), true);
    assert.deepEqual(acceptAuthData(bundle), bundle.user);
    assert.equal(getAccessToken(), bundle.access_token);
    assert.equal(getSessionId(), bundle.session.sid);
    assert.deepEqual(JSON.parse(localStorage.getItem('user')), bundle.user);
  });

  test('accepts rotation only for the active login session', () => {
    acceptAuthData(bundle);

    assert.equal(
      applyAuthRotation({
        access_token: 'rotated-token',
        token_type: 'Bearer',
        access_expires_at: 4102444900,
        session: { sid: bundle.session.sid, current: true },
      }),
      true,
    );
    assert.equal(getAccessToken(), 'rotated-token');

    assert.equal(
      applyAuthRotation({
        access_token: 'wrong-session-token',
        token_type: 'Bearer',
        access_expires_at: 4102445000,
        session: { sid: 'session-2', current: true },
      }),
      false,
    );
    assert.equal(getAccessToken(), 'rotated-token');
  });
});
