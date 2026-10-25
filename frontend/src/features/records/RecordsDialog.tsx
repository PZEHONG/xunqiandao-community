import { Chip, Dialog, DialogContent, DialogTitle, IconButton, Stack, Typography } from '@mui/material';
import CloseOutlined from '@mui/icons-material/CloseOutlined';
import type { SignSnapshot } from '../../api/contracts';
import { recordDate } from './calendar';
export function RecordsDialog({ snapshot, open, close }: { snapshot: SignSnapshot; open: boolean; close: () => void }) {
  return <Dialog open={open} onClose={close} aria-labelledby="records-detail-title" maxWidth="md" scroll="paper"><DialogTitle id="records-detail-title" sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>{snapshot.month_date} 签到明细<IconButton aria-label="关闭明细" onClick={close}><CloseOutlined /></IconButton></DialogTitle><DialogContent dividers><Stack gap={2}>
    {snapshot.records.length === 0 && <Typography color="text.secondary">本月暂无签到记录。</Typography>}
    {snapshot.records.map((record, index) => { const status = record.status_code_text || (String(record.status_code) === '0' ? '正常' : String(record.status_code) === '1' ? '异常' : '未知'); return <article className="record-detail" key={index}><Stack direction="row" justifyContent="space-between" gap={1} alignItems="center"><Typography fontWeight={600}>{record.sign_time_text || recordDate(record) || '时间未知'}</Typography><Chip size="small" label={status} color={status === '正常' ? 'primary' : status === '异常' ? 'error' : 'default'} /></Stack><dl><div><dt>地点</dt><dd>{record.address_name || '—'}</dd></div><div><dt>地址</dt><dd>{record.address || '—'}</dd></div><div><dt>备注</dt><dd>{record.remark_text || '—'}</dd></div>{record.comment && <div><dt>说明</dt><dd>{record.comment}</dd></div>}</dl></article>; })}
  </Stack></DialogContent></Dialog>;
}
