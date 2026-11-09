import { Feedback } from '../../components/Feedback';
import { useEffect, useLayoutEffect, useRef } from 'react';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, Typography } from '@mui/material';
import { MainCard } from '../../components/MainCard';
import { useLocation } from '../locations/useLocation';
import { LocationPicker } from '../locations/LocationPicker';
import { useSign } from '../sign/useSign';
import { useAutoSign } from './useAutoSign';
import { TaskEditor } from './TaskEditor';
import { TaskList } from './TaskList';
import { RunHistory } from './RunHistory';
export function AutoSignPanel({ onBusy }: { onBusy: (value: boolean) => void }) {
  const auto = useAutoSign(); const location = useLocation();
  const sign = useSign({ confirmed: auto.savedPlace || location.confirmed, pending: auto.savedPlace ? false : location.pending });
  const { invalidateConfirmation, editing } = auto; const { clear } = location; const { reset, setRemark, setComment } = sign;
  const latest = useRef(auto); useLayoutEffect(() => { latest.current = auto; });
  useLayoutEffect(() => { invalidateConfirmation(); }, [sign.draft, location.confirmed, location.pending, invalidateConfirmation]);
  useEffect(() => { onBusy(auto.busy); return () => onBusy(false); }, [auto.busy, onBusy]);
  useEffect(() => {
    if (editing) { clear(); reset(); setRemark(editing.remark); setComment(editing.comment); }
  }, [editing, clear, reset, setRemark, setComment]);
  useEffect(() => {
    const timer = setInterval(() => { const value = latest.current; if (!document.hidden && !value.busy && !value.blocked && !value.confirmation && !value.editing) void value.load(); }, 30000);
    return () => clearInterval(timer);
  }, []);
  const disabled = auto.busy || auto.blocked; const configDisabled = disabled;
  const newTask = () => { auto.cancelEdit(); location.clear(); sign.reset(); };
  const confirmation = auto.confirmation; const task = confirmation?.task;
  return <Stack gap={2}>
    <Stack direction="row" gap={1} flexWrap="wrap"><Button disabled={configDisabled || auto.loading} onClick={newTask}>新建任务</Button><Button disabled={auto.busy || auto.loading} onClick={() => void auto.load()}>刷新任务与记录</Button>{auto.loading && <Typography role="status" sx={{ alignSelf: 'center' }}>正在读取任务与记录…</Typography>}{auto.busy && <Typography role="status" sx={{ alignSelf: 'center' }}>正在处理任务…</Typography>}</Stack>
    {auto.error && <Feedback severity="error">{auto.error}</Feedback>}{auto.message && <Feedback severity="success">{auto.message}</Feedback>}
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'minmax(0, 1.35fr) minmax(0, 1fr)' }, gap: 2, alignItems: 'start' }}>
      <MainCard title="任务地点">{auto.savedPlace ? <Stack gap={2} sx={{ overflowWrap: 'anywhere' }}><Typography fontWeight={600}>保留已保存地点</Typography><Typography>{auto.savedPlace.address_name}</Typography><Typography>{auto.savedPlace.address}</Typography><Typography color="text.secondary">仅修改任务时间或说明可保留此地点。更换后需重新搜索并确认。</Typography><Button disabled={configDisabled} onClick={() => { location.clear(); auto.changePlace(); }} sx={{ alignSelf: 'flex-start' }}>更换地点</Button></Stack> : <Box component="fieldset" disabled={configDisabled} sx={{ border: 0, m: 0, p: 0, minWidth: 0 }}><LocationPicker location={location} disabled={configDisabled} /></Box>}</MainCard>
      <MainCard title={auto.editing ? '编辑每日任务' : '新建每日任务'}><Box component="fieldset" disabled={configDisabled} sx={{ border: 0, m: 0, p: 0, minWidth: 0 }}><TaskEditor auto={auto} sign={sign} locationReady={!!auto.savedPlace || !!location.confirmed && !location.pending} /></Box></MainCard>
    </Box>
    <MainCard title="每日任务"><TaskList tasks={auto.tasks} disabled={disabled || auto.loading} pause={task => void auto.pause(task)} resume={auto.resume} edit={task => void auto.edit(task)} remove={auto.remove} /></MainCard>
    <MainCard title="执行记录"><RunHistory runs={auto.runs} /></MainCard>
    <Dialog open={!!confirmation} onClose={() => { if (!auto.busy) auto.cancelConfirmation(); }} disableEscapeKeyDown={auto.busy} aria-labelledby="auto-confirm-title" fullWidth maxWidth="sm" scroll="paper">
      <DialogTitle id="auto-confirm-title">{confirmation?.kind === 'remove' ? '确认删除每日任务' : '确认启用每日任务'}</DialogTitle>
      <DialogContent dividers><Stack gap={2} sx={{ overflowWrap: 'anywhere' }}>{task && <><Typography>学号：{task.account}</Typography><Typography>学校 ID：{task.school_id}</Typography><Typography>任务：{task.name}</Typography><Typography>每天 {task.time}（北京时间，包含周末）</Typography><Typography>签到备注：{task.remark_label}</Typography><Typography>地点：{task.address_name}</Typography><Typography>完整地址：{task.address}</Typography><Typography>参考坐标（GCJ-02）：{task.longitude}, {task.latitude}</Typography><Typography sx={{ whiteSpace: 'pre-wrap' }}>说明：{task.comment || '无'}</Typography><Typography>照片：不附带</Typography></>}<Typography color="text.secondary">{confirmation?.kind === 'remove' ? '删除后不再按此计划执行，执行记录仍保留。已经发出的签到无法撤回。' : '确认后从下一个计划时间开始执行，今天已执行过时下一次为明天。保存不会立即签到。任务由服务器执行，关闭网页不影响已启用的任务；请保持部署服务器运行及习讯云账号可正常连接。'}</Typography>{auto.busy && <Typography role="status">正在处理任务，请稍候…</Typography>}</Stack></DialogContent>
      <DialogActions><Button disabled={auto.busy} onClick={auto.cancelConfirmation}>取消</Button><Button disabled={auto.busy} variant="contained" color={confirmation?.kind === 'remove' ? 'error' : 'primary'} onClick={() => { if (confirmation) void auto.submitConfirmed(confirmation); }}>{auto.busy ? '处理中…' : confirmation?.kind === 'remove' ? '确认删除' : '确认启用'}</Button></DialogActions>
    </Dialog>
  </Stack>;
}
