import { afterEach, expect, it, vi } from 'vitest';
const settings = { map_enabled: true, search_enabled: true, js_key: 'fixture-key', security_proxy: '/_AMapService', default_city: '北京' };
afterEach(() => { document.querySelectorAll('script[data-xun-amap]').forEach(node => node.remove()); Reflect.deleteProperty(window, 'AMap'); vi.useRealTimers(); vi.resetModules(); });
it('并发与重挂载共用一次SDK注入，安全代理保持同源，Key变化要求刷新', async () => {
  const { loadAMap } = await import('./amapLoader');
  const one = loadAMap(settings); const two = loadAMap(settings);
  expect(document.querySelectorAll('script[data-xun-amap]')).toHaveLength(1);
  expect(window._AMapSecurityConfig?.serviceHost).toBe(`${location.origin}/_AMapService`);
  const script = document.querySelector<HTMLScriptElement>('script[data-xun-amap]')!;
  const callback = new URL(script.src).searchParams.get('callback')!;
  Object.assign(window, { AMap: { Map: class {} } });
  (window as unknown as Record<string, () => void>)[callback]();
  expect(await one).toBe(await two); await expect(loadAMap({ ...settings, js_key: 'other-key' })).rejects.toThrow(/刷新/);
});
it('失败移除脚本并允许重试；超时不会保留未完成promise', async () => {
  vi.useFakeTimers(); const { loadAMap } = await import('./amapLoader');
  const fail = expect(loadAMap(settings)).rejects.toThrow(/连接失败/);
  document.querySelector('script')!.dispatchEvent(new Event('error')); await fail;
  expect(document.querySelectorAll('script[data-xun-amap]')).toHaveLength(0);
  const timeout = expect(loadAMap(settings)).rejects.toThrow(/超时/);
  await vi.advanceTimersByTimeAsync(15000); await timeout;
  expect(document.querySelectorAll('script[data-xun-amap]')).toHaveLength(0);
});
it('失败后换Key也要求刷新，避免晚到的旧SDK与新Key混用', async () => {
  vi.useFakeTimers(); const { loadAMap } = await import('./amapLoader');
  const fail = expect(loadAMap(settings)).rejects.toThrow(/连接失败/);
  document.querySelector('script')!.dispatchEvent(new Event('error')); await fail;
  const changed = loadAMap({ ...settings, js_key: 'other-key' }).then(() => '', (error: Error) => error.message);
  await vi.advanceTimersByTimeAsync(15000); expect(await changed).toMatch(/刷新/);
});
