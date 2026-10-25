export type Point = [longitude: number, latitude: number];
export interface LocationResult {
  id?: string; name: string; address: string; latitude: string; longitude: string; confirmation_token: string;
  province?: string; city?: string; district?: string; coordinate_system?: string; source?: string;
}
export interface ConfirmedLocation {
  readonly address: string; readonly address_name: string; readonly latitude: string; readonly longitude: string;
  readonly location_confirmed: true; readonly confirmation_token: string;
}
export interface MapConfig { map_enabled: boolean; search_enabled: boolean; js_key?: string; security_proxy?: string; default_city?: string; message?: string }
export function coordinates(lon: unknown, lat: unknown): Point | null {
  if ((typeof lon !== 'string' && typeof lon !== 'number') || (typeof lat !== 'string' && typeof lat !== 'number') || String(lon).trim() === '' || String(lat).trim() === '') return null;
  const point: Point = [Number(lon), Number(lat)];
  return point.every(Number.isFinite) && Math.abs(point[0]) <= 180 && Math.abs(point[1]) <= 90 ? point : null;
}
export function validPlace(value: unknown): value is LocationResult {
  if (!value || typeof value !== 'object') return false;
  const place = value as Partial<LocationResult>;
  return typeof place.address === 'string' && !!place.address.trim() && typeof place.confirmation_token === 'string' && !!place.confirmation_token && typeof place.name === 'string' && typeof place.latitude === 'string' && typeof place.longitude === 'string' && !!coordinates(place.longitude, place.latitude);
}
export interface MapEvent { lnglat?: { getLng: () => number; getLat: () => number } }
export type MapListener = (event: MapEvent) => void;
export interface AMapLayer { destroy?: () => void }
export interface AMapInstance {
  on: (event: string, callback: MapListener) => void; off: (event: string, callback: MapListener) => void;
  destroy: () => void; resize: () => void; getZoom: () => number; setZoom: (zoom: number, immediately?: boolean) => void;
  setZoomAndCenter: (zoom: number, point: Point) => void; setLayers: (layers: AMapLayer[]) => void;
}
export interface AMapMarker {
  on: (event: string, callback: MapListener) => void; off: (event: string, callback: MapListener) => void;
  setPosition: (point: Point) => void; show: () => void; hide: () => void; setMap: (map: AMapInstance | null) => void;
}
export interface AMapSDK {
  Map: new (container: HTMLDivElement, options: Record<string, unknown>) => AMapInstance;
  Marker: new (options: Record<string, unknown>) => AMapMarker;
  TileLayer: { new (): AMapLayer; Satellite: new () => AMapLayer; RoadNet: new () => AMapLayer };
}
declare global { interface Window { AMap?: AMapSDK; _AMapSecurityConfig?: { serviceHost: string } } }
