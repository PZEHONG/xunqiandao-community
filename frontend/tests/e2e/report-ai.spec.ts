import { expect, test, type Page } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
import { mockReports, reportDraftKey } from '../fixtures/reports';

const template = [{ title: '工作内容', sort: 1, require: '1', content: '' }, { title: '实习体会', sort: 2, require: '0', content: '' }];
function streamBody(start: string, fail = false) {
 const fields = template.map((f, i) => ({ ...f, content: `${start} ${i === 0 ? '整理项目素材，完成剪辑练习。' : '学习沟通与流程规范。'}` }));
 const events: unknown[] = fields.flatMap((f, index) => [{ type: 'field', index, field: { ...f, content: '' } }, { type: 'delta', index, delta: f.content }]);
 events.push(fail ? { type: 'error', error: '模拟模型连接中断' } : { type: 'done', fields });
 return events.map(e => `data: ${JSON.stringify(e)}\n\n`).join('');
}
async function mockAI(page: Page) {
 const state = { generations: [] as Record<string, string>[], fail: false, dateDelay: 0, dateCalls: 0, missingRanges: [] as string[] };
 await page.route('**/api/reports/ai/**', async route => {
  const url = new URL(route.request().url());
  if (url.pathname.endsWith('/dates')) {
   state.dateCalls++;
   if (state.dateDelay) await new Promise(resolve => setTimeout(resolve, state.dateDelay));
   const month = url.searchParams.get('business_type') === 'month';
   return route.fulfill({ json: { start: month ? '2026-07-01' : '2026-09-08', end: '2026-10-29', reason: '已根据已有报告补齐日期，优先从历史缺口开始。' } });
  }
  if (url.pathname.endsWith('/missing')) { state.missingRanges.push(`${url.searchParams.get('start')}:${url.searchParams.get('end')}`); return route.fulfill({ json: { periods: url.searchParams.get('business_type') === 'month' ? [
   { start: '2026-06-01', end: '2026-06-30', state: 'existing' },
   { start: '2026-07-01', end: '2026-07-31', state: 'missing' },
   { start: '2026-08-01', end: '2026-08-31', state: 'overlap' },
   { start: '2026-09-01', end: '2026-09-30', state: 'missing' },
  ] : [
   { start: '2026-09-01', end: '2026-09-07', state: 'existing' },
   { start: '2026-09-08', end: '2026-09-14', state: 'missing' },
   { start: '2026-09-15', end: '2026-09-21', state: 'overlap' },
   { start: '2026-09-22', end: '2026-09-28', state: 'missing' },
  ] } }); }
  const input = route.request().postDataJSON(); state.generations.push(input);
  return route.fulfill({ contentType: 'text/event-stream', body: streamBody(input.start, state.fail) });
 });
 return state;
}
async function enterMaterial(page: Page) {
 await page.getByLabel('工作素材', { exact: true }).fill('实习岗位：视频剪辑。整理素材，学习剪辑流程；无明确成果数据。');
 await expect(page.getByText(/AI 连接配置/)).toHaveCount(0);
 await expect(page.getByLabel('API 密钥', { exact: true })).toHaveCount(0);
}

