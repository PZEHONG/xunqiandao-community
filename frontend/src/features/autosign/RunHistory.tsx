import { Box, Chip, Stack, Typography } from '@mui/material';
import { runLabels, type AutoRun } from './types';
export function RunHistory({ runs }: { runs: AutoRun[] }) {
  return <Stack gap={2}>{!runs.length && <Typography color="text.secondary">暂无执行记录。删除任务后已有执行记录仍会保留。</Typography>}{runs.map(run => <Box component="article" aria-label={`${run.name}执行记录`} key={run.id} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, overflowWrap: 'anywhere' }}><Stack gap={1}><Typography fontWeight={600}>{run.name}</Typography><Typography>{run.date} {run.scheduled_time}（北京时间）</Typography><Chip label={runLabels[run.status] || '失败'} size="small" color={run.status === 'success' ? 'success' : run.status === 'running' ? 'default' : 'error'} sx={{ alignSelf: 'flex-start' }} /><Typography color="text.secondary">{run.message || (run.status === 'running' ? '后台正在执行' : '暂无结果说明')}</Typography></Stack></Box>)}</Stack>;
}
