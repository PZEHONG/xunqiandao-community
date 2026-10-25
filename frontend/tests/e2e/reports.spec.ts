import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
import { mockReports, reportDraftKey } from '../fixtures/reports';
for (const [kind, width] of [['week', 390], ['month', 1440]] as const) test(`${kind} ${width}px 旧草稿恢复、模板变化、附件重选及确认失败重试`, async ({ page }, testInfo) => {
  await page.setViewportSize({ width, height: 900 }); const api = await mockAPI(page); const state = await mockReports(page); state.fail = true;
  await page.addInitScript(({ key }) => { if (!localStorage.getItem(key)) localStorage.setItem(key, JSON.stringify({ start: '2026-10-01', end: '2026-10-07', content: [{ title: '工作内容', sort: 8, content: '旧版保留的实习正文' }, { title: '模板已删除', sort: 2, content: '不可丢失的旧字段' }], attachments: [{ name: '已上传.pdf', uri: '/reports/old.pdf' }], pendingNames: ['待重选.pdf'] })); }, { key: reportDraftKey(kind) });
  await page.goto(`#/reports/${kind}`); await page.getByRole('button', { name: '新建报告', exact: true }).click();
  await expect(page.getByLabel('工作内容', { exact: false })).toHaveValue('旧版保留的实习正文'); await expect(page.getByLabel('新增字段', { exact: true })).toHaveValue(''); await expect(page.getByText('不可丢失的旧字段', { exact: true })).toBeVisible();
  await expect(page.getByText('待重选.pdf', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '核对并提交', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0); expect(state.writes).toEqual([]);
  await page.getByLabel('选择报告附件').setInputFiles({ name: '待重选.pdf', mimeType: 'application/pdf', buffer: Buffer.from('fixture') });
  await page.evaluate(() => scrollTo(0, 0)); await expect.poll(() => page.evaluate(() => scrollY)).toBe(0); await page.screenshot({ path: testInfo.outputPath(`editor-${width}.png`), fullPage: true }); await page.getByRole('button', { name: '核对并提交', exact: true }).click(); const dialog = page.getByRole('dialog', { name: '确认提交报告' }); await expect(dialog).toContainText('旧版保留的实习正文'); expect(state.writes).toEqual([]);
  await page.keyboard.press('Escape'); expect(state.writes).toEqual([]); await page.getByRole('button', { name: '核对并提交', exact: true }).click(); await expect(dialog.locator('..')).toHaveCSS('opacity', '1'); await page.screenshot({ path: testInfo.outputPath(`confirm-${width}.png`) }); await dialog.getByRole('button', { name: '确认提交', exact: true }).click();
  await expect(page.getByText('模拟报告提交失败', { exact: true })).toBeVisible(); await expect(page.getByLabel('工作内容', { exact: false })).toHaveValue('旧版保留的实习正文'); expect(state.writes.map(w => w.path)).toEqual(['/api/reports/attachments', '/api/reports']);
  expect(state.writes[1].body).toMatchObject({ account: '2026001', business_type: kind, attachments: ['/reports/old.pdf', '/reports/uploaded.pdf'] });
  state.fail = false; await page.getByRole('button', { name: '核对并提交', exact: true }).click(); await dialog.getByRole('button', { name: '确认提交', exact: true }).click(); await expect(page.getByText('报告提交成功。', { exact: true })).toBeVisible(); expect(state.writes.filter(w => w.path.endsWith('/attachments'))).toHaveLength(1); expect(await page.evaluate(key => localStorage.getItem(key), reportDraftKey(kind))).toBeNull();
  await page.getByRole('button', { name: '查看详情', exact: true }).click(); const detail = page.getByRole('dialog', { name: '报告详情' }); await expect(detail).toContainText('继续保持，注意总结。'); await expect(detail).toContainText('报告正文只显示文本<script>danger</script>'); expect(await detail.locator('a[href^="javascript:"]').count()).toBe(0);
  await expect(detail).toContainText('2026-10-01 至 2026-10-07'); const download = page.waitForEvent('download'); await detail.getByRole('button', { name: '下载文本', exact: true }).click(); expect((await download).suggestedFilename()).toMatch(/\.txt$/); await page.screenshot({ path: testInfo.outputPath(`detail-${width}.png`) }); await detail.getByRole('button', { name: '关闭', exact: true }).click(); await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(page.getByText('第 2 / 2 页', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); expect(api.unexpected).toEqual([]);
});
test('刷新丢失本地File明确重选，结果未知保留草稿且不自动重发', async ({ page }) => {
  await mockAPI(page); const state = await mockReports(page); state.unknown = true; await page.goto('#/reports/week'); await page.getByRole('button', { name: '新建报告', exact: true }).click(); await page.getByLabel('开始日期', { exact: true }).fill('2026-10-01'); await page.getByLabel('结束日期', { exact: true }).fill('2026-10-07'); await page.getByLabel('工作内容', { exact: false }).fill('保留刷新前的正文'); await page.getByLabel('选择报告附件').setInputFiles({ name: '本地.txt', mimeType: 'text/plain', buffer: Buffer.from('fixture') }); await page.reload(); await page.getByRole('button', { name: '新建报告', exact: true }).click(); await expect(page.getByText('本地.txt', { exact: true })).toBeVisible(); await expect(page.getByLabel('工作内容', { exact: false })).toHaveValue('保留刷新前的正文');
  await page.getByRole('button', { name: '移除待重选附件 本地.txt' }).click(); await page.getByRole('button', { name: '核对并提交', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认提交', exact: true }).click(); await expect(page.getByText('报告提交结果暂时无法确认，请先查询报告列表，再决定是否重新提交。', { exact: true })).toBeVisible(); expect(state.writes).toHaveLength(1); expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!).submissionUncertain, reportDraftKey())).toBe(true);
});

test('JSON502保留报告结果待核对状态，刷新后不重发', async ({ page }) => {
  await mockAPI(page); await mockReports(page); let writes = 0;
  await page.route('**/api/reports', async route => {
    if (route.request().method() === 'GET') return route.fallback();
    writes++; return route.fulfill({ status: 502, json: { error: '读取报告响应失败' } });
  });
  await page.goto('#/reports/week'); await page.getByRole('button', { name: '新建报告', exact: true }).click();
  await page.getByLabel('开始日期', { exact: true }).fill('2026-10-01'); await page.getByLabel('结束日期', { exact: true }).fill('2026-10-07'); await page.getByLabel('工作内容', { exact: false }).fill('提交结果未知的正文');
  await page.getByRole('button', { name: '核对并提交', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: '确认提交', exact: true }).click();
  await expect(page.getByText('报告提交结果暂时无法确认，请先查询报告列表，再决定是否重新提交。', { exact: true })).toBeVisible();
  await page.reload(); await page.getByRole('button', { name: '新建报告', exact: true }).click();
  await expect(page.getByText('上次提交结果待核对，请先查询报告列表，确认没有重复记录后再重新提交。', { exact: true })).toBeVisible(); expect(writes).toBe(1);
});

test('损坏草稿编辑刷新后仍可下载原文', async ({ page }) => {
  await mockAPI(page); await mockReports(page); await page.goto('#/reports/week');
  await page.evaluate(key => localStorage.setItem(key, '{损坏但有原文'), reportDraftKey());
  await page.getByRole('button', { name: '新建报告', exact: true }).click(); await page.getByLabel('工作内容', { exact: false }).fill('新的正文');
  await page.reload(); await page.getByRole('button', { name: '新建报告', exact: true }).click(); await expect(page.getByLabel('工作内容', { exact: false })).toHaveValue('新的正文');
  const download = page.waitForEvent('download'); await page.getByRole('button', { name: '下载损坏草稿原文', exact: true }).click();
  expect((await download).suggestedFilename()).toBe('原始报告草稿.txt');
  expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key + ':recovery')!), reportDraftKey())).toEqual(['{损坏但有原文']);
});