for (const width of [390, 1023]) test(`缺报可勾选与全选，操作并排且悬停显示图标 ${width}px`, async ({ page }, info) => {
 await page.setViewportSize({ width, height: 871 });
 await mockAPI(page); const reports = await mockReports(page); const ai = await mockAI(page);
 await page.goto('#/reports/week'); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 const fill = page.getByRole('button', { name: '智能填写日期', exact: true });
 const query = page.getByRole('button', { name: '查询缺报', exact: true });
 await expect(query).toBeEnabled();
 await expect(page.getByText('已根据已有报告补齐日期，优先从历史缺口开始。')).toHaveCount(0);
 const a = await fill.boundingBox(); const b = await query.boundingBox();
 expect(Math.abs(a!.y - b!.y)).toBeLessThan(2); expect(b!.x).toBeGreaterThan(a!.x);
 const icon = fill.getByTestId('date-fill-reveal');
 await page.getByLabel('报告起始日期').hover(); await expect(icon).toHaveCSS('opacity', '0');
 await fill.hover(); await expect(icon).toHaveCSS('opacity', '1');
 await page.getByLabel('报告起始日期').hover(); await query.focus(); await page.keyboard.press('Shift+Tab'); await expect(fill).toBeFocused(); await expect(icon).toHaveCSS('opacity', '1');
 await query.click();
 const generate = page.getByRole('button', { name: /生成所选草稿/ });
 await expect(generate).toBeDisabled();
 await expect(page.getByRole('checkbox', { name: '补全 2026-09-01 至 2026-09-07', exact: true })).toBeDisabled();
 await expect(page.getByRole('checkbox', { name: '补全 2026-09-15 至 2026-09-21', exact: true })).toBeDisabled();
 const first = page.getByRole('checkbox', { name: '补全 2026-09-08 至 2026-09-14', exact: true });
 const second = page.getByRole('checkbox', { name: '补全 2026-09-22 至 2026-09-28', exact: true });
 const all = page.getByRole('checkbox', { name: '全选缺报', exact: true });
 await first.check(); await expect(page.getByText('已选 1 段', { exact: true })).toBeVisible();
 await expect(all).toHaveAttribute('aria-checked', 'mixed');
 await all.check(); await expect(first).toBeChecked(); await expect(second).toBeChecked();
 await all.uncheck(); await expect(generate).toBeDisabled(); await expect(first).not.toBeChecked();
 await second.check(); await enterMaterial(page);
 await page.screenshot({ path: info.outputPath(`selective-completion-${width}.png`), fullPage: true });
 expect(await page.getByRole('dialog').evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
 await page.getByRole('button', { name: '生成所选草稿（1）', exact: true }).click();
 await expect(page.getByText('草稿已生成，请逐份核对、补充真实细节后再提交。')).toBeVisible();
 expect(ai.generations.map(g => [g.start, g.end])).toEqual([['2026-09-22', '2026-09-28']]);
 expect(reports.writes).toEqual([]);
});

test.describe('触屏操作', () => {
 test.use({ hasTouch: true, isMobile: true, viewport: { width: 390, height: 844 } });
 test('智能填写图标直接显示，操作按钮同排', async ({ page }) => {
  await mockAPI(page); await mockReports(page); await mockAI(page);
  await page.goto('#/reports/week'); await page.getByRole('button', { name: 'AI补全', exact: true }).tap();
  const fill = page.getByRole('button', { name: '智能填写日期', exact: true });
  const query = page.getByRole('button', { name: '查询缺报', exact: true });
  await expect(query).toBeEnabled();
  await expect(fill.getByTestId('date-fill-reveal')).toHaveCSS('opacity', '1');
  const a = await fill.boundingBox(); const b = await query.boundingBox();
  expect(Math.abs(a!.y - b!.y)).toBeLessThan(2);
  await query.tap(); await page.getByRole('checkbox', { name: '全选缺报', exact: true }).tap();
  await expect(page.getByRole('button', { name: '生成所选草稿（2）', exact: true })).toBeEnabled();
 });
});

test('重新查询及修改日期清空缺报勾选', async ({ page }) => {
 await mockAPI(page); await mockReports(page); await mockAI(page);
 await page.goto('#/reports/week'); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 const query = page.getByRole('button', { name: '查询缺报', exact: true });
 await expect(query).toBeEnabled(); await query.click();
 await page.getByRole('checkbox', { name: '全选缺报', exact: true }).check();
 await expect(page.getByRole('button', { name: '生成所选草稿（2）', exact: true })).toBeEnabled();
 await query.click(); await expect(page.getByRole('checkbox', { name: '全选缺报', exact: true })).not.toBeChecked();
 await expect(page.getByRole('button', { name: /生成所选草稿/ })).toBeDisabled();
 await page.getByRole('checkbox', { name: '全选缺报', exact: true }).check();
 await page.getByLabel('报告截止日期').fill('2026-10-28');
 await expect(page.getByRole('checkbox')).toHaveCount(0);
 await query.click(); await expect(page.getByRole('checkbox', { name: '全选缺报', exact: true })).not.toBeChecked();
 await expect(page.getByRole('button', { name: /生成所选草稿/ })).toBeDisabled();
});

for (const [kind, width] of [['week', 390], ['month', 1440]] as const) test(`AI ${kind} ${width}px 批量补全仅生成草稿，恢复后逐份核对`, async ({ page }, info) => {
 await page.setViewportSize({ width, height: 900 }); await mockAPI(page); const reports = await mockReports(page); const ai = await mockAI(page);
 const range = kind === 'month' ? ['2026-06-01', '2026-09-30'] : ['2026-09-01', '2026-09-28'];
 const first = kind === 'month' ? '2026-07-01' : '2026-09-08'; const second = kind === 'month' ? '2026-09-01' : '2026-09-22';
 await page.goto(`#/reports/${kind}`); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await page.getByLabel('报告起始日期').fill(range[0]); await page.getByLabel('报告截止日期').fill(range[1]);
 await page.getByRole('button', { name: '查询缺报', exact: true }).click(); await expect(page.getByText('待补全 2 段', { exact: true })).toBeVisible();
 await page.getByRole('checkbox', { name: '全选缺报', exact: true }).check();
 await enterMaterial(page); await page.getByRole('button', { name: '生成所选草稿（2）', exact: true }).click();
 await expect(page.getByText('草稿已生成，请逐份核对、补充真实细节后再提交。')).toBeVisible();
 expect(ai.generations.map(g => g.start)).toEqual([first, second]); expect(reports.writes).toEqual([]);
 await page.screenshot({ path: info.outputPath(`ai-batch-${width}.png`), fullPage: true });
 expect(await page.getByRole('dialog').evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
 expect(ai.generations.every(input => !('api_key' in input))).toBe(true);
 await page.getByRole('button', { name: '关闭', exact: true }).click(); await page.reload();
 await page.getByRole('button', { name: 'AI补全', exact: true }).click(); await expect(page.getByText('AI 草稿 · 2 份')).toBeVisible();
 await page.getByRole('button', { name: `${second} · 待核对`, exact: true }).click();
 await page.getByRole('button', { name: '载入编辑器核对', exact: true }).click(); expect(reports.writes).toEqual([]);
 await page.getByRole('dialog', { name: '载入 AI 草稿' }).getByRole('button', { name: '确认载入', exact: true }).click();
 await expect(page.getByRole('dialog')).toHaveCount(0);
 await expect(page.getByLabel('开始日期', { exact: true })).toHaveValue(second);
 await expect(page.getByLabel('工作内容', { exact: false })).toHaveValue(`${second} 整理项目素材，完成剪辑练习。`);
 expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!).map((d: { start: string }) => d.start), reportDraftKey(kind) + ':ai-drafts-v1')).toEqual([first]);
 expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!).content[0].content, reportDraftKey(kind))).toBe(`${second} 整理项目素材，完成剪辑练习。`);
 await page.getByRole('button', { name: '核对并提交', exact: true }).click();
 await expect(page.getByRole('dialog', { name: '确认提交报告' })).toBeVisible(); expect(reports.writes).toEqual([]);
 await page.getByRole('button', { name: '取消', exact: true }).click();
 await page.getByRole('button', { name: 'AI 起草', exact: true }).click();
 await expect(page.getByText('AI 草稿 · 1 份', { exact: true })).toBeVisible();
 await expect(page.getByRole('button', { name: `${second} · 待核对`, exact: true })).toHaveCount(0);
 await page.getByRole('button', { name: '关闭', exact: true }).click();
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

