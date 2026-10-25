import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { SessionContext } from '../session/useSession';
import { testSession } from '../test/session';
import { AccountConnection } from './AccountConnection';
const body = { data: { records: [] } };
function mount(overrides = {}) { return render(<SessionContext.Provider value={testSession(overrides)}><AccountConnection /></SessionContext.Provider>); }
it('绑定账号尚未核验时不显示绿色连接，只有上游读取成功后显示已连接', async () => {
  let resolve!: (r: Response) => void;
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(r => { resolve = r; })));
  mount(); expect(screen.getByRole('status')).toHaveTextContent('正在检查连接');
  expect(screen.queryByText('习讯云已连接')).not.toBeInTheDocument();
  await act(async () => resolve(Response.json(body)));
  expect(await screen.findByText('习讯云已连接')).toBeVisible();
});
it('网络故障可手动重试，不误显示登录过期', async () => {
  const fetcher = vi.fn().mockRejectedValueOnce(new TypeError('offline')).mockResolvedValue(Response.json(body)); vi.stubGlobal('fetch', fetcher);
  mount(); expect(await screen.findByText('暂时无法连接')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: 'test' }));
  await userEvent.click(screen.getByRole('button', { name: '重新检查' }));
  expect(await screen.findByText('习讯云已连接')).toBeVisible();
});
it('上游明确失效时提示重新连接并调用已有登录入口', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({code:'UPSTREAM_LOGIN_REQUIRED'}, {status:401})));
  const reconnect=vi.fn(); mount({reconnect});
  expect(await screen.findByText('请重新连接')).toBeVisible();
  await userEvent.click(screen.getByRole('button', {name:'test'}));
  expect(reconnect).toHaveBeenCalledOnce();
});
it('没有习讯云关联时不请求上游，点击进入首次连接', async () => {
  const fetcher=vi.fn(); const openLogin=vi.fn(); vi.stubGlobal('fetch',fetcher); mount({bindings:[],openLogin});
  expect(screen.getByText('未连接习讯云')).toBeVisible();
  await userEvent.click(screen.getByRole('button',{name:'test'}));
  expect(openLogin).toHaveBeenCalledWith('official'); expect(fetcher).not.toHaveBeenCalled();
});
it('其他页面报告当前会话过期后，撤销已连接提示', async () => {
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(Response.json(body))); mount();
  await screen.findByText('习讯云已连接');
  act(()=>window.dispatchEvent(new CustomEvent('xun-api-auth-error',{detail:{code:'UPSTREAM_LOGIN_REQUIRED',epoch:0}})));
  await waitFor(()=>expect(screen.getByRole('status')).toHaveTextContent('请重新连接'));
});

it('切换账号后旧请求晚到不会覆盖新账号连接状态', async () => {
  let finish!: (r: Response) => void;
  vi.stubGlobal('fetch',vi.fn().mockImplementationOnce(()=>new Promise<Response>(r=>{finish=r;})).mockResolvedValue(Response.json({code:'UPSTREAM_LOGIN_REQUIRED'},{status:401})));
  const view=mount();
  view.rerender(<SessionContext.Provider value={testSession({user:{id:2,username:'other'},bindings:[{account:'B',school_id:'2',school_name:'另一个学校'}]})}><AccountConnection /></SessionContext.Provider>);
  expect(await screen.findByText('请重新连接')).toBeVisible();
  await act(async()=>finish(Response.json(body)));
  expect(screen.getByRole('status')).toHaveTextContent('请重新连接');
  expect(screen.queryByText('习讯云已连接')).not.toBeInTheDocument();
});
