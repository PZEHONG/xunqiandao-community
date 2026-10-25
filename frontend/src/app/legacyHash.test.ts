import { describe, expect, it } from 'vitest';
import { normalizeLegacyHash } from './legacyHash';
describe('兼容旧链接', () => {
  it.each([['#records-panel', '/records'], ['#sign-panel', '/sign'], ['#week-panel', '/reports/week'], ['#month-panel', '/reports/month'], ['#account-panel', '/account'], ['#/reports/month', '/reports/month'], ['#invalid', '/records']])('%s → %s', (hash, path) => { expect(normalizeLegacyHash(hash)).toEqual({ path }); });
  it('每日模式在签到页内打开', () => { expect(normalizeLegacyHash('#auto-panel')).toEqual({ path: '/sign', signMode: 'daily' }); });
  it('路由器补齐斜杠后仍识别旧哈希', () => { expect(normalizeLegacyHash('#/week-panel')).toEqual({ path: '/reports/week' }); expect(normalizeLegacyHash('#/auto-panel')).toEqual({ path: '/sign', signMode: 'daily' }); });
});