for (const kind of ['week', 'month'] as const) test(`${kind}重新生成先清空旧稿，失败和刷新也不恢复旧正文`, async ({ page }) => {
 await mockAPI(page); const reports = await mockReports(page); await mockAI(page);
 const key = reportDraftKey(kind) + ':ai-drafts-v1';
 const otherKey = reportDraftKey(kind === 'week' ? 'month' : 'week') + ':ai-drafts-v1';
 await page.addInitScript(({ key, otherKey }) => {
  if (!sessionStorage.getItem('ai-retry-fixture-seeded')) {
   sessionStorage.setItem('ai-retry-fixture-seeded', 'true');
  localStorage.setItem(key, JSON.stringify([
   { start: '2026-09-20', end: '2026-09-27', status: 'ready', fields: [{ title: '工作内容', sort: 1, require: '1', content: '将被清空的旧正文' }] },
   { start: '2026-09-27', end: '2026-10-04', status: 'ready', fields: [{ title: '工作内容', sort: 1, require: '1', content: '同批另一份保留正文' }] },
  ]));
  localStorage.setItem(otherKey, '另一报告类型的草稿');
  }
  const original = window.fetch.bind(window);
  window.fetch = async (input, options) => {
   if (!String(input).endsWith('/api/reports/ai/generate')) return original(input, options);
   const encoder = new TextEncoder();
   const field = { title: '工作内容', sort: 1, require: '1', content: '' };
   return new Response(new ReadableStream({ start(controller) {
    const send = (event: unknown) => controller.enqueue(encoder.encode(`data: ${JSON.stringify(event)}\n\n`));
    send({ type: 'field', index: 0, field });
    const release = (event: Event) => {
     if ((event as CustomEvent).detail === 'fail') send({ type: 'error', error: '测试首段生成失败' });
     else { field.content = '全新生成的工作正文'; send({ type: 'delta', index: 0, delta: field.content }); send({ type: 'done', fields: [field] }); }
     controller.close();
    };
    window.addEventListener('release-ai-retry', release, { once: true });
    options?.signal?.addEventListener('abort', () => { window.removeEventListener('release-ai-retry', release); controller.error(new DOMException('Aborted', 'AbortError')); }, { once: true });
   } }), { headers: { 'Content-Type': 'text/event-stream' } });
  };
 }, { key, otherKey });
 await page.goto(`#/reports/${kind}`); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await expect(page.getByText('将被清空的旧正文', { exact: true })).toBeVisible();
 await page.getByRole('button', { name: '重新生成此份', exact: true }).click();
 await expect(page.getByLabel('工作素材', { exact: true })).toBeVisible();
 await expect(page.getByText('将被清空的旧正文', { exact: true })).toBeVisible();
 await enterMaterial(page);
 for (const outcome of ['fail', 'success']) {
  await page.getByRole('button', { name: '重新生成此份', exact: true }).click();
  await expect(page.getByRole('button', { name: '停止生成', exact: true })).toBeEnabled();
  await expect(page.getByText('将被清空的旧正文', { exact: true })).toHaveCount(0);
  const stored = await page.evaluate(key => JSON.parse(localStorage.getItem(key)!), key);
  expect(stored[0].status).toBe('generating'); expect(stored[0].fields.every((f: { content: string }) => !f.content)).toBe(true);
  expect(stored[1].fields[0].content).toBe('同批另一份保留正文');
  await page.evaluate(outcome => window.dispatchEvent(new CustomEvent('release-ai-retry', { detail: outcome })), outcome);
  await expect(page.getByRole('button', { name: '停止生成', exact: true })).toHaveCount(0);
  if (outcome === 'fail') {
   await expect(page.getByText('测试首段生成失败', { exact: true })).toBeVisible();
   expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!)[0].status, key)).toBe('partial');
  } else await expect(page.getByText('全新生成的工作正文', { exact: true })).toBeVisible();
 }
 await page.getByRole('button', { name: '关闭', exact: true }).click();
 await page.reload();
 await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await expect(page.getByText('全新生成的工作正文', { exact: true })).toBeVisible();
 await expect(page.getByText('将被清空的旧正文', { exact: true })).toHaveCount(0);
 expect(await page.evaluate(key => localStorage.getItem(key), otherKey)).toBe('另一报告类型的草稿');
 expect(reports.writes).toEqual([]);
});

