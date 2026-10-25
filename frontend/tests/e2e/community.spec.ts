import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';

test('community workspace has no private entry points or requests', async ({ page }) => {
  const api = await mockAPI(page);
  const privateRequests: string[] = [];
  page.on('request', request => {
    if (/\/api\/(membership|admin)(\/|$)/.test(new URL(request.url()).pathname)) privateRequests.push(request.url());
  });
  await page.goto('#/records');
  await expect(page.locator('.workspace')).toBeVisible();
  await expect(page.getByRole('link', { name: '会员中心' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /管理端|后台管理/ })).toHaveCount(0);
  await page.goto('#/membership');
  await expect(page).toHaveURL(/#\/records$/);
  expect(privateRequests).toEqual([]);
  expect(api.unexpected).toEqual([]);
});
