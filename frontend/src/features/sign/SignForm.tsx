import { Feedback } from '../../components/Feedback';
import { Box, Button, Stack, TextField, Typography } from '@mui/material';
import { Link } from 'react-router-dom';
import { PhotoPicker } from './PhotoPicker';
import type { useSign } from './useSign';
export function SignForm({ sign, locationReady }: { sign: ReturnType<typeof useSign>; locationReady: boolean }) {
  const count = [...sign.draft.comment].length;
  const date = new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: 'long', day: 'numeric' }).format(new Date());
  return <Box component="form" onSubmit={event => { event.preventDefault(); sign.prepareConfirmation(); }}>
    <Stack gap={2}>
      <Typography color="text.secondary">签到日期：{date}（北京时间）</Typography>
      <Stack gap={1}>
        <Typography fontWeight={600}>签到备注</Typography>
        {sign.loadingRemarks && <Typography role="status">正在加载账号可用备注…</Typography>}
        {sign.remarkError && <Feedback severity="error">{sign.remarkError}</Feedback>}
        <Stack direction="row" gap={1} flexWrap="wrap">{sign.remarks.map(mark => <Button key={mark.key} disabled={sign.busy || sign.loadingRemarks} variant={sign.draft.remark === mark.key ? 'contained' : 'outlined'} aria-pressed={sign.draft.remark === mark.key} onClick={() => sign.setRemark(mark.key)}>{mark.value}</Button>)}<Button disabled={sign.busy || sign.loadingRemarks} onClick={() => void sign.reloadRemarks()}>刷新备注</Button></Stack>
      </Stack>
      <TextField label="签到说明（选填）" value={sign.draft.comment} disabled={sign.busy} onChange={event => sign.setComment(event.target.value)} multiline minRows={3} error={count > 1000} helperText={`${count} / 1000 字`} />
      <PhotoPicker photos={sign.draft.photos} disabled={sign.busy} onChange={sign.setPhotos} />
      {sign.error && <Feedback severity={sign.result?.status === 'unknown' ? 'warning' : 'error'}><Typography>{sign.error}</Typography>{sign.result?.status === 'unknown' && <Button component={Link} to="/records">查看签到记录</Button>}</Feedback>}
      {!locationReady && <Typography variant="body2" color="text.secondary">请先确认实际签到地点。</Typography>}
      <Button type="submit" variant="contained" disabled={sign.busy || !locationReady || !sign.draft.remark || count > 1000 || sign.loadingRemarks}>{sign.busy ? '提交中…' : '核对签到信息'}</Button>
    </Stack>
  </Box>;
}