for (const failure of ['template', 'storage']) test(`载入${failure}失败时保留AI草稿`, async ({ page }) => {
 await mockAPI(page); const reports = await mockReports(page); await mockAI(page);
 await page.goto('#/reports/week'); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await page.getByRole('button', { name: '查询缺报', exact: true }).click();
 await page.getByRole('checkbox', { name: '补全 2026-09-08 至 2026-09-14', exact: true }).check();
 await enterMaterial(page); await page.getByRole('button', { name: '生成所选草稿（1）', exact: true }).click();
 await expect(page.getByText('草稿已生成，请逐份核对、补充真实细节后再提交。')).toBeVisible();
 if (failure === 'template') await page.route('**/api/reports/template?**', route => route.fulfill({ status: 503, json: { error: '测试模板读取失败' } }));
 else await page.evaluate(key => {
  const original = Storage.prototype.setItem;
  Storage.prototype.setItem = function (name, value) { if (name === key) throw new DOMException('quota', 'QuotaExceededError'); original.call(this, name, value); };
 }, reportDraftKey());
 await page.getByRole('button', { name: '载入编辑器核对', exact: true }).click();
 await page.getByRole('dialog', { name: '载入 AI 草稿' }).getByRole('button', { name: '确认载入', exact: true }).click();
 await expect(page.getByRole('dialog', { name: 'AI补全 · 实习周报' })).toBeVisible();
 await expect(page.getByText(failure === 'template' ? '测试模板读取失败' : '编辑器草稿未能保存', { exact: false }).last()).toBeVisible();
 expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!)[0].fields[0].content, reportDraftKey() + ':ai-drafts-v1')).toBe('2026-09-08 整理项目素材，完成剪辑练习。');
 expect(reports.writes).toEqual([]);
});

