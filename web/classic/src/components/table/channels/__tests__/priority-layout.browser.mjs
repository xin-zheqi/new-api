import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { test } from 'node:test';

// Run against a classic frontend with Playwright available via NODE_PATH.
// Only API responses are fixtures; layout uses the real modal and styles.
const { chromium } = createRequire(import.meta.url)('playwright');
const baseURL = process.env.NEWAPI_TEST_URL || 'http://127.0.0.1:3000';
const longChannelName =
  'Regional production endpoint with a long channel name / '.repeat(4);

for (const theme of ['light', 'dark']) {
  for (const width of [1440, 390]) {
    test(`${theme} theme at ${width}px keeps the priority modal content away from its bottom edge`, async () => {
      const browser = await chromium.launch({ headless: true });
      const page = await browser.newPage({ viewport: { width, height: 1050 } });
      const errors = [];
      page.on('pageerror', (error) => errors.push(error.message));
      await page.addInitScript((mode) => {
        localStorage.setItem('theme-mode', mode);
        localStorage.setItem('i18nextLng', 'en');
        localStorage.setItem('enable-tag-mode', 'false');
      }, theme);
      let items = [];
      const changes = [{ id: 2, previous: 80, priority: 110 }];
      const user = {
        id: 1,
        username: 'layout-fixture',
        display_name: 'Layout fixture',
        identity: 'personal',
        role: 100,
        status: 1,
        group: 'default',
        setting: '{}',
      };
      await page.route('**/api/**', async (route) => {
        const path = new URL(route.request().url()).pathname;
        let data = [];
        if (path === '/api/status') {
          data = { setup: true, system_name: 'New API', chats: [] };
        } else if (path === '/api/user/auth/refresh') {
          data = {
            access_token: 'layout-fixture-token',
            access_expires_at: Math.floor(Date.now() / 1000) + 3600,
            user,
            session: { sid: 'layout-fixture-session', current: true },
          };
        } else if (path === '/api/user/self') {
          data = user;
        } else if (path === '/api/channel/') {
          data = {
            items: items.map((item) => ({
              ...item,
              name: item.id === 2 ? longChannelName : `Channel ${item.id}`,
              type: 1,
              status: 1,
              group: 'default',
              models: 'test-model',
              balance: 0,
              used_quota: 0,
              response_time: 0,
            })),
            total: items.length,
            type_counts: { 1: items.length },
          };
        } else if (path === '/api/channel/priority/order') {
          data = { revision: 'v1', items, changes: [] };
        } else if (path === '/api/channel/priority/reorder') {
          data = { revision: 'v2', items, changes };
        }
        await route.fulfill({ json: { success: true, data } });
      });
      try {
        for (const count of [0, 24]) {
          // Enough rows to exercise the real list's scroll boundary.
          items = Array.from({ length: count }, (_, index) => ({
            id: index + 1,
            priority: index === 0 ? Number.MAX_SAFE_INTEGER : 100 - index * 10,
          }));
          await page.goto(`${baseURL}/console/channel`);
          if (width < 768) {
            await page.getByRole('button', { name: /Show actions$/ }).click();
          }
          await page
            .getByRole('button', {
              name: 'Drag to set channel priority',
              exact: true,
            })
            .click();
          const dialog = page.getByRole('dialog');
          const list = dialog.getByRole('region', {
            name: 'Channel priorities',
          });
          await list.waitFor();
          await dialog.evaluate(async (element) => {
            await Promise.all(
              element
                .getAnimations({ subtree: true })
                .map((animation) => animation.finished),
            );
          });
          await assertBottomGap(dialog, list);
          const channelControl = await dialog
            .getByRole('combobox', { name: 'Channel', exact: true })
            .boundingBox();
          const previewButton = await dialog
            .getByRole('button', { name: 'Preview', exact: true })
            .boundingBox();
          assert(
            Math.abs(channelControl.height - previewButton.height) <= 1,
            'channel picker and action button have consistent control heights',
          );
          const bounds = await dialog.boundingBox();
          assert(
            bounds.x >= 0 && bounds.x + bounds.width <= width,
            'dialog fits viewport width',
          );
          if (count) {
            assert(
              await list.evaluate(
                (element) => element.scrollWidth <= element.clientWidth,
              ),
              'long channel names do not widen the list beyond the modal',
            );
            assert(
              await list
                .getByTitle(`#2 ${longChannelName}`)
                .evaluate(
                  (element) => element.scrollWidth > element.clientWidth,
                ),
              'long channel name is truncated with its full name available on hover',
            );
            assert(
              await list.evaluate(
                (element) => element.scrollHeight > element.clientHeight,
              ),
              'long list scrolls',
            );
            await list.evaluate((element) => {
              element.scrollTop = element.scrollHeight;
            });
            await assertBottomGap(dialog, list);
            await dialog
              .getByRole('combobox', { name: 'Channel', exact: true })
              .click();
            await dialog
              .getByRole('combobox', { name: 'Channel', exact: true })
              .getByRole('textbox')
              .fill('Regional');
            await page.getByRole('option', { name: /#2 / }).click();
            await page.getByRole('listbox').waitFor({ state: 'hidden' });
            await dialog
              .getByRole('combobox', { name: 'Target channel', exact: true })
              .click();
            await page.getByRole('option', { name: /#1 Channel 1/ }).click();
            await page.getByRole('listbox').waitFor({ state: 'hidden' });
            await dialog
              .getByRole('button', { name: 'Preview', exact: true })
              .click();
            const preview = dialog.getByRole('status');
            await preview.waitFor();
            await assertBottomGap(
              dialog,
              dialog.getByRole('button', { name: 'Save Changes', exact: true }),
            );
            await dialog
              .getByRole('button', { name: 'Save Changes', exact: true })
              .click();
            const undo = dialog.getByRole('button', {
              name: 'Undo last move',
              exact: true,
            });
            await undo.waitFor();
            await assertBottomGap(dialog, undo);
          }
        }
        assert.deepEqual(errors, []);
      } finally {
        await browser.close();
      }
    });
  }
}

async function assertBottomGap(dialog, lastContent) {
  const panel = await dialog.boundingBox();
  const content = await lastContent.boundingBox();
  const gap = panel.y + panel.height - content.y - content.height;
  assert(
    gap >= 20 && gap <= 32,
    `modal bottom gap must remain comfortable (20–32px), got ${gap}px`,
  );
}
