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
// Keep the workbook in memory only. Never interpolate server data into markup.
export async function downloadWorkbook(blob: Blob): Promise<void> {
  if (!blob.type.includes('spreadsheetml')) {
    let message = 'Export failed'
    try {
      const error = JSON.parse(await blob.text()) as { message?: string }
      message = error.message || message
    } catch {
      /* A non-workbook response must never be downloaded as XLSX. */
    }
    throw new Error(message)
  }
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'token-usage.xlsx'
  document.body.appendChild(link)
  link.click()
  link.remove()
  // Browsers consume the URL asynchronously after the click.
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}
