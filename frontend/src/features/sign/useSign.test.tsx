import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { SessionContext } from '../../session/useSession';
import { testSession } from '../../test/session';
import { bumpSessionEpoch, getSessionEpoch } from '../../session/sessionEpoch';
import { useSign } from './useSign';
import type { SignConfirmation } from './types';
import type { ConfirmedLocation } from '../locations/types';
const confirmed: ConfirmedLocation = Object.freeze({ address: '北京市示例路1号', address_name: '园区', latitude: '39.900000', longitude: '116.400000', location_confirmed: true, confirmation_token: 'fixture-token' });
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
async function setup(options: { upload?: (init: RequestInit) => Promise<Response>; sign?: (init: RequestInit) => Promise<Response> } = {}) {
  const writes: { path: string; init: RequestInit }[] = []; let session = testSession({ epoch: getSessionEpoch() }); let location: { confirmed: ConfirmedLocation | null; pending: boolean } = { confirmed, pending: false };
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input, init = {}) => {
    const path = String(input);
    if (path.startsWith('/api/sign/remarks')) return json({ remarks: [{ key: '0', value: '上班' }, { key: '1', value: '下班' }] });
    writes.push({ path, init });
    if (path === '/api/sign/photos') return options.upload ? options.upload(init) : json({ uri: '/uploads/fixture.png' });
    if (path === '/api/sign') return options.sign ? options.sign(init) : json({ message: '签到成功' });
    throw new Error(`Unexpected request ${path}`);
  }));
  const wrapper = ({ children }: { children: ReactNode }) => <SessionContext.Provider value={session}>{children}</SessionContext.Provider>;
  const hook = renderHook(() => useSign(location), { wrapper }); await waitFor(() => expect(hook.result.current.draft.remark).toBe('0'));
  return { ...hook, writes, changeLocation(value: typeof location) { location = value; hook.rerender(); }, switchAccount() { session = testSession({ epoch: bumpSessionEpoch(), bindings: [{ account: 'B', school_id: '2', school_name: 'B学校' }] }); hook.rerender(); } };
}
function prepare(hook: Awaited<ReturnType<typeof setup>>) { let snapshot: SignConfirmation | null = null; act(() => { snapshot = hook.result.current.prepareConfirmation(); }); return snapshot!; }
const photo = (name = 'fixture.png') => new File(['fixture'], name, { type: 'image/png' });
afterEach(() => vi.useRealTimers());
it('未确认地点或候选待确认不能生成可提交快照', async () => {
  const hook = await setup();
  hook.changeLocation({ confirmed: null, pending: false }); expect(prepare(hook)).toBeNull();
  hook.changeLocation({ confirmed, pending: true }); expect(prepare(hook)).toBeNull(); expect(hook.writes).toEqual([]);
});
it('打开与取消确认都不上传，快照与照片列表不可变', async () => {
  const hook = await setup(); act(() => hook.result.current.setPhotos([photo()])); const snapshot = prepare(hook);
  expect(Object.isFrozen(snapshot)).toBe(true); expect(Object.isFrozen(snapshot.photos)).toBe(true); expect(snapshot.account).toBe('A'); expect(snapshot.remarkLabel).toBe('上班'); expect(hook.writes).toEqual([]);
  act(() => hook.result.current.cancelConfirmation()); await act(() => hook.result.current.submitConfirmed(snapshot)); expect(hook.writes).toEqual([]);
});
it('确认后改变说明、备注、照片或地点，旧快照不可提交', async () => {
  const hook = await setup();
  for (const mutate of [() => hook.result.current.setComment('变化'), () => hook.result.current.setRemark('1'), () => hook.result.current.setPhotos([photo()])]) {
    const snapshot = prepare(hook); act(mutate); await act(() => hook.result.current.submitConfirmed(snapshot)); expect(hook.writes).toEqual([]);
  }
  const snapshot = prepare(hook); hook.changeLocation({ confirmed: null, pending: true }); hook.changeLocation({ confirmed, pending: false });
  await act(() => hook.result.current.submitConfirmed(snapshot)); expect(hook.writes).toEqual([]);
});
it('切换账号清除正文与照片，旧快照不能写入新账号', async () => {
  const hook = await setup(); act(() => { hook.result.current.setComment('旧账号正文'); hook.result.current.setPhotos([photo()]); }); const snapshot = prepare(hook);
  hook.switchAccount(); expect(hook.result.current.draft.comment).toBe(''); expect(hook.result.current.draft.photos).toEqual([]);
  await act(() => hook.result.current.submitConfirmed(snapshot)); expect(hook.writes).toEqual([]);
});
it('连续点击只上传与签到一次，上传未完成前不发送签到', async () => {
  let finish!: (value: Response) => void; const hook = await setup({ upload: () => new Promise(resolve => { finish = resolve; }) });
  act(() => hook.result.current.setPhotos([photo()])); const snapshot = prepare(hook); let work!: Promise<void>;
  act(() => { work = hook.result.current.submitConfirmed(snapshot); void hook.result.current.submitConfirmed(snapshot); });
  expect(hook.result.current.busy).toBe(true); expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos']);
  await act(async () => { finish(json({ uri: '/uploads/fixture.png' })); await work; });
  expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos', '/api/sign']);
  expect(JSON.parse(String(hook.writes[1].init.body))).toEqual({ account: 'A', ...confirmed, remark: '0', comment: '', photos: ['/uploads/fixture.png'] });
  expect((hook.writes[0].init.body as FormData).get('account')).toBe('A');
  expect(hook.result.current.result?.status).toBe('success'); expect(hook.result.current.draft.photos).toEqual([]);
  await act(() => hook.result.current.submitConfirmed(snapshot)); expect(hook.writes).toHaveLength(2);
});
it('账号在上传期间变化，上传晚到不能发签到或显示成功', async () => {
  let finish!: (value: Response) => void; const hook = await setup({ upload: () => new Promise(resolve => { finish = resolve; }) });
  act(() => hook.result.current.setPhotos([photo()])); const snapshot = prepare(hook); let work!: Promise<void>;
  act(() => { work = hook.result.current.submitConfirmed(snapshot); }); hook.switchAccount();
  await act(async () => { finish(json({ uri: '/uploads/old.png' })); await work; });
  expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos']); expect(hook.result.current.result).toBeNull();
});
it('已成功上传的照片在明确失败后重新确认时复用，不自动补发', async () => {
  let attempts = 0; const hook = await setup({ sign: async () => ++attempts === 1 ? json({ error: '模拟签到失败' }, 400) : json({ message: '签到成功' }) });
  act(() => hook.result.current.setPhotos([photo()])); await act(() => hook.result.current.submitConfirmed(prepare(hook)));
  expect(hook.result.current.error).toContain('模拟签到失败'); expect(hook.result.current.draft.photos).toHaveLength(1);
  expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos', '/api/sign']);
  const next = prepare(hook); await act(() => hook.result.current.submitConfirmed(next));
  expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos', '/api/sign', '/api/sign']); expect(hook.result.current.result?.status).toBe('success');
});
it('上传失败时不发签到，保留照片与输入', async () => {
  const hook = await setup({ upload: async () => json({ error: '模拟上传失败' }, 400) }); act(() => { hook.result.current.setPhotos([photo()]); hook.result.current.setComment('说明'); });
  const snapshot = prepare(hook); await act(() => hook.result.current.submitConfirmed(snapshot));
  expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos']); expect(hook.result.current.error).toContain('模拟上传失败'); expect(hook.result.current.draft.comment).toBe('说明'); expect(hook.result.current.draft.photos).toHaveLength(1);
});
it('第二张上传失败后仅在再次明确确认时重传失败照片', async () => {
  let count = 0; const hook = await setup({ upload: async () => ++count === 2 ? json({ error: '第二张失败' }, 400) : json({ uri: count === 1 ? '/uploads/a.png' : '/uploads/b.png' }) });
  act(() => hook.result.current.setPhotos([photo('a.png'), photo('b.png')])); const first = prepare(hook); await act(() => hook.result.current.submitConfirmed(first));
  expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos', '/api/sign/photos']); expect(hook.result.current.error).toContain('第二张失败');
  const second = prepare(hook); await act(() => hook.result.current.submitConfirmed(second));
  expect(hook.writes.map(write => write.path)).toEqual(['/api/sign/photos', '/api/sign/photos', '/api/sign/photos', '/api/sign']);
  expect(JSON.parse(String(hook.writes[3].init.body)).photos).toEqual(['/uploads/a.png', '/uploads/b.png']);
});
it('上传结果无法解析不发签到，说明是照片上传未知且不自动补发', async () => {
  const hook = await setup({ upload: async () => new Response('not-json') }); act(() => hook.result.current.setPhotos([photo()]));
  const snapshot = prepare(hook); await act(() => hook.result.current.submitConfirmed(snapshot));
  expect(hook.writes).toHaveLength(1); expect(hook.result.current.result).toBeNull(); expect(hook.result.current.error).toContain('照片上传结果暂时无法确认，尚未发送签到');
});
it('不在服务返回列表中的备注不能提交', async () => {
  const hook = await setup(); act(() => hook.result.current.setRemark('forged')); expect(prepare(hook)).toBeNull(); expect(hook.writes).toEqual([]);
});
it.each(['non-json', 'invalid-json', 'null-json', 'timeout'])('签到%s结果不确定时只发一次且不显示成功', async mode => {
  const hook = await setup({ sign: async init => {
    if (mode === 'timeout') return new Promise((_resolve, reject) => init.signal!.addEventListener('abort', () => reject(init.signal!.reason), { once: true }));
    return mode === 'non-json' ? new Response('<html>error</html>') : json(mode === 'null-json' ? null : {});
  } });
  if (mode === 'timeout') vi.useFakeTimers(); const snapshot = prepare(hook); let work!: Promise<void>;
  act(() => { work = hook.result.current.submitConfirmed(snapshot); });
  if (mode === 'timeout') await act(() => vi.advanceTimersByTimeAsync(60000)); await act(async () => { await work; });
  expect(hook.writes).toHaveLength(1); expect(hook.result.current.result?.status).toBe('unknown');
  expect(hook.result.current.error).toBe('签到提交结果暂时无法确认，请先刷新签到记录，再决定是否重新提交。');
  await act(() => hook.result.current.submitConfirmed(snapshot)); expect(hook.writes).toHaveLength(1);
});
it('照片数量/MIME/空文件/10MB边界及1000字限制与原版一致', async () => {
  const hook = await setup();
  for (const files of [[photo(), photo(), photo(), photo()], [new File(['x'], 'x.svg', { type: 'image/svg+xml' })], [new File([], 'empty.png', { type: 'image/png' })], [new File([new Uint8Array(10 * 1024 * 1024 + 1)], 'large.jpg', { type: 'image/jpeg' })]]) {
    act(() => hook.result.current.setPhotos(files)); expect(hook.result.current.draft.photos).toEqual([]); expect(hook.result.current.error).not.toBe('');
  }
  const boundary = new File([new Uint8Array(10 * 1024 * 1024)], 'max.webp', { type: 'image/webp' }); act(() => hook.result.current.setPhotos([boundary])); expect(hook.result.current.draft.photos).toHaveLength(1);
  act(() => hook.result.current.setComment('😀'.repeat(1000))); expect(prepare(hook)).not.toBeNull();
  act(() => hook.result.current.setComment('😀'.repeat(1001))); expect(prepare(hook)).toBeNull(); expect(hook.writes).toEqual([]);
});
