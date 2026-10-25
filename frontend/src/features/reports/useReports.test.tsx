import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { expect, it, vi } from 'vitest';
import { SessionContext } from '../../session/useSession';
import { testSession } from '../../test/session';
import { bumpSessionEpoch, getSessionEpoch } from '../../session/sessionEpoch';
import { draftKey } from './draftStorage';
import { useReports } from './useReports';
import type { ReportConfirmation, ReportKind } from './types';
const template = [{ title: '工作内容', require: '1', sort: 1, content: '' }, { title: '实习体会', require: '0', sort: 2, content: '' }];
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
async function setup(options: { kind?: ReportKind; list?: (url: URL) => Promise<Response>; template?: unknown; submit?: (body: Record<string, unknown>) => Promise<Response>; upload?: (init: RequestInit) => Promise<Response> } = {}) {
  let kind = options.kind || 'week'; let session = testSession({ epoch: getSessionEpoch() });
  const reads: string[] = []; const writes: { path: string; body: Record<string, unknown> | FormData }[] = [];
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input, init = {}) => {
    const path = String(input); const url = new URL(path, 'http://localhost');
    if (!init.method || init.method === 'GET') {
      reads.push(path);
      if (url.pathname.endsWith('/template')) return json({ data: options.template ?? template });
      if (url.pathname.endsWith('/detail')) return json({ data: { id: url.searchParams.get('id'), student_name: '学生', content: [{ title: '工作内容', content: '详情正文' }], mark_result: '继续努力', attachment: ['/reports/old.pdf'] } });
      return options.list ? options.list(url) : json({ data: { list: [{ id: '1', teacher_name: '老师' }], count: '31' } });
    }
    const body = init.body instanceof FormData ? init.body : JSON.parse(String(init.body)); writes.push({ path: url.pathname, body });
    if (url.pathname.endsWith('/attachments')) return options.upload ? options.upload(init) : json({ uri: '/reports/new.pdf' });
    return options.submit ? options.submit(body as Record<string, unknown>) : json({ message: '报告提交成功。', data: {} });
  }));
  const wrapper = ({ children }: { children: ReactNode }) => <SessionContext.Provider value={session}>{children}</SessionContext.Provider>;
  const hook = renderHook(() => useReports(kind), { wrapper }); await waitFor(() => expect(hook.result.current.rows).toHaveLength(1));
  return { ...hook, reads, writes, changeKind(value: ReportKind) { kind = value; hook.rerender(); }, switchAccount() { session = testSession({ epoch: bumpSessionEpoch(), bindings: [{ account: 'B', school_id: '2', school_name: '学校B' }] }); hook.rerender(); } };
}
async function open(h: Awaited<ReturnType<typeof setup>>) { await act(() => h.result.current.openEditor()); }
function fill(h: Awaited<ReturnType<typeof setup>>) { act(() => { h.result.current.setStart('2026-10-01'); h.result.current.setEnd('2026-10-07'); h.result.current.setField(0, '本周实际实习内容'); }); }
function prepare(h: Awaited<ReturnType<typeof setup>>) { let c: ReportConfirmation | null = null; act(() => { c = h.result.current.prepareConfirmation(); }); return c!; }
const file = () => new File(['fixture'], '附件.pdf', { type: 'application/pdf' });
it('列表页大小15、分页与筛选使用当前业务类型，失败保留原快照', async () => {
  let fail = false; const h = await setup({ list: async () => fail ? json({ error: '查询失败' }, 502) : json({ data: { list: [{ id: '1' }], count: 31 } }) });
  expect(new URL(h.reads[0], 'http://localhost').searchParams.get('page_size')).toBe('15'); expect(h.result.current.pages).toBe(3);
  fail = true; await act(() => h.result.current.load(2)); expect(h.result.current.rows[0].id).toBe('1'); expect(h.result.current.page).toBe(1); expect(h.result.current.error).toMatch(/查询失败/);
});
it('快速切筛选时迟到响应不能覆盖最新列表', async () => {
  let resolveOld!: (response: Response) => void; const h = await setup({ list: async url => url.searchParams.get('mark_state') === 'approval' ? new Promise(resolve => { resolveOld = resolve; }) : json({ data: { list: [{ id: url.searchParams.get('mark_state') || 'initial' }], count: 1 } }) }); let first!: Promise<void>;
  act(() => { first = h.result.current.load(1, { start: '', end: '', status: 'approval' }); });
  await act(() => h.result.current.load(1, { start: '', end: '', status: 'without_approval' }));
  await act(async () => { resolveOld(json({ data: { list: [{ id: 'old' }], count: 1 } })); await first; }); expect(h.result.current.rows[0].id).toBe('without_approval');
});
it('无模板时阻止空白编辑器，读取错误明确显示', async () => { const h = await setup({ template: [] }); await open(h); expect(h.result.current.editor).toBe(false); expect(h.result.current.error).toMatch(/模板/); });
it('旧草稿按标题恢复，保留未匹配内容并明确附件重选', async () => {
  const key = draftKey({ userID: 1, account: 'A', school: '示例学校', kind: 'week' }); localStorage.setItem(key, JSON.stringify({ start: '2026-10-01', end: '2026-10-07', content: [{ title: '工作内容', sort: 9, content: '旧正文' }, { title: '旧字段', sort: 2, content: '保留我' }], attachments: [{ name: '已上传.pdf', uri: '/reports/old.pdf' }], pendingNames: ['附件.pdf'] }));
  const h = await setup(); await open(h); expect(h.result.current.fields[0].content).toBe('旧正文'); expect(h.result.current.orphaned[0].content).toBe('保留我'); expect(h.result.current.pendingNames).toEqual(['附件.pdf']); expect(h.result.current.attachments[0].uri).toBe('/reports/old.pdf'); expect(prepare(h)).toBeNull();
  act(() => h.result.current.addFiles([file()])); expect(h.result.current.pendingNames).toEqual([]); expect(prepare(h)).not.toBeNull();
});
it('附件上限3个、1B至10MiB，重新打开页面不会伪造File', async () => {
  const h = await setup(); await open(h); fill(h);
  for (const files of [[file(), file(), file(), file()], [new File([], '空')], [new File([new Uint8Array(10 * 1024 * 1024 + 1)], '超大')]]) { act(() => h.result.current.addFiles(files)); expect(h.result.current.attachments).toEqual([]); }
  act(() => h.result.current.addFiles([new File([new Uint8Array(10 * 1024 * 1024)], '边界.bin')])); expect(h.result.current.attachments).toHaveLength(1); const key = h.result.current.key; h.unmount();
  const again = await setup(); await open(again); expect(again.result.current.attachments).toEqual([]); expect(again.result.current.pendingNames).toEqual(['边界.bin']); expect(localStorage.getItem(key)).toContain('边界.bin');
});
it.each(['week', 'month'] as const)('%s仅最终确认后上传并提交，wire类型准确，双击只提交一次', async kind => {
  const h = await setup({ kind }); await open(h); fill(h); act(() => h.result.current.addFiles([file()])); let c = prepare(h); expect(h.writes).toEqual([]); act(() => h.result.current.cancelConfirmation()); await act(() => h.result.current.submitConfirmed(c)); expect(h.writes).toEqual([]);
  c = prepare(h); expect(Object.isFrozen(c)).toBe(true); expect(Object.isFrozen(c.input.content)).toBe(true); await act(async () => { await Promise.all([h.result.current.submitConfirmed(c), h.result.current.submitConfirmed(c)]); });
  expect(h.writes.map(write => write.path)).toEqual(['/api/reports/attachments', '/api/reports']); expect(h.writes[1].body).toMatchObject({ account: 'A', business_type: kind, start_date: '2026-10-01', end_date: '2026-10-07', attachments: ['/reports/new.pdf'] }); expect(h.result.current.editor).toBe(false); expect(h.result.current.message).toMatch(/提交成功/); expect(localStorage.getItem(h.result.current.key)).toBeNull();
});
it('提交明确失败保留正文与已上传URI，再次核对不重复上传', async () => {
  let fail = true; const h = await setup({ submit: async () => fail ? json({ error: '模拟提交失败' }, 400) : json({ message: '报告提交成功。', data: {} }) }); await open(h); fill(h); act(() => h.result.current.addFiles([file()])); await act(() => h.result.current.submitConfirmed(prepare(h)));
  expect(h.result.current.fields[0].content).toContain('实习'); expect(h.result.current.attachments[0].uri).toBe('/reports/new.pdf'); fail = false; await act(() => h.result.current.submitConfirmed(prepare(h))); expect(h.writes.filter(write => write.path.endsWith('/attachments'))).toHaveLength(1); expect(h.writes.filter(write => write.path === '/api/reports')).toHaveLength(2);
});
it('附件上传失败不提交，输入保留；已上传附件可从旧草稿直接复用', async () => {
  const h = await setup({ upload: async () => json({ error: '上传失败' }, 400) }); await open(h); fill(h); act(() => h.result.current.addFiles([file()])); await act(() => h.result.current.submitConfirmed(prepare(h))); expect(h.writes).toHaveLength(1); expect(h.result.current.editor).toBe(true); expect(h.result.current.error).toMatch(/上传失败/);
});
it.each(['HTML', 'JSON502'])('未知提交%s不显示成功或自动重发，草稿保留待核对状态', async response => {
  const h = await setup({ submit: async () => response === 'HTML' ? new Response('<html>gateway</html>') : json({ error: '读取报告响应失败' }, 502) }); await open(h); fill(h); await act(() => h.result.current.submitConfirmed(prepare(h))); expect(h.writes).toHaveLength(1); expect(h.result.current.error).toMatch(/结果暂时无法确认/); expect(h.result.current.message).toBe(''); expect(JSON.parse(localStorage.getItem(h.result.current.key)!).submissionUncertain).toBe(true);
});
it('确认后修改内容或切换类型、账号均不能使用旧快照', async () => {
  const h = await setup(); await open(h); fill(h); const c = prepare(h); act(() => h.result.current.setField(0, '变更正文')); await act(() => h.result.current.submitConfirmed(c)); expect(h.writes).toEqual([]);
  const c2 = prepare(h); h.changeKind('month'); await act(() => h.result.current.submitConfirmed(c2)); expect(h.writes).toEqual([]); expect(h.result.current.editor).toBe(false);
  await open(h); fill(h); const c3 = prepare(h); h.switchAccount(); await act(() => h.result.current.submitConfirmed(c3)); expect(h.writes).toEqual([]); expect(h.result.current.fields).toEqual([]);
});
it('上传期间账号变化，迟到上传不能提交报告或污染新账号', async () => {
  let finish!: (response: Response) => void; const h = await setup({ upload: () => new Promise(resolve => { finish = resolve; }) }); await open(h); fill(h); act(() => h.result.current.addFiles([file()])); const c = prepare(h); let work!: Promise<void>;
  act(() => { work = h.result.current.submitConfirmed(c); }); h.switchAccount(); await act(async () => { finish(json({ uri: '/reports/old.pdf' })); await work; }); expect(h.writes).toHaveLength(1); expect(h.result.current.attachments).toEqual([]); expect(h.result.current.message).toBe('');
});
it('成功保留另一类型和同键更新的草稿', async () => {
  let finish!: (response: Response) => void; const h = await setup({ submit: () => new Promise(resolve => { finish = resolve; }) }); const monthKey = draftKey({ userID: 1, account: 'A', school: '示例学校', kind: 'month' }); localStorage.setItem(monthKey, '{"start":"month"}'); await open(h); fill(h); const c = prepare(h); let work!: Promise<void>;
  act(() => { work = h.result.current.submitConfirmed(c); }); localStorage.setItem(h.result.current.key, '{"start":"newer"}'); await act(async () => { finish(json({ message: '报告提交成功。', data: {} })); await work; }); expect(localStorage.getItem(h.result.current.key)).toBe('{"start":"newer"}'); expect(localStorage.getItem(monthKey)).toBe('{"start":"month"}');
});
it('必填、有效日期及内容长度在上传前校验', async () => {
  const h = await setup(); await open(h); expect(prepare(h)).toBeNull(); fill(h); act(() => h.result.current.setStart('2026-02-30')); expect(prepare(h)).toBeNull(); act(() => { h.result.current.setStart('2026-10-01'); h.result.current.setField(0, '中'.repeat(50001)); }); expect(prepare(h)).toBeNull(); expect(h.writes).toEqual([]);
});
it('仅查看损坏草稿后返回列表不能覆盖原始备份', async () => {
  const key = draftKey({ userID: 1, account: 'A', school: '示例学校', kind: 'week' }); localStorage.setItem(key, '{broken'); const h = await setup(); await open(h); act(() => h.result.current.closeEditor()); expect(localStorage.getItem(key)).toBe('{broken');
});
it('损坏草稿编辑并重新打开后仍可下载持久保留的原文', async () => {
  const key = draftKey({ userID: 1, account: 'A', school: '示例学校', kind: 'week' }); localStorage.setItem(key, '{broken 原有报告正文');
  const h = await setup(); await open(h); fill(h); expect(h.result.current.recoveryRaw).toBe('{broken 原有报告正文');
  act(() => h.result.current.closeEditor()); await open(h);
  expect(h.result.current.recoveryRaw).toBe('{broken 原有报告正文');
  expect(h.result.current.fields[0].content).toContain('实习');
  h.unmount(); const next = await setup(); await open(next);
  expect(next.result.current.recoveryRaw).toBe('{broken 原有报告正文');
  expect(next.result.current.fields[0].content).toContain('实习');
});
