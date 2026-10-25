import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
for (const width of [320, 1440]) test(`${width}px日历、失败快照与明细焦点`, async ({ page }, testInfo) => {
  const api = await mockAPI(page, { failMonth: '2026-09' }); await page.clock.setFixedTime(new Date('2026-10-17T04:00:00Z')); await page.setViewportSize({ width, height: 900 }); await page.goto('');
  await expect(page.getByText('2026-10 · 来源：习讯云')).toBeVisible(); await expect(page.getByText('6', { exact: true }).first()).toBeVisible();
  await expect(page.getByRole('gridcell', { name: /2026-10-15.*异常/ })).toBeVisible(); await expect(page.getByRole('gridcell', { name: /2026-10-16.*未知/ })).toBeVisible();
  await page.getByRole('button', { name: '上个月' }).click(); await expect(page.getByRole('alert')).toContainText('模拟查询失败'); await expect(page.getByRole('heading', { name: '2026年10月' })).toBeVisible();
  const open = page.getByRole('button', { name: '展开明细' }); await open.click(); await expect(page.getByRole('dialog')).toBeVisible(); await expect(page.getByRole('heading', { name: '2026-10 签到明细' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await expect(page.locator('.MuiDialog-container')).toHaveCSS('opacity', '1');
  await page.screenshot({ path: testInfo.outputPath(`records-dialog-${width}.png`) });
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(open).toBeFocused();
  await open.click(); await page.getByRole('button', { name: '关闭明细' }).click(); await expect(open).toBeFocused();
  await open.click(); await page.locator('.MuiDialog-container').click({ position: { x: 1, y: 1 } }); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(open).toBeFocused();
  await page.evaluate(() => scrollTo(0, 0)); await page.screenshot({ path: testInfo.outputPath(`records-${width}.png`), fullPage: true }); expect(api.writes).toHaveLength(0);
});
