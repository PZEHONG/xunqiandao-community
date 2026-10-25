import { Alert, Box, Button, Stack, TextField, Typography } from '@mui/material';
import type { useSign } from '../sign/useSign';
import type { useAutoSign } from './useAutoSign';
export function TaskEditor({ auto, sign, locationReady }: { auto: ReturnType<typeof useAutoSign>; sign: ReturnType<typeof useSign>; locationReady: boolean }) {
  const disabled = auto.busy || auto.blocked; const count = [...sign.draft.comment].length;
  return <Box component="form" onSubmit={event => { event.preventDefault(); const snapshot = sign.prepareConfirmation(); if (snapshot) auto.create(snapshot); }}><Stack gap={2}>
    {auto.editing && <Alert severity="info">编辑期间任务保持暂停。取消编辑不会重新启用。</Alert>}
    <TextField label="任务名称" value={auto.name} disabled={disabled} onChange={event => auto.setName(event.target.value)} error={[...auto.name].length > 60} helperText={`${[...auto.name].length} / 60 字`} />
    <TextField label="每日时间（北京时间）" type="time" slotProps={{ inputLabel: { shrink: true }, htmlInput: { step: 60 } }} value={auto.time} disabled={disabled} onChange={event => auto.setTime(event.target.value)} />
    <Typography fontWeight={600}>签到备注</Typography>
    {sign.loadingRemarks && <Typography role="status">正在加载可用备注…</Typography>}{sign.remarkError && <Alert severity="error">{sign.remarkError}</Alert>}
    <Stack direction="row" gap={1} flexWrap="wrap">{sign.remarks.map(mark => <Button key={mark.key} disabled={disabled || sign.loadingRemarks} aria-pressed={sign.draft.remark === mark.key} variant={sign.draft.remark === mark.key ? 'contained' : 'outlined'} onClick={() => sign.setRemark(mark.key)}>{mark.value}</Button>)}<Button disabled={disabled || sign.loadingRemarks} onClick={() => void sign.reloadRemarks()}>刷新备注</Button></Stack>
    <TextField label="签到说明（选填）" value={sign.draft.comment} disabled={disabled} onChange={event => sign.setComment(event.target.value)} multiline minRows={3} error={count > 1000} helperText={`${count} / 1000 字`} />
    <Typography color="text.secondary">每日任务不附带照片，包含周末；保存不会立即签到。</Typography>
    {sign.error && <Alert severity="error">{sign.error}</Alert>}
    {!locationReady && <Typography color="text.secondary">请先搜索并确认任务地点。</Typography>}
    <Button type="submit" variant="contained" disabled={disabled || auto.loading || sign.loadingRemarks || !locationReady || !sign.draft.remark || count > 1000}>{auto.editing ? '核对修改并启用' : '核对每日任务'}</Button>
    {auto.editing && <Button disabled={auto.busy} onClick={auto.cancelEdit}>取消编辑</Button>}
  </Stack></Box>;
}
