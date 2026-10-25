import type { Page } from '@playwright/test';
export async function mockSign(page: Page, options: { failOnce?: boolean; unknown?: boolean; uploadFailure?: boolean; holdSign?: boolean } = {}) {
  const state = { uploads: 0, submissions: [] as Record<string, unknown>[], release: () => {} };
  await page.route('**/api/sign/remarks?*', route => route.fulfill({ json: { remarks: [{ key: '0', value: '上班' }, { key: '1', value: '下班' }] } }));
  await page.route('**/api/sign/photos', route => { state.uploads++; return route.fulfill(options.uploadFailure ? { status: 400, json: { error: '模拟照片上传失败' } } : { json: { uri: '/uploads/fixture.png' } }); });
  await page.route('**/api/sign', async route => {
    state.submissions.push(route.request().postDataJSON());
    if (options.holdSign) await new Promise<void>(resolve => { state.release = resolve; });
    if (options.unknown) return route.fulfill({ contentType: 'text/html', body: '<html>not-json</html>' });
    if (options.failOnce && state.submissions.length === 1) return route.fulfill({ status: 400, json: { error: '模拟签到失败，请重新核对' } });
    return route.fulfill({ json: { message: '签到成功' } });
  });
  return state;
}
export const pngPhoto = { name: 'fixture.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB1sAAAAASUVORK5CYII=', 'base64') };
