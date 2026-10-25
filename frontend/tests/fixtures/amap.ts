import type { Page } from '@playwright/test';
export const fixturePlace = { id: 'fixture-1', name: '示例园区', address: '北京市示例区示例路1号', latitude: '39.900000', longitude: '116.400000', confirmation_token: 'fixture-token', coordinate_system: 'GCJ-02', source: 'amap' };
const stub = String.raw`
(() => {
  const maps = [], markers = []; window.__mapTest = { maps, markers };
  class Map {
    handlers = {}; destroyed = false; resizeCount = 0; layers = []; zoom = 5;
    constructor(container, options) {
      this.options = options; this.zoom = options.zoom; maps.push(this);
      this.container = container; this.click = () => this.emit('click', { lnglat: { getLng: () => 116.5, getLat: () => 39.8 } });
      container.addEventListener('click', this.click); this.timer = setTimeout(() => this.emit('complete'), 20);
    }
    on(name, fn) { this.handlers[name] = fn; } off(name, fn) { if (this.handlers[name] === fn) delete this.handlers[name]; }
    emit(name, event = {}) { this.handlers[name]?.(event); }
    destroy() { this.destroyed = true; clearTimeout(this.timer); this.container.removeEventListener('click', this.click); }
    resize() { this.resizeCount++; } getZoom() { return this.zoom; }
    setZoom(zoom) { this.zoom = zoom; this.emit('zoomchange'); }
    setZoomAndCenter(zoom, point) { this.center = point; this.setZoom(zoom); }
    setLayers(layers) { this.layers = layers; }
  }
  class Layer {} Layer.Satellite = class extends Layer {}; Layer.RoadNet = class extends Layer {};
  class Marker {
    handlers = {}; constructor(options) { Object.assign(this, options); markers.push(this); }
    on(name, fn) { this.handlers[name] = fn; } off(name) { delete this.handlers[name]; }
    setPosition(point) { this.position = point; } show() {} hide() {} setMap(map) { this.map = map; }
  }
  window.AMap = { Map, Marker, TileLayer: Layer };
})();`;
export async function mockLocations(page: Page, options: { mapEnabled?: boolean; searchEnabled?: boolean; sdkFailure?: boolean; reverseFailure?: boolean; address?: string } = {}) {
  const reads: { path: string; body: unknown }[] = []; let sdkLoads = 0;
  await page.route('**/api/maps/config', route => route.fulfill({ json: { map_enabled: options.mapEnabled ?? true, search_enabled: options.searchEnabled ?? true, js_key: 'fixture-key', security_proxy: '/_AMapService', default_city: '北京' } }));
  await page.route('**/api/locations/*', async route => {
    const path = new URL(route.request().url()).pathname; reads.push({ path, body: route.request().postDataJSON() });
    if (path.endsWith('/reverse') && options.reverseFailure) return route.fulfill({ status: 502, json: { error: '模拟反查失败' } });
    return route.fulfill({ json: path.endsWith('/search') ? { locations: [{ ...fixturePlace, address: options.address || fixturePlace.address }] } : { location: { ...fixturePlace, name: '调整后的地点', address: '北京市示例区示例路2号', longitude: '116.500000', latitude: '39.800000' } } });
  });
  await page.route('https://webapi.amap.com/maps?*', route => {
    sdkLoads++; if (options.sdkFailure) return route.abort();
    const callback = new URL(route.request().url()).searchParams.get('callback')!;
    return route.fulfill({ contentType: 'text/javascript', body: `${stub}\nwindow[${JSON.stringify(callback)}]();` });
  });
  return { reads, sdkLoads: () => sdkLoads };
}