test('AI 错误保留部分正文，关闭不会替换当前编辑草稿', async ({ page }) => {
 await mockAPI(page); const reports = await mockReports(page); const ai = await mockAI(page); ai.fail = true;
 await page.goto('#/reports/week'); await page.getByRole('button', { name: '新建报告', exact: true }).click();
 await page.getByLabel('开始日期', { exact: true }).fill('2026-10-01'); await page.getByLabel('结束日期', { exact: true }).fill('2026-10-07'); await page.getByLabel('工作内容', { exact: false }).fill('用户原有正文');
 await page.getByRole('button', { name: 'AI 起草', exact: true }).click(); await enterMaterial(page);
 await page.getByRole('button', { name: '生成草稿', exact: true }).click(); await expect(page.getByText('模拟模型连接中断', { exact: true })).toBeVisible();
 await expect(page.getByText('2026-10-01 整理项目素材，完成剪辑练习。', { exact: true })).toBeVisible();
 await page.getByRole('button', { name: '关闭', exact: true }).click(); await expect(page.getByLabel('工作内容', { exact: false })).toHaveValue('用户原有正文');
 expect(reports.writes).toEqual([]); expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!).content[0].content, reportDraftKey())).toBe('用户原有正文');
});

for (const kind of ['week','month'] as const) test(`根据历史${kind}自动填日期，不要求用户配置模型`,async ({ page }) => {
 await mockAPI(page); const reports=await mockReports(page); const ai=await mockAI(page);
 await page.goto(`#/reports/${kind}`); await page.getByRole('button',{name:'AI补全',exact:true}).click();
 await expect(page.getByLabel('报告起始日期')).toHaveValue(kind==='month'?'2026-07-01':'2026-09-08');
 await expect(page.getByLabel('报告截止日期')).toHaveValue('2026-10-29');
 await expect(page.getByText('已根据已有报告补齐日期，优先从历史缺口开始。')).toHaveCount(0);
 await expect(page.getByText(/AI 连接配置/)).toHaveCount(0); expect(ai.dateCalls).toBeGreaterThan(0); expect(reports.writes).toEqual([]);
});

