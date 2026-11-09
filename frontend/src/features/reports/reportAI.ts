import { request } from '../../api/client';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { validRange } from './reportKinds';
import type { ReportField, ReportKind } from './types';

export interface AIPeriod { start: string; end: string; state: 'missing' | 'existing' | 'overlap' }
export interface AIDraft { start: string; end: string; fields: ReportField[]; status: 'generating' | 'ready' | 'partial' }
export type AIEvent = { type: 'field'; index: number; field: ReportField } | { type: 'delta'; index: number; delta: string } | { type: 'done'; fields: ReportField[] } | { type: 'error'; error: string };
export interface AIDateSuggestion { start: string; end: string; reason: string }
function isField(value: unknown): value is ReportField {
 if (!value || typeof value !== 'object') return false;
 const f = value as ReportField;
 return typeof f.title === 'string' && !!f.title.trim() && typeof f.content === 'string' && f.content.length <= 50000 && Number.isInteger(f.sort);
}
export const aiDraftKey = (editorKey: string) => editorKey + ':ai-drafts-v1';
export function loadAIDrafts(key: string): AIDraft[] {
 try {
  const value: unknown = JSON.parse(localStorage.getItem(key) || '[]');
  if (!Array.isArray(value)) return [];
  return value.filter((d): d is AIDraft => d && typeof d.start === 'string' && typeof d.end === 'string' && Array.isArray(d.fields) && d.fields.every((f: ReportField) => f && typeof f.title === 'string' && typeof f.content === 'string' && Number.isInteger(f.sort))).map(d => ({ ...d, status: d.status === 'ready' ? 'ready' : 'partial' }));
 } catch { return []; }
}
export function saveAIDrafts(key: string, drafts: AIDraft[]): boolean { try { localStorage.setItem(key, JSON.stringify(drafts)); return true; } catch { return false; } }
export async function suggestReportDates(account: string, kind: ReportKind, mode: 'single' | 'batch', epoch: number, signal: AbortSignal) {
 const result = await request<AIDateSuggestion>(`/api/reports/ai/dates?${new URLSearchParams({ account, business_type: kind, mode })}`, { expectedEpoch: epoch, signal, timeoutMs: 110000 });
 if (!result || typeof result.start !== 'string' || typeof result.end !== 'string' || typeof result.reason !== 'string') throw new Error('报告日期建议格式不正确。');
 if (!validRange(result.start, result.end, true)) throw new Error('未能获取完整有效的起止日期，请重新智能填写。');
 return result;
}
export async function missingReports(account: string, kind: ReportKind, start: string, end: string, epoch: number, signal: AbortSignal) {
 const result = await request<{ periods: AIPeriod[] }>(`/api/reports/ai/missing?${new URLSearchParams({ account, business_type: kind, start, end })}`, { expectedEpoch: epoch, signal, timeoutMs: 110000 });
 if (!Array.isArray(result.periods) || result.periods.some(p => !p || typeof p.start !== 'string' || typeof p.end !== 'string' || !['missing', 'existing', 'overlap'].includes(p.state))) throw new Error('缺报查询结果不完整，请重试。');
 return result.periods;
}

export async function consumeAIStream(response: Response, onEvent: (event: AIEvent) => void) {
 if (!response.body) throw new Error('浏览器无法读取流式响应。');
 const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ''; let complete = false;
 try {
  for (;;) {
   const { done, value } = await reader.read(); buffer += decoder.decode(value, { stream: !done });
   buffer = buffer.replace(/\r\n/g, '\n');
   let split: number;
   while ((split = buffer.indexOf('\n\n')) >= 0) {
    const frame = buffer.slice(0, split); buffer = buffer.slice(split + 2);
    const data = frame.split('\n').filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n');
    if (!data) continue;
    const event = JSON.parse(data) as AIEvent;
    if (!event || typeof event !== 'object' || !['field', 'delta', 'done', 'error'].includes(event.type)) throw new Error('AI 数据格式错误。');
    if ((event.type === 'field' && (!Number.isInteger(event.index) || event.index < 0 || event.index >= 20 || !isField(event.field))) || (event.type === 'delta' && (!Number.isInteger(event.index) || event.index < 0 || event.index >= 20 || typeof event.delta !== 'string')) || (event.type === 'done' && (!Array.isArray(event.fields) || !event.fields.every(isField)))) throw new Error('AI 栏目格式错误，已保留生成内容。');
    if (event.type === 'error') throw new Error(event.error || 'AI 生成失败。');
    onEvent(event);
    if (event.type === 'done') { complete = true; return; }
   }
   if (buffer.length > 1024 * 1024) throw new Error('流式响应过长，已停止。');
   if (done) break;
  }
  if (!complete) throw new Error('生成连接中断，已保留部分内容，可重新生成。');
 } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}

export async function generateAIReport(input: { account: string; business_type: ReportKind; start: string; end: string; notes: string }, epoch: number, signal: AbortSignal, onEvent: (event: AIEvent) => void) {
 const controller = new AbortController(); const abort = () => controller.abort();
 signal.addEventListener('abort', abort, { once: true }); if (signal.aborted) controller.abort();
 const timer = setTimeout(abort, 115000);
 try {
  const response = await fetch('/api/reports/ai/generate', { method: 'POST', credentials: 'same-origin', cache: 'no-store', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input), signal: controller.signal });
  if (getSessionEpoch() !== epoch) throw new Error('账号已变化，已停止生成。');
  if (!response.ok) { const value = await response.json().catch(() => ({})); throw new Error(value.error || 'AI 服务暂时不可用。'); }
  await consumeAIStream(response, event => { if (signal.aborted || getSessionEpoch() !== epoch) throw new Error('已停止生成。'); onEvent(event); });
 } finally { clearTimeout(timer); signal.removeEventListener('abort', abort); }
}
