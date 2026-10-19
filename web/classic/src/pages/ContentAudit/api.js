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
import { API } from '../../helpers';
export async function auditRequest(path, method = 'GET', data) {
  const response = await API.request({
    url: '/api/content-audit' + path,
    method,
    data,
    headers: { 'Cache-Control': 'no-store' },
  });
  if (!response.data.success) throw new Error(response.data.message);
  return response.data.data;
}
export const regexFlagOptions = [
  { value: 'i', label: 'Ignore case (i)' },
  { value: 'm', label: 'Multiline mode (m)' },
  { value: 's', label: 'Dot matches newlines (s)' },
];
export function normalizePolicy(policy) {
  return {
    ...policy,
    rules: policy.rules.map((rule) => ({
      ...rule,
      mode: rule.action === 'replace' ? rule.mode : 'first',
      flags: ['i', 'm', 's']
        .filter((flag) => rule.flags.includes(flag))
        .join(''),
    })),
  };
}
export const presets = [
  {
    name: 'Email address',
    pattern: '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}',
    replacement: '[EMAIL]',
  },
  {
    name: 'Mainland China mobile number',
    pattern: '\\b1[3-9][0-9]{9}\\b',
    replacement: '[PHONE]',
  },
  {
    name: 'API key pattern',
    pattern: '\\bsk-[A-Za-z0-9_-]{16,}\\b',
    replacement: '[API_KEY]',
  },
  {
    name: 'IPv4 address',
    pattern: '\\b[0-9]{1,3}(\\.[0-9]{1,3}){3}\\b',
    replacement: '[IP_ADDRESS]',
  },
  {
    name: 'JWT token',
    pattern:
      '\\beyJ[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}\\b',
    replacement: '[JWT]',
  },
  {
    name: 'Private key block',
    pattern: '-----BEGIN [A-Z ]+ PRIVATE KEY-----',
    replacement: '[PRIVATE_KEY]',
  },
  {
    name: 'Credit card number',
    pattern: '\\b[0-9]{13,19}\\b',
    replacement: '[CARD_NUMBER]',
  },
  {
    name: 'Chinese ID number',
    pattern:
      '\\b[1-9][0-9]{5}(18|19|20)[0-9]{2}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]\\b',
    replacement: '[ID_NUMBER]',
  },
];
export function newRule() {
  const id = Array.from(crypto.getRandomValues(new Uint8Array(16)), (value) =>
    value.toString(16).padStart(2, '0'),
  ).join('');
  return {
    id,
    name: '',
    enabled: true,
    pattern: '',
    flags: '',
    mode: 'first',
    action: 'warn',
    replacement: '[REDACTED]',
  };
}
/*
Copyright (C) 2023-2026 QuantumNous

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
