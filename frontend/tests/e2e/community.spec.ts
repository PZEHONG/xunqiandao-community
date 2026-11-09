import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';

test('community workspace has no private entry points or requests', async ({ page }) => {
  const api = await mockAPI(page);
  const privateRequests: string[] = [];
  page.on('request', request => {
    if (/\/api\/(membership|admin|pay|recharge|alipay|invitation|announcements)(\/|$)/.test(new URL(request.url()).pathname)) privateRequests.push(request.url());
  });
  await page.goto('#/records');
  await expect(page.locator('.workspace')).toBeVisible();
  await expect(page.getByRole('link', { name: '会员中心' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /管理端|后台管理/ })).toHaveCount(0);
  await page.goto('#/membership');
  await expect(page).toHaveURL(/#\/records$/);
  await page.goto('#/sign?mode=daily');
  await expect(page.getByRole('button', { name: '每日自动签到', exact: true })).toBeVisible();
  await expect(page.getByLabel('会员权益')).toHaveCount(0);
  await expect(page.getByText(/会员或管控暂停/)).toHaveCount(0);
  expect(privateRequests).toEqual([]);
  expect(api.unexpected).toEqual([]);
});

test('community notification email can be verified, paused and removed', async ({ page }) => {
  const api = await mockAPI(page);
  let value: { email: string; enabled: boolean; version: number; verified: boolean; updated_at: number } | null = null;
  const writes: string[] = [];
  await page.route('**/api/auth/notification-email**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/captcha')) return route.fulfill({ json: { enabled: false, ready: false, captcha_id: '' } });
    if (route.request().method() === 'GET') return route.fulfill({ json: value || { message: '尚未绑定签到提醒邮箱。' } });
    writes.push(path);
    const input = route.request().postDataJSON();
    expect(input).not.toHaveProperty('user_id');
    expect(input).not.toHaveProperty('password');
    if (path.endsWith('/code')) return route.fulfill({ json: { message: '验证码已发送，请查收邮箱。' } });
    if (path.endsWith('/bind')) value = { email: input.email, enabled: true, version: 1, verified: true, updated_at: 1 };
    else if (path.endsWith('/enabled') && value) value = { ...value, enabled: input.enabled };
    else if (path.endsWith('/unbind')) value = null;
    return route.fulfill({ json: { notification_email: value, message: '设置已更新。' } });
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('#/account');
  await page.getByLabel('接收提醒的邮箱').fill('notify@example.com');
  await page.getByRole('button', { name: '发送验证码' }).click();
  await page.getByLabel('6 位验证码').fill('123456');
  await page.getByRole('button', { name: '绑定邮箱', exact: true }).click();
  await expect(page.getByText('notify@example.com', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '暂停提醒' }).click();
  await expect(page.getByText('提醒已关闭', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '解除绑定' }).click();
  await expect(page.getByLabel('接收提醒的邮箱')).toBeVisible();
  expect(writes.map(path => path.split('/').at(-1))).toEqual(['code', 'bind', 'enabled', 'unbind']);
  expect(api.unexpected).toEqual([]);
});
