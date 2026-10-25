import type { BrowserContext, Page } from '@playwright/test';
export const fixtureSelf = { user: { id: 11, username: '2026001', student_number: '2026001' }, bindings: [{ account: '2026001', school_id: '1', school_name: '示例实习技术学院' }], consent_accepted: true, agreement_version: 'fixture-v1' };
export function fixtureSnapshot(month: string) { return { month_date: month, continuous_sign_in: '6', records: [{ sign_time_text: `${month}-14 08:30:00`, status_code: '0', status_code_text: '正常', address_name: '示例园区', address: '用于布局测试的较长地址'.repeat(8), remark_text: '上班' }, { sign_time_text: `${month}-15 18:00:00`, status_code_text: '异常', comment: '示例说明' }, { sign_time_text: `${month}-16 09:00:00`, status_code: '99' }] }; }
export interface FixtureOptions { authenticated?: boolean; failMonth?: string; upstreamExpired?: boolean; schoolDelay?: number }
export async function mockAPI(target: BrowserContext | Page, options: FixtureOptions = {}) {
  const state = { authenticated: options.authenticated ?? true, writes: [] as { path: string; body: unknown }[], queries: [] as string[], unexpected: [] as string[] };
  await target.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (!['127.0.0.1', 'localhost'].includes(url.hostname)) { await route.abort(); return; }
    if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/_AMapService')) { await route.continue(); return; }
    const fulfill = (json: unknown, status = 200) => route.fulfill({ status, json });
    const path = url.pathname;
    if (path === '/api/membership/self') return fulfill({ status: 'active', auto_sign: true, expires_at: '2099-01-01T00:00:00Z', server_time: new Date().toISOString(), revision: 1 });
    if (path === '/api/membership/redemptions') return fulfill({ items: [], total: 0, page: 1, size: 20 });
    if (path === '/api/auth/status') return fulfill({ agreement_version: 'fixture-v1', official_login: true, registration: false, email_login: false });
    if (path === '/api/auth/self') return fulfill(state.authenticated ? fixtureSelf : { code: 'AUTH_REQUIRED', error: '请登录' }, state.authenticated ? 200 : 401);
    if (['/api/auth/login', '/api/auth/official', '/api/auth/logout'].includes(path)) {
      state.writes.push({ path, body: route.request().postDataJSON() }); state.authenticated = !path.endsWith('/logout'); return fulfill({ user: fixtureSelf.user, consent_accepted: true, agreement_version: 'fixture-v1' });
    }
    if (path === '/api/auth/schools') { if (options.schoolDelay) await new Promise(resolve => setTimeout(resolve, options.schoolDelay)); return fulfill({ schools: [{ school_id: '1', school_name: '示例实习技术学院' }] }); }
    if (path === '/api/query') {
      const month = url.searchParams.get('month_date') || '2026-10'; state.queries.push(month);
      if (options.upstreamExpired) return fulfill({ code: 'UPSTREAM_LOGIN_REQUIRED', error: '请重新连接习讯云' }, 401);
      if (month === options.failMonth) return fulfill({ error: '模拟查询失败' }, 502);
      return fulfill({ data: fixtureSnapshot(month) });
    }
    if (path.includes('map') || path.includes('location') || path.startsWith('/_AMapService')) return fulfill({ enabled: false, error: '测试中不连接真实地图' }, 503);
    if (path === '/api/sign/remarks') return fulfill({ remarks: [] });
    if (path.includes('/auto-sign')) return fulfill({ tasks: [], runs: [] });
    if (path.startsWith('/api/reports')) return fulfill({ data: { list: [], reports: [], total: 0 } });
    state.unexpected.push(`${route.request().method()} ${path}`);
    return fulfill({ error: '未定义的模拟接口已拦截' }, 501);
  });
  return state;
}
