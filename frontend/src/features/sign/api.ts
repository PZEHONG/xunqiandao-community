import { post, request } from '../../api/client';
import { UnknownSubmissionError } from '../../api/errors';
import type { SignConfirmation, SignRemark } from './types';
export async function loadRemarks(account: string, epoch: number, signal: AbortSignal): Promise<SignRemark[]> {
  const data = await request<{ remarks?: unknown[] }>(`/api/sign/remarks?account=${encodeURIComponent(account)}`, { expectedEpoch: epoch, signal });
  const remarks = Array.isArray(data?.remarks) ? data.remarks.filter((mark): mark is SignRemark => !!mark && typeof mark === 'object' && 'key' in mark && typeof mark.key === 'string' && !!mark.key && 'value' in mark && typeof mark.value === 'string' && !!mark.value) : [];
  if (!remarks.length) throw new Error('账号暂未返回可用备注，请刷新重试。');
  return remarks;
}
export async function uploadPhoto(account: string, file: File, epoch: number, signal: AbortSignal) {
  const form = new FormData(); form.set('account', account); form.set('file', file);
  const data = await request<{ uri?: string }>('/api/sign/photos', { method: 'POST', body: form, expectedEpoch: epoch, signal, operation: 'write' });
  if (typeof data?.uri !== 'string' || !/^\/(?!\/)[^?,#\r\n\0\\]*$/.test(data.uri)) throw new UnknownSubmissionError();
  return data.uri;
}
export async function sendSign(snapshot: SignConfirmation, photos: string[], signal: AbortSignal) {
  const data = await post<{ message?: string; error?: string }>('/api/sign', { account: snapshot.account, ...snapshot.location, remark: snapshot.remark, comment: snapshot.comment, photos }, { expectedEpoch: snapshot.epoch, signal, operation: 'sign' });
  if (typeof data?.message !== 'string' || !data.message.trim() || data.error) throw new UnknownSubmissionError(true);
  return data.message.trim();
}
