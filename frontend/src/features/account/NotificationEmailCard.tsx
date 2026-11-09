import { Alert, Box, Button, Stack, TextField, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { post, request } from '../../api/client';
import { MainCard } from '../../components/MainCard';
import { StatusChip } from '../../components/StatusChip';
import type { NotificationEmail } from '../../api/contracts';

type NotificationValue = NotificationEmail & { message?: string };
type CaptchaConfig = { enabled: boolean; ready: boolean; captcha_id: string };
type CaptchaProof = { lot_number: string; captcha_output: string; pass_token: string; gen_time: string };
declare global { interface Window { GeeTestVerification?: { verify: (scene: string, config: CaptchaConfig) => Promise<CaptchaProof | undefined> } } }
let captchaScript: Promise<void> | undefined;
function loadCaptcha(): Promise<void> {
  if (window.GeeTestVerification) return Promise.resolve();
  if (!captchaScript) captchaScript = new Promise<void>((resolve, reject) => {
    const script = document.createElement('script');
    const timer = window.setTimeout(fail, 12000);
    function fail() { window.clearTimeout(timer); script.remove(); captchaScript = undefined; reject(new Error('人机验证暂时无法加载，请检查网络后重试。')); }
    script.src = '/assets/geetest.js';
    script.onload = () => { window.clearTimeout(timer); if (window.GeeTestVerification) resolve(); else fail(); };
    script.onerror = fail;
    document.head.appendChild(script);
  });
  return captchaScript;
}
export function NotificationEmailCard() {
  const [value, setValue] = useState<NotificationEmail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [sent, setSent] = useState(false);
  useEffect(() => {
    const epoch = getSessionEpoch();
    let active = true;
    request<NotificationValue>('/api/auth/notification-email')
      .then(result => { if (active && getSessionEpoch() === epoch) setValue('email' in result ? result : null); })
      .catch(e => { if (active && getSessionEpoch() === epoch) setError(e instanceof Error ? e.message : '无法读取提醒设置。'); })
      .finally(() => { if (active && getSessionEpoch() === epoch) setLoading(false); });
    return () => { active = false; };
  }, []);
  const run = async (action: () => Promise<void>) => {
    if (busy) return;
    const epoch = getSessionEpoch();
    setBusy(true); setError(''); setMessage('');
    try { await action(); if (getSessionEpoch() !== epoch) return; }
    catch (e) { if (getSessionEpoch() === epoch) setError(e instanceof Error ? e.message : '操作失败，请稍后重试。'); }
    finally { if (getSessionEpoch() === epoch) setBusy(false); }
  };
  const send = () => run(async () => {
    const epoch = getSessionEpoch();
    const config = await request<CaptchaConfig>('/api/auth/notification-email/captcha');
    let captcha: CaptchaProof | undefined;
    if (config.enabled) {
      if (!config.ready) throw new Error('人机验证尚未配置，请联系管理员。');
      await loadCaptcha();
      captcha = await window.GeeTestVerification!.verify('notification-email', config);
    }
    if (getSessionEpoch() !== epoch) return;
    const result = await post<{ message?: string }>('/api/auth/notification-email/code', { email: email.trim(), captcha });
    setMessage(result.message || '验证码已发送，请查收邮箱。'); setSent(true);
  });
  const bind = () => run(async () => {
    const result = await post<{ notification_email: NotificationEmail; message?: string }>('/api/auth/notification-email/bind', { email: email.trim(), code: code.trim() });
    setValue(result.notification_email); setMessage(result.message || '签到提醒邮箱已绑定。'); setSent(false); setCode('');
  });
  const toggle = () => run(async () => {
    const result = await post<{ notification_email: NotificationEmail; message?: string }>('/api/auth/notification-email/enabled', { enabled: !value?.enabled });
    setValue(result.notification_email); setMessage(result.message || '提醒设置已更新。');
  });
  const unbind = () => run(async () => {
    const result = await post<{ message?: string }>('/api/auth/notification-email/unbind', {});
    setValue(null); setMessage(result.message || '已解除签到提醒邮箱绑定。');
  });
  return <MainCard title="签到提醒邮箱"><Stack gap={2}>
    <Box sx={{ borderRadius: 2, bgcolor: 'action.hover', px: 2, py: 1.5 }}><Typography variant="body2" color="text.secondary">每日签到最终失败，或提交结果经复核仍无法确认时，发送一封提醒邮件。不会自动重试签到。</Typography></Box>
    {loading ? <Typography color="text.secondary">正在读取提醒设置…</Typography> : !value ? <>
      <Typography variant="body2" color="text.secondary">尚未绑定提醒邮箱</Typography>
      <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5} alignItems={{ sm: 'center' }}>
        <TextField label="接收提醒的邮箱" type="email" size="small" value={email} disabled={busy} onChange={e => { setEmail(e.target.value); setSent(false); }} slotProps={{ htmlInput: { maxLength: 254 } }} sx={{ flex: 1, minWidth: 0 }} />
        <Button variant="contained" disabled={busy || !email.trim()} onClick={() => void send()} sx={{ minHeight: 40, whiteSpace: 'nowrap' }}>{busy ? '处理中…' : '发送验证码'}</Button>
      </Stack>
      {sent && <Stack direction={{ xs: 'column', sm: 'row' }} gap={1.5}><TextField label="6 位验证码" size="small" value={code} disabled={busy} onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))} slotProps={{ htmlInput: { maxLength: 6, inputMode: 'numeric' } }} /><Button variant="contained" disabled={busy || code.length !== 6} onClick={() => void bind()}>绑定邮箱</Button></Stack>}
    </> : <>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap"><StatusChip label={value.enabled ? '提醒已开启' : '提醒已关闭'} tone={value.enabled ? 'success' : 'warning'} /><Typography sx={{ overflowWrap: 'anywhere' }}>{value.email}</Typography></Stack>
      <Stack direction="row" gap={1} flexWrap="wrap"><Button variant="outlined" disabled={busy} onClick={() => void toggle()}>{value.enabled ? '暂停提醒' : '开启提醒'}</Button><Button disabled={busy} onClick={() => { setEmail(value.email); setSent(false); setCode(''); setValue(null); }}>更换邮箱</Button><Button color="error" disabled={busy} onClick={() => void unbind()}>解除绑定</Button></Stack>
      {sent && <Stack gap={1}><TextField label="6 位验证码" value={code} disabled={busy} onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))} slotProps={{ htmlInput: { maxLength: 6 } }} /><Button variant="contained" disabled={busy || code.length !== 6} onClick={() => void bind()}>确认新邮箱</Button></Stack>}
    </>}
    {message && <Alert severity="success">{message}</Alert>}
    {error && <Alert severity="error">{error}</Alert>}
  </Stack></MainCard>;
}
