import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { ApiError, UnknownSubmissionError } from '../../api/errors';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { useSession } from '../../session/useSession';
import { listReports, reportDetail, reportTemplate, submitReport, uploadReport } from './api';
import { clearSubmittedDraft, draftKey, loadDraft, loadRecovery, restoreFields, saveDraft } from './draftStorage';
import { attachmentURL, MAX_ATTACHMENT_SIZE, validRange } from './reportKinds';
import type { ReportAttachment, ReportConfirmation, ReportDraft, ReportField, ReportFilters, ReportKind, ReportRecord } from './types';
const emptyFilters: ReportFilters = { start: '', end: '', status: '' };
const initial = (owner: string) => ({ owner, rows: [] as ReportRecord[], count: 0, page: 1, filters: emptyFilters, loading: false, templateLoading: false, busy: false, error: '', message: '', editor: false, fields: [] as ReportField[], orphaned: [] as ReportField[], start: '', end: '', attachments: [] as ReportAttachment[], pendingNames: [] as string[], storage: '', recoveryRaw: '', submissionUncertain: false, confirmation: null as ReportConfirmation | null, detailOpen: false, detailLoading: false, detail: null as ReportRecord | null, detailError: '' });
type State = ReturnType<typeof initial>;
function toDraft(s: State): ReportDraft { return { start: s.start, end: s.end, content: s.fields.map(f => ({ ...f, content: f.content.trim() })), orphaned: s.orphaned, attachments: s.attachments.filter(a => a.uri).map(({ name, uri }) => ({ name, uri })), pendingNames: [...s.pendingNames, ...s.attachments.filter(a => !a.uri).map(a => a.name)], ...(s.submissionUncertain ? { submissionUncertain: true } : {}) }; }
export function useReports(kind: ReportKind) {
  const session = useSession(); const binding = session.bindings[0]; const account = binding?.account.trim() || '';
  const key = draftKey({ userID: session.user?.id ?? '', account, school: binding?.school_name ?? '', kind });
  const owner = JSON.stringify([session.epoch, session.status, session.user?.id, session.bindings, kind]);
  const [state, setState] = useState(() => initial(owner)); const model = useRef(state); const identity = useRef(owner); const alive = useRef(true); const lock = useRef(false); const version = useRef(0); const sequence = useRef(0); const templateSeq = useRef(0); const detailSeq = useRef(0); const readJob = useRef<AbortController | null>(null); const templateJob = useRef<AbortController | null>(null); const detailJob = useRef<AbortController | null>(null); const consumed = useRef(new WeakSet<ReportConfirmation>());
  const update = useCallback((fn: (s: State) => State) => { model.current = fn(model.current); if (alive.current) setState(model.current); }, []);
  const current = useCallback(() => alive.current && identity.current === owner && session.epoch === getSessionEpoch(), [owner, session.epoch]);
  useLayoutEffect(() => { if (identity.current !== owner) { readJob.current?.abort(); templateJob.current?.abort(); detailJob.current?.abort(); sequence.current++; templateSeq.current++; detailSeq.current++; version.current++; lock.current = false; update(() => initial(owner)); } identity.current = owner; }, [owner, update]);
  useEffect(() => { alive.current = true; return () => { alive.current = false; readJob.current?.abort(); templateJob.current?.abort(); detailJob.current?.abort(); }; }, []);
  const persist = useCallback(() => { const result = saveDraft(key, toDraft(model.current)); update(s => ({ ...s, storage: result.ok ? '草稿已自动保存在此浏览器' : '草稿未保存：浏览器存储不可用，请下载备份。' })); return result; }, [key, update]);
  const load = useCallback(async (page = 1, filters = model.current.filters) => {
    if (!account || session.status !== 'authenticated' || !current()) return;
    if (!validRange(filters.start, filters.end, true)) { update(s => ({ ...s, error: '查询日期需完整填写，且结束日期不能早于开始日期。' })); return; }
    readJob.current?.abort(); const job = new AbortController(); readJob.current = job; const seq = ++sequence.current; update(s => ({ ...s, loading: true, error: '', filters }));
    try { const result = await listReports(account, kind, page, filters, session.epoch, job.signal); if (current() && !job.signal.aborted && seq === sequence.current) update(s => ({ ...s, ...result, page, loading: false })); }
    catch (e) { if (current() && !job.signal.aborted && seq === sequence.current) update(s => ({ ...s, loading: false, error: e instanceof Error ? e.message : '报告查询失败。' })); }
  }, [account, session.status, session.epoch, kind, current, update]);
  useEffect(() => { void load(); return () => readJob.current?.abort(); }, [load]);
  const openEditor = useCallback(async () => {
    if (!account || session.status !== 'authenticated' || !current() || lock.current) return;
    templateJob.current?.abort(); const job = new AbortController(); templateJob.current = job; const seq = ++templateSeq.current; update(s => ({ ...s, templateLoading: true, error: '', message: '' }));
    try {
      const template = await reportTemplate(account, kind, session.epoch, job.signal); if (!current() || job.signal.aborted || seq !== templateSeq.current) return;
      const loaded = loadDraft(key); const inMemory = model.current.fields.length > 0; const draft = inMemory ? toDraft(model.current) : loaded.draft; const restored = restoreFields(template, draft); version.current++;
      update(s => ({ ...s, ...restored, editor: true, templateLoading: false, start: draft?.start ?? '', end: draft?.end ?? '', attachments: inMemory ? s.attachments : (draft?.attachments ?? []).map(a => ({ ...a, id: crypto.randomUUID() })), pendingNames: inMemory ? s.pendingNames : draft?.pendingNames ?? [], submissionUncertain: draft?.submissionUncertain === true, confirmation: null, recoveryRaw: loaded.status === 'corrupt' ? loaded.raw ?? '' : loadRecovery(key), storage: loaded.status === 'unavailable' ? '草稿未保存：浏览器存储不可用，请下载备份。' : loaded.status === 'corrupt' ? '旧草稿损坏，可下载原始内容后恢复。' : loaded.draft ? '已恢复此账号的本地草稿' : '输入后自动保存草稿' }));
    } catch (e) { if (current() && !job.signal.aborted && seq === templateSeq.current) update(s => ({ ...s, templateLoading: false, error: e instanceof Error ? e.message : '读取模板失败。' })); }
  }, [account, session.status, session.epoch, kind, key, current, update]);
  const change = useCallback((fn: (s: State) => State) => { if (!current() || lock.current || !model.current.editor) return; version.current++; update(s => ({ ...fn(s), confirmation: null, error: '' })); persist(); }, [current, update, persist]);
  const setStart = (value: string) => change(s => ({ ...s, start: value })); const setEnd = (value: string) => change(s => ({ ...s, end: value }));
  const setField = (index: number, content: string) => change(s => ({ ...s, fields: s.fields.map((f, i) => i === index ? { ...f, content } : f) }));
  const addFiles = (files: File[]) => {
    if (!current() || lock.current) return;
    if (model.current.attachments.length + files.length > 3 || files.some(f => f.size < 1 || f.size > MAX_ATTACHMENT_SIZE)) { update(s => ({ ...s, error: '附件最多3个，每个需为1字节至10MB。' })); return; }
    change(s => { const pendingNames = [...s.pendingNames]; for (const file of files) { const index = pendingNames.indexOf(file.name); if (index >= 0) pendingNames.splice(index, 1); } return { ...s, pendingNames, attachments: [...s.attachments, ...files.map(file => ({ id: crypto.randomUUID(), name: file.name, uri: '', file }))] }; });
  };
  const cancelConfirmation = () => { if (!lock.current) { version.current++; update(s => ({ ...s, confirmation: null })); } };
  const prepareConfirmation = (): ReportConfirmation | null => {
    if (!current() || lock.current || !model.current.editor || !account) return null;
    const s = model.current; const content = s.fields.map(f => ({ ...f, content: f.content.trim() })); let error = '';
    if (!validRange(s.start, s.end)) error = '请填写有效的报告日期，结束日期不能早于开始日期。';
    else if (!content.length || content.some(f => f.require === '1' && !f.content)) error = '请填写全部必填内容。';
    else if (content.some(f => f.content.length > 50000) || new TextEncoder().encode(content.map(f => f.content).join('')).length > 200000) error = '正文过长：每项最多50000字，总内容最多200000字节。';
    else if (s.pendingNames.length) error = '本地附件需要重新选择，或明确移除待重选附件后再提交。';
    else if (s.attachments.length > 3 || s.attachments.some(a => a.uri ? !attachmentURL(a.uri) : !a.file || a.file.size < 1 || a.file.size > MAX_ATTACHMENT_SIZE)) error = '请重新选择有效附件。';
    if (error) { update(v => ({ ...v, error })); return null; }
    persist(); const c: ReportConfirmation = Object.freeze({ owner, epoch: session.epoch, key, version: version.current, input: Object.freeze({ account, business_type: kind, start_date: s.start, end_date: s.end, content: Object.freeze(content.map(f => Object.freeze(f))) }), attachments: Object.freeze(s.attachments.map(a => Object.freeze({ ...a }))) }); update(v => ({ ...v, confirmation: c, error: '' })); return c;
  };
  const submitConfirmed = async (c: ReportConfirmation | null) => {
    if (!c || !current() || lock.current || consumed.current.has(c) || c.owner !== owner || c.version !== version.current || model.current.confirmation !== c) return;
    consumed.current.add(c); lock.current = true; update(s => ({ ...s, busy: true, error: '', message: '' })); let posting = false;
    try {
      const uris: string[] = [];
      for (const a of c.attachments) {
        const uri = a.uri || await uploadReport(account, a.file!, c.epoch); if (!current()) return;
        uris.push(uri); update(s => ({ ...s, attachments: s.attachments.map(item => item.id === a.id ? { ...item, uri } : item) })); persist();
      }
      if (!current()) return;
      // Persist before the request so a reload or account switch cannot erase an uncertain write.
      update(s => ({ ...s, submissionUncertain: true })); const submitted = persist(); posting = true;
      const message = await submitReport(c, uris); if (!current()) return;
      const cleared = clearSubmittedDraft(c.key, submitted.raw); version.current++;
      update(s => ({ ...s, editor: false, fields: [], orphaned: [], start: '', end: '', attachments: [], pendingNames: [], submissionUncertain: false, confirmation: null, message, storage: cleared ? '' : '提交成功；本地较新或无法清理的草稿已保留。' })); void load(1);
    } catch (e) {
      if (!current()) return;
      const uncertain = posting && (e instanceof UnknownSubmissionError || !(e instanceof ApiError));
      update(s => ({ ...s, submissionUncertain: uncertain || s.submissionUncertain && !posting, error: uncertain ? '报告提交结果暂时无法确认，请先查询报告列表，再决定是否重新提交。' : e instanceof UnknownSubmissionError ? '附件上传结果暂时无法确认，报告尚未提交，请核对附件后重试。' : e instanceof Error ? e.message : '报告提交失败。', confirmation: null })); persist();
    } finally { if (current()) { lock.current = false; update(s => ({ ...s, busy: false, confirmation: null })); } }
  };
  const openDetail = async (id: string) => {
    if (!current() || !account) return;
    detailJob.current?.abort(); const job = new AbortController(); detailJob.current = job; const seq = ++detailSeq.current; update(s => ({ ...s, detailOpen: true, detailLoading: true, detail: null, detailError: '' }));
    try { const detail = await reportDetail(account, id, session.epoch, job.signal); if (current() && !job.signal.aborted && seq === detailSeq.current) update(s => ({ ...s, detail, detailLoading: false })); }
    catch (e) { if (current() && !job.signal.aborted && seq === detailSeq.current) update(s => ({ ...s, detailLoading: false, detailError: e instanceof Error ? e.message : '读取详情失败。' })); }
  };
  const closeDetail = () => { detailJob.current?.abort(); detailSeq.current++; update(s => ({ ...s, detailOpen: false, detail: null, detailError: '' })); };
  const setFilter = (filters: ReportFilters) => { readJob.current?.abort(); sequence.current++; update(s => ({ ...s, filters, loading: false })); };
  const closeEditor = () => { if (!lock.current) { version.current++; update(s => ({ ...s, editor: false, confirmation: null, error: '' })); } };
  return { ...state, key, account, school: binding?.school_name ?? '', pages: Math.max(1, Math.ceil(state.count / 15)), load, openEditor, closeEditor, setStart, setEnd, setField, addFiles, removeAttachment: (id: string) => change(s => ({ ...s, attachments: s.attachments.filter(a => a.id !== id) })), ignorePending: (index: number) => change(s => ({ ...s, pendingNames: s.pendingNames.filter((_, i) => i !== index) })), save: persist, prepareConfirmation, cancelConfirmation, submitConfirmed, openDetail, closeDetail, setFilter };
}
export type ReportsModel = ReturnType<typeof useReports>;
