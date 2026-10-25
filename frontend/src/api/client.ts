import { bumpSessionEpoch, getSessionEpoch, type SessionEpoch } from '../session/sessionEpoch';
import { ApiError, StaleSessionError, UnknownSubmissionError } from './errors';
export { bumpSessionEpoch };
export type RequestOptions = RequestInit & { timeoutMs?: number; expectedEpoch?: SessionEpoch; operation?: 'read' | 'write' | 'sign' };
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { timeoutMs, expectedEpoch = getSessionEpoch(), operation = options.method && !['GET', 'HEAD'].includes(options.method.toUpperCase()) ? 'write' : 'read', signal, ...init } = options;
  const controller = new AbortController(); let timedOut = false;
  const abort = () => controller.abort(signal?.reason);
  if (signal?.aborted) abort(); else signal?.addEventListener('abort', abort, { once: true });
  const timer = setTimeout(() => { timedOut = true; controller.abort(new DOMException('请求超时', 'TimeoutError')); }, timeoutMs ?? (operation === 'sign' ? 60000 : 30000));
  try {
    const response = await fetch(path, { ...init, credentials: 'same-origin', cache: 'no-store', signal: controller.signal });
    if (expectedEpoch !== getSessionEpoch()) throw new StaleSessionError();
    let value: unknown;
    try { value = await response.json(); } catch { if (operation !== 'read') throw new UnknownSubmissionError(operation === 'sign'); throw new ApiError(response.status, '服务返回内容无法解析，请稍后刷新。'); }
    if (expectedEpoch !== getSessionEpoch()) throw new StaleSessionError();
    if (signal?.aborted) throw signal.reason ?? new DOMException('已取消', 'AbortError');
    if (!response.ok) {
      // The upstream may have accepted a write before its gateway response failed.
      if (operation !== 'read' && response.status >= 500) throw new UnknownSubmissionError(operation === 'sign');
      const body = value && typeof value === 'object' ? value as Record<string, unknown> : {};
      const code = typeof body.code === 'string' ? body.code : undefined;
      const error = new ApiError(response.status, String(body.error || body.message || '请求未成功，请稍后重试。'), code);
      if (['AUTH_REQUIRED', 'UPSTREAM_LOGIN_REQUIRED'].includes(code ?? '') && !path.startsWith('/api/auth/') && typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('xun-api-auth-error', { detail: { code, epoch: expectedEpoch } }));
      throw error;
    }
    return value as T;
  } catch (error) {
    if (expectedEpoch !== getSessionEpoch()) throw new StaleSessionError();
    if (error instanceof ApiError || error instanceof StaleSessionError) throw error;
    if (operation !== 'read') throw new UnknownSubmissionError(operation === 'sign');
    if (signal?.aborted) throw signal.reason ?? new DOMException('已取消', 'AbortError');
    throw new ApiError(0, timedOut ? '请求超时，请稍后刷新。' : '无法连接服务，请稍后重试。', timedOut ? 'TIMEOUT' : 'NETWORK_ERROR');
  } finally { clearTimeout(timer); signal?.removeEventListener('abort', abort); }
}
export function post<T>(path: string, data: unknown, options: RequestOptions = {}) { return request<T>(path, { ...options, method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) }); }
