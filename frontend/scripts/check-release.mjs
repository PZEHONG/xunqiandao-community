import { readdir, readFile, stat } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { URL } from 'node:url';
import process from 'node:process';
import console from 'node:console';

export async function checkRelease(directory) {
  const root = resolve(directory);
  await stat(join(root, 'index.html'));
  let bundles = 0;
  const forbidden = ['DEMO2026001', 'Demo123456', 'DEMO-VIP-30', 'xun-demo-active:', '/api/auth/demo', '广东大学（演示）'];
  async function visit(folder) {
    for (const item of await readdir(folder, { withFileTypes: true })) {
      const path = join(folder, item.name);
      if (item.isDirectory()) await visit(path);
      else if (/\.(js|html|map)$/.test(item.name)) {
        if (item.name.endsWith('.js')) bundles++;
        const text = await readFile(path, 'utf8');
        const marker = forbidden.find(value => text.includes(value));
        if (marker) throw new Error(`Release blocked: demo marker ${marker} found in ${path}`);
      }
    }
  }
  await visit(root);
  if (!bundles) throw new Error('Release blocked: no JavaScript bundle found');
  return bundles;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const directory = process.argv[2] || fileURLToPath(new URL('../../web/static/berry', import.meta.url));
  try { console.log(`Release isolation passed (${await checkRelease(directory)} bundles)`); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
