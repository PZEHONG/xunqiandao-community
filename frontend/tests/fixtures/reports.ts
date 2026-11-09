import type { Page } from '@playwright/test';
export const reportDraftKey = (kind = 'week') => `xixun-report-draft:11:2026001:${encodeURIComponent('示例实习技术学院')}:${kind}`;
export async function mockReports(page: Page) {
  const state = { fail: false, unknown: false, writes: [] as { path: string; body: Record<string, unknown> | null }[], queries: [] as string[] };
  await page.route('**/api/reports**', async route => {
    const r = route.request(); const url = new URL(r.url());
    if (r.method() !== 'GET') {
      state.writes.push({ path: url.pathname, body: url.pathname.endsWith('/attachments') ? null : r.postDataJSON() });
      if (url.pathname.endsWith('/attachments')) return route.fulfill({ json: { uri: '/reports/uploaded.pdf' } });
      if (state.unknown) return route.fulfill({ contentType: 'text/html', body: '<html>gateway</html>' });
      return state.fail ? route.fulfill({ status: 400, json: { error: '模拟报告提交失败' } }) : route.fulfill({ json: { message: '报告提交成功。', data: {} } });
    }
    if (url.pathname.endsWith('/template')) return route.fulfill({ json: { data: [{ title: '工作内容', sort: 1, require: '1' }, { title: '实习体会', sort: 2, require: '0' }, { title: '新增字段', sort: 3, require: '0' }] } });
    if (url.pathname.endsWith('/detail')) return route.fulfill({ json: { data: { id: '42', student_name: '示例学生', start_date: 1790784000, end_date: '1791302400000', score: 1, state: 1, teacher_name: '张老师', mark_result: '继续保持，注意总结。', content: JSON.stringify([{ title: '工作内容', content: '报告正文只显示文本<script>danger</script>' }]), attachment: '/reports/approved.pdf,javascript:alert(1)' } } });
    state.queries.push(url.search); return route.fulfill({ json: { data: { list: [{ id: '42', start_date: '2026-10-01', end_date: '2026-10-07', create_time: 1791244800, state: 1, score: 1, teacher_name: '张老师', repulsed: '0' }], count: '16' } } });
  });
  return state;
}
