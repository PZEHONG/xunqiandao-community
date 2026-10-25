import { readFileSync } from 'node:fs';
import { expect, it, vi } from 'vitest';

function legacy(userID = 1) {
  document.body.innerHTML = readFileSync('../web/static/index.html', 'utf8');
  const toast = vi.fn();
  Object.assign(window, { Workspace: { userID: () => userID, version: () => 1, toast } });
  window.eval(readFileSync('../web/static/reports.js', 'utf8'));
  return { button: document.getElementById('clear-report-drafts') as HTMLButtonElement, toast };
}
it('legacy cleanup requires confirmation and removes only current user drafts and recovery copies', () => {
  const keys = ['xixun-report-draft:1:A:school:week', 'xixun-report-draft:1:A:school:week:recovery', 'xixun-report-draft:11:A:school:week', 'xixun-workspace-config:1'];
  for (const key of keys) localStorage.setItem(key, 'private');
  const { button, toast } = legacy(); expect(button).not.toBeNull();
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false); button.click();
  expect(localStorage.getItem(keys[0])).toBe('private');
  confirm.mockReturnValue(true); button.click();
  expect(localStorage.getItem(keys[0])).toBeNull(); expect(localStorage.getItem(keys[1])).toBeNull();
  expect(localStorage.getItem(keys[2])).toBe('private'); expect(localStorage.getItem(keys[3])).toBe('private');
  expect(toast).toHaveBeenCalledWith(expect.stringContaining('此浏览器'));
});
it('legacy cleanup reports denied storage without claiming deletion', () => {
  localStorage.setItem('xixun-report-draft:1:A:school:week', 'keep');
  const { button, toast } = legacy(); expect(button).not.toBeNull();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => { throw new DOMException('Denied'); });
  button.click(); expect(localStorage.getItem('xixun-report-draft:1:A:school:week')).toBe('keep');
  expect(toast).toHaveBeenCalledWith(expect.stringContaining('未能完整清除'), true);
});
