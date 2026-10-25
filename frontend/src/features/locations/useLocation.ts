import { useCallback, useEffect, useRef, useState } from 'react';
import { useSession } from '../../session/useSession';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { reverseLocation, searchLocations } from './api';
import { coordinates, type ConfirmedLocation, type LocationResult, type Point, validPlace } from './types';

export function useLocation() {
  const { epoch, user, bindings, status } = useSession();
  const owner = `${epoch}:${status}:${user?.id ?? ''}:${bindings.map(binding => binding.account).join(',')}`;
  const [state, setState] = useState({ owner, candidate: null as LocationResult | null, saved: null as ConfirmedLocation | null, pending: false, point: null as Point | null, results: [] as LocationResult[], searching: false, resolving: false, searched: false, error: '' });
  const searchJob = useRef<{ version: number; controller?: AbortController }>({ version: 0 });
  const selectionJob = useRef<{ version: number; controller?: AbortController }>({ version: 0 });
  const cancelSearch = useCallback(() => { searchJob.current.version++; searchJob.current.controller?.abort(); }, []);
  const cancelSelection = useCallback(() => { selectionJob.current.version++; selectionJob.current.controller?.abort(); }, []);
  const clear = useCallback(() => {
    cancelSearch(); cancelSelection();
    setState({ owner, candidate: null, saved: null, pending: false, point: null, results: [], searching: false, resolving: false, searched: false, error: '' });
  }, [owner, cancelSearch, cancelSelection]);
  useEffect(() => { clear(); return () => { cancelSearch(); cancelSelection(); }; }, [clear, cancelSearch, cancelSelection]);
  const current = state.owner === owner && epoch === getSessionEpoch();
  const preview = useCallback((place: LocationResult) => {
    cancelSelection();
    if (!validPlace(place)) { setState(value => ({ ...value, error: '此地点缺少完整地址、坐标或确认凭据，请重新搜索。' })); return; }
    setState(value => value.owner === owner ? { ...value, candidate: { ...place }, pending: true, point: coordinates(place.longitude, place.latitude), searched: true, resolving: false, error: '' } : value);
  }, [owner, cancelSelection]);
  const confirm = useCallback(() => {
    setState(value => {
      if (value.owner !== owner || epoch !== getSessionEpoch() || !value.pending || value.resolving || !validPlace(value.candidate)) return value;
      const place = value.candidate;
      const saved: ConfirmedLocation = Object.freeze({ address: place.address, address_name: place.name || '', latitude: place.latitude, longitude: place.longitude, location_confirmed: true, confirmation_token: place.confirmation_token });
      return { ...value, saved, candidate: null, pending: false, error: '' };
    });
  }, [owner, epoch]);
  const cancel = useCallback(() => {
    cancelSelection();
    setState(value => ({ ...value, candidate: null, pending: false, resolving: false, point: value.saved ? coordinates(value.saved.longitude, value.saved.latitude) : null, error: '' }));
  }, [cancelSelection]);
  const invalidateSearch = useCallback(() => { cancelSearch(); setState(value => ({ ...value, results: [], searching: false, error: '' })); }, [cancelSearch]);
  const search = useCallback(async (keyword: string, city: string) => {
    cancelSearch(); const version = searchJob.current.version;
    if (!keyword.trim() || keyword.trim().length > 80 || city.trim().length > 40) { setState(value => ({ ...value, error: '请输入1–80字的地点名称或地址，城市不超过40字。', results: [], searching: false })); return; }
    const controller = new AbortController(); searchJob.current.controller = controller;
    setState(value => ({ ...value, searching: true, results: [], error: '' }));
    try {
      const results = await searchLocations(keyword.trim(), city.trim(), controller.signal, epoch);
      if (version === searchJob.current.version && epoch === getSessionEpoch()) setState(value => value.owner === owner ? { ...value, results, searching: false } : value);
    } catch (error) {
      if (version === searchJob.current.version && !controller.signal.aborted && epoch === getSessionEpoch()) setState(value => value.owner === owner ? { ...value, searching: false, error: error instanceof Error ? error.message : '地址搜索失败，请重试。' } : value);
    }
  }, [cancelSearch, epoch, owner]);
  const pickPoint = useCallback(async (point: Point) => {
    if (!current || !state.searched) { setState(value => ({ ...value, error: '请先搜索并选择地点，再用地图调整参考位置。' })); return; }
    if (!coordinates(point[0], point[1])) return;
    cancelSelection(); const version = selectionJob.current.version;
    const controller = new AbortController(); selectionJob.current.controller = controller;
    setState(value => ({ ...value, candidate: null, pending: true, point, resolving: true, error: '' }));
    try {
      const candidate = await reverseLocation(point[0], point[1], controller.signal, epoch);
      if (version === selectionJob.current.version && epoch === getSessionEpoch()) setState(value => value.owner === owner ? { ...value, candidate, point: coordinates(candidate.longitude, candidate.latitude), resolving: false } : value);
    } catch (error) {
      if (version === selectionJob.current.version && !controller.signal.aborted && epoch === getSessionEpoch()) setState(value => value.owner === owner ? { ...value, resolving: false, error: error instanceof Error ? error.message : '无法识别此位置，请重试。' } : value);
    }
  }, [current, state.searched, owner, epoch, cancelSelection]);
  return { candidate: current ? state.candidate : null, confirmed: current && !state.pending ? state.saved : null, pending: current && state.pending, point: current ? state.point : null, results: current ? state.results : [], busy: current && (state.searching || state.resolving), searching: current && state.searching, resolving: current && state.resolving, error: current ? state.error : '', search, preview, confirm, clear, cancel, pickPoint, invalidateSearch };
}
