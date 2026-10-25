import { useLayoutEffect, useRef, useState } from 'react';
import { Box, Button, Stack, Typography } from '@mui/material';
import { MainCard } from '../../components/MainCard';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { Feedback } from '../../components/Feedback';
import { StatusChip } from '../../components/StatusChip';
import { useSession } from '../../session/useSession';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { clearUserDrafts } from '../reports/draftStorage';
type Confirmation = Readonly<{ kind: 'clear' | 'logout' | 'drafts'; owner: string; epoch: number }>;
export function AccountPage() {
  const session = useSession(); const binding = session.bindings[0];
  const owner = JSON.stringify([session.epoch, session.status, session.user?.id, session.bindings]); const activeOwner = useRef(owner);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null); const lock = useRef(false); const used = useRef(new WeakSet<Confirmation>());
  const [draftResult, setDraftResult] = useState<{ owner: string; ok: boolean } | null>(null);
  useLayoutEffect(() => { activeOwner.current = owner; }, [owner]);
  const c = confirmation?.owner === owner ? confirmation : null;
  const confirm = async () => {
    if (!c || c.owner !== activeOwner.current || c.epoch !== getSessionEpoch() || lock.current || session.authBusy || used.current.has(c)) return;
    lock.current = true; used.current.add(c); setConfirmation(null);
    try { if (c.kind === 'drafts') setDraftResult({ owner, ok: session.user !== null && clearUserDrafts(session.user.id) }); else if (c.kind === 'clear') session.clearConfig(); else await session.logout(); } finally { lock.current = false; }
  };
  if (session.status === 'loading') return <MainCard><Feedback busy>正在恢复会话…</Feedback></MainCard>;
  return <Stack gap={2}>
    {draftResult?.owner === owner && <Feedback severity={draftResult.ok ? 'success' : 'error'}>{draftResult.ok ? '已清除当前用户在此浏览器的报告草稿和恢复副本。已提交到服务器的报告不受影响。' : '未能完整清除本地报告草稿，请检查浏览器存储权限后重试。'}</Feedback>}
    {session.message && <Feedback severity="success">{session.message}</Feedback>}
    {session.authBusy && <Feedback busy>正在处理账号请求，请稍候…</Feedback>}
    <MainCard title="讯签到账号"><Stack gap={2}>
      {!session.user ? <><Typography color="text.secondary">登录后自动关联习讯云学号，管理签到记录与实习报告。</Typography><Button disabled={session.authBusy} variant="contained" onClick={() => session.openLogin()} sx={{ alignSelf: 'flex-start' }}>登录</Button></> : <>
        <Box><StatusChip label="已登录讯签到" tone="success" /></Box>
        <Box component="dl" sx={{ m: 0, display: 'grid', gridTemplateColumns: '80px minmax(0,1fr)', gap: 1.5, '& dt': { color: 'text.secondary' }, '& dd': { m: 0, overflowWrap: 'anywhere' } }}><Typography component="dt">学号</Typography><Typography component="dd">{binding?.account || session.user.student_number || session.user.username}</Typography><Typography component="dt">账号</Typography><Typography component="dd">{session.user.username}</Typography></Box>
        <Stack direction="row" gap={1} flexWrap="wrap"><Button disabled={session.authBusy} variant="outlined" onClick={() => void session.restore()}>刷新登录状态</Button><Button disabled={session.authBusy} onClick={() => setConfirmation(Object.freeze({ kind: 'logout', owner, epoch: session.epoch }))}>退出登录</Button></Stack>
      </>}
    </Stack></MainCard>
    {session.user && <><MainCard title="习讯云关联"><Stack gap={2}>
      <Box><StatusChip label={binding ? '已关联习讯云' : '尚未关联习讯云'} tone={binding ? 'success' : 'warning'} /></Box>
      {binding && <Typography>{binding.school_name || '学校名称暂未返回'}</Typography>}
      <Typography variant="body2" color="text.secondary">{binding ? '已读取服务器保存的账号关联。需要更新凭证或请求提示登录过期时，可重新连接。' : session.configCleared ? '本页面连接已清空，可重新连接习讯云。服务器保存的账号仍保留。' : '首次使用请选择学校并验证习讯云账号。'}</Typography>
      <Button disabled={session.authBusy} variant="contained" sx={{ alignSelf: 'flex-start' }} onClick={() => binding || session.configCleared ? session.reconnect() : session.openLogin('official')}>{binding || session.configCleared ? '重新连接习讯云' : '连接习讯云'}</Button>
    </Stack></MainCard><MainCard title="浏览器配置"><Stack gap={2}><Typography variant="body2" color="text.secondary">仅清除当前用户的本地账号与学校配置，并清空本页面的临时输入。报告草稿和每日任务仍保留。</Typography><Button disabled={session.authBusy} sx={{ alignSelf: 'flex-start' }} onClick={() => setConfirmation(Object.freeze({ kind: 'clear', owner, epoch: session.epoch }))}>清除浏览器配置</Button></Stack></MainCard></>}
    {session.user && <MainCard title="本地报告草稿"><Stack gap={2}><Typography variant="body2" color="text.secondary">草稿和损坏草稿的恢复副本保存在此浏览器，退出登录后仍保留。共用设备时，可主动清除当前用户所有账号与学校的本地周报、月报草稿。此操作无法撤销；已提交到服务器的报告不受影响。</Typography><Button disabled={session.authBusy} color="error" sx={{ alignSelf: 'flex-start' }} onClick={() => setConfirmation(Object.freeze({ kind: 'drafts', owner, epoch: session.epoch }))}>清除本地报告草稿</Button></Stack></MainCard>}
    <ConfirmDialog open={!!c} title={c?.kind === 'drafts' ? '确认清除本地报告草稿' : c?.kind === 'clear' ? '确认清除浏览器配置' : '确认退出登录'} confirmLabel={c?.kind === 'drafts' ? '确认清除草稿' : c?.kind === 'clear' ? '确认清除' : '确认退出'} danger={c?.kind !== 'logout'} busy={session.authBusy} cancel={() => setConfirmation(null)} confirm={() => void confirm()}><Typography>{c?.kind === 'drafts' ? '清除当前用户在此浏览器所有账号与学校的周报、月报草稿及恢复副本，无法撤销。请先下载需要保留的草稿。已提交到服务器的报告、每日任务和浏览器配置不受影响。' : c?.kind === 'clear' ? '清除当前用户在此浏览器保存的账号与学校配置，并清空当前页面的临时输入。服务器账号、报告草稿和每日任务仍保留。' : '退出当前讯签到账号。此浏览器其他标签页也会更新登录状态，报告草稿和每日任务仍保留。'}</Typography></ConfirmDialog>
  </Stack>;
}
