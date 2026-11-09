import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { NotificationEmailCard } from './NotificationEmailCard';
afterEach(() => { vi.restoreAllMocks(); delete window.GeeTestVerification; });

it('完成极验后才发送邮箱验证码，并提交验证凭据', async () => {
  const proof = { lot_number: 'lot', captcha_output: 'output', pass_token: 'pass', gen_time: '123' };
  const verify = vi.fn().mockResolvedValue(proof);
  Object.assign(window, { GeeTestVerification: { verify } });
  const fetchMock = vi.spyOn(window, 'fetch').mockImplementation(async (url) => {
    const path = String(url);
    const body = path.endsWith('/captcha') ? { enabled: true, ready: true, captcha_id: 'public' } : { message: '已发送' };
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
  });
  render(<NotificationEmailCard />);
  await userEvent.type(await screen.findByLabelText('接收提醒的邮箱'), 'notify@example.com');
  await userEvent.click(screen.getByRole('button', { name: '发送验证码' }));
  await waitFor(() => expect(verify).toHaveBeenCalledOnce());
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/auth/notification-email/code', expect.objectContaining({ method: 'POST', body: JSON.stringify({ email: 'notify@example.com', captcha: proof }) })));
  expect(await screen.findByLabelText('6 位验证码')).toBeInTheDocument();
});

it('取消人机验证时不发送邮件', async () => {
  Object.assign(window, { GeeTestVerification: { verify: vi.fn().mockRejectedValue(new Error('已取消人机验证')) } });
  const fetchMock = vi.spyOn(window, 'fetch').mockImplementation(async (url) => new Response(JSON.stringify(String(url).endsWith('/captcha') ? { enabled: true, ready: true, captcha_id: 'public' } : { message: '未绑定' }), { status: 200 }));
  render(<NotificationEmailCard />);
  await userEvent.type(await screen.findByLabelText('接收提醒的邮箱'), 'notify@example.com');
  await userEvent.click(screen.getByRole('button', { name: '发送验证码' }));
  expect(await screen.findByText('已取消人机验证')).toBeInTheDocument();
  expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/code'))).toBe(false);
});
