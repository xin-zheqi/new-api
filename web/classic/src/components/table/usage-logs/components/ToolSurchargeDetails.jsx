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
import React from 'react';
import { useTranslation } from 'react-i18next';

export default function ToolSurchargeDetails({ items }) {
  const { t } = useTranslation();
  if (!Array.isArray(items)) return null;
  const charges = items.filter(
    (item) =>
      typeof item?.name === 'string' &&
      item.name.trim() &&
      Number.isFinite(item.count) &&
      item.count > 0 &&
      Number.isFinite(item.price) &&
      item.price > 0,
  );
  if (!charges.length) return null;
  return (
    <div className='overflow-auto'>
      <table className='w-full text-left'>
        <thead>
          <tr>
            <th>{t('Tool identifier')}</th>
            <th>{t('Count')}</th>
            <th>{t('Price ($/1K calls)')}</th>
          </tr>
        </thead>
        <tbody>
          {charges.map((item) => (
            <tr key={`${item.name}:${item.price}`}>
              <td>{item.name}</td>
              <td>{item.count}</td>
              <td>{item.price}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
