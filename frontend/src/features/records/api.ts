import { request } from '../../api/client';
import type { QueryResponse, SignSnapshot } from '../../api/contracts';
import { buildMonth } from './calendar';
export async function queryRecords(account: string, month: string, epoch: number, signal: AbortSignal): Promise<SignSnapshot> {
  const result = await request<QueryResponse>(`/api/query?account=${encodeURIComponent(account)}&month_date=${encodeURIComponent(month)}`, { expectedEpoch: epoch, signal });
  const data = result.data;
  if (!data || !Array.isArray(data.records) || data.records.some(record => !record || typeof record !== 'object' || Array.isArray(record))) throw new Error('查询结果不完整，暂时无法核实签到明细，请重新刷新。');
  if (data.month_date && data.month_date !== month) throw new Error('平台返回的月份与查询不一致，请重新刷新。');
  buildMonth(data.month_date || month); return { ...data, month_date: data.month_date || month };
}
