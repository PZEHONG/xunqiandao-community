import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { StrictMode } from 'react';
import { beforeEach, expect, it, vi } from 'vitest';
import { MapView } from './MapView';
const fake = vi.hoisted(() => {
  const maps: { on: ReturnType<typeof vi.fn>; off: ReturnType<typeof vi.fn>; destroy: ReturnType<typeof vi.fn>; resize: ReturnType<typeof vi.fn>; emit: (event: string) => void }[] = [];
  class Map {
    handlers = new globalThis.Map<string, () => void>(); zoom = 5;
    on = vi.fn((event: string, listener: () => void) => this.handlers.set(event, listener));
    off = vi.fn((event: string) => this.handlers.delete(event)); destroy = vi.fn(); resize = vi.fn();
    constructor() { maps.push(this); } emit(event: string) { this.handlers.get(event)?.(); }
    getZoom() { return this.zoom; } setZoom(value: number) { this.zoom = value; this.emit('zoomchange'); }
    setZoomAndCenter = vi.fn(); setLayers = vi.fn();
  }
  class Layer { static Satellite = Layer; static RoadNet = Layer; destroy = vi.fn(); }
  class Marker { on = vi.fn(); off = vi.fn(); setPosition = vi.fn(); show = vi.fn(); hide = vi.fn(); setMap = vi.fn(); }
  return { maps, sdk: { Map, TileLayer: Layer, Marker }, load: vi.fn() };
});
vi.mock('./amapLoader', () => ({ loadAMap: fake.load }));
const config = { map_enabled: true, search_enabled: true, js_key: 'fixture', security_proxy: '/_AMapService', default_city: '' };
let resizeCallback: ResizeObserverCallback; const disconnect = vi.fn();
beforeEach(() => {
  fake.maps.length = 0; fake.load.mockReset().mockResolvedValue(fake.sdk); disconnect.mockClear();
  vi.stubGlobal('ResizeObserver', class { constructor(callback: ResizeObserverCallback) { resizeCallback = callback; } observe = vi.fn(); disconnect = disconnect; });
});
it('StrictMode及卸载清理地图、事件、observer；容器与窗口resize会重排', async () => {
  const props = { config, point: null, onPick: vi.fn() };
  const first = render(<StrictMode><MapView {...props} /></StrictMode>);
  await waitFor(() => expect(fake.maps).toHaveLength(1)); const map = fake.maps[0];
  act(() => map.emit('complete')); await screen.findByRole('button', { name: '放大地图' });
  const beforeResize = map.resize.mock.calls.length;
  act(() => { resizeCallback([], {} as ResizeObserver); fireEvent(window, new Event('resize')); });
  expect(map.resize).toHaveBeenCalledTimes(beforeResize + 2); first.unmount(); expect(map.destroy).toHaveBeenCalledOnce();
  fireEvent(window, new Event('resize')); expect(map.resize).toHaveBeenCalledTimes(beforeResize + 2);
  expect(map.off.mock.calls.map(call => call[0])).toEqual(expect.arrayContaining(['complete', 'error', 'click', 'zoomchange'])); expect(disconnect).toHaveBeenCalled();
  const second = render(<MapView {...props} />); await waitFor(() => expect(fake.maps).toHaveLength(2));
  second.unmount(); expect(fake.maps[1].destroy).toHaveBeenCalledOnce();
});
it('缩放边界2–20，按钮44px且滚轮不改变地点', async () => {
  const onPick = vi.fn(); render(<MapView config={config} point={null} onPick={onPick} />);
  await waitFor(() => expect(fake.maps).toHaveLength(1)); act(() => fake.maps[0].emit('complete'));
  const plus = screen.getByRole('button', { name: '放大地图' }); const minus = screen.getByRole('button', { name: '缩小地图' });
  for (let i = 0; i < 30; i++) fireEvent.click(plus); expect(plus).toBeDisabled();
  for (let i = 0; i < 30; i++) fireEvent.click(minus); expect(minus).toBeDisabled();
  fireEvent.wheel(screen.getByRole('region', { name: '地点地图' }), { deltaY: -120 }); expect(minus).toBeEnabled(); expect(onPick).not.toHaveBeenCalled();
});
it('未配置和SDK失败明确降级，不显示虚构地图；失败可重试', async () => {
  const { rerender } = render(<MapView config={{ ...config, map_enabled: false }} point={null} onPick={vi.fn()} />);
  expect(screen.getByText('高德地图尚未连接')).toBeVisible(); expect(fake.load).not.toHaveBeenCalled();
  fake.load.mockRejectedValueOnce(new Error('模拟SDK连接失败'));
  rerender(<MapView config={config} point={null} onPick={vi.fn()} />); await screen.findByText('模拟SDK连接失败');
  fireEvent.click(screen.getByRole('button', { name: '重新连接地图' })); await waitFor(() => expect(fake.maps).toHaveLength(1));
});
it('Marker初始化抛错时立即销毁已创建地图，重试不留旧实例', async () => {
  fake.load.mockResolvedValueOnce({ ...fake.sdk, Marker: class { constructor() { throw new Error('标记初始化失败'); } } });
  render(<MapView config={config} point={null} onPick={vi.fn()} />);
  await screen.findByText('标记初始化失败'); expect(fake.maps[0].destroy).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole('button', { name: '重新连接地图' }));
  await waitFor(() => expect(fake.maps).toHaveLength(2)); expect(fake.maps[0].destroy).toHaveBeenCalledOnce();
});
it('observer初始化失败仍释放map与marker；底图error立即移除滚轮监听', async () => {
  vi.stubGlobal('ResizeObserver', class { observe() { throw new Error('容器监听失败'); } disconnect = disconnect; });
  const first = render(<MapView config={config} point={null} onPick={vi.fn()} />);
  await screen.findByText('容器监听失败'); expect(fake.maps[0].destroy).toHaveBeenCalledOnce(); expect(disconnect).toHaveBeenCalledOnce(); first.unmount();
  vi.stubGlobal('ResizeObserver', class { observe = vi.fn(); disconnect = disconnect; });
  render(<MapView config={config} point={null} onPick={vi.fn()} />); await waitFor(() => expect(fake.maps).toHaveLength(2));
  act(() => fake.maps[1].emit('complete'));
  const remove = vi.spyOn(screen.getByRole('region', { name: '地点地图' }), 'removeEventListener');
  act(() => fake.maps[1].emit('error')); await screen.findByText('底图加载失败，请重新连接。');
  expect(remove).toHaveBeenCalledWith('wheel', expect.any(Function), true); expect(fake.maps[1].destroy).toHaveBeenCalledOnce();
});
