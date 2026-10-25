import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, Typography } from '@mui/material';
import type { SignConfirmation } from './types';
export function SignConfirmDialog({ snapshot, busy, progress, cancel, submit }: { snapshot: SignConfirmation | null; busy: boolean; progress: string; cancel: () => void; submit: (snapshot: SignConfirmation) => Promise<void> }) {
  return <Dialog open={!!snapshot} onClose={() => { if (!busy) cancel(); }} aria-labelledby="sign-confirm-title" scroll="paper">
    <DialogTitle id="sign-confirm-title">核对本次签到</DialogTitle>
    <DialogContent dividers>{snapshot && <Stack gap={2} sx={{ overflowWrap: 'anywhere' }}>
      {[['学号', snapshot.account], ['学校', snapshot.school || '—'], ['地点', snapshot.location.address_name || '所选地点'], ['完整地址', snapshot.location.address], ['参考坐标（GCJ-02）', `${snapshot.location.longitude}, ${snapshot.location.latitude}`], ['签到备注', snapshot.remarkLabel], ['签到说明', snapshot.comment || '未填写'], ['照片', `${snapshot.photos.length} 张`]].map(([label, value]) => <Box key={label}><Typography variant="body2" color="text.secondary">{label}</Typography><Typography sx={{ whiteSpace: 'pre-wrap' }}>{value}</Typography></Box>)}
      {!!snapshot.photos.length && <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap' }}>{snapshot.photos.map(file => file.name).join('\n')}</Typography>}
      <Typography variant="body2" color="text.secondary">确认后将上传所选照片并提交本次签到，请核对实际地点与备注。</Typography>
      {busy && <Typography role="status">{progress || '正在提交…'}</Typography>}
    </Stack>}</DialogContent>
    <DialogActions sx={{ flexWrap: 'wrap', gap: 1, px: 3, py: 2 }}><Button disabled={busy} onClick={cancel}>取消</Button><Button variant="contained" disabled={busy || !snapshot} onClick={() => { if (snapshot) void submit(snapshot); }}>{busy ? '提交中…' : '确认提交签到'}</Button></DialogActions>
  </Dialog>;
}
