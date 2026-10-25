import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { expect, it, vi } from 'vitest';
import { SessionContext } from '../../session/useSession';
import { testSession } from '../../test/session';
import { bumpSessionEpoch, getSessionEpoch } from '../../session/sessionEpoch';
import { useLocation } from './useLocation';

const place = { name: '示例园区', address: '北京市示例路1号', latitude: '39.900000', longitude: '116.400000', confirmation_token: 'fixture-token', coordinate_system: 'GCJ-02', source: 'amap' };
function setup() {
  let session = testSession({ epoch: getSessionEpoch() });
  const wrapper = ({ children }: { children: ReactNode }) => <SessionContext.Provider value={session}>{children}</SessionContext.Provider>;
  return { ...renderHook(useLocation, { wrapper }), switchAccount: () => { session = testSession({ epoch: bumpSessionEpoch(), bindings: [{ account: 'B', school_id: '2', school_name: 'B' }] }); } };
}
const json = (body: unknown) => new Response(JSON.stringify(body));
it('候选不是确认地点；明确确认生成不可变wire快照，新候选阻止提交，取消恢复', () => {
  const { result } = setup();
  act(() => result.current.preview(place));
  expect(result.current.confirmed).toBeNull(); expect(result.current.pending).toBe(true);
  act(() => result.current.confirm());
  const snapshot = result.current.confirmed;
  expect(snapshot).toMatchObject({ address_name: place.name, location_confirmed: true, confirmation_token: place.confirmation_token, latitude: place.latitude });
  expect(Object.isFrozen(snapshot)).toBe(true);
  act(() => result.current.preview({ ...place, address: '另一地址' }));
  expect(result.current.confirmed).toBeNull(); expect(snapshot?.address).toBe(place.address);
  act(() => result.current.cancel()); expect(result.current.confirmed).toBe(snapshot);
});
it('无候选、无令牌或不完整坐标不能确认', () => {
  const { result } = setup(); act(() => result.current.confirm()); expect(result.current.confirmed).toBeNull();
  for (const invalid of [{ ...place, latitude: '' }, { ...place, confirmation_token: '' }, { ...place, longitude: '181' }]) {
    act(() => { result.current.preview(invalid); result.current.confirm(); }); expect(result.current.confirmed).toBeNull();
  }
});
it('账号变化立即清除地点，晚到搜索不能恢复旧账号状态', async () => {
  let finish!: (value: Response) => void;
  vi.stubGlobal('fetch', vi.fn(() => new Promise(resolve => { finish = resolve; })));
  const { result, rerender, switchAccount } = setup();
  act(() => result.current.preview(place)); act(() => result.current.confirm());
  let work!: Promise<void>; act(() => { work = result.current.search('园区', '北京'); });
  switchAccount(); rerender(); expect(result.current.confirmed).toBeNull(); expect(result.current.candidate).toBeNull();
  await act(async () => { finish(json({ locations: [place] })); await work; }); expect(result.current.results).toEqual([]);
});
it('旧搜索不覆盖新搜索，也不提前解除新请求busy', async () => {
  const finishes: ((value: Response) => void)[] = [];
  vi.stubGlobal('fetch', vi.fn(() => new Promise(resolve => finishes.push(resolve))));
  const { result } = setup(); let a!: Promise<void>; let b!: Promise<void>;
  act(() => { a = result.current.search('旧', ''); }); act(() => { b = result.current.search('新', ''); });
  await act(async () => { finishes[0](json({ locations: [place] })); await a; }); expect(result.current.busy).toBe(true);
  await act(async () => { finishes[1](json({ locations: [{ ...place, name: '新结果' }] })); await b; });
  expect(result.current.results[0].name).toBe('新结果'); expect(result.current.busy).toBe(false);
});
it('地图反查期间和失败后均阻止提交；取消后忽略晚到反查', async () => {
  let finish!: (value: Response) => void;
  vi.stubGlobal('fetch', vi.fn(() => new Promise(resolve => { finish = resolve; })));
  const { result } = setup(); act(() => result.current.preview(place)); act(() => result.current.confirm());
  const snapshot = result.current.confirmed; let work!: Promise<void>;
  act(() => { work = result.current.pickPoint([116.5, 39.8]); });
  expect(result.current.pending).toBe(true); expect(result.current.confirmed).toBeNull(); expect(result.current.candidate).toBeNull();
  act(() => result.current.cancel());
  await act(async () => { finish(json({ location: { ...place, address: '晚到地址' } })); await work; });
  expect(result.current.confirmed).toBe(snapshot); expect(result.current.pending).toBe(false);
  act(() => { work = result.current.pickPoint([116.5, 39.8]); });
  await act(async () => { finish(new Response(JSON.stringify({ error: '反查失败' }), { status: 502 })); await work; });
  expect(result.current.confirmed).toBeNull(); expect(result.current.error).toContain('反查失败');
});
it('首次必须搜索选择地点，地图点击不能绕过', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); const { result } = setup();
  await act(() => result.current.pickPoint([116.4, 39.9])); expect(fetch).not.toHaveBeenCalled(); expect(result.current.error).toContain('先搜索');
});
