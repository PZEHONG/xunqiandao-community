import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
test('旧版五个页面桌面和手机基线截图（模拟数据）', async ({ page }, testInfo) => {
  await mockAPI(page); await page.clock.setFixedTime(new Date('2026-10-17T04:00:00Z'));
  for (const width of [390, 1440]) { await page.setViewportSize({ width, height: 900 }); for (const hash of ['records-panel', 'sign-panel', 'week-panel', 'month-panel', 'account-panel']) { await page.goto(`http://127.0.0.1:8080/legacy/#${hash}`); await expect(page.locator(`#${hash}`)).toBeVisible(); await page.screenshot({ path: testInfo.outputPath(`legacy-${hash}-${width}.png`), fullPage: true }); } }
});
