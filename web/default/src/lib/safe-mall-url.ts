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
export function getSafeEmbeddedMallUrl(
  value: string,
  currentHostname: string
): string | undefined {
  const candidate = value.trim()
  if (!candidate || candidate.length > 2048) return undefined

  try {
    const parsed = new URL(candidate)
    const mallHostname = parsed.hostname.toLowerCase().replace(/\.+$/, '')
    const applicationHostname = currentHostname
      .toLowerCase()
      .replace(/\.+$/, '')
    if (
      parsed.protocol !== 'https:' ||
      parsed.username !== '' ||
      parsed.password !== '' ||
      mallHostname === '' ||
      applicationHostname === '' ||
      mallHostname === applicationHostname
    ) {
      return undefined
    }
    return parsed.href
  } catch {
    return undefined
  }
}
