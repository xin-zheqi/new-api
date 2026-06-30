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
import type { LotteryMode } from './types'

export const WEEKDAYS = [0, 1, 2, 3, 4, 5, 6]

export const defaultLotteryForm = {
  title: '',
  description: '',
  prizeName: '',
  mode: 'once' as LotteryMode,
  winnerCount: 1,
  prizePerWinner: 1,
  registrationStart: '',
  registrationEnd: '',
  drawTime: '',
  scheduleWeekdays: [1, 3, 5],
  scheduleStartTime: '09:00',
  scheduleEndTime: '18:00',
  scheduleDrawTime: '20:00',
  prizeCodes: '',
}
