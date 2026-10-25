import { expect, it } from 'vitest';
import { attachmentURL, detailAttachments, detailFields, reportDate, reportGrade, reportTime, validDate } from './reportKinds';
it('报告附件保留相对路径与查询参数，仅允许http/https链接', () => {
  expect(attachmentURL('/reports/a.pdf?token=fixture')).toBe('https://file.xixunyun.com/reports/a.pdf?token=fixture'); expect(attachmentURL('https://files.example/a.pdf')).toBe('https://files.example/a.pdf');
  for (const value of ['javascript:alert(1)', 'data:text/html,a', '/a\nb', '/a,b', '', null]) expect(attachmentURL(value)).toBeNull();
  expect(detailAttachments('/a.pdf,javascript:alert(1)')).toEqual(['/a.pdf']);
});
it('报告起止日期兼容年月日、秒/毫秒时间戳及数字字符串，统一北京时间日期', () => {
  expect(reportDate('2026-10-01')).toBe('2026-10-01');
  for (const v of [1790784000, 1790784000000, '1790784000', '1790784000000']) expect(reportDate(v)).toBe('2026-10-01');
  for (const v of [null, undefined, '', 0, '0']) expect(reportDate(v)).toBe('—'); expect(reportDate('未知日期')).toBe('未知日期');
});
it('兼容JSON数组和旧版纯文本详情，不解析HTML', () => {
  expect(detailFields('[{"title":"工作","content":"<script>x</script>"}]')).toEqual([{ title: '工作', content: '<script>x</script>' }]); expect(detailFields('旧正文')).toEqual([{ title: '报告内容', content: '旧正文' }]);
  expect(reportGrade({ score: '1' })).toBe('优秀'); expect(reportGrade({ state: '1' })).toBe('已批阅'); expect(reportTime(0)).toBe('—'); expect(validDate('2026-02-30')).toBe(false);
});
it('批阅等级沿用现有中文业务名称', () => { expect(reportGrade({ score: 3 })).toBe('及格'); expect(reportGrade({ score: '4' })).toBe('不及格'); });
