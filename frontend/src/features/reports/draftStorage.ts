import { attachmentURL } from './reportKinds';
import type { ReportDraft, ReportField, ReportKind } from './types';
export function draftKey({ userID, account, school, kind }: { userID: number | string; account: string; school: string; kind: ReportKind }) { return `xixun-report-draft:${userID}:${encodeURIComponent(account.trim())}:${encodeURIComponent(school.trim())}:${kind}`; }
export function clearUserDrafts(userID: number | string): boolean {
  if (!/^[1-9]\d*$/.test(String(userID))) return false;
  const pattern = new RegExp(`^xixun-report-draft:${userID}:[^:]*:[^:]*:(week|month)(:recovery)?$`);
  try {
    const keys = Array.from({ length: localStorage.length }, (_, index) => localStorage.key(index)).filter((key): key is string => key !== null && pattern.test(key));
    for (const key of keys) localStorage.removeItem(key);
    return true;
  } catch { return false; }
}
function fields(value: unknown): ReportField[] { return Array.isArray(value) ? value.filter(v => v && typeof v.title === 'string' && typeof v.content === 'string').map(v => ({ title: v.title, content: v.content, sort: Number(v.sort) || 0, require: String(v.require ?? '0') })) : []; }
function validFields(value: unknown): boolean { return Array.isArray(value) && value.every(v => v && typeof v.title === 'string' && typeof v.content === 'string'); }
export function loadDraft(key: string): { draft: ReportDraft | null; raw: string | null; status: 'none' | 'restored' | 'corrupt' | 'unavailable' } {
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(key); if (raw === null) return { draft: null, raw, status: 'none' };
    const v = JSON.parse(raw); if (!v || typeof v !== 'object' || Array.isArray(v) || !validFields(v.content) || (v.orphaned !== undefined && !validFields(v.orphaned))) return { draft: null, raw, status: 'corrupt' };
    return { raw, status: 'restored', draft: { start: typeof v.start === 'string' ? v.start : '', end: typeof v.end === 'string' ? v.end : '', content: fields(v.content), orphaned: fields(v.orphaned).filter(f => f.content.trim()), attachments: (Array.isArray(v.attachments) ? v.attachments : []).filter((a: { name?: unknown; uri?: unknown } | null) => a && typeof a.name === 'string' && typeof a.uri === 'string' && attachmentURL(a.uri)).slice(0, 3).map((a: { name: string; uri: string }) => ({ name: a.name, uri: a.uri })), pendingNames: (Array.isArray(v.pendingNames) ? v.pendingNames : []).filter((n: unknown) => typeof n === 'string'), ...(v.submissionUncertain === true ? { submissionUncertain: true } : {}) } };
  } catch { return { draft: null, raw, status: raw === null ? 'unavailable' : 'corrupt' }; }
}
export function loadRecovery(key: string): string {
  try { const raw = localStorage.getItem(key + ':recovery'); if (!raw) return ''; const copies: unknown = JSON.parse(raw); return Array.isArray(copies) && copies.every(v => typeof v === 'string') ? copies.length === 1 ? copies[0] : JSON.stringify(copies, null, 2) : raw; } catch { return ''; }
}
export function saveDraft(key: string, draft: ReportDraft) {
  const raw = JSON.stringify(draft);
  try {
    const previous = loadDraft(key);
    if (previous.status === 'unavailable') return { ok: false, raw: null };
    if (previous.status === 'corrupt' && previous.raw !== null) {
      const backupKey = key + ':recovery'; const existing = localStorage.getItem(backupKey);
      const copies: unknown = existing === null ? [] : JSON.parse(existing);
      if (!Array.isArray(copies) || !copies.every(v => typeof v === 'string')) return { ok: false, raw: null };
      if (!copies.includes(previous.raw)) localStorage.setItem(backupKey, JSON.stringify([...copies, previous.raw]));
    }
    localStorage.setItem(key, raw); return { ok: true, raw };
  } catch { return { ok: false, raw: null }; }
}
export function clearSubmittedDraft(key: string, raw: string | null) { try { if (raw !== null && localStorage.getItem(key) === raw) { localStorage.removeItem(key); return true; } } catch { /* Retain the draft if storage is unavailable. */ } return false; }
export function restoreFields(template: ReportField[], draft: ReportDraft | null) {
  const used = new Set<ReportField>(); const saved = draft?.content ?? [];
  const result = template.map(field => {
    const oldTitle = saved.filter(f => f.title === field.title); const newTitle = template.filter(f => f.title === field.title);
    const oldPair = oldTitle.filter(f => f.sort === field.sort); const newPair = newTitle.filter(f => f.sort === field.sort);
    const match = oldTitle.length === 1 && newTitle.length === 1 ? oldTitle[0] : oldPair.length === 1 && newPair.length === 1 ? oldPair[0] : undefined;
    if (match && !used.has(match)) { used.add(match); return { ...field, content: match.content }; }
    return { ...field, content: '' };
  });
  return { fields: result, orphaned: [...(draft?.orphaned ?? []), ...saved.filter(f => !used.has(f) && f.content.trim())] };
}
