import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
test('375px与横屏：五页不溢出、点击区域及减少动态效果', async ({ page }) => {
  await mockAPI(page); await page.emulateMedia({ reducedMotion: 'reduce' });
  for (const viewport of [{ width: 375, height: 812 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport);
    for (const path of ['/records', '/sign', '/reports/week', '/reports/month', '/account']) {
      await page.goto(`#${path}`); await expect(page.getByRole('main')).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      const links = page.getByRole('navigation', { name: viewport.width <= 760 ? '手机导航' : '主导航', exact: true }).getByRole('link');
      expect(await links.count()).toBe(5);
      for (const link of await links.all()) { const box = await link.boundingBox(); expect(box?.width).toBeGreaterThanOrEqual(44); expect(box?.height).toBeGreaterThanOrEqual(44); }
    }
  }
  await page.setViewportSize({ width: 375, height: 812 }); await page.goto('#/records');
  await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
