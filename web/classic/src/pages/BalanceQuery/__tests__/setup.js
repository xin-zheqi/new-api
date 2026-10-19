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
import { vi } from 'vitest';
import '@douyinfe/semi-ui/lib/es/react19-adapter';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.stubGlobal(
  'ResizeObserver',
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);

// Semi's icon bundle probes canvas during import; jsdom does not implement it.
vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
  fillStyle: '',
  fillRect: () => {},
});

vi.stubGlobal('matchMedia', () => ({
  matches: false,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
}));
