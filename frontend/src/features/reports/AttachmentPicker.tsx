import { Alert, Box, Button, Stack, Typography } from '@mui/material';
import type { ReportsModel } from './useReports';
export function AttachmentPicker({ model: m }: { model: ReportsModel }) {
  return <Stack gap={1.5}><Typography component="h3" variant="h3">附件 <Typography component="span" variant="body2" color="text.secondary">{m.attachments.length} / 3</Typography></Typography>
    <Typography variant="body2" color="text.secondary">每个1字节至10MB。确认提交后才上传；本地文件刷新后需重新选择。</Typography>
    {!!m.pendingNames.length && <Alert severity="warning"><Stack gap={1}><Typography variant="body2">以下本地附件无法从草稿恢复，请重新选择，或明确移除：</Typography>{m.pendingNames.map((name, index) => <Stack key={`${index}:${name}`} direction="row" alignItems="center" gap={1} justifyContent="space-between"><Typography sx={{ overflowWrap: 'anywhere', minWidth: 0 }}>{name}</Typography><Button disabled={m.busy} aria-label={`移除待重选附件 ${name}`} onClick={() => m.ignorePending(index)}>移除</Button></Stack>)}</Stack></Alert>}
    {m.attachments.map(a => <Box key={a.id} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}><Stack direction="row" gap={1} alignItems="center" justifyContent="space-between"><Box sx={{ minWidth: 0 }}><Typography sx={{ overflowWrap: 'anywhere' }}>{a.name}</Typography><Typography variant="caption" color="text.secondary">{a.uri ? '已上传，可复用' : '待确认后上传'}</Typography></Box><Button disabled={m.busy} aria-label={`移除附件 ${a.name}`} onClick={() => m.removeAttachment(a.id)}>移除</Button></Stack></Box>)}
    <Button component="label" variant="outlined" disabled={m.busy || m.attachments.length >= 3} sx={{ alignSelf: 'flex-start' }}>选择附件<input aria-label="选择报告附件" type="file" multiple disabled={m.busy || m.attachments.length >= 3} style={{ position: 'absolute', width: 1, height: 1, opacity: 0 }} onChange={e => { m.addFiles(Array.from(e.target.files ?? [])); e.target.value = ''; }} /></Button>
  </Stack>;
}