for (const kind of ['week', 'month'] as const) test(`点击智能填写同时恢复${kind}起始和截止日期`, async ({ page }) => {
 await mockAPI(page); const reports = await mockReports(page); await mockAI(page);
 await page.goto(`#/reports/${kind}`); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 const start = kind === 'week' ? '2026-09-08' : '2026-07-01';
 const end = '2026-10-29';
 await expect(page.getByLabel('报告截止日期')).toHaveValue(end);
 for (const clear of [true, false]) {
  await page.getByLabel('报告起始日期').fill(clear ? '' : '2026-10-01');
  await page.getByLabel('报告截止日期').fill(clear ? '' : '2026-10-07');
  await page.getByRole('button', { name: '智能填写日期', exact: true }).click();
  await expect(page.getByLabel('报告起始日期')).toHaveValue(start);
  await expect(page.getByLabel('报告截止日期')).toHaveValue(end);
 }
 expect(reports.writes).toEqual([]);
});

test('智能日期请求延迟时不会覆盖用户已修改的日期',async ({ page }) => {
 await mockAPI(page); await mockReports(page); const ai=await mockAI(page); ai.dateDelay=600;
 await page.goto('#/reports/week'); await page.getByRole('button',{name:'AI补全',exact:true}).click();
 await expect.poll(()=>ai.dateCalls).toBeGreaterThan(0);
 await page.getByLabel('报告起始日期').fill('2026-10-01'); await page.getByLabel('报告截止日期').fill('2026-10-07');
 await page.getByLabel('工作素材',{exact:true}).fill('用户在等待日期建议时填写的工作素材。');
 await expect(page.getByRole('button',{name:'智能填写日期',exact:true})).toBeEnabled();
 await page.getByRole('button',{name:'查询缺报',exact:true}).click();
 await expect(page.getByText('待补全 2 段',{exact:true})).toBeVisible();
 await expect(page.getByLabel('报告起始日期')).toHaveValue('2026-10-01'); await expect(page.getByLabel('报告截止日期')).toHaveValue('2026-10-07');
});

test('重新智能填日期期间暂停缺报查询，避免旧范围结果混入',async ({ page }) => {
 await mockAPI(page); await mockReports(page); const ai=await mockAI(page); ai.dateDelay=900;
 await page.goto('#/reports/week'); await page.getByLabel('查询开始日期').fill('2026-10-01'); await page.getByLabel('查询结束日期').fill('2026-10-07');
 await page.getByRole('button',{name:'AI补全',exact:true}).click();
 await page.getByRole('button',{name:'智能填写日期',exact:true}).click();
 await expect(page.getByRole('button',{name:'查询缺报',exact:true})).toBeDisabled();
 await expect(page.getByLabel('报告起始日期')).toHaveValue('2026-09-08');
 await expect(page.getByRole('button',{name:'查询缺报',exact:true})).toBeEnabled();
 await page.getByRole('button',{name:'查询缺报',exact:true}).click();
 await expect(page.getByText('待补全 2 段',{exact:true})).toBeVisible();
 expect(ai.missingRanges).toEqual(['2026-09-08:2026-10-29']);
});

