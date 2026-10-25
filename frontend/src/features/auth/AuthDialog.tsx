import { Feedback } from '../../components/Feedback';
import { useEffect, useRef, useState } from 'react';
import { Box, Button, Checkbox, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, IconButton, InputAdornment, Link, Stack, Tab, Tabs, TextField, Typography } from '@mui/material';
import CloseOutlined from '@mui/icons-material/CloseOutlined';
import { useSession, type LoginPane } from '../../session/useSession';
import { ApiError } from '../../api/errors';
import type { School } from '../../api/contracts';
import { SchoolAutocomplete } from './SchoolAutocomplete';

function LoginForm({ initialPane }: { initialPane: LoginPane }) {
  const session = useSession(); const [pane, setPane] = useState(initialPane); const [account, setAccount] = useState(session.reconnecting ? session.user?.student_number || session.user?.username || '' : '');
  const [password, setPassword] = useState(''); const [showPassword, setShowPassword] = useState(false); const [school, setSchool] = useState<School | null>(null);
  const [consent, setConsent] = useState(false); const [forceConsent, setForceConsent] = useState(false); const [error, setError] = useState(''); const [busy, setBusy] = useState(false);
  const passwordInput = useRef<HTMLInputElement>(null); const [focusFailure, setFocusFailure] = useState(false);
  useEffect(() => { if (!busy && focusFailure) { passwordInput.current?.focus(); setFocusFailure(false); } }, [busy, focusFailure]);
  const version = session.authConfig?.agreement_version;
  let remembered = false; try { remembered = !!version && !!account.trim() && localStorage.getItem(`xun-agreement:${account.trim().toLowerCase()}`) === version; } catch { /* Ask for consent again. */ }
  const needsConsent = forceConsent || !remembered;
  const switchPane = (next: LoginPane) => { setPane(next); setPassword(''); setShowPassword(false); setConsent(false); setSchool(null); setError(''); };
  return <Box component="form" autoComplete="on" onSubmit={async event => {
    event.preventDefault(); if (busy || !version || (needsConsent && !consent)) return;
    if (pane === 'official' && !school) { setError('请先从匹配列表中选择学校。'); return; }
    setBusy(true); setError('');
    try { await session.authenticate(pane, pane === 'official' ? { account: account.trim(), password, school_id: school!.school_id, school_name: school!.school_name, accept_terms: consent } : { identity: account.trim(), password, accept_terms: consent }); }
    catch (failure) { if (failure instanceof ApiError && failure.code === 'CONSENT_REQUIRED') { setForceConsent(true); setConsent(false); } setError(failure instanceof Error ? failure.message : '登录未成功，请稍后重试。'); setFocusFailure(true); }
    finally { setBusy(false); }
  }}>
    <Tabs value={pane} onChange={(_, value: LoginPane) => switchPane(value)} variant="fullWidth" aria-label="登录方式" sx={{ px: 2 }}><Tab value="login" label="学号登录" disabled={busy} /><Tab value="official" label="首次习讯云登录" disabled={busy} /></Tabs>
    <DialogContent><Stack gap={2.5}>
      {session.reconnecting && <Feedback severity="info">请重新登录以恢复习讯云连接。</Feedback>}
      {pane === 'official' && <SchoolAutocomplete value={school} onSelect={setSchool} disabled={busy} />}
      <TextField label="学号" name="username" autoComplete="username" autoFocus value={account} required disabled={busy} inputProps={{ maxLength: 256 }} onChange={event => { setAccount(event.target.value); setConsent(false); setForceConsent(false); }} />
      <TextField label="密码" name="password" inputRef={passwordInput} type={showPassword ? 'text' : 'password'} autoComplete="current-password" value={password} required disabled={busy} inputProps={{ maxLength: 256 }} onChange={event => setPassword(event.target.value)} slotProps={{ input: { endAdornment: <InputAdornment position="end"><Button aria-label={showPassword ? '隐藏密码' : '显示密码'} size="small" disabled={busy} onClick={() => setShowPassword(!showPassword)}>{showPassword ? '隐藏' : '显示'}</Button></InputAdornment> } }} />
      {!version && <Feedback severity="warning">协议版本暂时无法读取，请刷新页面后登录。</Feedback>}
      {needsConsent && <FormControlLabel control={<Checkbox checked={consent} disabled={busy} onChange={event => setConsent(event.target.checked)} />} label={<Typography variant="body2">我已阅读并同意 <Link href="/terms" target="_blank" rel="noreferrer">用户协议</Link> 和 <Link href="/privacy" target="_blank" rel="noreferrer">隐私政策</Link></Typography>} />}
      {error && <Feedback severity="error">{error}</Feedback>}
      {pane === 'official' && <Typography variant="body2" color="text.secondary">通过习讯云验证后，系统会自动创建讯签到账号。</Typography>}
    </Stack></DialogContent>
    <DialogActions sx={{ px: 3, pb: 3, gap: 1, flexWrap: 'wrap' }}><Button disabled={busy} onClick={() => setError('请在习讯云官方 App 中找回密码，再用学号和新密码登录。')}>忘记密码</Button><Button type="submit" variant="contained" disabled={busy || !version || (needsConsent && !consent)}>{busy ? '登录中…' : '登录'}</Button></DialogActions>
  </Box>;
}
export function AuthDialog() {
  const session = useSession(); const [formKey, setFormKey] = useState(0);
  useEffect(() => { if (!session.loginOpen) setFormKey(previous => previous + 1); }, [session.loginOpen]);
  return <Dialog open={session.loginOpen} onClose={session.closeLogin} disableEscapeKeyDown={session.authBusy} slotProps={{ transition: { onExited: () => { if (document.activeElement === document.body) document.getElementById('workspace-main')?.focus(); } } }} aria-labelledby="auth-title"><DialogTitle id="auth-title" sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>登录讯签到<IconButton aria-label="关闭登录" disabled={session.authBusy} onClick={session.closeLogin}><CloseOutlined /></IconButton></DialogTitle>{session.loginOpen && <LoginForm key={`${formKey}:${session.loginKey}`} initialPane={session.loginPane} />}</Dialog>;
}
