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
import { expect, test } from 'vitest';
import {
  reasoningEffortMappingRows,
  serializeReasoningEffortMapping,
} from '../modals/reasoningEffortMapping';

test('existing mappings round trip without limiting custom effort names', () => {
  const mapping = { xhigh: 'max', HIGH: 'provider-custom' };
  expect(
    serializeReasoningEffortMapping(reasoningEffortMappingRows(mapping)),
  ).toEqual(mapping);
});

test('empty mappings remain absent and whitespace is trimmed', () => {
  expect(serializeReasoningEffortMapping([])).toBeUndefined();
  expect(reasoningEffortMappingRows(undefined)).toEqual([]);
  expect(
    serializeReasoningEffortMapping([{ from: ' xhigh ', to: ' max ' }]),
  ).toEqual({ xhigh: 'max' });
});

test.each([[{ from: '', to: 'max' }], [{ from: 'xhigh', to: ' ' }]])(
  'incomplete mapping rows cannot be silently saved: %j',
  (...rows) => {
    expect(() => serializeReasoningEffortMapping(rows)).toThrow(
      'Reasoning effort mappings require both values.',
    );
  },
);

test('duplicate trimmed sources are rejected while different casing is valid', () => {
  expect(() =>
    serializeReasoningEffortMapping([
      { from: 'xhigh', to: 'max' },
      { from: ' xhigh ', to: 'high' },
    ]),
  ).toThrow('Reasoning effort mapping sources must be unique.');
  expect(
    serializeReasoningEffortMapping([
      { from: 'high', to: 'low' },
      { from: 'HIGH', to: 'max' },
    ]),
  ).toEqual({ high: 'low', HIGH: 'max' });
});

test.each([
  { from: 'hi\u0000gh', to: 'max' },
  { from: 'high', to: 'ma\u007fx' },
  { from: 'a'.repeat(65), to: 'max' },
  { from: 'high', to: '深'.repeat(65) },
])('invalid value length or controls prevent saving: %j', (row) => {
  expect(() => serializeReasoningEffortMapping([row])).toThrow(
    'Reasoning effort mapping values must be at most 64 characters and contain no control characters.',
  );
});

test('value length counts Unicode code points like the server', () => {
  expect(
    serializeReasoningEffortMapping([
      { from: '🧠'.repeat(64), to: '深'.repeat(64) },
    ]),
  ).toEqual({ ['🧠'.repeat(64)]: '深'.repeat(64) });
});

test('at most 64 rows can be saved', () => {
  const rows = Array.from({ length: 64 }, (_, index) => ({
    from: `effort-${index}`,
    to: 'max',
  }));
  expect(Object.keys(serializeReasoningEffortMapping(rows))).toHaveLength(64);
  expect(() =>
    serializeReasoningEffortMapping([...rows, { from: 'extra', to: 'max' }]),
  ).toThrow('Reasoning effort mapping supports at most 64 entries.');
});

test('prototype-like effort names remain ordinary JSON object keys', () => {
  const mapping = serializeReasoningEffortMapping([
    { from: '__proto__', to: 'max' },
    { from: 'constructor', to: 'high' },
  ]);
  expect(JSON.parse(JSON.stringify(mapping))).toEqual(
    JSON.parse('{"__proto__":"max","constructor":"high"}'),
  );
});
