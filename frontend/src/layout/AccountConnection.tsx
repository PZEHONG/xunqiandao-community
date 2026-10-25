import { useEffect, useId, useState, type MouseEvent } from 'react';
import { Box, Button, CircularProgress, Popover, Stack, Typography } from '@mui/material';
import KeyboardArrowDownRounded from '@mui/icons-material/KeyboardArrowDownRounded';
import { useSession } from '../session/useSession';
import { ApiError, StaleSessionError } from '../api/errors';
import { queryRecords } from '../features/records/api';
import './accountConnection.css';

type Connection = 'checking' | 'connected' | 'expired' | 'unavailable';
const labels = { checking: '正在检查连接', connected: '习讯云已连接', expired: '请重新连接', unavailable: '暂时无法连接' };
export function AccountConnection() {
  const session = useSession();
  const binding = session.bindings[0];
  const owner = JSON.stringify([session.epoch, session.status, session.user?.id, binding?.account, binding?.school_id]);
  return <ConnectionButton key={owner} />;
}
function ConnectionButton() {
  const session = useSession(); const binding = session.bindings[0]; const statusId = useId();
  const [connection, setConnection] = useState<Connection>('checking');
  const [checkedAt, setCheckedAt] = useState<Date | null>(null);
  const [attempt, setAttempt] = useState(0); const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const account = session.user?.student_number || session.user?.username || '登录';
  const boundAccount = binding?.account; const epoch = session.epoch;
  const eligible = session.status === 'authenticated' && !!boundAccount;
  useEffect(() => {
    if (!eligible || !boundAccount) return;
    let stopped = false; let active: AbortController | null = null; let lastStarted = 0; let expired = false;
    const check = async () => {
      if (stopped || active || expired) return;
      lastStarted = Date.now(); const controller = new AbortController(); active = controller;
      try {
        const now = new Date(); const month = now.getFullYear() + '-' + String(now.getMonth() + 1).padStart(2, '0');
        await queryRecords(boundAccount, month, epoch, controller.signal);
        if (!stopped && !controller.signal.aborted && !expired) { setConnection('connected'); setCheckedAt(new Date()); }
      } catch (error) {
        if (stopped || controller.signal.aborted || error instanceof StaleSessionError) return;
        if (error instanceof ApiError && error.code === 'UPSTREAM_LOGIN_REQUIRED') { expired = true; setConnection('expired'); }
        else if (!expired) setConnection('unavailable');
      } finally { if (active === controller) active = null; }
    };
    const authError = (event: Event) => {
      const detail = (event as CustomEvent<{epoch: number; code: string}>).detail;
      if (detail?.epoch !== epoch || detail.code !== 'UPSTREAM_LOGIN_REQUIRED') return;
      expired = true; active?.abort(); setConnection('expired');
    };
    const offline = () => { active?.abort(); active = null; setConnection(previous => previous === 'expired' ? previous : 'unavailable'); };
    const online = () => { void check(); };
    const visible = () => { if (!document.hidden && Date.now() - lastStarted > 60000) void check(); };
    window.addEventListener('xun-api-auth-error', authError); window.addEventListener('offline', offline); window.addEventListener('online', online);
    document.addEventListener('visibilitychange', visible);
    void check(); const timer = setInterval(() => { if (!document.hidden) void check(); }, 5 * 60000);
    return () => { stopped = true; active?.abort(); clearInterval(timer); window.removeEventListener('xun-api-auth-error', authError); window.removeEventListener('offline', offline); window.removeEventListener('online', online); document.removeEventListener('visibilitychange', visible); };
  }, [eligible, boundAccount, epoch, attempt]);
  const tone = session.status === 'loading' ? 'checking' : !session.user || !binding ? 'idle' : connection;
  const label = tone === 'idle' ? '未连接习讯云' : labels[tone];
  const open = (event: MouseEvent<HTMLButtonElement>) => {
    if (!session.user) { session.openLogin(); return; }
    if (!binding) { if (session.configCleared) session.reconnect(); else session.openLogin('official'); return; }
    if (connection === 'expired') { session.reconnect(); return; }
    setAnchor(event.currentTarget);
  };
  const retry = () => { setConnection('checking'); setAttempt(n => n + 1); };
  return <>
    <Button className="connection-account" size="small" color="inherit" aria-label={account} aria-describedby={statusId} aria-haspopup={eligible && connection !== 'expired' ? 'dialog' : undefined} aria-expanded={anchor ? true : undefined} disabled={session.authBusy || session.status === 'loading'} onClick={open}>
      <span className="connection-account-copy"><span className="connection-account-number">{account}</span><span id={statusId} role="status" className="connection-status" data-tone={tone}>{tone === 'checking' ? <CircularProgress size={8} thickness={5} color="inherit" /> : <span className="connection-dot" aria-hidden="true" />}<span className="connection-label">{label}</span><span className="connection-label-mobile" aria-hidden="true">{tone === 'connected' ? '已连接' : tone === 'checking' ? '连接中' : tone === 'expired' ? '需重连' : tone === 'idle' ? '未连接' : '连接异常'}</span></span></span><KeyboardArrowDownRounded className="connection-chevron" />
    </Button>
    <Popover open={!!anchor} anchorEl={anchor} onClose={() => setAnchor(null)} anchorOrigin={{vertical:'bottom',horizontal:'right'}} transformOrigin={{vertical:'top',horizontal:'right'}} slotProps={{paper:{sx:{mt:1,borderRadius:2.5,border:'1px solid',borderColor:'divider',boxShadow:'0 12px 40px rgba(29,41,61,.10)',maxWidth:'calc(100vw - 24px)'}}}}>
      <Box role="dialog" aria-label="习讯云连接信息" sx={{p:2.25,width:280,maxWidth:'100%'}}><Stack spacing={1.25}>
        <Typography fontWeight={650} fontSize={14}>习讯云连接</Typography>
        <Typography fontSize={13} sx={{overflowWrap:'anywhere'}}>{binding?.account || account}</Typography>
        {binding?.school_name && <Typography variant="body2" color="text.secondary">{binding.school_name}</Typography>}
        <Typography variant="caption" color="text.secondary">{connection === 'connected' ? '连接正常，可继续查看签到与实习报告。' : connection === 'checking' ? '正在核验习讯云连接，请稍候。' : '暂时无法确认连接，请检查网络后重试。'}</Typography>
        {checkedAt && <Typography variant="caption" color="text.secondary">最近验证：{checkedAt.toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'})}</Typography>}
        <Stack direction="row" spacing={1}><Button size="small" variant="outlined" disabled={connection === 'checking'} onClick={retry}>重新检查</Button><Button size="small" onClick={() => { setAnchor(null); session.reconnect(); }}>重新连接</Button></Stack>
      </Stack></Box>
    </Popover>
  </>;
}
