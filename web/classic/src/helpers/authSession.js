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

import axios from 'axios';

const serverURL = import.meta.env?.VITE_REACT_APP_SERVER_URL || '';
const authClient = axios.create({
  baseURL: serverURL,
  withCredentials: true,
  headers: { 'Cache-Control': 'no-store' },
});

let accessToken = '';
let accessExpiresAt = 0;
let sessionId = '';
let authenticatedUser = null;
let refreshPromise = null;

const isRecord = (value) => Boolean(value) && typeof value === 'object';

const isAuthUser = (value) =>
  isRecord(value) &&
  Number.isInteger(value.id) &&
  value.id > 0 &&
  typeof value.username === 'string' &&
  typeof value.role === 'number';

const isLoginSession = (value) =>
  isRecord(value) &&
  typeof value.sid === 'string' &&
  value.sid.length > 0 &&
  value.current === true;

export const isAuthBundle = (value) =>
  isRecord(value) &&
  typeof value.access_token === 'string' &&
  value.access_token.length > 0 &&
  typeof value.access_expires_at === 'number' &&
  Number.isFinite(value.access_expires_at) &&
  value.access_expires_at > 0 &&
  isAuthUser(value.user) &&
  isLoginSession(value.session);

const readStoredUser = () => {
  if (authenticatedUser) return authenticatedUser;
  try {
    const value = localStorage.getItem('user');
    const user = value ? JSON.parse(value) : null;
    return isAuthUser(user) ? user : null;
  } catch {
    return null;
  }
};

const storeUser = (user) => {
  if (!isAuthUser(user)) return null;
  const {
    token: _legacyToken,
    access_token: _accessToken,
    access_expires_at: _accessExpiresAt,
    token_type: _tokenType,
    session: _session,
    ...safeUser
  } = user;
  authenticatedUser = safeUser;
  try {
    localStorage.setItem('user', JSON.stringify(safeUser));
  } catch {
    // Authentication remains valid in memory when browser storage is unavailable.
  }
  return safeUser;
};

export const getAuthUser = (value) => {
  if (isAuthBundle(value)) return value.user;
  return isAuthUser(value) ? value : null;
};

export const applyAuthBundle = (bundle) => {
  if (!isAuthBundle(bundle)) return null;
  accessToken = bundle.access_token;
  accessExpiresAt = bundle.access_expires_at;
  sessionId = bundle.session.sid;
  return storeUser(bundle.user);
};

export const acceptAuthData = (value) => {
  if (isAuthBundle(value)) return applyAuthBundle(value);
  return storeUser(value);
};

export const applyAuthRotation = (value) => {
  const user = readStoredUser();
  if (
    !user ||
    !isRecord(value) ||
    typeof value.access_token !== 'string' ||
    typeof value.access_expires_at !== 'number' ||
    !isLoginSession(value.session) ||
    (sessionId && value.session.sid !== sessionId)
  ) {
    return false;
  }
  return Boolean(
    applyAuthBundle({
      ...value,
      token_type: value.token_type || 'Bearer',
      user,
    }),
  );
};

export const getAccessToken = () => accessToken;
export const getSessionId = () => sessionId;
export const getStoredAuthUser = () => readStoredUser();

export const hasAuthenticationHint = () =>
  Boolean(accessToken || readStoredUser());

const clearRuntimeAuthentication = () => {
  accessToken = '';
  accessExpiresAt = 0;
  sessionId = '';
  authenticatedUser = null;
};

export const clearAuthentication = () => {
  clearRuntimeAuthentication();
  try {
    localStorage.removeItem('user');
  } catch {
    // Nothing else is required when browser storage is unavailable.
  }
};

const wait = (delay) => new Promise((resolve) => setTimeout(resolve, delay));

const performRefresh = async (attempt = 0, includeExpectedSession = true) => {
  const expectedSession = includeExpectedSession ? sessionId : '';
  const response = await authClient.post('/api/user/auth/refresh', undefined, {
    headers: expectedSession
      ? { 'X-Auth-Session': expectedSession }
      : undefined,
    validateStatus: () => true,
  });
  const body = response.data;
  if (response.status === 200 && body?.success && isAuthBundle(body.data)) {
    applyAuthBundle(body.data);
    return body.data;
  }
  if (
    response.status === 409 &&
    body?.code === 'AUTH_REFRESH_RACE' &&
    attempt < 3
  ) {
    await wait([80, 200, 500][attempt]);
    return performRefresh(attempt + 1, includeExpectedSession);
  }
  if (
    response.status === 409 &&
    body?.code === 'AUTH_SESSION_MISMATCH' &&
    includeExpectedSession
  ) {
    clearRuntimeAuthentication();
    return performRefresh(0, false);
  }
  if (response.status === 401) {
    clearAuthentication();
    return null;
  }
  throw new Error(
    body?.message || `Authentication refresh failed (${response.status})`,
  );
};

export const refreshAuthentication = () => {
  if (!refreshPromise) {
    refreshPromise = performRefresh().finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise;
};

export const getFreshAccessToken = async () => {
  if (accessToken && accessExpiresAt > Math.floor(Date.now() / 1000) + 60) {
    return accessToken;
  }
  const bundle = await refreshAuthentication();
  return bundle?.access_token || '';
};

export const bootstrapAuthentication = async () => {
  try {
    const bundle = await refreshAuthentication();
    return bundle?.user || readStoredUser();
  } catch {
    // A temporary network failure must not discard the local UI hint. The
    // request interceptor will retry the refresh on the first protected call.
    return readStoredUser();
  }
};

const requestLogout = async () => {
  const headers = {};
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
  if (sessionId) headers['X-Auth-Session'] = sessionId;
  return authClient.post('/api/user/auth/logout', undefined, {
    headers,
    validateStatus: () => true,
  });
};

export const logoutAuthentication = async () => {
  try {
    let response = await requestLogout();
    if (
      response.status === 409 &&
      response.data?.code === 'AUTH_SESSION_MISMATCH'
    ) {
      const bundle = await refreshAuthentication();
      if (bundle) response = await requestLogout();
    }
    if (response.status >= 400 && response.status !== 401) {
      throw new Error(
        response.data?.message || `Logout failed (${response.status})`,
      );
    }
  } finally {
    clearAuthentication();
  }
};
