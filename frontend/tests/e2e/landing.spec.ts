import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';

for (const width of [320, 390, 760, 1024, 1440, 1920]) {
  test(`landing is readable without horizontal overflow at ${width}px`, async ({ page }, testInfo) => {
    const state = await mockAPI(page, { authenticated: false });
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.setViewportSize({ width, height: 960 });
    await page.goto('');
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('让实习签到，更从容。');
    await expect(page).toHaveURL(/#\/$/);
    await expect(page).toHaveTitle('讯签到 · 让实习签到，更从容');
    await expect(page.getByRole('link', { name: '立即使用' })).toHaveAttribute('href', '#/sign');
    await expect(page.getByRole('link', { name: '设置自动签到' })).toHaveAttribute('href', '#/sign?mode=daily');
    for (const name of ['自动签到', '实习报告', '异地签到']) {
      await page.getByRole('tab', { name }).click();
      await expect(page.getByRole('tab', { name })).toHaveAttribute('aria-selected', 'true');
      await expect(page.getByRole('tabpanel')).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    }
    await page.locator('summary').filter({ hasText: '目前支持查看哪些实习报告？' }).click();
    await expect(page.getByText('目前支持实习周报和月报的列表、详情查看。日报功能即将支持，开放后可在工作台使用。')).toBeVisible();
    expect(state.writes).toEqual([]);
    expect(errors).toEqual([]);
    await page.screenshot({ path: testInfo.outputPath(`landing-${width}.png`), fullPage: true, animations: 'disabled' });
  });
}

test('homepage preview keyboard navigation and workspace links work', async ({ page }) => {
  await mockAPI(page);
  await page.goto('#/');
  await page.getByRole('tab', { name: '异地签到' }).focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('tab', { name: '自动签到' })).toBeFocused();
  await expect(page.getByRole('tab', { name: '自动签到' })).toHaveAttribute('aria-selected', 'true');
  await page.getByRole('link', { name: '设置自动签到' }).click();
  await expect(page).toHaveURL(/#\/sign\?mode=daily$/);
  await expect(page.locator('.workspace')).toBeVisible();
  await page.goBack();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('让实习签到，更从容。');
  await page.getByRole('link', { name: '查看实习周报' }).click();
  await expect(page).toHaveURL(/#\/reports\/week$/);
  await expect(page.locator('.workspace')).toBeVisible();
});

test('homepage works with reduced motion', async ({ page }) => {
  await mockAPI(page, { authenticated: false });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('#/');
  await page.getByRole('button', { name: '了解产品' }).click();
  await expect(page.locator('#features-title')).toBeInViewport();
});
