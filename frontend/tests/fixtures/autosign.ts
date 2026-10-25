import type { Page } from '@playwright/test';
import type { AutoTask } from '../../src/features/autosign/types';
export const fixtureTask: AutoTask = { id: 1, account: '2026001', school_id: '1', name: '每日上班', time: '09:00', enabled: true, revision: 3, remark: '0', remark_label: '上班', comment: '原说明', address: '北京市已保存路2号', address_name: '已保存园区', latitude: '39.9', longitude: '116.4', next_at: '2026-10-18T09:00:00+08:00' };
export async function mockAutoSign(page: Page, options: { conflict?: boolean; hold?: boolean; unknown?: boolean } = {}) {
  let release!: () => void; const gate = new Promise<void>(resolve => { release = resolve; });
  const state = { tasks: [{ ...fixtureTask }], writes: [] as { method: string; path: string; body: Record<string, unknown> }[], conflict: options.conflict || false, reads: 0, release };
  await page.route('**/api/auto-sign/**', async route => {
    const path = new URL(route.request().url()).pathname; const method = route.request().method();
    if (method === 'GET') {
      state.reads++;
      return route.fulfill({ json: path.endsWith('/runs') ? { runs: [{ id: 8, task_id: 1, account: '2026001', name: '每日上班', revision: 2, date: '2026-10-17', scheduled_time: '09:00', status: 'unknown', message: '请先核对签到记录' }] } : { tasks: state.tasks, school_id: '1', mode: 'local' } });
    }
    const body = route.request().postDataJSON(); state.writes.push({ path, method, body });
    if (options.hold) await gate;
    if (options.unknown) return route.fulfill({ status: 502, contentType: 'text/html', body: '<html>gateway</html>' });
    if (state.conflict) return route.fulfill({ status: 409, json: { error: '任务已变更，请刷新后重新确认' } });
    const id = Number(path.split('/')[4] || 0); const old = state.tasks.find(task => task.id === id);
    if (method === 'DELETE') { state.tasks = state.tasks.filter(task => task.id !== id); return route.fulfill({ json: { message: '任务已删除，执行记录仍保留' } }); }
    const task = { ...(old || fixtureTask), ...body, id: id || 2, revision: Number(body.revision || 0) + 1 };
    state.tasks = [task, ...state.tasks.filter(item => item.id !== task.id)];
    return route.fulfill({ json: { task } });
  });
  await page.route('**/api/sign/remarks?*', route => route.fulfill({ json: { remarks: [{ key: '0', value: '上班' }, { key: '1', value: '下班' }] } }));
  return state;
}
