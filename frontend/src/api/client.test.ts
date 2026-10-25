import { afterEach, describe, expect, it, vi } from 'vitest';
import { request } from './client';
import { ApiError, StaleSessionError, UnknownSubmissionError } from './errors';
import { bumpSessionEpoch } from '../session/sessionEpoch';
afterEach(() => vi.useRealTimers());
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
describe('同源请求与不可重发写入', () => {
  it('带 Cookie、禁缓存且保留真实封装', async () => {
    const fetcher = vi.fn().mockResolvedValue(json({ data: { records: [] } })); vi.stubGlobal('fetch', fetcher);
    expect(await request('/api/query')).toEqual({ data: { records: [] } });
    expect(fetcher.mock.calls[0][1]).toMatchObject({ credentials: 'same-origin', cache: 'no-store' });
  });
  it.each(['AUTH_REQUIRED', 'UPSTREAM_LOGIN_REQUIRED'])('保留认证错误码 %s', async code => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ error: '请连接账号', code }, 401)));
    await expect(request('/api/query')).rejects.toMatchObject({ status: 401, code, message: '请连接账号' });
  });
  it('非JSON读取失败且不伪装为成功', async () => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>'))); await expect(request('/api/query')).rejects.toBeInstanceOf(ApiError); });
  it.each(['write', 'sign'] as const)('%s响应不可解析时仅发送一次并标记结果未知', async operation => {
    const fetcher = vi.fn().mockResolvedValue(new Response('broken')); vi.stubGlobal('fetch', fetcher);
    await expect(request('/api/sign', { method: 'POST', operation })).rejects.toBeInstanceOf(UnknownSubmissionError);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it.each([['read', 30000], ['sign', 60000]] as const)('%s使用%d毫秒超时', async (operation, timeout) => {
    vi.useFakeTimers();
    const fetcher = vi.fn((_path, options: RequestInit) => new Promise((_resolve, reject) => { options.signal?.addEventListener('abort', () => reject(options.signal?.reason)); }));
    vi.stubGlobal('fetch', fetcher);
    const pending = request('/api/test', { operation });
    const checked = expect(pending).rejects.toBeInstanceOf(operation === 'sign' ? UnknownSubmissionError : ApiError);
    await vi.advanceTimersByTimeAsync(timeout); await checked; expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('外部取消传到网络且清理超时', async () => {
    const controller = new AbortController();
    vi.stubGlobal('fetch', vi.fn((_path, options: RequestInit) => new Promise((_resolve, reject) => { options.signal?.addEventListener('abort', () => reject(options.signal?.reason)); })));
    const pending = request('/api/query', { signal: controller.signal });
    controller.abort(); await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  });
  it('账号变化后晚到的成功响应被丢弃', async () => {
    let finish!: (value: Response) => void;
    vi.stubGlobal('fetch', vi.fn(() => new Promise(resolve => { finish = resolve; })));
    const pending = request('/api/query'); bumpSessionEpoch(); finish(json({ data: '旧账号' }));
    await expect(pending).rejects.toBeInstanceOf(StaleSessionError);
  });
  it('账号变化后晚到401不能使新账号退出', async () => {
    let finish!: (value: Response) => void;
    vi.stubGlobal('fetch', vi.fn(() => new Promise(resolve => { finish = resolve; })));
    const pending = request('/api/query'); bumpSessionEpoch(); finish(json({ code: 'AUTH_REQUIRED' }, 401));
    await expect(pending).rejects.toBeInstanceOf(StaleSessionError);
  });
  it.each(['write', 'sign'] as const)('%s写入收到JSON502仍是结果未知且只发送一次', async operation => {
    const fetcher = vi.fn().mockResolvedValue(json({ error: '读取上游响应失败' }, 502)); vi.stubGlobal('fetch', fetcher);
    await expect(request('/api/submission', { method: 'POST', operation })).rejects.toBeInstanceOf(UnknownSubmissionError);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it.each([400, 401, 409])('写入明确%d错误保留原语义', async status => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ error: '明确拒绝', code: status === 401 ? 'AUTH_REQUIRED' : 'CONFLICT' }, status)));
    await expect(request('/api/submission', { method: 'POST' })).rejects.toMatchObject({ status, message: '明确拒绝' });
  });
  it('JSON502读取仍显示可刷新的API错误', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ error: '读取失败' }, 502)));
    await expect(request('/api/query')).rejects.toMatchObject({ status: 502, message: '读取失败' });
  });
});
