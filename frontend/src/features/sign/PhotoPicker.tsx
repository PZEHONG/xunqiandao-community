import { useEffect, useRef, useState } from 'react';
import { Box, Button, IconButton, Stack, Typography } from '@mui/material';
import CloseOutlined from '@mui/icons-material/CloseOutlined';
export function PhotoPicker({ photos, disabled, onChange }: { photos: readonly File[]; disabled: boolean; onChange: (files: readonly File[]) => void }) {
  const input = useRef<HTMLInputElement>(null); const urls = useRef(new Map<File, string>()); const [previews, setPreviews] = useState<{ file: File; url: string }[]>([]);
  useEffect(() => {
    for (const [file, url] of urls.current) if (!photos.includes(file)) { URL.revokeObjectURL(url); urls.current.delete(file); }
    for (const file of photos) if (!urls.current.has(file)) urls.current.set(file, URL.createObjectURL(file));
    setPreviews(photos.map(file => ({ file, url: urls.current.get(file)! })));
  }, [photos]);
  useEffect(() => { const cache = urls.current; return () => { cache.forEach(url => URL.revokeObjectURL(url)); cache.clear(); }; }, []);
  return <Stack gap={1}>
    <Typography fontWeight={600}>签到照片（选填）</Typography>
    <Typography variant="body2" color="text.secondary">最多3张，单张10 MB以内，支持JPG、PNG、GIF、WebP。确认提交后才上传。</Typography>
    <input ref={input} type="file" aria-label="选择签到照片" accept="image/jpeg,image/png,image/gif,image/webp" multiple disabled={disabled} style={{ display: 'none' }} onChange={event => { const files = [...(event.target.files || [])]; event.target.value = ''; if (files.length && !disabled) onChange([...photos, ...files]); }} />
    <Stack direction="row" gap={1} flexWrap="wrap">
      {previews.map(({ file, url }, index) => <Box key={`${file.name}:${index}`} sx={{ position: 'relative', width: 108, maxWidth: '100%', border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 0.5 }}>
        <Box component="img" src={url} alt={file.name} sx={{ display: 'block', width: '100%', height: 96, objectFit: 'cover', borderRadius: 0.5 }} />
        <IconButton disabled={disabled} aria-label={`移除照片 ${file.name}`} onClick={() => onChange(photos.filter((_file, photoIndex) => photoIndex !== index))} sx={{ position: 'absolute', right: 0, top: 0, bgcolor: 'background.paper' }}><CloseOutlined /></IconButton>
        <Typography variant="caption" sx={{ display: 'block', overflowWrap: 'anywhere' }}>{file.name}</Typography>
      </Box>)}
      {photos.length < 3 && <Button variant="outlined" disabled={disabled} onClick={() => input.current?.click()} sx={{ alignSelf: 'flex-start' }}>添加照片</Button>}
    </Stack>
  </Stack>;
}
