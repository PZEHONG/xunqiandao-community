const legacy: Record<string, string> = { '#records-panel': '/records', '#sign-panel': '/sign', '#week-panel': '/reports/week', '#month-panel': '/reports/month', '#account-panel': '/account' };
const paths = ['/', '/records', '/sign', '/reports/week', '/reports/month', '/account'];
export function normalizeLegacyHash(hash: string): { path: string; signMode?: 'daily' } {
  if (['', '#', '#/'].includes(hash.split('?')[0])) return { path: '/' };
  hash = hash.replace(/^#\/(?=[a-z]+-panel$)/, '#');
  if (hash === '#auto-panel') return { path: '/sign', signMode: 'daily' };
  const path = legacy[hash] ?? hash.replace(/^#/, '').split('?')[0];
  return { path: paths.includes(path) ? path : '/records' };
}
export function replaceLegacyHash() {
  const normalized = normalizeLegacyHash(location.hash);
  const target = `#${normalized.path}${normalized.signMode || location.hash === '#/sign?mode=daily' ? '?mode=daily' : ''}`;
  if (location.hash !== target) history.replaceState(null, '', location.pathname + location.search + target);
}
