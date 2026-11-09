import { useState } from 'react';
import { Box, Button, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { Feedback } from '../../components/Feedback';
import { MainCard } from '../../components/MainCard';
import { useSession } from '../../session/useSession';
import { ReportAIDialog } from './ReportAIDialog';
import { ReportDetailDialog } from './ReportDetailDialog';
import { ReportEditor } from './ReportEditor';
import { ReportTable } from './ReportTable';
import type { ReportKind, ReportRecord } from './types';
import { useReports } from './useReports';

export function ReportsPage({ kind }: { kind: ReportKind }) {
  const session = useSession(); const m = useReports(kind);
  const [aiMode, setAIMode] = useState<'single' | 'batch' | null>(null);
  if (session.status === 'loading') return <MainCard><Typography role="status">正在恢复会话…</Typography></MainCard>;
  if (!session.user) return <MainCard><Feedback severity="info" action={<Button onClick={() => session.openLogin()}>登录</Button>}>登录后查看和填写报告。</Feedback></MainCard>;
  if (!m.account) return <MainCard><Feedback severity="info" action={<Button onClick={() => session.openLogin('official')}>连接账号</Button>}>请先连接习讯云账号。</Feedback></MainCard>;
  const view = (row: ReportRecord) => <Button disabled={!row.id} onClick={() => void m.openDetail(String(row.id))}>查看详情</Button>;
  return <>
    {m.editor ? <ReportEditor model={m} kind={kind} onAI={() => setAIMode('single')} /> : <Stack gap={2}>
      <MainCard><Stack component="form" gap={2} onSubmit={e => { e.preventDefault(); void m.load(1); }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
          <TextField label="查询开始日期" type="date" value={m.filters.start} onChange={e => m.setFilter({ ...m.filters, start: e.target.value })} slotProps={{ inputLabel: { shrink: true } }} />
          <TextField label="查询结束日期" type="date" value={m.filters.end} onChange={e => m.setFilter({ ...m.filters, end: e.target.value })} slotProps={{ inputLabel: { shrink: true } }} />
          <TextField label="批阅状态" select value={m.filters.status} onChange={e => m.setFilter({ ...m.filters, status: e.target.value })}><MenuItem value="">全部</MenuItem><MenuItem value="without_approval">未批阅</MenuItem><MenuItem value="approval">已批阅</MenuItem></TextField>
        </Stack>
        <Stack direction="row" gap={1} flexWrap="wrap">
          <Button type="submit" variant="outlined" disabled={m.loading}>{m.loading ? '查询中…' : '查询报告'}</Button>
          <Button variant="contained" disabled={m.templateLoading} onClick={() => void m.openEditor()}>{m.templateLoading ? '读取模板…' : '新建报告'}</Button>
          <Button variant="outlined" disabled={m.templateLoading} onClick={() => setAIMode('batch')}>AI补全</Button>
        </Stack>
      </Stack></MainCard>
      {m.error && <Feedback severity="error">{m.error}{m.rows.length ? ' 以下仍为上次查询结果。' : ''}</Feedback>}
      {m.message && <Feedback severity="success">{m.message}</Feedback>}
      {m.storage.includes('提交成功') && <Feedback severity="info">{m.storage}</Feedback>}
      <MainCard><Stack gap={2}>
        <Stack direction="row" justifyContent="space-between" gap={1} flexWrap="wrap" alignItems="center"><Typography component="h2" variant="h3">报告列表</Typography><Typography variant="body2" color="text.secondary">共 {m.count} 份 · 每页 15 份</Typography></Stack>
        <Box aria-busy={m.loading} sx={{ opacity: m.loading ? .6 : 1 }}>{!m.rows.length ? <Typography color="text.secondary" sx={{ py: 3 }}>{m.loading ? '正在查询…' : '暂无报告'}</Typography> : <ReportTable rows={m.rows} view={view} />}</Box>
        <Stack direction="row" gap={1} justifyContent="space-between" alignItems="center"><Button disabled={m.loading || m.page <= 1} onClick={() => void m.load(m.page - 1)}>上一页</Button><Typography variant="body2">第 {m.page} / {m.pages} 页</Typography><Button disabled={m.loading || m.page >= m.pages} onClick={() => void m.load(m.page + 1)}>下一页</Button></Stack>
      </Stack></MainCard>
      <ReportDetailDialog model={m} kind={kind} />
    </Stack>}
    {aiMode && <ReportAIDialog key={`${m.key}:${session.epoch}`} model={m} kind={kind} mode={aiMode} onClose={() => setAIMode(null)} />}
  </>;
}