for (const kind of ['week', 'month'] as const) test(`${kind}长正文滚动时底部可停止生成，保留草稿且不提交`, async ({ page }, info) => {
 await page.setViewportSize({ width: kind === 'week' ? 1023 : 390, height: 871 });
 await mockAPI(page); const reports = await mockReports(page); await mockAI(page);
 await page.addInitScript(() => {
  const original = window.fetch.bind(window);
  window.fetch = async (input, options) => {
   if (!String(input).endsWith('/api/reports/ai/generate')) return original(input, options);
   const encoder = new TextEncoder();
   return new Response(new ReadableStream({ start(controller) {
    controller.enqueue(encoder.encode(`data: ${JSON.stringify({ type: 'field', index: 0, field: { title: '工作内容', require: '1', sort: 1, content: '' } })}\n\ndata: ${JSON.stringify({ type: 'delta', index: 0, delta: '正在逐步输出的正文。\n'.repeat(80) })}\n\n`));
    options?.signal?.addEventListener('abort', () => controller.error(new DOMException('Aborted', 'AbortError')), { once: true });
   } }), { headers: { 'Content-Type': 'text/event-stream' } });
  };
 });
 await page.goto(`#/reports/${kind}`); await page.getByRole('button', { name: '新建报告', exact: true }).click();
 await page.getByLabel('开始日期', { exact: true }).fill('2026-10-01'); await page.getByLabel('结束日期', { exact: true }).fill('2026-10-07');
 await page.getByRole('button', { name: 'AI 起草', exact: true }).click(); await enterMaterial(page); await page.getByRole('button', { name: '生成草稿', exact: true }).click();
 const dialog = page.getByRole('dialog');
 const stop = dialog.locator('.MuiDialogActions-root').getByRole('button', { name: '停止生成', exact: true });
 await expect(stop).toBeEnabled();
 const body = dialog.locator('.MuiDialogContent-root');
 for (const bottom of [true, false]) {
  await body.evaluate((el, bottom) => { el.scrollTop = bottom ? el.scrollHeight : 0; }, bottom);
  await expect(stop).toBeInViewport();
  expect(await stop.evaluate(el => { const r = el.getBoundingClientRect(); return el.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)); })).toBe(true);
 }
 await page.screenshot({ path: info.outputPath(`ai-stop-${kind}.png`), fullPage: true });
 await stop.click(); await expect(page.getByText('已停止生成，已保留生成内容。', { exact: true })).toBeVisible();
 await expect(dialog).not.toHaveClass(/is-generating/);
 await expect(page.getByRole('button', { name: '关闭', exact: true })).toBeEnabled();
 const stored = await page.evaluate(key => JSON.parse(localStorage.getItem(key)!), reportDraftKey(kind) + ':ai-drafts-v1');
 expect(stored[0].status).toBe('partial'); expect(stored[0].fields[0].content).toBe('正在逐步输出的正文。\n'.repeat(80));
 await page.getByRole('button', { name: '关闭', exact: true }).click(); await page.reload();
 await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await expect(page.getByText(/正在逐步输出的正文/)).toBeVisible();
 await expect(page.getByRole('button', { name: '停止生成', exact: true })).toHaveCount(0);
 expect(reports.writes).toEqual([]);
});

for (const kind of ['week', 'month'] as const) test(`${kind}等待首段时停止批量生成，取消请求与队列且不再显示等待`, async ({ page }) => {
 await mockAPI(page); const reports = await mockReports(page); await mockAI(page);
 await page.addInitScript(() => {
  const original = window.fetch.bind(window);
  window.fetch = async (input, options) => {
   if (!String(input).endsWith('/api/reports/ai/generate')) return original(input, options);
   sessionStorage.setItem('ai-stop-calls', String(Number(sessionStorage.getItem('ai-stop-calls') || 0) + 1));
   return new Response(new ReadableStream({ start(controller) {
    controller.enqueue(new TextEncoder().encode('data: {"type":"field","index":0,"field":{"title":"工作内容","require":"1","sort":1,"content":""}}\n\n'));
    options?.signal?.addEventListener('abort', () => {
     sessionStorage.setItem('ai-stop-aborted', 'true');
     controller.error(new DOMException('Aborted', 'AbortError'));
    }, { once: true });
   } }), { headers: { 'Content-Type': 'text/event-stream' } });
  };
 });
 await page.goto(`#/reports/${kind}`); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await page.getByRole('button', { name: '查询缺报', exact: true }).click();
 await page.getByRole('checkbox', { name: '全选缺报', exact: true }).check(); await enterMaterial(page);
 await page.getByRole('button', { name: '生成所选草稿（2）', exact: true }).click();
 await expect(page.getByText('等待生成…', { exact: true })).toBeVisible();
 await page.getByRole('button', { name: '停止生成', exact: true }).click();
 await expect(page.getByText('已停止生成，已保留生成内容。', { exact: true })).toBeVisible();
 await expect(page.getByText('等待生成…', { exact: true })).toHaveCount(0);
 await expect(page.getByText('尚未生成正文，可重新生成此份。', { exact: true })).toBeVisible();
 expect(await page.evaluate(() => [sessionStorage.getItem('ai-stop-calls'), sessionStorage.getItem('ai-stop-aborted')])).toEqual(['1', 'true']);
 expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!).length, reportDraftKey(kind) + ':ai-drafts-v1')).toBe(1);
 await page.getByRole('button', { name: '关闭', exact: true }).click(); await page.reload();
 await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await expect(page.getByText('尚未生成正文，可重新生成此份。', { exact: true })).toBeVisible();
 await expect(page.getByRole('button', { name: '停止生成', exact: true })).toHaveCount(0);
 expect(reports.writes).toEqual([]);
});

