import { Feedback } from '../../components/Feedback';
import { useEffect, useState } from 'react';
import { Box, Button, Chip, Stack, TextField, Typography } from '@mui/material';
import { mapConfig } from './api';
import { MapView } from './MapView';
import type { MapConfig } from './types';
import type { useLocation } from './useLocation';
export function LocationPicker({ location, disabled = false }: { location: ReturnType<typeof useLocation>; disabled?: boolean }) {
  const [config, setConfig] = useState<MapConfig>(); const [configError, setConfigError] = useState(''); const [attempt, setAttempt] = useState(0);
  const [keyword, setKeyword] = useState(''); const [city, setCity] = useState(''); const [searched, setSearched] = useState(false); const [showResults, setShowResults] = useState(false);
  useEffect(() => {
    const controller = new AbortController(); setConfig(undefined); setConfigError('');
    void mapConfig(controller.signal).then(value => { if (!controller.signal.aborted) { setConfig(value); setCity(previous => previous || value.default_city || ''); } }).catch(error => { if (!controller.signal.aborted) setConfigError(error instanceof Error ? error.message : '地图配置暂时无法读取。'); });
    return () => controller.abort();
  }, [attempt]);
  return <Stack gap={2}>
    <Box component="form" onSubmit={event => { event.preventDefault(); if (disabled || !config?.search_enabled) return; setSearched(true); setShowResults(true); void location.search(keyword, city); }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} gap={1} alignItems={{ sm: 'flex-start' }}>
        <TextField label="城市（选填）" value={city} inputProps={{ maxLength: 40 }} onChange={event => { setCity(event.target.value); location.invalidateSearch(); setSearched(false); }} sx={{ width: { xs: '100%', sm: 160 } }} />
        <TextField label="地点名称或详细地址" value={keyword} inputProps={{ maxLength: 80 }} onChange={event => { setKeyword(event.target.value); location.invalidateSearch(); setSearched(false); }} sx={{ flex: 1, width: { xs: '100%', sm: 'auto' } }} />
        <Button variant="contained" type="submit" disabled={!config?.search_enabled || location.searching || !keyword.trim()} sx={{ minHeight: 44 }}>{location.searching ? '搜索中…' : '搜索地点'}</Button>
      </Stack>
    </Box>
    {config && !config.search_enabled && <Feedback severity="warning">地址搜索尚未连接，请完成高德配置后重试。</Feedback>}
    {location.error && <Feedback severity="error">{location.error}</Feedback>}
    <Box role="status" aria-live="polite">
      {location.searching ? <Typography>正在查找地点…</Typography> : searched && showResults && !location.results.length && !location.error ? <Typography color="text.secondary">未找到匹配地点，请补充城市、街道或建筑名称。</Typography> : null}
    </Box>
    {showResults && location.results.length > 0 && <Stack gap={1} aria-label="地点搜索结果">{location.results.map((place, index) => <Button key={`${place.id || ''}:${index}`} variant="outlined" color="inherit" onClick={() => { location.preview(place); setShowResults(false); }} sx={{ textAlign: 'left', justifyContent: 'flex-start', whiteSpace: 'normal', overflowWrap: 'anywhere', py: 1 }}><Stack alignItems="flex-start"><Typography component="span" fontWeight={600}>{place.name || '所选地点'}</Typography><Typography component="span" variant="body2" color="text.secondary">{place.address}</Typography></Stack></Button>)}</Stack>}
    <MapView config={config} configError={configError} point={location.point} onPick={point => { if (!disabled && config?.search_enabled) void location.pickPoint(point); }} onRetry={() => { if (!disabled) setAttempt(value => value + 1); }} />
    {location.pending && <Stack gap={1} sx={{ p: 2, border: '1px solid', borderColor: 'primary.main', borderRadius: 1, overflowWrap: 'anywhere' }}>
      <Chip label="候选地点待确认" color="primary" variant="outlined" sx={{ alignSelf: 'flex-start' }} />
      <Typography fontWeight={600}>{location.resolving ? '正在识别所选位置…' : location.candidate?.name || '暂时无法识别此位置'}</Typography>
      <Typography>{location.candidate?.address || '请核对地址后使用，或取消候选重新搜索。'}</Typography>
      {location.candidate && <Typography variant="caption" color="text.secondary">GCJ-02 · 经度 {location.candidate.longitude} · 纬度 {location.candidate.latitude}</Typography>}
      <Stack direction="row" gap={1} flexWrap="wrap"><Button variant="contained" disabled={!location.candidate || location.resolving} onClick={location.confirm}>使用此地点</Button><Button onClick={location.cancel}>取消候选</Button></Stack>
    </Stack>}
    {location.confirmed ? <Stack gap={0.5} sx={{ p: 2, bgcolor: 'primary.light', borderRadius: 1, overflowWrap: 'anywhere' }}>
      <Typography fontWeight={600}>已确认地点</Typography><Typography>{location.confirmed.address_name}</Typography><Typography>{location.confirmed.address}</Typography>
      <Typography variant="caption" color="text.secondary">用户搜索并确认的参考地点 · GCJ-02 · 经度 {location.confirmed.longitude} · 纬度 {location.confirmed.latitude}</Typography>
      <Button sx={{ alignSelf: 'flex-start' }} onClick={() => { location.clear(); setShowResults(false); setSearched(false); setKeyword(''); }}>清除地点</Button>
    </Stack> : null}
  </Stack>;
}
