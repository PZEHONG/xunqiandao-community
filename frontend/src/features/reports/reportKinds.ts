import type { ReportKind, ReportRecord } from './types';
export const reportLabel = (kind: ReportKind) => kind === 'week' ? '实习周报' : '实习月报';
export const MAX_ATTACHMENT_SIZE = 10 * 1024 * 1024;
export function attachmentURL(uri: unknown): string | null {
  if (typeof uri !== 'string' || !uri.trim() || /[,\r\n\0]/.test(uri)) return null;
  try { const url = new URL(uri.trim(), 'https://file.xixunyun.com/'); return ['http:', 'https:'].includes(url.protocol) ? url.href : null; } catch { return null; }
}
export function validDate(value: string) { if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false; const date = new Date(`${value}T00:00:00Z`); return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value; }
export function validRange(start: string, end: string, optional = false) { return optional && !start && !end || validDate(start) && validDate(end) && start <= end; }
export function reportTime(value: unknown) {
  if (value === undefined || value === null || value === '' || value === 0 || value === '0') return '—';
  const n = Number(value); const date = Number.isFinite(n) ? new Date(n < 1e12 ? n * 1000 : n) : new Date(String(value));
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });
}
export function reportDate(value: unknown) {
  if (value === undefined || value === null || value === '' || value === 0 || value === '0') return '—';
  if (typeof value === 'string' && validDate(value)) return value;
  const n = Number(value); const date = Number.isFinite(n) ? new Date(n < 1e12 ? n * 1000 : n) : new Date(String(value));
  if (Number.isNaN(date.getTime())) return String(value);
  const parts = new Intl.DateTimeFormat('en-GB', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(date);
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find(p => p.type === type)?.value ?? '';
  return `${part('year')}-${part('month')}-${part('day')}`;
}
export function reportGrade(row: ReportRecord) { return ({ 1: '优秀', 2: '良好', 3: '及格', 4: '不及格' } as Record<string, string>)[String(row.score)] || (String(row.state) === '1' ? '已批阅' : '未批阅'); }
export function detailFields(value: unknown): { title: string; content: string }[] {
  if (typeof value === 'string') { try { const parsed: unknown = JSON.parse(value); if (Array.isArray(parsed)) return detailFields(parsed); } catch { /* Plain text is a supported legacy format. */ } return [{ title: '报告内容', content: value }]; }
  return Array.isArray(value) ? value.filter((v): v is ReportRecord => !!v && typeof v === 'object').map(v => ({ title: String(v.title ?? '报告内容'), content: String(v.content ?? '') })) : [];
}
export function detailAttachments(value: unknown) { const values = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : []; return values.filter((v): v is string => typeof v === 'string' && !!attachmentURL(v)); }
export function downloadText(name: string, text: string) { const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' })); const a = document.createElement('a'); a.href = url; a.download = name; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); }
