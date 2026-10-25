import { expect, it, vi } from 'vitest';
import { clearSubmittedDraft, clearUserDrafts, draftKey, loadDraft, restoreFields, saveDraft } from './draftStorage';
const field = (title: string, sort: number, content = '') => ({ title, sort, content, require: '1' });
it('explicit cleanup removes only the current user report drafts and recovery copies across bindings', () => {
  const removed = ['xixun-report-draft:1:A:school:week', 'xixun-report-draft:1:A:school:week:recovery', 'xixun-report-draft:1:B:other:month', 'xixun-report-draft:1:B:other:month:recovery'];
  const kept = ['xixun-report-draft:11:A:school:week', 'xixun-report-draft:2:A:school:week:recovery', 'xixun-workspace-config:1', 'xun-agreement:A', 'unrelated'];
  for (const key of [...removed, ...kept]) localStorage.setItem(key, 'private');
  expect(clearUserDrafts(1)).toBe(true);
  for (const key of removed) expect(localStorage.getItem(key)).toBeNull();
  for (const key of kept) expect(localStorage.getItem(key)).toBe('private');
});
it('cleanup refuses empty identity and reports denied storage instead of claiming success', () => {
  localStorage.setItem('xixun-report-draft:1:A:school:week', 'keep');
  expect(clearUserDrafts('')).toBe(false);
  expect(localStorage.getItem('xixun-report-draft:1:A:school:week')).toBe('keep');
  vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new DOMException('Denied'); });
  expect(clearUserDrafts(1)).toBe(false);
  expect(localStorage.getItem('xixun-report-draft:1:A:school:week')).toBe('keep');
});
const draft = { start: '2026-10-01', end: '2026-10-07', content: [field('工作内容', 1, '已有正文')], orphaned: [], attachments: [{ name: '已上传.pdf', uri: '/reports/old.pdf' }], pendingNames: ['待重选.pdf'] };
it('精确兼容旧草稿键，使用学校名称且各用户/账号/学校/类型隔离', () => {
  const identity = { userID: 11, account: ' A ', school: ' 示例学校 ', kind: 'week' as const };
  const key = draftKey(identity); expect(key).toBe(`xixun-report-draft:11:A:${encodeURIComponent('示例学校')}:week`);
  localStorage.setItem(key, JSON.stringify(draft)); expect(loadDraft(key).draft).toEqual(draft);
  for (const change of [{ userID: 12 }, { account: 'B' }, { school: '另一学校' }, { kind: 'month' as const }]) expect(loadDraft(draftKey({ ...identity, ...change })).draft).toBeNull();
});
it('损坏JSON/不合法草稿安全降级，保留原始内容供恢复', () => {
  for (const raw of ['{broken', 'null', '[]', '"string"', '{"content":"旧正文"}', '{"content":[{"title":"工作内容","content":42}]}']) { localStorage.setItem('draft', raw); const saved = loadDraft('draft'); expect(saved.draft).toBeNull(); expect(saved.status).toBe('corrupt'); expect(saved.raw).toBe(raw); }
});
it('存储不可用明确返回未保存，不假装已写入', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('Quota exceeded'); }); expect(saveDraft('draft', draft).ok).toBe(false);
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new DOMException('Denied'); }); expect(loadDraft('draft').status).toBe('unavailable');
});
it('模板重新排序按唯一标题匹配；新增字段留空，旧正文进入保留区', () => {
  const restored = restoreFields([field('体会', 1), field('工作内容', 2), field('新字段', 3)], { ...draft, content: [field('工作内容', 1, '工作'), field('体会', 2, '体会'), field('已删除字段', 3, '不能丢失')] });
  expect(restored.fields.map(item => item.content)).toEqual(['体会', '工作', '']); expect(restored.orphaned.map(item => item.content)).toEqual(['不能丢失']);
});
it('同名字段按sort逐项消费，歧义内容不写入错误字段', () => {
  const result = restoreFields([field('同名', 2), field('同名', 1), field('同名', 9)], { ...draft, content: [field('同名', 1, '甲'), field('同名', 2, '乙'), field('同名', 3, '丙'), field('同名', 4, '丁')], orphaned: [field('先前保留', 8, '历史')] });
  expect(result.fields.map(item => item.content)).toEqual(['乙', '甲', '']); expect(result.orphaned.map(item => item.content)).toEqual(['历史', '丙', '丁']);
});
it('成功仅删除完全相同的已提交草稿，保留同键更新和其他类型', () => {
  const saved = saveDraft('week', draft); saveDraft('month', draft); expect(clearSubmittedDraft('week', saved.raw)).toBe(true); expect(loadDraft('month').draft).not.toBeNull();
  const before = saveDraft('week', draft); const newer = { ...draft, start: '2026-11-01' }; saveDraft('week', newer); expect(clearSubmittedDraft('week', before.raw)).toBe(false); expect(loadDraft('week').draft?.start).toBe('2026-11-01');
});
it('新模板同名字段不能抢先消耗后面精确sort匹配的正文', () => {
  const result = restoreFields([field('工作内容', 9), field('工作内容', 1)], draft);
  expect(result.fields.map(f => f.content)).toEqual(['', '已有正文']);
  expect(result.orphaned).toEqual([]);
});
it('剩余一个候选也不能把未匹配sort正文填到新增同名字段', () => {
  const result = restoreFields([field('同名', 1), field('同名', 9)], { ...draft, content: [field('同名', 1, '甲'), field('同名', 2, '乙')] });
  expect(result.fields.map(f => f.content)).toEqual(['甲', '']);
  expect(result.orphaned.map(f => f.content)).toEqual(['乙']);
});
it('新旧任一侧标题和sort重复时保留正文，不猜测归属', () => {
  const result = restoreFields([field('同名', 1), field('同名', 1)], { ...draft, content: [field('同名', 1, '甲')] });
  expect(result.fields.map(f => f.content)).toEqual(['', '']);
  expect(result.orphaned.map(f => f.content)).toEqual(['甲']);
  const duplicateSaved = restoreFields([field('同名', 1)], { ...draft, content: [field('同名', 1, '甲'), field('同名', 1, '乙')] });
  expect(duplicateSaved.fields[0].content).toBe('');
  expect(duplicateSaved.orphaned.map(f => f.content)).toEqual(['甲', '乙']);
});
it('原文备份失败时不能覆盖损坏草稿', () => {
  localStorage.setItem('broken', '{原文');
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('Quota exceeded'); });
  expect(saveDraft('broken', draft).ok).toBe(false);
  expect(localStorage.getItem('broken')).toBe('{原文');
});
