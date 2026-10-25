import { Feedback } from '../../components/Feedback';
import { useState } from 'react';
import { Box, Button, IconButton, Stack, Typography } from '@mui/material';
import ChevronLeft from '@mui/icons-material/ChevronLeft';
import ChevronRight from '@mui/icons-material/ChevronRight';
import { MainCard } from '../../components/MainCard';
import { useSession } from '../../session/useSession';
import { useRecords } from './useRecords';
import { buildMonth, currentMonth, shiftMonth } from './calendar';
import { SignCalendar } from './SignCalendar';
import { RecordsDialog } from './RecordsDialog';
export function RecordsPage() {
  const session = useSession(); const records = useRecords(); const [details, setDetails] = useState(false);
  const snapshot = records.snapshot; const rawStreak = snapshot?.continuous_sign_in ?? snapshot?.continuous_days; const streak = /^\d+$/.test(String(rawStreak)) ? String(rawStreak) : '—';
  return <MainCard><Stack gap={2.5}>
    {session.status === 'loading' ? <Typography role="status">正在恢复会话…</Typography> : !session.user ? <Feedback severity="info" action={<Button onClick={() => session.openLogin()}>登录</Button>}>登录后查看签到记录。</Feedback> : session.bindings.length === 0 ? <Feedback severity="info" action={<Button onClick={() => session.openLogin('official')}>连接账号</Button>}>请先连接习讯云账号。</Feedback> : null}
    {records.error && <Feedback severity="error">{records.error}{snapshot ? ` 以下仍为 ${snapshot.month_date} 上次查询结果。` : ''}</Feedback>}
    <Box className="calendar-toolbar"><Stack direction="row" gap={0.5} alignItems="center"><IconButton aria-label="上个月" disabled={records.busy || !session.bindings.length} onClick={() => void records.loadMonth(shiftMonth(records.month, -1))}><ChevronLeft /></IconButton><Typography component="h2" sx={{ fontSize: { xs: 17, sm: 21 }, fontWeight: 650, whiteSpace: 'nowrap' }}>{buildMonth(records.month).title}</Typography><IconButton aria-label="下个月" disabled={records.busy || !session.bindings.length} onClick={() => void records.loadMonth(shiftMonth(records.month, 1))}><ChevronRight /></IconButton></Stack><Stack direction="row" gap={1}><Button disabled={records.busy || !session.bindings.length} onClick={() => void records.loadMonth(currentMonth())}>本月</Button><Button variant="outlined" disabled={records.busy || !session.bindings.length} onClick={() => void records.refresh()}>{records.busy ? '查询中…' : '刷新'}</Button></Stack></Box>
    <Stack direction="row" gap={2} justifyContent="space-between" flexWrap="wrap"><Typography color="text.secondary" variant="body2">平台连续签到 <strong style={{ color: '#2456D6', fontSize: 18 }}>{streak}</strong> 天</Typography><Typography color="text.secondary" variant="body2">北京时间 · 正常 / 异常 / 未知</Typography></Stack>
    <Box aria-busy={records.busy} sx={{ opacity: records.busy ? 0.65 : 1 }}><SignCalendar month={records.month} records={snapshot?.records ?? []} /></Box>
    <Stack direction="row" gap={2} justifyContent="space-between" alignItems="center"><Typography variant="caption" color="text.secondary">{snapshot ? `${snapshot.month_date} · 来源：习讯云` : '尚未查询签到记录'}</Typography><Button disabled={!snapshot} onClick={() => setDetails(true)}>展开明细</Button></Stack>
  </Stack>{snapshot && <RecordsDialog key={`${session.epoch}:${snapshot.month_date}`} snapshot={snapshot} open={details} close={() => setDetails(false)} />}</MainCard>;
}
