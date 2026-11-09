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

export function reasoningEffortMappingRows(mapping) {
  if (!mapping || typeof mapping !== 'object' || Array.isArray(mapping)) {
    return [];
  }
  return Object.entries(mapping).map(([from, to]) => ({
    from,
    to: typeof to === 'string' ? to : '',
  }));
}

// Validate before constructing an object so duplicate sources cannot silently
// overwrite an earlier rule. No rows means the setting is omitted entirely.
export function serializeReasoningEffortMapping(rows = []) {
  if (!Array.isArray(rows) || rows.length > 64) {
    throw new Error('Reasoning effort mapping supports at most 64 entries.');
  }
  if (rows.length === 0) return undefined;
  const mapping = Object.create(null);
  for (const row of rows) {
    const from = typeof row.from === 'string' ? row.from.trim() : '';
    const to = typeof row.to === 'string' ? row.to.trim() : '';
    if (!from || !to) {
      throw new Error('Reasoning effort mappings require both values.');
    }
    if (
      Array.from(from).length > 64 ||
      Array.from(to).length > 64 ||
      /\p{Cc}/u.test(from) ||
      /\p{Cc}/u.test(to)
    ) {
      throw new Error(
        'Reasoning effort mapping values must be at most 64 characters and contain no control characters.',
      );
    }
    if (Object.hasOwn(mapping, from)) {
      throw new Error('Reasoning effort mapping sources must be unique.');
    }
    mapping[from] = to;
  }
  return mapping;
}
