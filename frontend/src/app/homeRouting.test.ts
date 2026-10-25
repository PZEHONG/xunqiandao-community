import { expect, it } from 'vitest';
import { normalizeLegacyHash } from './legacyHash';
it.each(['', '#', '#/', '#/?from=share'])('keeps homepage visit on landing page: %s', hash => {
  expect(normalizeLegacyHash(hash)).toEqual({ path: '/' });
});
it('preserves daily sign deep links', () => {
  expect(normalizeLegacyHash('#/sign?mode=daily')).toEqual({ path: '/sign' });
  expect(normalizeLegacyHash('#auto-panel')).toEqual({ path: '/sign', signMode: 'daily' });
});
