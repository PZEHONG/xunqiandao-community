import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
import { mockLocations } from '../fixtures/amap';
for (const width of [320, 1440]) test(`${width}px：地点预览确认、缩放图层、重挂载和侧栏尺寸`, async ({ page }) => {
  await page.setViewportSize({ width, height: 900 }); const api = await mockAPI(page); const maps = await mockLocations(page);
  await page.goto('#/sign'); await expect(page.getByRole('button', { name: '放大地图' })).toBeVisible();
  const use = page.getByRole('button', { name: '使用此地点', exact: true }); await expect(use).toHaveCount(0);
  await page.getByRole('textbox', { name: '地点名称或详细地址' }).fill('示例园区'); await page.getByRole('button', { name: '搜索地点', exact: true }).click();
  await page.getByRole('button', { name: /示例园区 北京市/ }).click();
  await expect(page.getByText('候选地点待确认', { exact: true })).toBeVisible(); await expect(page.getByText('已确认地点', { exact: true })).toHaveCount(0);
  await use.click(); await expect(page.getByText('已确认地点', { exact: true })).toBeVisible();
  const plus = page.getByRole('button', { name: '放大地图' }); const minus = page.getByRole('button', { name: '缩小地图' });
  expect((await plus.boundingBox())?.width).toBeGreaterThanOrEqual(44);
  for (let i = 0; i < 8; i++) { if (await plus.isEnabled()) await plus.click(); } await expect(plus).toBeDisabled();
  await page.getByRole('region', { name: '地点地图' }).dispatchEvent('wheel', { deltaY: 120, deltaMode: 0 }); await expect(plus).toBeEnabled();
  for (let i = 0; i < 24; i++) { if (await minus.isEnabled()) await minus.click(); } await expect(minus).toBeDisabled();
  await page.getByRole('button', { name: '卫星图', exact: true }).click(); await expect(page.getByRole('button', { name: '卫星图', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: '定位点回中' }).click();
  await page.getByRole('region', { name: '地点地图' }).click({ position: { x: 40, y: 160 } }); await expect(page.getByText('候选地点待确认', { exact: true })).toBeVisible();
  await expect(page.getByText('已确认地点', { exact: true })).toHaveCount(0); await expect(page.getByText('调整后的地点', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '取消候选' }).click(); await expect(page.getByText('已确认地点', { exact: true })).toBeVisible();
  if (width > 760) { await page.getByRole('button', { name: '收起侧栏' }).click(); }
  await page.setViewportSize({ width: width + 10, height: 880 });
  expect(await page.evaluate(() => (window as unknown as { __mapTest: { maps: { resizeCount: number }[] } }).__mapTest.maps.at(-1)!.resizeCount)).toBeGreaterThan(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('main').focus(); await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath(`location-${width}.png`) });
  await page.getByRole('link', { name: '签到记录', exact: true }).last().click();
  expect(await page.evaluate(() => { const map = (window as unknown as { __mapTest: { maps: { destroyed: boolean; handlers: object }[] } }).__mapTest.maps.at(-1)!; return map.destroyed && Object.keys(map.handlers).length === 0; })).toBe(true);
  await page.getByRole('link', { name: '签到', exact: true }).last().click(); await expect(page.getByRole('button', { name: '放大地图' })).toBeVisible();
  await expect(page.getByText('已确认地点', { exact: true })).toHaveCount(0); expect(maps.sdkLoads()).toBe(1); expect(api.writes).toEqual([]); expect(api.unexpected).toEqual([]);
  expect(maps.reads.some(read => read.path.endsWith('/reverse'))).toBe(true);
});
test('底图未配置仍可搜索确认；搜索未配置明确禁用', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { mapEnabled: false }); await page.goto('#/sign');
  await expect(page.getByText('高德地图尚未连接')).toBeVisible(); await page.getByRole('textbox', { name: '地点名称或详细地址' }).fill('示例园区');
  await page.screenshot({ path: test.info().outputPath('map-unconfigured.png') });
  await page.getByRole('button', { name: '搜索地点', exact: true }).click(); await page.getByRole('button', { name: /示例园区 北京市/ }).click(); await page.getByRole('button', { name: '使用此地点', exact: true }).click();
  await expect(page.getByText('已确认地点', { exact: true })).toBeVisible();
  await mockLocations(page, { mapEnabled: false, searchEnabled: false }); await page.reload();
  await expect(page.getByText('地址搜索尚未连接，请完成高德配置后重试。')).toBeVisible(); await expect(page.getByRole('button', { name: '搜索地点', exact: true })).toBeDisabled();
});
test('375px与横屏：减少动态效果和放大文字下地点表单不溢出', async ({ page }) => {
  await mockAPI(page); await mockLocations(page); await page.emulateMedia({ reducedMotion: 'reduce' }); await page.goto('#/sign');
  await expect(page.getByRole('button', { name: '放大地图' })).toBeVisible();
  for (const viewport of [{ width: 375, height: 812 }, { width: 844, height: 390 }]) {
    await page.setViewportSize(viewport); await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    for (const button of await page.getByRole('main').getByRole('button').all()) {
      const box = await button.boundingBox(); if (box) { expect(box.width).toBeGreaterThanOrEqual(44); expect(box.height).toBeGreaterThanOrEqual(44); }
    }
  }
});
test('SDK失败与反查失败可见，失败候选不能确认', async ({ page }) => {
  await mockAPI(page); await mockLocations(page, { sdkFailure: true }); await page.goto('#/sign'); await expect(page.getByText('高德地图暂时不可用')).toBeVisible();
  await mockLocations(page, { reverseFailure: true }); await page.getByRole('button', { name: '重新连接地图' }).click(); await expect(page.getByRole('button', { name: '放大地图' })).toBeVisible();
  await page.getByRole('textbox', { name: '地点名称或详细地址' }).fill('示例园区'); await page.getByRole('button', { name: '搜索地点', exact: true }).click(); await page.getByRole('button', { name: /示例园区 北京市/ }).click(); await page.getByRole('button', { name: '使用此地点', exact: true }).click();
  await page.getByRole('region', { name: '地点地图' }).click({ position: { x: 40, y: 160 } });
  await expect(page.getByText('模拟反查失败', { exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: '使用此地点', exact: true })).toBeDisabled(); await expect(page.getByText('已确认地点', { exact: true })).toHaveCount(0);
});
