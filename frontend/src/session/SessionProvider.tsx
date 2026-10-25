import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { request, post } from '../api/client';
import { ApiError, StaleSessionError, UnknownSubmissionError } from '../api/errors';
import type { AuthStatus, SelfResponse } from '../api/contracts';
import { bumpSessionEpoch, getSessionEpoch } from './sessionEpoch';
import { SessionContext, type LoginPane, type SessionState } from './useSession';

type Identity = Pick<SessionState, 'status' | 'user' | 'bindings' | 'epoch' | 'error'>;
function broadcast() { try { localStorage.setItem('xun-auth-change', `${Date.now()}-${Math.random()}`); } catch { /* Current tab works independently. */ } }
export function SessionProvider({ children }: { children: ReactNode }) {
  const [identity, setIdentity] = useState<Identity>(() => ({ status: 'loading', user: null, bindings: [], epoch: getSessionEpoch(), error: '' }));
  const [authConfig, setAuthConfig] = useState<AuthStatus | null>(null);
  const [loginOpen, setLoginOpen] = useState(false); const [loginPane, setLoginPane] = useState<LoginPane>('login'); const [reconnecting, setReconnecting] = useState(false);
  const [authBusy, setAuthBusy] = useState(false); const [loginKey, setLoginKey] = useState(0); const [configCleared, setConfigCleared] = useState(false); const [message, setMessage] = useState('');
  const generation = useRef(0); const signature = useRef(''); const controller = useRef<AbortController | null>(null); const mutationBusy = useRef(false);
  const dialogOpen = useRef(false); const clearedSignature = useRef('');
  const closeDialog = useCallback(() => { dialogOpen.current = false; setLoginOpen(false); }, []);
  const openLogin = useCallback((pane: LoginPane = 'login') => { if (mutationBusy.current || dialogOpen.current) return; dialogOpen.current = true; setLoginKey(n => n + 1); setLoginPane(pane); setReconnecting(false); setLoginOpen(true); }, []);
  const invalidate = useCallback((status: Identity['status'] = 'loading', resetDialog = true) => {
    generation.current++; controller.current?.abort(); signature.current = '';
    if (resetDialog) closeDialog(); setMessage('');
    const epoch = bumpSessionEpoch(); setIdentity({ status, user: null, bindings: [], epoch, error: '' }); return epoch;
  }, [closeDialog]);
  const accept = useCallback((value: SelfResponse) => {
    if (!value.user || !Number.isFinite(value.user.id) || !Array.isArray(value.bindings)) throw new ApiError(502, '账号信息不完整，请重新登录。');
    const nextSignature = JSON.stringify([value.user.id, value.bindings]);
    const cleared = clearedSignature.current === nextSignature;
    if (!cleared) clearedSignature.current = ''; setConfigCleared(cleared);
    const epoch = signature.current === nextSignature ? getSessionEpoch() : bumpSessionEpoch(); signature.current = nextSignature;
    setIdentity({ status: 'authenticated', user: value.user, bindings: cleared ? [] : value.bindings, epoch, error: '' });
    const account = (value.user.student_number || value.user.username || '').trim().toLowerCase();
    try { if (account && value.consent_accepted && value.agreement_version) localStorage.setItem(`xun-agreement:${account}`, value.agreement_version); } catch { /* Optional browser storage. */ }
  }, []);
  const restore = useCallback(async () => {
    const seq = ++generation.current; controller.current?.abort(); const aborter = new AbortController(); controller.current = aborter;
    // Background checks must not dismiss a login form opened while in flight.
    try { const value = await request<SelfResponse>('/api/auth/self', { signal: aborter.signal }); if (seq !== generation.current) return; accept(value); }
    catch (error) {
      if (seq !== generation.current || aborter.signal.aborted || error instanceof StaleSessionError) return;
      // An unavailable check says nothing about the validity of the cookie.
      // Keep the current identity and page epoch until the server rejects it.
      if (!(error instanceof ApiError && error.status === 401)) {
        setIdentity(previous => ({ ...previous, error: error instanceof Error ? error.message : '账号服务暂时无法连接。' }));
        return;
      }
      signature.current = ''; const epoch = bumpSessionEpoch();
      setIdentity({ status: 'anonymous', user: null, bindings: [], epoch, error: '' });
    }
  }, [accept]);
  const logout = useCallback(async () => {
    if (mutationBusy.current) return; mutationBusy.current = true; setAuthBusy(true); clearedSignature.current = ''; setConfigCleared(false); invalidate('anonymous');
    try { await post('/api/auth/logout', {}); broadcast(); setMessage('已退出登录。'); }
    catch (error) {
      if (error instanceof StaleSessionError) { invalidate(); await restore(); broadcast(); }
      else if (error instanceof ApiError && error.status === 401) { broadcast(); setMessage('当前已退出登录。'); }
      else { await restore(); broadcast(); setIdentity(previous => ({ ...previous, error: error instanceof UnknownSubmissionError ? '退出结果暂时无法确认，已重新核对登录状态，请确认当前账号后再操作。' : '退出未成功，请重试。' })); }
    }
    finally { mutationBusy.current = false; setAuthBusy(false); }
  }, [invalidate, restore]);
  const authenticate = useCallback(async (pane: LoginPane, data: Record<string, unknown>) => {
    if (mutationBusy.current) return; mutationBusy.current = true; setAuthBusy(true);
    const epoch = invalidate('loading', false); const seq = generation.current;
    try {
      await post(`/api/auth/${pane === 'official' ? 'official' : 'login'}`, data, { expectedEpoch: epoch });
      const value = await request<SelfResponse>('/api/auth/self', { expectedEpoch: epoch });
      if (seq !== generation.current) throw new StaleSessionError();
      clearedSignature.current = ''; accept(value); closeDialog(); setReconnecting(false); setMessage('登录成功，已更新账号关联。'); broadcast();
    } catch (error) {
      // The browser may have applied Set-Cookie even when its JSON belongs to
      // an obsolete epoch. Reconcile the actual shared cookie, never that body.
      if (error instanceof StaleSessionError) { invalidate(); await restore(); broadcast(); return; }
      if (seq === generation.current) setIdentity(previous => ({ ...previous, status: 'anonymous', error: '' }));
      throw error;
    } finally { mutationBusy.current = false; setAuthBusy(false); }
  }, [accept, invalidate, restore, closeDialog]);
  const reconnect = useCallback(() => { if (mutationBusy.current || dialogOpen.current) return; dialogOpen.current = true; setLoginKey(n => n + 1); setLoginPane('login'); setReconnecting(true); setLoginOpen(true); }, []);
  const clearConfig = useCallback(() => {
    if (mutationBusy.current || identity.status !== 'authenticated' || !identity.user || identity.epoch !== getSessionEpoch()) return;
    let removed = true; try { localStorage.removeItem(`xixun-workspace-config:${identity.user.id}`); } catch { removed = false; }
    generation.current++; controller.current?.abort(); closeDialog();
    if (!clearedSignature.current) clearedSignature.current = JSON.stringify([identity.user.id, identity.bindings]);
    setConfigCleared(true); setIdentity({ ...identity, bindings: [], epoch: bumpSessionEpoch(), error: removed ? '' : '浏览器配置未能删除，当前页面的临时连接已清空；请检查浏览器存储权限后重试。' });
    setMessage(removed ? '已清除浏览器配置，服务器账号、报告草稿和每日任务仍保留。' : '');
  }, [identity, closeDialog]);
  const cancelRestore = useCallback(() => { generation.current++; controller.current?.abort(); }, []);
  useEffect(() => {
    if (authConfig) return;
    const configController = new AbortController();
    void request<AuthStatus>('/api/auth/status', { signal: configController.signal }).then(value => { if (!configController.signal.aborted) setAuthConfig(value); }).catch(() => { /* Login stays disabled without a protocol version. */ });
    return () => configController.abort();
  }, [identity.epoch, authConfig]);
  useEffect(() => {
    if (!identity.error || identity.status === 'anonymous') return;
    const retry = setInterval(() => {
      if (!document.hidden && !mutationBusy.current && !dialogOpen.current) void restore();
    }, 30000);
    return () => clearInterval(retry);
  }, [identity.error, identity.status, restore]);
  useEffect(() => {
    void restore();
    const storage = (event: StorageEvent) => { if (event.key === 'xun-auth-change') { invalidate(); void restore(); } };
    const authError = (event: Event) => { const detail = (event as CustomEvent<{ epoch: number; code: string }>).detail; if (!detail || detail.epoch !== getSessionEpoch()) return; if (detail.code === 'AUTH_REQUIRED') { invalidate('anonymous'); openLogin(); } else reconnect(); };
    const visible = () => { if (!document.hidden && !mutationBusy.current && !dialogOpen.current) void restore(); };
    const pageshow = (event: PageTransitionEvent) => { if (event.persisted) visible(); };
    const interval = setInterval(visible, 60 * 60 * 1000);
    window.addEventListener('storage', storage); window.addEventListener('xun-api-auth-error', authError); window.addEventListener('pageshow', pageshow); window.addEventListener('online', visible); document.addEventListener('visibilitychange', visible);
    return () => { cancelRestore(); clearInterval(interval); window.removeEventListener('storage', storage); window.removeEventListener('xun-api-auth-error', authError); window.removeEventListener('pageshow', pageshow); window.removeEventListener('online', visible); document.removeEventListener('visibilitychange', visible); };
  }, [invalidate, openLogin, reconnect, restore, cancelRestore]);
  return <SessionContext.Provider value={{ ...identity, authConfig, loginOpen, loginPane, reconnecting, authBusy, loginKey, configCleared, message, clearConfig, restore, logout, openLogin, closeLogin: () => { if (!mutationBusy.current) closeDialog(); }, reconnect, authenticate }}>{children}</SessionContext.Provider>;
}
