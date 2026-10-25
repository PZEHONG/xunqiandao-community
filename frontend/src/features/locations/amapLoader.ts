import type { AMapSDK, MapConfig } from './types';
let loading: Promise<AMapSDK> | undefined; let activeKey = ''; let sequence = 0;
export function loadAMap(config: MapConfig): Promise<AMapSDK> {
  if (!config.map_enabled || !config.js_key || config.security_proxy !== '/_AMapService') return Promise.reject(new Error('高德地图尚未配置，请检查配置后重试。'));
  if (activeKey && activeKey !== config.js_key) return Promise.reject(new Error('地图Key已更新，请刷新页面后连接。'));
  window._AMapSecurityConfig = { serviceHost: `${location.origin}${config.security_proxy}` };
  activeKey = config.js_key;
  if (window.AMap) return Promise.resolve(window.AMap);
  if (loading) return loading;
  loading = new Promise<AMapSDK>((resolve, reject) => {
    const script = document.createElement('script'); const callback = `xunMapReady${++sequence}`; let settled = false;
    const timer = setTimeout(() => finish(new Error('地图连接超时，请重新连接。')), 15000);
    function finish(error?: Error) {
      if (settled) return; settled = true; clearTimeout(timer); script.onerror = null;
      // Keep a no-op for a callback from an already downloaded but timed-out script.
      Object.assign(window, { [callback]: () => {} });
      if (error) { script.remove(); loading = undefined; reject(error); }
      else resolve(window.AMap!);
    }
    Object.assign(window, { [callback]: () => finish(window.AMap ? undefined : new Error('地图暂时无法加载。')) });
    script.dataset.xunAmap = ''; script.async = true;
    script.onerror = () => finish(new Error('地图连接失败，请检查网络后重新连接。'));
    script.src = `https://webapi.amap.com/maps?v=2.0&key=${encodeURIComponent(config.js_key!)}&callback=${callback}`;
    document.head.append(script);
  });
  return loading;
}
