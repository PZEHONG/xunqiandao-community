import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { checkRelease } from './check-release.mjs';

test('release guard accepts normal output and rejects demo code or missing bundles', async () => {
  const root = await mkdtemp(join(tmpdir(), 'xun-release-check-'));
  try {
    await writeFile(join(root, 'index.html'), '<div id="root"></div>');
    await assert.rejects(checkRelease(root), /no JavaScript bundle/);
    await mkdir(join(root, 'assets'));
    const bundle = join(root, 'assets', 'app.js');
    await writeFile(bundle, 'console.log("normal application");');
    assert.equal(await checkRelease(root), 1);
    for (const marker of ['DEMO2026001', 'Demo123456', 'DEMO-VIP-30', 'xun-demo-active:', '/api/auth/demo', '广东大学（演示）']) {
      await writeFile(bundle, `console.log(${JSON.stringify(marker)});`);
      await assert.rejects(checkRelease(root), /Release blocked: demo marker/);
    }
  } finally { await rm(root, { recursive: true, force: true }); }
});
