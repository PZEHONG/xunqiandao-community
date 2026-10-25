import { expect, it, vi } from 'vitest';
import { searchLocations, reverseLocation } from './api';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { ApiError, UnknownSubmissionError } from '../../api/errors';
it.each(['network', 'non-json'])('搜索与反查是只读查询，%s失败不能提示提交结果未知', async mode => {
  const fetch = vi.fn<typeof globalThis.fetch>(() => mode === 'network' ? Promise.reject(new TypeError('offline')) : Promise.resolve(new Response('<html>error</html>'))); vi.stubGlobal('fetch', fetch);
  const controller = new AbortController(); const epoch = getSessionEpoch();
  for (const work of [() => searchLocations('园区', '北京', controller.signal, epoch), () => reverseLocation(116.4, 39.9, controller.signal, epoch)]) {
    const error = await work().catch(error => error);
    expect(error).toBeInstanceOf(ApiError); expect(error).not.toBeInstanceOf(UnknownSubmissionError);
    expect(error.message).not.toMatch(/提交|查询结果/);
  }
  expect(fetch).toHaveBeenCalledTimes(2); expect(fetch.mock.calls[0]?.[1]).toMatchObject({ method: 'POST', credentials: 'same-origin', cache: 'no-store' });
});
