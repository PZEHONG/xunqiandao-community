import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { SessionContext } from '../../session/useSession';
import { testSession } from '../../test/session';
import { bumpSessionEpoch, getSessionEpoch } from '../../session/sessionEpoch';
import { AccountPage } from './AccountPage';
const accountSession = (overrides: Parameters<typeof testSession>[0] = {}) => testSession({ epoch: getSessionEpoch(), ...overrides });
it('report draft cleanup is explicit, cancellable, local only, and includes recovery copies', async () => {
  const key = 'xixun-report-draft:1:A:school:week'; const other = 'xixun-report-draft:2:A:school:week';
  for (const item of [key, key + ':recovery', other]) localStorage.setItem(item, 'private');
  render(<SessionContext.Provider value={accountSession()}><AccountPage /></SessionContext.Provider>);
  const clear = screen.getByRole('button', { name: '清除本地报告草稿' }); await userEvent.click(clear);
  expect(screen.getByRole('dialog', { name: '确认清除本地报告草稿' })).toHaveTextContent('已提交到服务器的报告');
  expect(localStorage.getItem(key)).toBe('private'); await userEvent.keyboard('{Escape}');
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument()); expect(clear).toHaveFocus();
  await userEvent.click(clear); await userEvent.click(screen.getByRole('button', { name: '确认清除草稿' }));
  expect(localStorage.getItem(key)).toBeNull(); expect(localStorage.getItem(key + ':recovery')).toBeNull(); expect(localStorage.getItem(other)).toBe('private');
  expect(screen.getByText(/已清除当前用户在此浏览器的报告草稿和恢复副本/)).toBeInTheDocument();
});
it('draft cleanup cannot use an old confirmation after the account epoch expires', async () => {
  const key = 'xixun-report-draft:1:A:school:week'; localStorage.setItem(key, 'keep');
  render(<SessionContext.Provider value={accountSession()}><AccountPage /></SessionContext.Provider>);
  await userEvent.click(screen.getByRole('button', { name: '清除本地报告草稿' })); bumpSessionEpoch();
  await userEvent.click(screen.getByRole('button', { name: '确认清除草稿' })); expect(localStorage.getItem(key)).toBe('keep');
});
it('draft cleanup reports browser storage failures', async () => {
  localStorage.setItem('xixun-report-draft:1:A:school:week', 'keep');
  render(<SessionContext.Provider value={accountSession()}><AccountPage /></SessionContext.Provider>);
  await userEvent.click(screen.getByRole('button', { name: '清除本地报告草稿' }));
  vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new DOMException('Denied'); });
  await userEvent.click(screen.getByRole('button', { name: '确认清除草稿' }));
  expect(screen.getByText(/未能完整清除本地报告草稿/)).toBeInTheDocument();
  expect(localStorage.getItem('xixun-report-draft:1:A:school:week')).toBe('keep');
});
it('账号摘要不改写旧配置，连接状态如实展示，重新连接走会话入口', async () => {
  const reconnect = vi.fn(); localStorage.setItem('xixun-workspace-config:1', '{"account":"A","schoolName":"旧学校"}'); const storage = vi.spyOn(Storage.prototype, 'setItem');
  render(<SessionContext.Provider value={accountSession({ reconnect })}><AccountPage /></SessionContext.Provider>);
  expect(screen.getByText('A', { exact: true })).toBeInTheDocument(); expect(screen.getByText('示例学校')).toBeInTheDocument(); expect(screen.getByText('已关联习讯云')).toBeInTheDocument(); expect(storage).not.toHaveBeenCalled(); await userEvent.click(screen.getByRole('button', { name: '重新连接习讯云' })); expect(reconnect).toHaveBeenCalledOnce();
});
it('清理与退出均需确认，Esc取消无操作且返回焦点', async () => {
  const clearConfig = vi.fn(); const logout = vi.fn(); render(<SessionContext.Provider value={accountSession({ clearConfig, logout })}><AccountPage /></SessionContext.Provider>);
  const clear = screen.getByRole('button', { name: '清除浏览器配置' }); await userEvent.click(clear); expect(screen.getByRole('dialog', { name: '确认清除浏览器配置' })).toHaveTextContent('报告草稿和每日任务'); await userEvent.keyboard('{Escape}'); await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument()); expect(clear).toHaveFocus(); expect(clearConfig).not.toHaveBeenCalled();
  await userEvent.click(clear); await userEvent.click(screen.getByRole('button', { name: '确认清除' })); expect(clearConfig).toHaveBeenCalledOnce(); await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument()); await userEvent.click(screen.getByRole('button', { name: '退出登录' })); expect(logout).not.toHaveBeenCalled(); await userEvent.click(screen.getByRole('button', { name: '确认退出' })); expect(logout).toHaveBeenCalledOnce();
});
it('旧确认在账号epoch变化后失效，匿名/恢复中/无绑定反馈可操作', async () => {
  const logout = vi.fn(); const { rerender } = render(<SessionContext.Provider value={accountSession({ logout })}><AccountPage /></SessionContext.Provider>); await userEvent.click(screen.getByRole('button', { name: '退出登录' }));
  rerender(<SessionContext.Provider value={accountSession({ epoch: 2, user: { id: 2, username: 'B' }, bindings: [] })}><AccountPage /></SessionContext.Provider>); await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument()); expect(logout).not.toHaveBeenCalled(); expect(screen.getByText('尚未关联习讯云')).toBeInTheDocument();
  rerender(<SessionContext.Provider value={accountSession({ status: 'loading', user: null, bindings: [] })}><AccountPage /></SessionContext.Provider>); expect(screen.getByRole('status')).toHaveTextContent('正在恢复会话');
  await act(async () => { rerender(<SessionContext.Provider value={accountSession({ status: 'anonymous', user: null, bindings: [] })}><AccountPage /></SessionContext.Provider>); }); expect(screen.getByRole('button', { name: '登录' })).toBeEnabled();
});
it('epoch已失效但旧界面尚未重绘时不能执行旧退出确认', async () => {
  const logout = vi.fn(); render(<SessionContext.Provider value={accountSession({ epoch: getSessionEpoch(), logout })}><AccountPage /></SessionContext.Provider>); await userEvent.click(screen.getByRole('button', { name: '退出登录' })); bumpSessionEpoch(); await userEvent.click(screen.getByRole('button', { name: '确认退出' })); expect(logout).not.toHaveBeenCalled();
});
