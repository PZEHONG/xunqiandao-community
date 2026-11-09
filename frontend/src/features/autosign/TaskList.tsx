import { Box, Button, Chip, Stack, Typography } from '@mui/material';
import type { AutoTask } from './types';
import { localTime, precheckLabels, runLabels } from './types';
export function TaskList({ tasks, disabled, pause, resume, edit, remove }: { tasks: AutoTask[]; disabled: boolean; pause: (task: AutoTask) => void; resume: (task: AutoTask) => void; edit: (task: AutoTask) => void; remove: (task: AutoTask) => void }) {
  return <Stack gap={2}>
    <Typography color="text.secondary">{tasks.length} 个任务 · {tasks.filter(task => task.enabled).length} 个启用</Typography>
    {!tasks.length && <Typography>暂无每日任务。确认地点与内容后创建每日计划。</Typography>}
    {tasks.map(task => <Box component="article" aria-label={`${task.name}任务`} key={task.id} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, overflowWrap: 'anywhere' }}>
      <Stack gap={1}>
        <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center"><Typography variant="h3" component="h3">{task.name}</Typography><Chip size="small" label={task.enabled ? '已启用' : '已暂停'} color={task.enabled ? 'success' : 'default'} /></Stack>
        <Typography fontWeight={600}>每天 {task.time} · {task.remark_label}</Typography>
        <Typography>{task.address_name}</Typography><Typography color="text.secondary">{task.address}</Typography>
        <Typography variant="body2">{task.enabled ? task.next_at ? `下次执行：${localTime(task.next_at)}（北京时间）` : '下次执行时间暂未返回，请刷新任务。' : '重新启用前需要核对已保存的信息。'}</Typography>
        {task.last_run && <Typography variant="body2">最近结果：{runLabels[task.last_run.status] || '失败'} · {task.last_run.message}</Typography>}
        {task.precheck && <Typography variant="body2" color="text.secondary">签到预检查（{localTime(task.precheck.checked_at)}）：{precheckLabels[task.precheck.status] || task.precheck.status} · {task.precheck.message}</Typography>}
        <Stack direction="row" gap={1} flexWrap="wrap"><Button disabled={disabled} variant="outlined" onClick={() => task.enabled ? pause(task) : resume(task)}>{task.enabled ? '暂停' : '重新启用'}</Button><Button disabled={disabled} onClick={() => edit(task)}>编辑</Button><Button disabled={disabled} color="error" onClick={() => remove(task)}>删除</Button></Stack>
      </Stack>
    </Box>)}
  </Stack>;
}
