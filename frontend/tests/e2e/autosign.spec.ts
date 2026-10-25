import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
import { mockLocations } from '../fixtures/amap';
import { mockAutoSign } from '../fixtures/autosign';
import { pngPhoto } from '../fixtures/sign';
for (const width of [320, 1440]) test(`${width}px每日任务完整生命周期、照片隔离和执行历史`, async ({ page }, testInfo) => {
  await page.setViewportSize({ width, height: 900 }); const api = await mockAPI(page); await mockLocations(page, { mapEnabled: false }); const state = await mockAutoSign(page);
  await page.goto('#/sign'); await page.getByLabel('选择签到照片').setInputFiles(pngPhoto); await page.getByRole('button', { name: '每日自动签到', exact: true }).click();
  await expect(page.getByLabel('选择签到照片')).toHaveCount(0); await expect(page.getByText(/包含周末/).first()).toBeVisible();
  await page.getByRole('textbox', { name: '地点名称或详细地址' }).fill('园区'); await page.getByRole('button', { name: '搜索地点', exact: true }).click(); await page.getByRole('button', { name: /示例园区 北京市/ }).click(); await page.getByRole('button', { name: '使用此地点', exact: true }).click();
  await page.getByLabel('任务名称').fill('每日下班'); await page.getByLabel('每日时间（北京时间）').fill('18:30'); await page.getByRole('button', { name: '下班', exact: true }).click(); await page.getByLabel('签到说明（选填）').fill('每日任务说明');
  await page.getByRole('button', { name: '核对每日任务', exact: true }).click(); const confirm = page.getByRole('dialog', { name: '确认启用每日任务' }); await expect(confirm).toBeVisible(); expect(state.writes).toEqual([]);
  await page.keyboard.press('Escape'); expect(state.writes).toEqual([]); await page.getByRole('button', { name: '核对每日任务', exact: true }).click(); await expect(confirm.locator('..')).toHaveCSS('opacity', '1');
  await page.screenshot({ path: testInfo.outputPath(`daily-confirm-${width}.png`) }); await confirm.getByRole('button', { name: '确认启用', exact: true }).click();
  await expect(page.getByRole('alert').filter({ hasText: '每日任务已确认启用' })).toBeVisible(); expect(state.writes).toHaveLength(1); expect(state.writes[0].body).toMatchObject({ name: '每日下班', time: '18:30', confirmed: true, location_confirmed: true, remark: '1', comment: '每日任务说明' }); expect(state.writes[0].body).not.toHaveProperty('photos');
  const card = page.getByRole('article', { name: '每日上班任务' }); await card.getByRole('button', { name: '编辑', exact: true }).click(); await expect(page.getByLabel('任务名称')).toHaveValue('每日上班'); await expect(card.getByText('已暂停', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '取消编辑', exact: true }).click(); await expect(card.getByText('已暂停', { exact: true })).toBeVisible(); await card.getByRole('button', { name: '重新启用', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认启用', exact: true }).click(); await expect(card.getByText('已启用', { exact: true })).toBeVisible();
  await card.getByRole('button', { name: '暂停', exact: true }).click(); await expect(card.getByText('已暂停', { exact: true })).toBeVisible(); await card.getByRole('button', { name: '删除', exact: true }).click(); const removal=page.getByRole('dialog', { name: '确认删除每日任务' }); await expect(removal.locator('..')).toHaveCSS('opacity','1'); await page.screenshot({path:testInfo.outputPath(`daily-remove-${width}.png`)}); await removal.getByRole('button', { name: '确认删除', exact: true }).click();
  await expect(card).toHaveCount(0); await expect(page.getByRole('article', { name: '每日上班执行记录' })).toContainText('失败'); await expect(page.getByRole('article', { name: '每日上班执行记录' })).toContainText('2026-10-17 09:00');
  await page.evaluate(() => scrollTo(0, 0)); await expect.poll(() => page.evaluate(() => scrollY)).toBe(0);
  await page.screenshot({ path: testInfo.outputPath(`daily-tasks-${width}.png`), fullPage: true }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); expect(api.unexpected).toEqual([]);
});
test('保存原地点可直接修改；主动改地点必须重新搜索确认', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { mapEnabled: false }); const state = await mockAutoSign(page); await page.goto('#/sign?mode=daily');
  const card = page.getByRole('article', { name: '每日上班任务' }); await card.getByRole('button', { name: '编辑', exact: true }).click(); await expect(page.getByText('保留已保存地点')).toBeVisible(); await expect(page.getByLabel('签到说明（选填）')).toHaveValue('原说明');
  await page.getByLabel('签到说明（选填）').fill('仅改说明'); await page.getByRole('button', { name: '核对修改并启用', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认启用', exact: true }).click();
  await expect.poll(() => state.writes.filter(write => write.method === 'PUT').length).toBe(1); expect(state.writes.find(write => write.method === 'PUT')?.body).toMatchObject({ address: '北京市已保存路2号', confirmation_token: '', revision: 4 });
  await card.getByRole('button', { name: '编辑', exact: true }).click(); await page.getByRole('button', { name: '更换地点', exact: true }).click(); await expect(page.getByRole('button', { name: '核对修改并启用', exact: true })).toBeDisabled();
  await page.getByRole('textbox', { name: '地点名称或详细地址' }).fill('新园区'); await page.getByRole('button', { name: '搜索地点', exact: true }).click(); await page.getByRole('button', { name: /示例园区 北京市/ }).click(); await page.getByRole('button', { name: '使用此地点', exact: true }).click(); await page.getByRole('button', { name: '核对修改并启用', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认启用', exact: true }).click();
  await expect.poll(() => state.writes.filter(write => write.method === 'PUT').length).toBe(2); expect(state.writes.at(-1)?.body).toMatchObject({ confirmation_token: 'fixture-token', address: '北京市示例区示例路1号' });
});
test('版本冲突要求刷新，不强制覆盖或自动重发', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { mapEnabled: false }); const state = await mockAutoSign(page, { conflict: true }); await page.goto('#/sign?mode=daily');
  const card = page.getByRole('article', { name: '每日上班任务' }); await card.getByRole('button', { name: '暂停', exact: true }).click(); await expect(page.getByText('任务已在其他页面变更，请刷新任务后重新核对。')).toBeVisible(); await expect(card.getByRole('button', { name: '暂停', exact: true })).toBeDisabled(); expect(state.writes).toHaveLength(1);
  state.conflict = false; await page.getByRole('button', { name: '刷新任务与记录', exact: true }).click(); await expect(card.getByRole('button', { name: '暂停', exact: true })).toBeEnabled(); await card.getByRole('button', { name: '暂停', exact: true }).click(); await expect(card.getByText('已暂停', { exact: true })).toBeVisible(); expect(state.writes).toHaveLength(2);
});
test('写入中锁定取消和模式切换，重复点击只发一次；未知结果先刷新', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { mapEnabled: false }); const state = await mockAutoSign(page, { hold: true, unknown: true }); await page.goto('#/sign?mode=daily');
  await page.getByRole('article', { name: '每日上班任务' }).getByRole('button', { name: '删除', exact: true }).click(); const dialog = page.getByRole('dialog', { name: '确认删除每日任务' }); await dialog.getByRole('button', { name: '确认删除', exact: true }).dispatchEvent('click');
  await expect.poll(() => state.writes.length).toBe(1); await dialog.getByRole('button', { name: '处理中…', exact: true }).dispatchEvent('click'); await expect(dialog.getByRole('button', { name: '取消', exact: true })).toBeDisabled(); await expect(page.getByRole('button', { name: '立即签到', exact: true, includeHidden: true })).toBeDisabled();
  await page.keyboard.press('Escape'); await expect(dialog).toBeVisible(); expect(state.writes).toHaveLength(1); state.release();
  await expect(page.getByText('任务操作结果暂时无法确认，请先刷新任务与执行记录，再决定是否重新操作。')).toBeVisible(); await expect(dialog).toHaveCount(0); await expect(page.getByRole('article', { name: '每日上班任务' }).getByRole('button', { name: '删除', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '刷新任务与记录', exact: true }).click(); await expect(page.getByRole('article', { name: '每日上班任务' }).getByRole('button', { name: '删除', exact: true })).toBeEnabled(); expect(state.writes).toHaveLength(1);
});
