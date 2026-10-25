import { post, request } from '../../api/client';
import { ApiError, UnknownSubmissionError } from '../../api/errors';
import { attachmentURL } from './reportKinds';
import type { ReportConfirmation, ReportField, ReportFilters, ReportKind, ReportRecord } from './types';
export async function listReports(account: string, kind: ReportKind, page: number, filters: ReportFilters, epoch: number, signal: AbortSignal) {
  const query = new URLSearchParams({ account, business_type: kind, page: String(page), page_size: '15', start_time: filters.start, end_time: filters.end, mark_state: filters.status });
  const result = await request<{ data?: { list?: unknown; count?: unknown } }>(`/api/reports?${query}`, { expectedEpoch: epoch, signal }); const data = result?.data;
  if (!data || !Array.isArray(data.list)) throw new ApiError(200, '报告列表格式不正确，请重新查询。');
  const rows = data.list.filter((row): row is ReportRecord => !!row && typeof row === 'object'); const count = Number(data.count ?? rows.length);
  return { rows, count: Number.isFinite(count) && count >= 0 ? count : rows.length };
}
export async function reportTemplate(account: string, kind: ReportKind, epoch: number, signal: AbortSignal): Promise<ReportField[]> {
  const value = await request<{ data?: unknown }>(`/api/reports/template?${new URLSearchParams({ account, business_type: kind })}`, { expectedEpoch: epoch, signal });
  if (!Array.isArray(value?.data) || !value.data.length || value.data.some(f => !f || typeof f.title !== 'string' || !f.title.trim() || !Number.isInteger(Number(f.sort)))) throw new ApiError(200, '报告模板暂时不可用，请稍后重试。');
  return value.data.map(f => ({ title: f.title, require: f.require === true || String(f.require) === '1' ? '1' : '0', sort: Number(f.sort), content: '' }));
}
export async function reportDetail(account: string, id: string, epoch: number, signal: AbortSignal): Promise<ReportRecord> { const r = await request<{ data?: unknown }>(`/api/reports/detail?${new URLSearchParams({ account, id })}`, { expectedEpoch: epoch, signal }); if (!r?.data || typeof r.data !== 'object' || Array.isArray(r.data)) throw new ApiError(200, '报告详情格式不正确。'); return r.data as ReportRecord; }
export async function uploadReport(account: string, file: File, epoch: number) { const body = new FormData(); body.append('account', account); body.append('file', file); const result = await request<{ uri?: unknown }>('/api/reports/attachments', { method: 'POST', body, expectedEpoch: epoch }); if (typeof result?.uri !== 'string' || !attachmentURL(result.uri)) throw new UnknownSubmissionError(); return result.uri; }
export async function submitReport(confirmation: ReportConfirmation, attachments: string[]) { const result = await post<{ message?: unknown }>('/api/reports', { ...confirmation.input, attachments }, { expectedEpoch: confirmation.epoch }); if (typeof result?.message !== 'string' || !result.message.trim()) throw new UnknownSubmissionError(); return result.message; }
