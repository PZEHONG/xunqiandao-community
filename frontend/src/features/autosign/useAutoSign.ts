import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { ApiError, StaleSessionError, UnknownSubmissionError } from '../../api/errors';
import { useSession } from '../../session/useSession';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { coordinates, type ConfirmedLocation } from '../locations/types';
import type { SignConfirmation } from '../sign/types';
import { deleteTask, readAuto, saveTask, stateTask } from './api';
import type { AutoConfirmation, AutoRun, AutoTask, SaveTask } from './types';
const initial = (owner: string) => ({ owner, tasks: [] as AutoTask[], runs: [] as AutoRun[], schoolID: '', loading: false, busy: false, blocked: false, error: '', message: '', editing: null as AutoTask | null, savedPlace: null as ConfirmedLocation | null, name: '每日签到', time: '09:00', confirmation: null as AutoConfirmation | null });
export function useAutoSign() {
  const session = useSession(); const account = session.bindings[0]?.account || '';
  const owner = JSON.stringify([session.epoch, session.status, session.user?.id, session.bindings]);
  const [state, setState] = useState(() => initial(owner)); const model = useRef(state);
  const identity = useRef({ owner, epoch: session.epoch, account });
  const alive = useRef(true); const lock = useRef(false); const version = useRef(0); const readVersion = useRef(0);
  const readJob = useRef<AbortController | null>(null); const writeJob = useRef<AbortController | null>(null);
  const consumed = useRef(new WeakSet<AutoConfirmation>());
  const update = useCallback((change: (value: ReturnType<typeof initial>) => ReturnType<typeof initial>) => { model.current = change(model.current); if (alive.current) setState(model.current); }, []);
  const current = useCallback((expectedOwner: string, epoch: number) => alive.current && identity.current.owner === expectedOwner && epoch === getSessionEpoch(), []);
  const invalidateConfirmation = useCallback(() => { version.current++; update(value => ({ ...value, confirmation: null })); }, [update]);
  useLayoutEffect(() => {
    if (identity.current.owner !== owner) {
      readJob.current?.abort(); writeJob.current?.abort(); readVersion.current++; version.current++; lock.current = false;
      update(() => initial(owner));
    }
    identity.current = { owner, epoch: session.epoch, account };
  }, [owner, session.epoch, account, update]);
  useEffect(() => { alive.current = true; return () => { alive.current = false; readJob.current?.abort(); writeJob.current?.abort(); }; }, []);
  const load = useCallback(async () => {
    if (!account || session.status !== 'authenticated' || lock.current) return;
    readJob.current?.abort(); const job = new AbortController(); readJob.current = job; const seq = ++readVersion.current;
    update(value => ({ ...value, loading: true, error: '' }));
    try {
      const result = await readAuto(account, session.epoch, job.signal);
      if (!current(owner, session.epoch) || job.signal.aborted || seq !== readVersion.current) return;
      // A queried snapshot is required before retrying an uncertain or conflicting write.
      const recovering = model.current.blocked;
      if (recovering) version.current++;
      update(value => ({ ...value, ...result, loading: false, blocked: false, ...(recovering ? { editing: null, savedPlace: null, name: '每日签到', time: '09:00', confirmation: null } : {}) }));
    } catch (error) {
      if (current(owner, session.epoch) && !job.signal.aborted && seq === readVersion.current) update(value => ({ ...value, loading: false, error: error instanceof Error ? error.message : '读取任务失败。' }));
    }
  }, [account, session.status, session.epoch, owner, current, update]);
  useEffect(() => { void load(); return () => readJob.current?.abort(); }, [load]);
  const failure = useCallback((error: unknown) => {
    const blocked = error instanceof UnknownSubmissionError || error instanceof ApiError && error.status === 409;
    const message = error instanceof ApiError && error.status === 409 ? '任务已在其他页面变更，请刷新任务后重新核对。' : error instanceof UnknownSubmissionError ? '任务操作结果暂时无法确认，请先刷新任务与执行记录，再决定是否重新操作。' : error instanceof Error ? error.message : '任务操作未完成，请刷新核对。';
    update(value => ({ ...value, blocked: value.blocked || blocked, error: message, message: '', confirmation: null }));
  }, [update]);
  const working = useCallback(async (action: (job: AbortController) => Promise<void>) => {
    if (lock.current || model.current.blocked || !account || !current(owner, session.epoch)) return;
    lock.current = true; readJob.current?.abort(); readVersion.current++;
    const job = new AbortController(); writeJob.current = job;
    update(value => ({ ...value, busy: true, loading: false, error: '', message: '' }));
    try { await action(job); }
    catch (error) { if (current(owner, session.epoch) && !job.signal.aborted && !(error instanceof StaleSessionError)) failure(error); }
    finally { if (current(owner, session.epoch) && writeJob.current === job) { lock.current = false; update(value => ({ ...value, busy: false, confirmation: null })); } }
  }, [account, owner, session.epoch, current, update, failure]);
  const replace = useCallback((task: AutoTask) => update(value => ({ ...value, tasks: [task, ...value.tasks.filter(item => item.id !== task.id)] })), [update]);
  const setName = useCallback((name: string) => { if (!lock.current) { invalidateConfirmation(); update(value => ({ ...value, name })); } }, [invalidateConfirmation, update]);
  const setTime = useCallback((time: string) => { if (!lock.current) { invalidateConfirmation(); update(value => ({ ...value, time })); } }, [invalidateConfirmation, update]);
  const cancelEdit = useCallback(() => { if (!lock.current) { invalidateConfirmation(); update(value => ({ ...value, editing: null, savedPlace: null, name: '每日签到', time: '09:00', message: value.editing ? '已取消编辑，原任务保持暂停。' : '', error: '' })); } }, [invalidateConfirmation, update]);
  const changePlace = useCallback(() => { if (!lock.current) { invalidateConfirmation(); update(value => ({ ...value, savedPlace: null })); } }, [invalidateConfirmation, update]);
  const pause = useCallback(async (task: AutoTask) => {
    if (task.account !== account) return;
    await working(async job => { const saved = await stateTask(task, false, session.epoch, job.signal); if (current(owner, session.epoch)) { replace(saved); invalidateConfirmation(); update(value => ({ ...value, message: '任务已暂停。已经发出的签到无法撤回。' })); } });
  }, [account, working, session.epoch, current, owner, replace, invalidateConfirmation, update]);
  const edit = useCallback(async (task: AutoTask) => {
    if (task.account !== account) return;
    await working(async job => {
      const paused = task.enabled ? await stateTask(task, false, session.epoch, job.signal) : task;
      if (!current(owner, session.epoch)) return;
      replace(paused); invalidateConfirmation();
      const savedPlace: ConfirmedLocation = Object.freeze({ address: paused.address, address_name: paused.address_name, latitude: paused.latitude, longitude: paused.longitude, location_confirmed: true, confirmation_token: '' });
      update(value => ({ ...value, editing: paused, savedPlace, name: paused.name, time: paused.time, message: '编辑期间任务保持暂停，重新核对并确认后启用。' }));
    });
  }, [account, working, session.epoch, current, owner, replace, invalidateConfirmation, update]);
  const prepare = useCallback((kind: AutoConfirmation['kind'], task: AutoTask | SaveTask) => {
    if (lock.current || model.current.blocked || task.account !== account || !current(owner, session.epoch)) return;
    invalidateConfirmation(); const confirmation: AutoConfirmation = Object.freeze({ owner, epoch: session.epoch, version: version.current, kind, task: Object.freeze({ ...task }) });
    update(value => ({ ...value, confirmation, error: '', message: '' }));
  }, [account, current, owner, session.epoch, invalidateConfirmation, update]);
  const create = useCallback((snapshot: SignConfirmation) => {
    const value = model.current; const place = snapshot.location;
    const usingSaved = !!value.savedPlace && ['address', 'address_name', 'latitude', 'longitude'].every(key => value.savedPlace![key as keyof ConfirmedLocation] === place[key as keyof ConfirmedLocation]);
    const error = snapshot.epoch !== getSessionEpoch() || snapshot.account !== account ? '账号已变化，请重新核对。' : !value.schoolID ? '请先连接账号并刷新任务，核对学校信息。' : !value.name.trim() || [...value.name].length > 60 ? '任务名称需为1–60字。' : !/^([01]\d|2[0-3]):[0-5]\d$/.test(value.time) ? '请选择有效的每日时间（HH:MM）。' : [...snapshot.comment].length > 1000 ? '签到说明最多1000字。' : !place.location_confirmed || !place.address.trim() || [...place.address].length > 500 || [...place.address_name].length > 200 || !coordinates(place.longitude, place.latitude) || (!usingSaved && !place.confirmation_token) ? '请重新搜索并确认完整地点。' : '';
    if (error) { invalidateConfirmation(); update(previous => ({ ...previous, error })); return; }
    const task: SaveTask = { id: value.editing?.id || 0, revision: value.editing?.revision || 0, account, school_id: value.schoolID, name: value.name.trim(), time: value.time, remark: snapshot.remark, remark_label: snapshot.remarkLabel, comment: snapshot.comment, ...place, enabled: true, confirmed: true };
    prepare('save', task);
  }, [account, prepare, invalidateConfirmation, update]);
  const resume = useCallback((task: AutoTask) => prepare('resume', task), [prepare]);
  const remove = useCallback((task: AutoTask) => prepare('remove', task), [prepare]);
  const cancelConfirmation = useCallback(() => { if (!lock.current) invalidateConfirmation(); }, [invalidateConfirmation]);
  const submitConfirmed = useCallback(async (confirmation: AutoConfirmation) => {
    if (!confirmation || consumed.current.has(confirmation) || model.current.confirmation !== confirmation || confirmation.version !== version.current || confirmation.owner !== owner || !current(owner, confirmation.epoch)) return;
    consumed.current.add(confirmation);
    await working(async job => {
      const task = confirmation.task;
      if (confirmation.kind === 'remove') await deleteTask(task as AutoTask, confirmation.epoch, job.signal);
      else {
        const saved = confirmation.kind === 'save' ? await saveTask(task as SaveTask, confirmation.epoch, job.signal) : await stateTask(task as AutoTask, true, confirmation.epoch, job.signal);
        if (!current(owner, confirmation.epoch)) return;
        replace(saved);
      }
      if (!current(owner, confirmation.epoch)) return;
      version.current++;
      update(value => ({ ...value, tasks: confirmation.kind === 'remove' ? value.tasks.filter(item => item.id !== task.id) : value.tasks, ...(confirmation.kind === 'save' || value.editing?.id === task.id ? { editing: null, savedPlace: null, name: '每日签到', time: '09:00' } : {}), message: confirmation.kind === 'remove' ? '任务已删除，执行记录仍保留。已经发出的签到无法撤回。' : '每日任务已确认启用，保存不会立即签到。任务由服务器按计划执行，无需保持网页打开或个人电脑开机。' }));
    });
  }, [owner, current, working, replace, update]);
  const visible = state.owner === owner ? state : initial(owner);
  return { ...visible, load, create, edit, pause, resume, remove, cancelEdit, changePlace, setName, setTime, invalidateConfirmation, cancelConfirmation, submitConfirmed };
}
