import { expect, test } from '@playwright/test';
import { fixtureSelf, mockAPI } from '../fixtures/api';
test('首次登录必须选学校与确认协议，确认后只写一次', async ({ page }, testInfo) => {
  const api = await mockAPI(page, { authenticated: false }); await page.goto('#/records');
  await page.getByRole('button', { name: '登录', exact: true }).first().click();
  await page.getByRole('tab', { name: '首次习讯云登录' }).click(); await page.getByLabel('学校名称').fill('示例'); await page.getByRole('option', { name: '示例实习技术学院' }).click();
  await page.getByLabel('学号', { exact: false }).fill('2026001'); await page.getByLabel('密码', { exact: false }).first().fill('test-password');
  const dialog = page.getByRole('dialog'); await expect(dialog.getByRole('button', { name: '登录', exact: true })).toBeDisabled(); expect(api.writes).toHaveLength(0);
  await page.getByRole('checkbox').check(); await expect(dialog.getByRole('button', { name: '登录', exact: true })).toBeEnabled(); await expect(page.locator('.MuiDialog-container')).toHaveCSS('opacity', '1'); await page.screenshot({ path: testInfo.outputPath('auth-official.png') }); await dialog.getByRole('button', { name: '登录', exact: true }).click();
  await expect(dialog).toHaveCount(0); await expect(page.getByRole('button', { name: '2026001' })).toBeVisible();
  expect(api.writes).toEqual([{ path: '/api/auth/official', body: { account: '2026001', password: 'test-password', school_id: '1', school_name: '示例实习技术学院', accept_terms: true } }]);
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain('test-password');
});
test('密码语义、Esc关闭、焦点返回、重新打开为空', async ({ page }) => {
  await mockAPI(page, { authenticated: false }); await page.goto('#/records'); const open = page.getByRole('button', { name: '登录', exact: true }).first(); await open.click();
  const password = page.locator('input[name="password"]'); await expect(password).toHaveAttribute('autocomplete', 'current-password'); await password.fill('temporary-password'); await page.keyboard.press('Escape'); await expect(open).toBeFocused(); await open.click(); await expect(password).toHaveValue('');
});
test('上游会话过期打开重新连接并预填学号', async ({ page }) => { await mockAPI(page, { upstreamExpired: true }); await page.goto('#/records'); await expect(page.getByRole('dialog')).toBeVisible(); await expect(page.locator('input[name="username"]')).toHaveValue('2026001'); await expect(page.getByText('请重新登录以恢复习讯云连接。')).toBeVisible(); });
test('同一浏览器两标签页退出后清空日历', async ({ context }) => {
  const api = await mockAPI(context); const one = await context.newPage(); const two = await context.newPage();
  await one.goto('#/records'); await two.goto('#/records'); await expect(two.getByRole('button', { name: '展开明细' })).toBeEnabled();
  await one.goto('#/account'); await one.getByRole('button', { name: '退出登录' }).click(); await one.getByRole('dialog', { name: '确认退出登录' }).getByRole('button', { name: '确认退出', exact: true }).click();
  await expect(two.getByRole('button', { name: '展开明细' })).toBeDisabled(); await expect(two.getByRole('button', { name: '2026001' })).toHaveCount(0); expect(api.writes.filter(write => write.path.endsWith('/logout'))).toHaveLength(1);
});

test('登录核验短暂失败、页面切回和自动恢复保留所选月份及打开的明细', async ({ page }) => {
  const api = await mockAPI(page); let failure: 'none' | 'server' | 'network' = 'none'; let checks = 0;
  await page.route('**/api/auth/self', async route => {
    checks++;
    if (failure === 'network') return route.abort('internetdisconnected');
    return route.fulfill({ status: failure === 'server' ? 503 : 200, json: failure === 'server' ? { error: '账号服务暂时繁忙' } : fixtureSelf });
  });
  await page.clock.install({ time: new Date('2026-10-18T04:00:00Z') }); await page.goto('#/records');
  await expect(page.getByText('2026-10 · 来源：习讯云')).toBeVisible();
  await page.getByRole('button', { name: '上个月' }).click(); await expect(page.getByText('2026-09 · 来源：习讯云')).toBeVisible();
  await page.getByRole('button', { name: '展开明细' }).click(); const dialog = page.getByRole('dialog'); await expect(dialog).toBeVisible();
  failure = 'server'; await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  await expect(page.getByText('账号服务暂时繁忙', { exact: true })).toBeVisible(); await expect(dialog).toBeVisible(); await expect(dialog).toContainText('2026-09 签到明细');
  failure = 'none'; const before = checks; await page.clock.fastForward(30000);
  await expect.poll(() => checks).toBeGreaterThan(before); await expect(page.getByText('账号服务暂时繁忙', { exact: true })).toHaveCount(0); await expect(dialog).toBeVisible();
  failure = 'network'; await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  await expect(page.getByText('无法连接服务，请稍后重试。', { exact: true })).toBeVisible(); await expect(dialog).toContainText('2026-09 签到明细');
  failure = 'none'; await page.evaluate(() => window.dispatchEvent(new Event('online')));
  await expect(page.getByText('无法连接服务，请稍后重试。', { exact: true })).toHaveCount(0); await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape'); await expect(page.getByRole('button', { name: '2026001', exact: true })).toBeVisible(); await expect(page.getByText('2026-09 · 来源：习讯云')).toBeVisible();
  expect(api.writes).toEqual([]);
});
