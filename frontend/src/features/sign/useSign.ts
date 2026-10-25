import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useSession } from '../../session/useSession';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { StaleSessionError, UnknownSubmissionError } from '../../api/errors';
import { loadRemarks, sendSign, uploadPhoto } from './api';
import { photoError, type SignConfirmation, type SignDraft, type SignLocation, type SignRemark, type SignResult } from './types';
const emptyDraft = (): SignDraft => ({ remark: '', comment: '', photos: [] });
function empty(owner: string) { return { owner, draft: emptyDraft(), remarks: [] as SignRemark[], loadingRemarks: false, error: '', remarkError: '', confirmation: null as SignConfirmation | null, result: null as SignResult | null, progress: '' }; }
export function useSign(location: SignLocation) {
  const session = useSession(); const binding = session.bindings[0]; const account = binding?.account || '';
  const owner = JSON.stringify([session.epoch, session.status, session.user?.id, session.bindings]);
  const [state, setState] = useState(() => empty(owner)); const model = useRef(state);
  const busy = useRef(false); const alive = useRef(true); const version = useRef(0); const locationVersion = useRef(0);
  const readJob = useRef<AbortController | null>(null); const writeJob = useRef<AbortController | null>(null);
  const uploads = useRef(new Map<File, string>()); const consumed = useRef(new WeakSet<SignConfirmation>()); const pending = useRef<SignConfirmation | null>(null);
  const latest = useRef({ owner, epoch: session.epoch, account, school: binding?.school_name || '', authenticated: session.status === 'authenticated', confirmed: location.confirmed, pending: location.pending });
  const update = useCallback((change: (previous: ReturnType<typeof empty>) => ReturnType<typeof empty>) => { const next = change(model.current); model.current = next; if (alive.current) setState(next); }, []);
  const cancelRead = useCallback(() => readJob.current?.abort(), []);
  const cancelWrite = useCallback(() => writeJob.current?.abort(), []);
  useLayoutEffect(() => {
    const previous = latest.current;
    latest.current = { owner, epoch: session.epoch, account, school: binding?.school_name || '', authenticated: session.status === 'authenticated', confirmed: location.confirmed, pending: location.pending };
    if (previous.owner !== owner) {
      cancelRead(); cancelWrite(); busy.current = false; pending.current = null; uploads.current.clear(); version.current++; locationVersion.current++;
      update(() => empty(owner));
    } else if (previous.confirmed !== location.confirmed || previous.pending !== location.pending) {
      locationVersion.current++; pending.current = null; update(value => ({ ...value, confirmation: null }));
    }
  }, [owner, session.epoch, session.status, account, binding?.school_name, location.confirmed, location.pending, cancelRead, cancelWrite, update]);
  useEffect(() => { alive.current = true; const cache = uploads.current; return () => { alive.current = false; cancelRead(); cancelWrite(); cache.clear(); }; }, [cancelRead, cancelWrite]);
  const reloadRemarks = useCallback(async () => {
    if (!account || session.status !== 'authenticated' || busy.current) return;
    cancelRead(); const controller = new AbortController(); readJob.current = controller;
    update(value => ({ ...value, loadingRemarks: true, remarkError: '' }));
    try {
      const remarks = await loadRemarks(account, session.epoch, controller.signal);
      if (controller.signal.aborted || latest.current.owner !== owner || !alive.current) return;
      const selected = remarks.find(mark => mark.key === model.current.draft.remark) || remarks.find(mark => mark.value === '上班') || remarks[0];
      version.current++; pending.current = null;
      update(value => ({ ...value, remarks, draft: { ...value.draft, remark: selected.key }, confirmation: null, loadingRemarks: false }));
    } catch (error) {
      if (!controller.signal.aborted && latest.current.owner === owner && alive.current) update(value => ({ ...value, loadingRemarks: false, remarkError: error instanceof Error ? error.message : '签到备注加载失败。' }));
    }
  }, [account, session.status, session.epoch, owner, cancelRead, update]);
  useEffect(() => { void reloadRemarks(); return cancelRead; }, [reloadRemarks, cancelRead]);
  const changeDraft = useCallback((patch: Partial<SignDraft>) => {
    if (busy.current || latest.current.owner !== owner) return;
    version.current++; pending.current = null;
    update(value => ({ ...value, draft: { ...value.draft, ...patch }, error: '', result: null, confirmation: null }));
  }, [owner, update]);
  const setRemark = useCallback((remark: string) => changeDraft({ remark }), [changeDraft]);
  const setComment = useCallback((comment: string) => changeDraft({ comment }), [changeDraft]);
  const setPhotos = useCallback((photos: readonly File[]) => {
    if (busy.current || latest.current.owner !== owner) return;
    const error = photoError(photos); if (error) { update(value => ({ ...value, error })); return; }
    for (const file of uploads.current.keys()) if (!photos.includes(file)) uploads.current.delete(file);
    changeDraft({ photos: Object.freeze([...photos]) });
  }, [owner, changeDraft, update]);
  const prepareConfirmation = useCallback((): SignConfirmation | null => {
    if (busy.current) return null;
    const identity = latest.current; const value = model.current; const mark = value.remarks.find(item => item.key === value.draft.remark);
    const error = !identity.authenticated || !identity.account || identity.epoch !== getSessionEpoch() ? '请先连接习讯云账号。' : identity.pending ? '请先确认使用候选地点，或取消候选。' : !identity.confirmed ? '请先搜索、选择并确认完整地点。' : !mark ? '请先选择可用签到备注。' : [...value.draft.comment].length > 1000 ? '签到说明最多1000字。' : photoError(value.draft.photos);
    if (error) { pending.current = null; update(previous => ({ ...previous, error, confirmation: null })); return null; }
    const snapshot: SignConfirmation = Object.freeze({ owner: identity.owner, epoch: identity.epoch, account: identity.account, school: identity.school, version: version.current, locationVersion: locationVersion.current, location: Object.freeze({ ...identity.confirmed! }), remark: mark!.key, remarkLabel: mark!.value, comment: value.draft.comment, photos: Object.freeze([...value.draft.photos]) });
    pending.current = snapshot; update(previous => ({ ...previous, error: '', result: null, confirmation: snapshot })); return snapshot;
  }, [update]);
  const cancelConfirmation = useCallback(() => { if (!busy.current) { pending.current = null; update(value => ({ ...value, confirmation: null })); } }, [update]);
  const submitConfirmed = useCallback(async (snapshot: SignConfirmation) => {
    if (busy.current || !snapshot || consumed.current.has(snapshot)) return;
    const valid = () => alive.current && pending.current === snapshot && latest.current.owner === snapshot.owner && latest.current.epoch === getSessionEpoch() && snapshot.epoch === getSessionEpoch() && version.current === snapshot.version && locationVersion.current === snapshot.locationVersion && !latest.current.pending && !!latest.current.confirmed;
    if (!valid()) { if (alive.current && latest.current.owner === snapshot.owner) update(value => ({ ...value, error: '签到信息已变更，请重新核对后提交。', confirmation: null })); return; }
    consumed.current.add(snapshot); busy.current = true; cancelRead();
    const controller = new AbortController(); writeJob.current = controller;
    const current = () => alive.current && latest.current.owner === snapshot.owner && snapshot.epoch === getSessionEpoch() && writeJob.current === controller;
    update(value => ({ ...value, error: '', result: null, progress: '准备提交…', loadingRemarks: false }));
    let sendingSign = false;
    try {
      const uris: string[] = [];
      for (let index = 0; index < snapshot.photos.length; index++) {
        if (!valid()) throw new Error('签到信息已变更，请重新核对后提交。');
        const file = snapshot.photos[index]; let uri = uploads.current.get(file);
        if (!uri) {
          update(value => ({ ...value, progress: `正在上传照片 ${index + 1} / ${snapshot.photos.length}…` }));
          uri = await uploadPhoto(snapshot.account, file, snapshot.epoch, controller.signal);
          if (!valid()) throw new Error('签到信息已变更，请重新核对后提交。');
          uploads.current.set(file, uri);
        }
        uris.push(uri);
      }
      if (!valid()) throw new Error('签到信息已变更，请重新核对后提交。');
      sendingSign = true; update(value => ({ ...value, progress: '正在提交签到…' }));
      const message = await sendSign(snapshot, uris, controller.signal);
      if (!current()) return;
      uploads.current.clear(); version.current++;
      update(value => ({ ...value, draft: { ...value.draft, photos: [], comment: '' }, result: { status: 'success', message } }));
    } catch (error) {
      if (!current() || error instanceof StaleSessionError) return;
      const unknown = error instanceof UnknownSubmissionError;
      const message = unknown && !sendingSign ? '照片上传结果暂时无法确认，尚未发送签到。请重新核对照片后再提交。' : error instanceof Error ? error.message : '签到未完成，请重新核对。';
      update(value => ({ ...value, error: message, result: unknown && sendingSign ? { status: 'unknown', message } : null }));
    } finally {
      if (current()) { busy.current = false; pending.current = null; update(value => ({ ...value, confirmation: null, progress: '' })); }
    }
  }, [cancelRead, update]);
  const reset = useCallback(() => { if (!busy.current) { uploads.current.clear(); changeDraft({ comment: '', photos: [] }); } }, [changeDraft]);
  const dismissResult = useCallback(() => update(value => ({ ...value, result: null })), [update]);
  const visible = state.owner === owner ? state : empty(owner);
  return { draft: visible.draft, remarks: visible.remarks, loadingRemarks: visible.loadingRemarks, remarkError: visible.remarkError, busy: state.owner === owner && busy.current, error: visible.error, result: visible.result, confirmation: visible.confirmation, progress: visible.progress, setRemark, setComment, setPhotos, prepareConfirmation, submitConfirmed, cancelConfirmation, reset, reloadRemarks, dismissResult };
}
