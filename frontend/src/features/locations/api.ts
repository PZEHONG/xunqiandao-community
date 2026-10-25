import { post, request } from '../../api/client';
import { type MapConfig, validPlace } from './types';
export const mapConfig = (signal: AbortSignal) => request<MapConfig>('/api/maps/config', { signal });
export async function searchLocations(keyword: string, city: string, signal: AbortSignal, expectedEpoch: number) {
  const data = await post<{ locations?: unknown[] }>('/api/locations/search', { keyword, city }, { signal, expectedEpoch, operation: 'read' });
  return Array.isArray(data.locations) ? data.locations.filter(validPlace) : [];
}
export async function reverseLocation(longitude: number, latitude: number, signal: AbortSignal, expectedEpoch: number) {
  const data = await post<{ location?: unknown }>('/api/locations/reverse', { longitude: longitude.toFixed(6), latitude: latitude.toFixed(6) }, { signal, expectedEpoch, operation: 'read' });
  if (!validPlace(data.location)) throw new Error('未找到完整地址，请重新选点或搜索地点。');
  return data.location;
}
