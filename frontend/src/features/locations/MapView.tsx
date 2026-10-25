import { useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, IconButton, Stack, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import RemoveIcon from '@mui/icons-material/Remove';
import { loadAMap } from './amapLoader';
import { coordinates, type AMapInstance, type AMapLayer, type AMapMarker, type MapConfig, type MapListener, type Point } from './types';

interface Props { config?: MapConfig; point: Point | null; onPick: (point: Point) => void; configError?: string; onRetry?: () => void }
interface ActiveMap { map: AMapInstance; marker: AMapMarker; layers: AMapLayer[] }
export function MapView({ config, point, onPick, configError = '', onRetry }: Props) {
  const container = useRef<HTMLDivElement>(null); const frame = useRef<HTMLDivElement>(null); const active = useRef<ActiveMap | null>(null);
  const latest = useRef({ point, onPick });
  const [attempt, setAttempt] = useState(0); const [state, setState] = useState({ ready: false, error: '', zoom: 5, satellite: false });
  useEffect(() => { latest.current = { point, onPick }; }, [point, onPick]);
  useEffect(() => {
    let disposed = false; let destroyed = false;
    const disposers: (() => void)[] = [];
    const release = () => {
      if (destroyed) return; destroyed = true;
      // Register each allocation immediately; one broken SDK destructor must
      // not prevent the remaining listeners and resources from being released.
      for (const dispose of disposers.reverse()) { try { dispose(); } catch { /* Continue releasing the remaining resources. */ } }
      disposers.length = 0;
    };
    setState({ ready: false, error: '', zoom: 5, satellite: false });
    if (!config?.map_enabled || !container.current) return;
    void (async () => {
      try {
        const sdk = await loadAMap(config); if (disposed || !container.current) return;
        const layers: AMapLayer[] = [];
        for (const Layer of [sdk.TileLayer, sdk.TileLayer.Satellite, sdk.TileLayer.RoadNet]) { const layer = new Layer(); layers.push(layer); disposers.push(() => layer.destroy?.()); }
        const initialPoint = latest.current.point;
        const map = new sdk.Map(container.current, { viewMode: '2D', zoom: initialPoint ? 16 : 5, zooms: [2, 20], center: initialPoint || [104, 35], layers: [layers[0]], zoomEnable: true, scrollWheel: false, touchZoom: true, doubleClickZoom: true });
        disposers.push(() => { if (active.current?.map === map) active.current = null; map.destroy(); });
        const marker = new sdk.Marker({ position: initialPoint || [104, 35], draggable: true, title: '拖动标记查看参考地点', map });
        disposers.push(() => marker.setMap(null));
        if (!initialPoint) marker.hide();
        const value = { map, marker, layers }; active.current = value;
        let ready = false;
        const listen = (name: string, handler: MapListener) => { disposers.push(() => map.off(name, handler)); map.on(name, handler); };
        const pick: MapListener = event => { const selected = coordinates(event.lnglat?.getLng(), event.lnglat?.getLat()); if (ready && selected) latest.current.onPick(selected); };
        const resize = () => { if (!disposed && !destroyed) map.resize(); };
        const observer = new ResizeObserver(resize); disposers.push(() => observer.disconnect()); observer.observe(container.current);
        disposers.push(() => window.removeEventListener('resize', resize));
        window.addEventListener('resize', resize);
        const timer = setTimeout(() => fail('底图加载超时，请检查网络与地图配置后重新连接。'), 15000);
        disposers.push(() => clearTimeout(timer));
        function fail(message: string) { if (disposed || destroyed) return; release(); setState(previous => ({ ...previous, ready: false, error: message })); }
        listen('complete', () => { if (disposed || destroyed) return; clearTimeout(timer); ready = true; setState(previous => ({ ...previous, ready: true, zoom: map.getZoom() })); resize(); });
        listen('error', () => fail('底图加载失败，请重新连接。'));
        listen('zoomchange', () => { if (!disposed && !destroyed) setState(previous => ({ ...previous, zoom: map.getZoom() })); });
        listen('click', pick); disposers.push(() => marker.off('dragend', pick)); marker.on('dragend', pick);
        let delta = 0; let wheelTime = 0; let lastZoom = -Infinity;
        const wheel = (event: WheelEvent) => {
          if (!ready || destroyed || !event.deltaY) return;
          event.preventDefault(); event.stopPropagation(); const now = performance.now();
          if (now - wheelTime > 200 || Math.sign(delta) !== Math.sign(event.deltaY)) delta = 0;
          wheelTime = now; delta += event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? (frame.current?.clientHeight || 320) : 1);
          if (Math.abs(delta) < 40 || now - lastZoom < 100) return;
          map.setZoom(Math.max(2, Math.min(20, map.getZoom() + (delta < 0 ? 1 : -1))), true); delta = 0; lastZoom = now;
        };
        const wheelTarget = frame.current;
        disposers.push(() => wheelTarget?.removeEventListener('wheel', wheel, true));
        wheelTarget?.addEventListener('wheel', wheel, { capture: true, passive: false });
        resize();
      } catch (error) {
        release(); if (!disposed) setState(previous => ({ ...previous, ready: false, error: error instanceof Error ? error.message : '高德地图暂时不可用。' }));
      }
    })();
    return () => { disposed = true; release(); };
  }, [config, attempt]);
  useEffect(() => {
    const value = active.current; if (!value) return;
    if (point) { value.marker.setPosition(point); value.marker.show(); value.map.setZoomAndCenter(16, point); }
    else { value.marker.hide(); value.map.setZoomAndCenter(5, [104, 35]); }
  }, [point]);
  const changeZoom = (delta: number) => { const map = active.current?.map; if (state.ready && map) map.setZoom(Math.max(2, Math.min(20, map.getZoom() + delta)), true); };
  const layer = (satellite: boolean) => { const value = active.current; if (!value || !state.ready) return; value.map.setLayers(satellite ? value.layers.slice(1) : [value.layers[0]]); setState(previous => ({ ...previous, satellite })); };
  const error = configError || state.error;
  const title = error ? '高德地图暂时不可用' : !config ? '正在读取地图配置' : !config.map_enabled ? '高德地图尚未连接' : '正在连接高德地图';
  return <Stack gap={1}>
    <Stack direction="row" flexWrap="wrap" gap={1} alignItems="center">
      <Button size="small" disabled={!state.ready} aria-pressed={!state.satellite} onClick={() => layer(false)}>标准图</Button>
      <Button size="small" disabled={!state.ready} aria-pressed={state.satellite} onClick={() => layer(true)}>卫星图</Button>
      <Button size="small" disabled={!state.ready || !point} onClick={() => { if (point) active.current?.map.setZoomAndCenter(16, point); }}>定位点回中</Button>
    </Stack>
    <Box ref={frame} role="region" aria-label="地点地图" aria-busy={!state.ready && !!config?.map_enabled && !error} sx={{ position: 'relative', height: { xs: 300, sm: 360 }, border: '1px solid', borderColor: 'divider', borderRadius: 1, overflow: 'hidden', bgcolor: 'background.default' }}>
      <Box ref={container} sx={{ position: 'absolute', inset: 0, '& .amap-logo, & .amap-copyright': { display: 'none !important' } }} />
      {!state.ready && <Stack sx={{ position: 'absolute', inset: 0, p: 2, justifyContent: 'center', alignItems: 'center', textAlign: 'center', bgcolor: 'background.default' }} gap={1}>
        <Typography fontWeight={600}>{title}</Typography><Typography variant="body2" color="text.secondary">{error || config?.message || (config?.search_enabled ? '地址搜索仍可使用，核对完整地址后明确确认地点。' : '请完成高德配置后搜索并确认地点。')}</Typography>
        <Button onClick={() => { onRetry?.(); setAttempt(value => value + 1); }}>重新连接地图</Button>
      </Stack>}
      {state.ready && <Stack sx={{ position: 'absolute', top: 12, right: 12, bgcolor: 'background.paper', borderRadius: 1, boxShadow: 1 }}>
        <IconButton aria-label="放大地图" disabled={state.zoom >= 20} onClick={() => changeZoom(1)} sx={{ width: 44, height: 44 }}><AddIcon /></IconButton>
        <IconButton aria-label="缩小地图" disabled={state.zoom <= 2} onClick={() => changeZoom(-1)} sx={{ width: 44, height: 44 }}><RemoveIcon /></IconButton>
      </Stack>}
    </Box>
    {state.ready && config && !config.search_enabled && <Alert severity="warning">地点搜索尚未连接，暂时无法选择或调整地点。</Alert>}
  </Stack>;
}
