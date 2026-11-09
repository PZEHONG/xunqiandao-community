import type { ReactNode } from 'react';
import { Box, Chip, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography, useMediaQuery } from '@mui/material';
import { reportDate, reportGrade, reportTime } from './reportKinds';
import type { ReportRecord } from './types';

function ReportTimestamp({ value }: { value: unknown }) {
  const [date, ...time] = reportTime(value).split(' ');
  return <Box sx={{ fontVariantNumeric: 'tabular-nums', lineHeight: 1.6 }}>
    <Typography variant="body2" component="span" sx={{ display: 'block' }}>{date}</Typography>
    {time.length > 0 && <Typography variant="body2" component="span" color="text.secondary" sx={{ display: 'block', fontSize: 12 }}>{time.join(' ')}</Typography>}
  </Box>;
}

function ReportGrade({ row }: { row: ReportRecord }) {
  const label = reportGrade(row);
  const color = label === '优秀' || label === '良好' ? 'success.main' : label === '不及格' ? 'error.main' : 'text.secondary';
  return <Chip label={label} size="small" variant="outlined" sx={{ color, borderColor: 'divider', bgcolor: 'background.paper', fontSize: 12, height: 26, maxWidth: '100%', '& .MuiChip-label': { px: .75 } }} />;
}

function ReportPeriod({ row }: { row: ReportRecord }) {
  if (!row.start_date || !row.end_date) return <>—</>;
  return <Box sx={{ fontVariantNumeric: 'tabular-nums', lineHeight: 1.6 }}>
    <Typography variant="body2" component="span" sx={{ display: 'block' }}>{reportDate(row.start_date)}</Typography>
    <Typography variant="body2" component="span" color="text.secondary" sx={{ display: 'block' }}>{reportDate(row.end_date)}</Typography>
  </Box>;
}

export function ReportTable({ rows, view }: { rows: ReportRecord[]; view: (row: ReportRecord) => ReactNode }) {
  const phone = useMediaQuery('(max-width:960px)');
  if (phone) return <Stack gap={1.5}>{rows.map((row, index) => <Box component="article" aria-label={`报告 ${String(row.id ?? index + 1)}`} key={String(row.id ?? index)} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1.5, overflow: 'hidden', overflowWrap: 'anywhere' }}>
    <Stack direction="row" justifyContent="space-between" alignItems="center" gap={1} sx={{ bgcolor: 'primary.light', px: 2, py: .5, borderBottom: '1px solid', borderColor: 'divider' }}>
      <ReportGrade row={row} />
      {view(row)}
    </Stack>
    <Box component="dl" sx={{ display: 'grid', gridTemplateColumns: '76px minmax(0,1fr)', alignItems: 'start', columnGap: 2, rowGap: 1.5, p: 2, m: 0, '& dt': { color: 'text.secondary', fontSize: 13 }, '& dd': { m: 0, fontSize: 14 } }}>
      <Box component="dt">报告日期</Box><Box component="dd"><ReportPeriod row={row} /></Box>
      <Box component="dt">批阅老师</Box><Box component="dd">{String(row.teacher_name || '—')}</Box>
      <Box component="dt">批阅时间</Box><Box component="dd">{reportTime(row.mark_time)}</Box>
      <Box component="dt">是否退回</Box><Box component="dd" sx={{ color: String(row.repulsed) === '1' ? 'error.main' : 'text.primary' }}>{String(row.repulsed) === '1' ? '是' : '否'}</Box>
      <Box component="dt">提交时间</Box><Box component="dd">{reportTime(row.create_time)}</Box>
    </Box>
  </Box>)}</Stack>;

  return <TableContainer sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}>
    <Table size="small" aria-label="报告列表" sx={{ tableLayout: 'fixed', '& th, & td': { px: { xs: .75, md: 1.5 }, borderColor: 'divider', overflowWrap: 'anywhere' }, '& th': { bgcolor: 'primary.light', color: 'text.secondary', fontWeight: 650, fontSize: 13, py: 1.75 }, '& td': { py: 1.25 }, '& tbody tr:nth-of-type(even)': { bgcolor: 'background.default' }, '& tbody tr:hover': { bgcolor: 'primary.light' }, '& tbody tr:last-of-type td': { borderBottom: 0 }, '& td .MuiButton-root': { minWidth: 0, px: .5, fontSize: 12, whiteSpace: 'nowrap' } }}>
      <colgroup>{[24, 12, 9, 17, 7, 14, 17].map((width, index) => <col key={index} style={{ width: `${width}%` }} />)}</colgroup>
      <TableHead><TableRow>
        <TableCell scope="col">报告日期</TableCell>
        <TableCell scope="col">批阅结果</TableCell>
        <TableCell scope="col">老师</TableCell>
        <TableCell scope="col">批阅时间</TableCell>
        <TableCell scope="col" align="center">退回</TableCell>
        <TableCell scope="col" align="right">操作</TableCell>
        <TableCell scope="col">提交时间</TableCell>
      </TableRow></TableHead>
      <TableBody>{rows.map((row, index) => <TableRow key={String(row.id ?? index)}>
        <TableCell><ReportPeriod row={row} /></TableCell>
        <TableCell><ReportGrade row={row} /></TableCell>
        <TableCell>{String(row.teacher_name || '—')}</TableCell>
        <TableCell><ReportTimestamp value={row.mark_time} /></TableCell>
        <TableCell align="center" sx={{ color: String(row.repulsed) === '1' ? 'error.main' : 'text.secondary' }}>{String(row.repulsed) === '1' ? '是' : '否'}</TableCell>
        <TableCell align="right">{view(row)}</TableCell>
        <TableCell><ReportTimestamp value={row.create_time} /></TableCell>
      </TableRow>)}</TableBody>
    </Table>
  </TableContainer>;
}
