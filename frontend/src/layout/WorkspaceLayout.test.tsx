import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, it } from 'vitest';
import { WorkspaceLayout } from './WorkspaceLayout';
it('五个入口切换后标题、选中项和主内容焦点同步', async () => {
  render(<MemoryRouter><WorkspaceLayout><p>测试内容</p></WorkspaceLayout></MemoryRouter>);
  for (const name of ['签到记录', '签到', '实习周报', '实习月报', '账号设置']) expect(screen.getAllByRole('link', { name }).length).toBeGreaterThan(0);
  const link = screen.getAllByRole('link', { name: '实习周报' })[0];
  await userEvent.click(link);
  await waitFor(() => expect(document.title).toBe('实习周报 · 讯签到'));
  expect(link).toHaveAttribute('aria-current', 'page');
  expect(screen.getByRole('main')).toHaveFocus();
});
