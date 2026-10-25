import { useCallback, useEffect, useRef, useState } from 'react';
import { useSession } from '../../session/useSession';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { StaleSessionError } from '../../api/errors';
import type { SignSnapshot } from '../../api/contracts';
import { currentMonth } from './calendar';
import { queryRecords } from './api';
export function useRecords() {
  const { epoch, bindings, status } = useSession(); const account = status === 'authenticated' ? bindings[0]?.account ?? '' : '';
  const [state, setState] = useState<{ epoch: number; account: string; month: string; snapshot: SignSnapshot | null; busy: boolean; error: string }>({ epoch, account, month: currentMonth(), snapshot: null, busy: false, error: '' });
  const sequence = useRef(0); const controller = useRef<AbortController | null>(null); const monthRef = useRef(currentMonth());
  const loadMonth = useCallback(async (month: string) => {
    if (!account) return;
    const seq = ++sequence.current; controller.current?.abort(); const aborter = new AbortController(); controller.current = aborter;
    setState(previous => ({ ...previous, epoch, account, busy: true, error: '' }));
    try { const snapshot = await queryRecords(account, month, epoch, aborter.signal); if (seq !== sequence.current || epoch !== getSessionEpoch()) return; monthRef.current = snapshot.month_date; setState({ epoch, account, month: snapshot.month_date, snapshot, busy: false, error: '' }); }
    catch (error) { if (seq !== sequence.current || aborter.signal.aborted || epoch !== getSessionEpoch() || error instanceof StaleSessionError) return; setState(previous => ({ ...previous, busy: false, error: error instanceof Error ? error.message : '查询失败，请刷新。' })); }
  }, [account, epoch]);
  const cancelRequests = useCallback(() => { sequence.current++; controller.current?.abort(); }, []);
  useEffect(() => {
    monthRef.current = currentMonth(); setState({ epoch, account, month: monthRef.current, snapshot: null, busy: false, error: '' });
    if (account) void loadMonth(monthRef.current);
    return cancelRequests;
  }, [account, epoch, loadMonth, cancelRequests]);
  const visible = state.epoch === epoch && state.account === account ? state : { month: currentMonth(), snapshot: null, busy: false, error: '' };
  return { ...visible, loadMonth, refresh: () => loadMonth(monthRef.current) };
}
