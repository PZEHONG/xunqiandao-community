import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { SessionProvider } from '../../session/SessionProvider';
import { useSession } from '../../session/useSession';
import { AuthDialog } from './AuthDialog';
function Entry() { const s = useSession(); return <><button onClick={() => s.openLogin()}>打开登录</button><AuthDialog /></>; }
function setup() {
  const fetcher = vi.fn((path: string) => Promise.resolve(new Response(JSON.stringify(path.endsWith('/status') ? { agreement_version: 'v1' } : path.includes('/schools') ? { schools: [{ school_id: '1', school_name: '示例学校' }] } : { error: '请登录' }), { status: path.endsWith('/self') ? 401 : 200 })));
  vi.stubGlobal('fetch', fetcher); render(<SessionProvider><Entry /></SessionProvider>); return fetcher;
}
it('有学号与首次学校登录且关闭时清除密码', async () => {
  setup(); await userEvent.click(screen.getByText('打开登录'));
  await userEvent.type(screen.getByLabelText(/^密码/), 'temporary-password');
  await userEvent.click(screen.getByRole('button', { name: '关闭登录' }));
  await userEvent.click(screen.getByText('打开登录'));
  expect(screen.getByLabelText(/^密码/)).toHaveValue('');
  await userEvent.click(screen.getByRole('tab', { name: '首次习讯云登录' }));
  expect(screen.getByLabelText('学校名称')).toBeInTheDocument();
});
it('不足两字及输入法组合不查学校，结束后350ms防抖查询', async () => {
  const fetcher = setup(); await userEvent.click(screen.getByText('打开登录'));
  await userEvent.click(screen.getByRole('tab', { name: '首次习讯云登录' }));
  const field = screen.getByLabelText('学校名称');
  fireEvent.change(field, { target: { value: '示' } });
  await new Promise(resolve => setTimeout(resolve, 400)); expect(fetcher.mock.calls.filter(([path]) => path.includes('/schools'))).toHaveLength(0);
  fireEvent.compositionStart(field); fireEvent.change(field, { target: { value: '示例' } });
  await new Promise(resolve => setTimeout(resolve, 400)); expect(fetcher.mock.calls.filter(([path]) => path.includes('/schools'))).toHaveLength(0);
  fireEvent.compositionEnd(field); expect(fetcher.mock.calls.filter(([path]) => path.includes('/schools'))).toHaveLength(0);
  expect(await screen.findByRole('option', { name: '示例学校' })).toBeInTheDocument();
});
it('未确认协议阻止提交，旧版协议记忆只接受当前版本', async () => {
  localStorage.setItem('xun-agreement:2026001', 'old'); const fetcher = setup();
  await userEvent.click(screen.getByText('打开登录'));
  await userEvent.type(screen.getByLabelText(/^学号/), '2026001'); await userEvent.type(screen.getByLabelText(/^密码/), 'test');
  await waitFor(() => expect(screen.getByRole('button', { name: '登录' })).toBeDisabled());
  expect(fetcher.mock.calls.filter(([path]) => path.endsWith('/login'))).toHaveLength(0);
});
it('较旧学校搜索晚返回不能覆盖新候选', async () => {
  let finishOld!: (response: Response) => void;
  const fetcher = vi.fn((path: string) => {
    if (path.includes('/schools')) {
      if (decodeURIComponent(path).includes('旧校')) return new Promise(resolve => { finishOld = resolve; });
      return Promise.resolve(new Response(JSON.stringify({ schools: [{ school_id: '2', school_name: '新学校' }] })));
    }
    return Promise.resolve(new Response(JSON.stringify(path.endsWith('/status') ? { agreement_version: 'v1' } : { error: '请登录' }), { status: path.endsWith('/self') ? 401 : 200 }));
  });
  vi.stubGlobal('fetch', fetcher); render(<SessionProvider><Entry /></SessionProvider>);
  await userEvent.click(screen.getByText('打开登录')); await userEvent.click(screen.getByRole('tab', { name: '首次习讯云登录' }));
  fireEvent.change(screen.getByLabelText('学校名称'), { target: { value: '旧校' } }); await waitFor(() => expect(finishOld).toBeDefined());
  fireEvent.change(screen.getByLabelText('学校名称'), { target: { value: '新校' } }); expect(await screen.findByRole('option', { name: '新学校' })).toBeInTheDocument();
  finishOld(new Response(JSON.stringify({ schools: [{ school_id: '1', school_name: '旧学校' }] })));
  await waitFor(() => expect(screen.queryByRole('option', { name: '旧学校' })).not.toBeInTheDocument()); expect(screen.getByRole('option', { name: '新学校' })).toBeInTheDocument();
});
