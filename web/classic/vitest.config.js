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
import path from 'node:path';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  resolve: {
    dedupe: ['react', 'react-dom'],
    alias: {
      // Use router ESM entries so Vite's React aliases also apply to its hooks.
      'react-router-dom': path.resolve(
        'node_modules/react-router-dom/dist/index.mjs',
      ),
      'react-router/dom': path.resolve(
        'node_modules/react-router/dist/development/dom-export.mjs',
      ),
      'react-router': path.resolve(
        'node_modules/react-router/dist/development/index.mjs',
      ),
      react: path.resolve('../node_modules/react'),
      'react-dom': path.resolve('../node_modules/react-dom'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/__tests__/*.{test,spec}.{js,jsx}'],
    // This existing suite uses node:test and is run by Node, not Vitest.
    exclude: ['src/components/topup/__tests__/payment-error.test.js'],
    setupFiles: ['./src/pages/BalanceQuery/__tests__/setup.js'],
    server: {
      deps: {
        inline: [
          '@douyinfe/semi-ui',
          '@douyinfe/semi-foundation',
          'react-i18next',
          'react-router-dom',
          'react-router',
          '@lobehub/icons',
        ],
      },
    },
    clearMocks: true,
    restoreMocks: true,
  },
});
