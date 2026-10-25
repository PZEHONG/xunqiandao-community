import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { expect, it, vi } from 'vitest';
import { SessionContext } from '../../session/useSession';
import { testSession } from '../../test/session';
import { bumpSessionEpoch, getSessionEpoch } from '../../session/sessionEpoch';
import type { SignConfirmation } from '../sign/types';
import { useAutoSign } from './useAutoSign';
export const task = { id: 1, account: 'A', school_id: '1', name: '每日上班', time: '09:00', enabled: true, revision: 3, remark: '0', remark_label: '上班', comment: '原说明', address: '北京市示例路1号', address_name: '园区', latitude: '39.9', longitude: '116.4', next_at: '2026-10-18T09:00:00+08:00' };
const run = { id: 8, task_id: 1, account: 'A', name: '每日上班', revision: 2, date: '2026-10-17', scheduled_time: '09:00', status: 'unknown', message: '请先核对记录' };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
function snapshot(overrides: Partial<SignConfirmation> = {}): SignConfirmation { return Object.freeze({ owner: 'fixture', epoch: getSessionEpoch(), account: 'A', school: '示例学校', version: 1, locationVersion: 1, location: Object.freeze({ address: task.address, address_name: task.address_name, latitude: task.latitude, longitude: task.longitude, location_confirmed: true as const, confirmation_token: 'fresh-token' }), remark: '0', remarkLabel: '上班', comment: '新说明', photos: [], ...overrides }); }
async function setup(options: { write?: (path: string, body: Record<string, unknown>) => Promise<Response>; read?: () => Promise<Response> } = {}) {
  let session = testSession({ epoch: getSessionEpoch() }); let tasks = [{ ...task }];
  const writes: { path: string; method: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input, init = {}) => {
    const path = String(input); const method = init.method || 'GET';
    if (method === 'GET') {
      if (path.includes('/runs')) return json({ runs: [run] });
      return options.read ? options.read() : json({ tasks, school_id: '1' });
    }
    const body = JSON.parse(String(init.body)); writes.push({ path, method, body });
    if (options.write) return options.write(path, body);
    if (method === 'DELETE') { tasks = []; return json({ message: '任务已删除，执行记录仍保留' }); }
    const saved = { ...task, ...body, id: 1, revision: Number(body.revision || 0) + 1 };
    tasks = [saved]; return json({ task: saved });
  }));
  const wrapper = ({ children }: { children: ReactNode }) => <SessionContext.Provider value={session}>{children}</SessionContext.Provider>;
  const hook = renderHook(() => useAutoSign(), { wrapper }); await waitFor(() => expect(hook.result.current.tasks).toHaveLength(1));
  return { ...hook, writes, switchAccount() { session = testSession({ epoch: bumpSessionEpoch(), bindings: [{ account: 'B', school_id: '2', school_name: '学校B' }] }); hook.rerender(); } };
}
function create(h: Awaited<ReturnType<typeof setup>>, value = snapshot()) { act(() => h.result.current.create(value)); return h.result.current.confirmation!; }
it('创建先核对，取消不写入；确认后只保存任务、不立即签到、不携带照片', async () => {
  const h = await setup(); create(h); expect(h.writes).toEqual([]); act(() => h.result.current.cancelConfirmation()); expect(h.writes).toEqual([]);
  const confirmation = create(h, snapshot({ photos: [new File(['x'], 'photo.png', { type: 'image/png' })] }));
  expect(Object.isFrozen(confirmation)).toBe(true); await act(() => h.result.current.submitConfirmed(confirmation));
  expect(h.writes).toHaveLength(1); expect(h.writes[0]).toMatchObject({ path: '/api/auto-sign/tasks', method: 'POST', body: { account: 'A', school_id: '1', name: '每日签到', time: '09:00', enabled: true, confirmed: true, location_confirmed: true, confirmation_token: 'fresh-token', remark_label: '上班' } });
  expect(h.writes[0].body).not.toHaveProperty('photos'); expect(h.result.current.message).toMatch(/不会立即签到/);
});
it('编辑启用任务必须先成功暂停，使用返回revision；取消编辑保持暂停', async () => {
  let finish!: (response: Response) => void; const h = await setup({ write: () => new Promise(resolve => { finish = resolve; }) }); let work!: Promise<unknown>;
  act(() => { work = h.result.current.edit(h.result.current.tasks[0]); }); expect(h.result.current.editing).toBeNull(); expect(h.result.current.busy).toBe(true);
  expect(h.writes[0]).toMatchObject({ path: '/api/auto-sign/tasks/1/state', body: { account: 'A', revision: 3, enabled: false, confirmed: false } });
  await act(async () => { finish(json({ task: { ...task, enabled: false, revision: 4 } })); await work; });
  expect(h.result.current.editing?.revision).toBe(4); act(() => h.result.current.cancelEdit()); expect(h.result.current.editing).toBeNull(); expect(h.result.current.tasks[0].enabled).toBe(false); expect(h.writes).toHaveLength(1);
});
it('暂停失败不进入编辑器', async () => { const h = await setup({ write: async () => json({ error: '暂停失败' }, 400) }); await act(() => h.result.current.edit(h.result.current.tasks[0])); expect(h.result.current.editing).toBeNull(); expect(h.result.current.tasks[0].enabled).toBe(true); });
it('已暂停任务直接编辑；不改地点可用原地址与空token，改地点必须新确认', async () => {
  const h = await setup(); await act(() => h.result.current.edit({ ...h.result.current.tasks[0], enabled: false })); expect(h.writes).toEqual([]);
  expect(h.result.current.savedPlace?.confirmation_token).toBe('');
  const old = snapshot({ location: h.result.current.savedPlace! }); const c = create(h, old); await act(() => h.result.current.submitConfirmed(c));
  expect(h.writes[0]).toMatchObject({ method: 'PUT', path: '/api/auto-sign/tasks/1', body: { id: 1, revision: 3, confirmation_token: '' } });
  await act(() => h.result.current.edit({ ...h.result.current.tasks[0], enabled: false })); act(() => h.result.current.changePlace()); expect(h.result.current.savedPlace).toBeNull();
  expect(create(h, snapshot({ location: { ...old.location, confirmation_token: '' } })) ).toBeNull();
  const fresh = create(h); await act(() => h.result.current.submitConfirmed(fresh)); expect(h.writes.at(-1)?.body.confirmation_token).toBe('fresh-token');
});
it('重新启用先核对，传当前revision和confirmed；取消无写入', async () => {
  const h = await setup(); const paused = { ...h.result.current.tasks[0], enabled: false }; act(() => h.result.current.resume(paused)); expect(h.writes).toEqual([]);
  act(() => h.result.current.cancelConfirmation()); act(() => h.result.current.resume(paused)); const c = h.result.current.confirmation!;
  await act(() => h.result.current.submitConfirmed(c)); expect(h.writes[0]).toMatchObject({ body: { enabled: true, confirmed: true, revision: 3 } });
});
it('删除只删任务、保留执行历史', async () => { const h = await setup(); act(() => h.result.current.remove(h.result.current.tasks[0])); await act(() => h.result.current.submitConfirmed(h.result.current.confirmation!)); expect(h.writes[0].method).toBe('DELETE'); expect(h.result.current.tasks).toEqual([]); expect(h.result.current.runs[0].id).toBe(8); });
it('409阻止旧revision重复提交，显式刷新后重新编辑', async () => {
  const h = await setup({ write: async () => json({ error: '版本冲突' }, 409) }); const c = create(h); await act(() => h.result.current.submitConfirmed(c)); expect(h.result.current.error).toMatch(/刷新/); expect(h.result.current.blocked).toBe(true);
  create(h); expect(h.result.current.confirmation).toBeNull(); expect(h.writes).toHaveLength(1); await act(() => h.result.current.load()); expect(h.result.current.blocked).toBe(false);
});
it('未知写入结果不自动重发或显示成功，查询前禁止再次写入', async () => {
  const h = await setup({ write: async () => new Response('<html>gateway</html>', { status: 502 }) }); const c = create(h); await act(() => h.result.current.submitConfirmed(c)); expect(h.result.current.error).toMatch(/刷新任务与执行记录/); expect(h.result.current.message).toBe(''); expect(h.result.current.blocked).toBe(true); create(h); expect(h.writes).toHaveLength(1);
});
it('重复确认只写一次；字段/地点变化使确认失效', async () => {
  const h = await setup(); let c = create(h); act(() => h.result.current.setName('改名')); await act(() => h.result.current.submitConfirmed(c)); expect(h.writes).toEqual([]);
  c = create(h); act(() => h.result.current.invalidateConfirmation()); await act(() => h.result.current.submitConfirmed(c)); expect(h.writes).toEqual([]);
  c = create(h); await act(async () => { await Promise.all([h.result.current.submitConfirmed(c), h.result.current.submitConfirmed(c)]); }); expect(h.writes).toHaveLength(1);
});
it('账号切换清除编辑与历史，迟到暂停响应不能开启旧任务编辑', async () => {
  let finish!: (response: Response) => void; const h = await setup({ write: () => new Promise(resolve => { finish = resolve; }) }); let work!: Promise<unknown>;
  act(() => { work = h.result.current.edit(h.result.current.tasks[0]); }); h.switchAccount(); await act(async () => { finish(json({ task: { ...task, enabled: false, revision: 4 } })); await work; });
  expect(h.result.current.editing).toBeNull(); expect(h.result.current.tasks.every(t => t.account === 'B')).toBe(true); expect(h.result.current.runs.every(r => r.account === 'B')).toBe(true);
});
it('刷新失败保留已有列表与历史', async () => {
  let failed = false; const h = await setup({ read: async () => failed ? json({ error: '读取失败' }, 500) : json({ tasks: [task], school_id: '1' }) }); failed = true; await act(() => h.result.current.load()); expect(h.result.current.tasks).toHaveLength(1); expect(h.result.current.runs).toHaveLength(1); expect(h.result.current.error).toMatch(/读取失败/);
});
it('名称60字、有效HH:MM、1000字说明和有效地点规则', async () => {
  const h = await setup(); act(() => h.result.current.setName('😀'.repeat(61))); expect(create(h)).toBeNull(); act(() => { h.result.current.setName('😀'.repeat(60)); h.result.current.setTime('24:00'); }); expect(create(h)).toBeNull(); act(() => h.result.current.setTime('00:00')); expect(create(h, snapshot({ comment: '😀'.repeat(1001) }))).toBeNull(); expect(create(h, snapshot({ comment: '😀'.repeat(1000) }))).not.toBeNull();
});
it.each(['resume', 'remove'] as const)('编辑期间%s成功后关闭对应旧编辑器，取消不再错误报告暂停', async action => {
  const h = await setup(); await act(() => h.result.current.edit(h.result.current.tasks[0])); expect(h.result.current.editing?.revision).toBe(4);
  act(() => h.result.current[action](h.result.current.tasks[0])); await act(() => h.result.current.submitConfirmed(h.result.current.confirmation!));
  expect(h.result.current.editing).toBeNull(); expect(h.result.current.savedPlace).toBeNull();
  expect(h.result.current.tasks.length).toBe(action === 'remove' ? 0 : 1); if (action === 'resume') expect(h.result.current.tasks[0].enabled).toBe(true);
  act(() => h.result.current.cancelEdit()); expect(h.result.current.message).not.toMatch(/原任务保持暂停/); expect(h.result.current.runs[0].id).toBe(8);
});