test('最后提交截止日衔接周报，按完整七天跨度生成', async ({ page }, info) => {
 await page.setViewportSize({ width: 1023, height: 871 });
 await mockAPI(page); const reports = await mockReports(page); const ai = await mockAI(page);
 const gaps = [
  { start: '2026-09-20', end: '2026-09-27', state: 'missing' },
  { start: '2026-09-27', end: '2026-10-04', state: 'missing' },
  { start: '2026-10-04', end: '2026-10-11', state: 'missing' },
  { start: '2026-10-11', end: '2026-10-18', state: 'missing' },
  { start: '2026-10-18', end: '2026-10-25', state: 'missing' },
  { start: '2026-10-25', end: '2026-10-29', state: 'missing' },
 ];
 await page.route('**/api/reports/ai/dates?**', route => route.fulfill({ json: { start: '2026-09-20', end: '2026-10-29', reason: '已从最后一次提交周报的截止日衔接，每次向后推七天。截止日期已按北京时间填写为今天，最后一段补全至今天。' } }));
 await page.route('**/api/reports/ai/missing?**', route => route.fulfill({ json: { periods: gaps } }));
 await page.goto('#/reports/week');
 await page.getByLabel('查询开始日期').fill('2026-10-01'); await page.getByLabel('查询结束日期').fill('2026-10-14');
 await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await expect(page.getByLabel('报告起始日期')).toHaveValue('2026-09-20');
 await expect(page.getByLabel('报告截止日期')).toHaveValue('2026-10-29');
 await page.getByRole('button', { name: '查询缺报', exact: true }).click();
 await expect(page.getByText('待补全 6 段', { exact: true })).toBeVisible();
 await expect(page.getByText('2026-09-20 至 2026-09-27', { exact: true })).toBeVisible();
 await page.screenshot({ path: info.outputPath('weekly-continuation.png'), fullPage: true });
 await page.getByRole('checkbox', { name: '全选缺报', exact: true }).check();
 await enterMaterial(page); await page.getByRole('button', { name: '生成所选草稿（6）', exact: true }).click();
 await expect(page.getByText('草稿已生成，请逐份核对、补充真实细节后再提交。')).toBeVisible();
 expect(ai.generations.map(g => [g.start, g.end])).toEqual([
  ['2026-09-20', '2026-09-27'], ['2026-09-27', '2026-10-04'], ['2026-10-04', '2026-10-11'],
  ['2026-10-11', '2026-10-18'], ['2026-10-18', '2026-10-25'], ['2026-10-25', '2026-10-29'],
 ]);
 expect(reports.writes).toEqual([]);
});

test('仅有未结束周报周期时不填无效范围，可手动调整', async ({ page }) => {
 await mockAPI(page); const reports = await mockReports(page); await mockAI(page);
 await page.route('**/api/reports/ai/dates?**', route => route.fulfill({ json: { start: '', end: '', reason: '当前周报周期尚未结束，暂无可补全的已结束周期。可手动调整日期。' } }));
 await page.goto('#/reports/week'); await page.getByRole('button', { name: 'AI补全', exact: true }).click();
 await expect(page.getByText('当前周报周期尚未结束，暂无可补全的已结束周期。可手动调整日期。', { exact: true })).toBeVisible();
 await expect(page.getByLabel('报告起始日期')).toHaveValue(''); await expect(page.getByLabel('报告截止日期')).toHaveValue('');
 await expect(page.getByRole('button', { name: '查询缺报', exact: true })).toBeDisabled();
 await page.getByLabel('报告起始日期').fill('2026-10-19'); await page.getByLabel('报告截止日期').fill('2026-10-25');
 await expect(page.getByRole('button', { name: '查询缺报', exact: true })).toBeEnabled();
 expect(reports.writes).toEqual([]);
});
