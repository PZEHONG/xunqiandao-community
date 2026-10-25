import { request } from '../../api/client';
import { ApiError, UnknownSubmissionError } from '../../api/errors';
import type { AutoRun, AutoTask, SaveTask } from './types';
function isTask(value: unknown): value is AutoTask {
  if (!value || typeof value !== 'object') return false;
  const task = value as AutoTask;
  return Number.isSafeInteger(task.id) && task.id > 0 && Number.isSafeInteger(task.revision) && task.revision > 0 && typeof task.enabled === 'boolean' && ['account', 'school_id', 'name', 'time', 'remark', 'remark_label', 'comment', 'address', 'address_name', 'latitude', 'longitude'].every(key => typeof (task as unknown as Record<string, unknown>)[key] === 'string');
}
export async function readAuto(account: string, epoch: number, signal: AbortSignal) {
  const query = `?account=${encodeURIComponent(account)}`;
  const [list, history] = await Promise.all([
    request<{ tasks: unknown[]; school_id: string }>('/api/auto-sign/tasks' + query, { expectedEpoch: epoch, signal }),
    request<{ runs: AutoRun[] }>('/api/auto-sign/runs' + query, { expectedEpoch: epoch, signal }),
  ]);
  if (!Array.isArray(list?.tasks) || !list.tasks.every(isTask) || typeof list.school_id !== 'string' || !Array.isArray(history?.runs)) throw new ApiError(0, '任务数据无法解析，请刷新重试。');
  return { tasks: list.tasks.filter(task => task.account === account), runs: history.runs.filter(run => run && run.account === account && typeof run.status === 'string' && typeof run.name === 'string'), schoolID: list.school_id };
}
async function write(path: string, method: string, body: unknown, epoch: number, signal: AbortSignal) {
  return request<{ task?: unknown; message?: string }>(path, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body), expectedEpoch: epoch, signal });
}
export async function saveTask(task: SaveTask, epoch: number, signal: AbortSignal) {
  const data = await write(`/api/auto-sign/tasks${task.id ? '/' + task.id : ''}`, task.id ? 'PUT' : 'POST', task, epoch, signal);
  if (!isTask(data?.task) || data.task.account !== task.account || (task.id && data.task.id !== task.id) || data.task.revision <= task.revision || !data.task.enabled) throw new UnknownSubmissionError();
  return data.task;
}
export async function stateTask(task: AutoTask, enabled: boolean, epoch: number, signal: AbortSignal) {
  const data = await write(`/api/auto-sign/tasks/${task.id}/state`, 'POST', { account: task.account, revision: task.revision, enabled, confirmed: enabled }, epoch, signal);
  if (!isTask(data?.task) || data.task.id !== task.id || data.task.account !== task.account || data.task.revision <= task.revision || data.task.enabled !== enabled) throw new UnknownSubmissionError();
  return data.task;
}
export async function deleteTask(task: AutoTask, epoch: number, signal: AbortSignal) {
  const data = await write(`/api/auto-sign/tasks/${task.id}`, 'DELETE', { account: task.account, revision: task.revision }, epoch, signal);
  if (typeof data?.message !== 'string' || !data.message) throw new UnknownSubmissionError();
}
