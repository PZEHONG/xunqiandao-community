import { describe, expect, it, vi } from 'vitest';
import { aiDraftKey, consumeAIStream, loadAIDrafts, saveAIDrafts, suggestReportDates } from './reportAI';
import { clearUserDrafts } from './draftStorage';
import { getSessionEpoch } from '../../session/sessionEpoch';

describe('AI paired date suggestions', () => {
 it.each([
  { start: '2026-09-20', end: '' },
  { start: '', end: '2026-10-25' },
  { start: '2026-09-20', end: '2026-02-30' },
  { start: '2026-10-25', end: '2026-09-20' },
 ])('rejects incomplete or invalid ranges: $start to $end', async range => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ ...range, reason: '日期建议' }))));
  await expect(suggestReportDates('A', 'week', 'batch', getSessionEpoch(), new AbortController().signal)).rejects.toThrow('起止日期');
 });
 it('retains a complete pair or the explicit no-completed-cycle result', async () => {
  for (const range of [{ start: '2026-09-20', end: '2026-10-25' }, { start: '', end: '' }]) {
   const result = { ...range, reason: '日期建议' };
   vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(result))));
   await expect(suggestReportDates('A', 'week', 'batch', getSessionEpoch(), new AbortController().signal)).resolves.toEqual(result);
  }
 });
});

function response(text: string) {
 const bytes = new TextEncoder().encode(text);
 return new Response(new ReadableStream({ start(c) { for (let i = 0; i < bytes.length; i += 3) c.enqueue(bytes.slice(i, i + 3)); c.close(); } }));
}
describe('AI report SSE', () => {
 it('decodes split UTF8 and CRLF frames and requires explicit completion', async () => {
  const events: unknown[] = [];
  await consumeAIStream(response('data: {"type":"delta","index":0,"delta":"实习总结"}\r\n\r\ndata: {"type":"done","fields":[]}\n\n'), e => events.push(e));
  expect(events).toEqual([{type:'delta',index:0,delta:'实习总结'},{type:'done',fields:[]}]);
 });
 it('retains emitted text when a stream disconnects', async () => {
  const events: unknown[] = [];
  await expect(consumeAIStream(response('data: {"type":"delta","index":0,"delta":"已写"}\n\n'), e => events.push(e))).rejects.toThrow('中断');
  expect(events).toHaveLength(1);
 });
 it('surfaces generation errors rather than successful completion', async () => {
  await expect(consumeAIStream(response('data: {"type":"error","error":"额度不足"}\n\n'), () => {})).rejects.toThrow('额度不足');
 });
 it('rejects malformed final fields', async () => {
  await expect(consumeAIStream(response('data: {"type":"done","fields":"broken"}\n\n'), () => {})).rejects.toThrow('格式');
 });
 it('isolates AI drafts by editor owner and restores interrupted drafts as partial', () => {
  const a = aiDraftKey('user-a:account:school:week'); const b = aiDraftKey('user-b:account:school:week');
  expect(saveAIDrafts(a, [{ start: '2026-10-01', end: '2026-10-07', fields: [{ title: '工作内容', sort: 1, require: '1', content: '待核对正文' }], status: 'generating' }])).toBe(true);
  expect(loadAIDrafts(a)[0].status).toBe('partial'); expect(loadAIDrafts(b)).toEqual([]);
 });
 it('clears AI drafts through the existing owner-scoped draft cleanup', () => {
  const own = aiDraftKey('xixun-report-draft:1:A:school:week'); const other = aiDraftKey('xixun-report-draft:11:A:school:week');
  localStorage.setItem(own, 'private'); localStorage.setItem(other, 'retain');
  expect(clearUserDrafts(1)).toBe(true); expect(localStorage.getItem(own)).toBeNull(); expect(localStorage.getItem(other)).toBe('retain');
 });
});
