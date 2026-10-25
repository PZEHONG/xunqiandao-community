import { expect, test } from '@playwright/test';
import { mockAPI } from '../fixtures/api';
const links = [['#records-panel', '/records', '签到记录'], ['#sign-panel', '/sign', '签到'], ['#week-panel', '/reports/week', '实习周报'], ['#month-panel', '/reports/month', '实习月报'], ['#account-panel', '/account', '账号设置'], ['#auto-panel', '/sign?mode=daily', '签到']];
for (const width of [320, 1440]) test(`${width}px旧哈希、五项导航、历史及焦点`, async ({ page }, testInfo) => {
  await mockAPI(page); await page.setViewportSize({ width, height: 900 });
  for (const [hash, path, label] of links) {
    await page.goto(hash); await expect(page).toHaveURL(new RegExp(`#${path.replace('?', '\\?')}$`)); await expect(page).toHaveTitle(`${label} · 讯签到`);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  }
  const nav = page.getByRole('navigation', { name: width <= 760 ? '手机导航' : '主导航', exact: true });
  await nav.getByRole('link', { name: '实习周报', exact: true }).click(); await expect(page.getByRole('main')).toBeFocused();
  await nav.getByRole('link', { name: '账号设置', exact: true }).click(); await page.goBack(); await expect(page).toHaveTitle('实习周报 · 讯签到'); await page.goForward(); await expect(page).toHaveTitle('账号设置 · 讯签到');
  await page.screenshot({ path: testInfo.outputPath(`navigation-${width}.png`), fullPage: true });
});
test('运行中收到旧哈希同样使用replace转换', async ({ page }) => { await mockAPI(page); await page.goto(''); await page.evaluate(() => { location.hash = 'week-panel'; }); await expect(page).toHaveURL(/#\/reports\/week$/); await expect(page).toHaveTitle('实习周报 · 讯签到'); });
