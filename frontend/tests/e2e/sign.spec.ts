import { expect, test, type Page } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
import { mockLocations } from '../fixtures/amap';
import { mockSign, pngPhoto } from '../fixtures/sign';
async function chooseLocation(page: Page) {
  await page.getByRole('textbox', { name: '地点名称或详细地址' }).fill('示例园区'); await page.getByRole('button', { name: '搜索地点', exact: true }).click();
  await page.getByRole('button', { name: /示例园区 北京市/ }).click(); await page.getByRole('button', { name: '使用此地点', exact: true }).click();
}
for (const width of [320, 1440]) test(`${width}px照片与长地址核对、取消不上传、确认只提交一次`, async ({ page }) => {
  const address = '北京市示例区示例路1号' + '园区较长楼栋地址用于确认核对'.repeat(18);
  await page.setViewportSize({ width, height: 900 }); const api = await mockAPI(page); await mockLocations(page, { mapEnabled: false, address }); const state = await mockSign(page, { holdSign: true });
  await page.goto('#/sign'); await expect(page.getByRole('button', { name: '核对签到信息' })).toBeDisabled(); await chooseLocation(page);
  await page.getByLabel('签到说明（选填）').fill('已核对当天实际签到地点。'); await page.getByLabel('选择签到照片').setInputFiles(pngPhoto); await expect(page.getByAltText('fixture.png')).toBeVisible();
  await page.getByAltText('fixture.png').scrollIntoViewIfNeeded(); await page.screenshot({ path: test.info().outputPath(`photos-${width}.png`) });
  await page.getByRole('button', { name: '核对签到信息' }).click(); const dialog = page.getByRole('dialog', { name: '核对本次签到' }); await expect(dialog).toBeVisible();
  expect(state.uploads).toBe(0); expect(state.submissions).toEqual([]); await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); expect(state.uploads).toBe(0);
  await page.getByRole('button', { name: '核对签到信息' }).click(); await expect(page.locator('.MuiDialog-container')).toHaveCSS('opacity', '1');
  await page.screenshot({ path: test.info().outputPath(`confirm-${width}.png`) });
  await dialog.getByRole('button', { name: '确认提交签到', exact: true }).dispatchEvent('click'); await dialog.getByRole('button', { name: /提交中/ }).dispatchEvent('click');
  await expect.poll(() => state.submissions.length).toBe(1); expect(state.uploads).toBe(1); await expect(dialog.getByRole('button', { name: '取消' })).toBeDisabled();
  await page.keyboard.press('Escape'); await expect(dialog).toBeVisible(); state.release(); await expect(page.getByRole('dialog', { name: '签到成功' })).toBeVisible();
  expect(state.submissions[0]).toMatchObject({ account: '2026001', address, remark: '0', comment: '已核对当天实际签到地点。', photos: ['/uploads/fixture.png'], location_confirmed: true });
  await page.getByRole('button', { name: '完成', exact: true }).click(); await expect(page.getByLabel('签到说明（选填）')).toHaveValue(''); await expect(page.getByAltText('fixture.png')).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); expect(api.unexpected).toEqual([]);
});
test('明确失败后保留照片正文，重新核对时复用上传结果', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { mapEnabled: false }); const state = await mockSign(page, { failOnce: true }); await page.goto('#/sign'); await chooseLocation(page);
  await page.getByLabel('签到说明（选填）').fill('保留这段说明'); await page.getByLabel('选择签到照片').setInputFiles(pngPhoto);
  await page.getByRole('button', { name: '核对签到信息' }).click(); await page.getByRole('button', { name: '确认提交签到', exact: true }).click();
  await expect(page.getByText('模拟签到失败，请重新核对', { exact: true })).toBeVisible(); await expect(page.getByLabel('签到说明（选填）')).toHaveValue('保留这段说明');
  expect(state.uploads).toBe(1); expect(state.submissions).toHaveLength(1);
  await page.getByRole('button', { name: '核对签到信息' }).click(); await page.getByRole('button', { name: '确认提交签到', exact: true }).click(); await expect(page.getByRole('dialog', { name: '签到成功' })).toBeVisible();
  expect(state.uploads).toBe(1); expect(state.submissions).toHaveLength(2);
});
test('未知结果不显示成功或自动重发，可进入记录查询', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { mapEnabled: false }); const state = await mockSign(page, { unknown: true }); await page.goto('#/sign'); await chooseLocation(page);
  await page.getByRole('button', { name: '核对签到信息' }).click(); await page.getByRole('button', { name: '确认提交签到', exact: true }).click();
  await expect(page.getByText('签到提交结果暂时无法确认，请先刷新签到记录，再决定是否重新提交。', { exact: true })).toBeVisible();
  await expect(page.getByRole('dialog', { name: '签到成功' })).toHaveCount(0); expect(state.submissions).toHaveLength(1);
  await page.getByRole('link', { name: '查看签到记录', exact: true }).click(); await expect(page).toHaveURL(/#\/records$/); expect(state.submissions).toHaveLength(1);
});
test('上传失败不签到；照片规则、删除与候选地点阻止核对', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { mapEnabled: false }); const state = await mockSign(page, { uploadFailure: true }); await page.goto('#/sign'); await chooseLocation(page);
  await page.getByLabel('选择签到照片').setInputFiles({ name: 'bad.svg', mimeType: 'image/svg+xml', buffer: Buffer.from('<svg/>') }); await expect(page.getByText(/请选择10 MB以内/)).toBeVisible();
  await page.getByLabel('选择签到照片').setInputFiles(pngPhoto); await page.getByRole('button', { name: '移除照片 fixture.png' }).click(); await expect(page.getByAltText('fixture.png')).toHaveCount(0);
  await page.getByLabel('选择签到照片').setInputFiles(pngPhoto); await page.getByRole('button', { name: '核对签到信息' }).click(); await page.getByRole('button', { name: '确认提交签到', exact: true }).click();
  await expect(page.getByText('模拟照片上传失败', { exact: true })).toBeVisible(); expect(state.submissions).toEqual([]);
  await page.getByRole('button', { name: '搜索地点', exact: true }).click(); await page.getByRole('button', { name: /示例园区 北京市/ }).click(); await expect(page.getByRole('button', { name: '核对签到信息' })).toBeDisabled();
});
