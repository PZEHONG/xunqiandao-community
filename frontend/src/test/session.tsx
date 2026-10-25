import type { SessionState } from '../session/useSession';
export function testSession(overrides: Partial<SessionState> = {}): SessionState {
  return { status: 'authenticated', user: { id: 1, username: 'test' }, bindings: [{ account: 'A', school_id: '1', school_name: '示例学校' }], epoch: 0, error: '', authConfig: { agreement_version: 'v1' }, loginOpen: false, loginPane: 'login', reconnecting: false, authBusy: false, loginKey: 0, configCleared: false, message: '', clearConfig: () => {}, restore: async () => {}, logout: async () => {}, openLogin: () => {}, closeLogin: () => {}, reconnect: () => {}, authenticate: async () => {}, ...overrides };
}
