import { expect, test, type Page } from '@playwright/test';
import { fixtureSelf, mockAPI } from '../fixtures/api';
import { mockLocations } from '../fixtures/amap';
import { mockSign, pngPhoto } from '../fixtures/sign';
import { mockReports } from '../fixtures/reports';
const screens = [320,375,390,760,768,1024,1440,1920].map(width => ({width,height:900})).concat({width:844,height:390});
async function fits(page: Page) { expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); }
async function actionVisible(page: Page, name: string) {
 const button=page.getByRole('button',{name,exact:true}); await button.scrollIntoViewIfNeeded(); const box=await button.boundingBox(); expect(box).not.toBeNull();
 expect(box!.y).toBeGreaterThanOrEqual(0); expect(box!.y+box!.height).toBeLessThanOrEqual(page.viewportSize()!.height+1);
 const nav=page.getByRole('navigation',{name:'手机导航',exact:true}); if(await nav.isVisible()) { const n=await nav.boundingBox(); expect(box!.y+box!.height).toBeLessThanOrEqual(n!.y+1); }
}
for(const viewport of screens) test(`${viewport.width}x${viewport.height} 五页、长内容和确认操作`,async({page},info)=>{
 test.setTimeout(90000); await page.setViewportSize(viewport); const api=await mockAPI(page); await mockLocations(page,{address:'北京市示例园区较长楼栋地址'.repeat(24)}); const sign=await mockSign(page); const reports=await mockReports(page);
 await page.route('**/api/auth/self',route=>route.fulfill({json:{...fixtureSelf,user:{...fixtureSelf.user,username:'示例学生账号'.repeat(14)},bindings:[{...fixtureSelf.bindings[0],school_name:'示例实习技术学院长学校名称'.repeat(12)}]}}));
 const capture=async(name:string)=>{if([320,760,1440].includes(viewport.width)||viewport.height===390) await page.screenshot({path:info.outputPath(`${name}-${viewport.width}x${viewport.height}.png`)});};
 for(const path of ['/records','/sign','/reports/week','/reports/month','/account']) {
  await page.goto(`#${path}`); await expect(page.getByRole('main')).toBeVisible(); await expect(page.getByRole('button',{name:'2026001',exact:true})).toBeVisible(); await fits(page); await capture(path.split('/').pop()!);
  if(path==='/sign') {
   await page.getByRole('textbox',{name:'地点名称或详细地址'}).fill('示例园区'); await page.getByRole('button',{name:'搜索地点',exact:true}).click(); await page.getByRole('button',{name:/示例园区 北京市/}).click(); await page.getByRole('button',{name:'使用此地点',exact:true}).click();
   await page.getByLabel('签到说明（选填）').fill('核对长说明与换行。'.repeat(60)); await page.getByLabel('选择签到照片').setInputFiles(pngPhoto); await fits(page); await capture('map-photo'); await page.getByRole('button',{name:'核对签到信息',exact:true}).click();
   const dialog=page.getByRole('dialog',{name:'核对本次签到'}); await expect(dialog).toBeVisible(); await expect(dialog.locator('..')).toHaveCSS('opacity','1'); await actionVisible(page,'确认提交签到'); await fits(page); await capture('sign-confirm'); await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); expect(sign.uploads).toBe(0); expect(sign.submissions).toEqual([]);
  }
  if(path.startsWith('/reports/')) {
   await page.getByRole('button',{name:'新建报告',exact:true}).click(); await page.getByLabel('开始日期',{exact:true}).fill('2026-10-01'); await page.getByLabel('结束日期',{exact:true}).fill('2026-10-07'); await page.getByLabel('工作内容',{exact:false}).fill('实习报告长正文用于换行和滚动核对。'.repeat(50)); await fits(page); await capture('report-editor'); await page.getByRole('button',{name:'核对并提交',exact:true}).click(); const dialog=page.getByRole('dialog',{name:'确认提交报告'}); await expect(dialog).toBeVisible(); await expect(dialog.locator('..')).toHaveCSS('opacity','1'); await actionVisible(page,'确认提交'); await fits(page); await capture('report-confirm'); await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); expect(reports.writes).toEqual([]);
  }
  if(path==='/account') {
   await actionVisible(page,'清除浏览器配置'); await page.getByRole('button',{name:'清除浏览器配置',exact:true}).click(); const dialog=page.getByRole('dialog',{name:'确认清除浏览器配置'}); await expect(dialog).toBeVisible(); await expect(dialog.locator('..')).toHaveCSS('opacity','1'); await actionVisible(page,'确认清除'); await fits(page); await capture('account-confirm'); await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0);
   await page.getByRole('button',{name:'退出登录',exact:true}).click(); const logout=page.getByRole('dialog',{name:'确认退出登录'}); await expect(logout.locator('..')).toHaveCSS('opacity','1'); await actionVisible(page,'确认退出'); await capture('logout-confirm'); await page.keyboard.press('Escape'); await expect(logout).toHaveCount(0);
   await page.getByRole('button',{name:'重新连接习讯云',exact:true}).click(); const auth=page.getByRole('dialog',{name:'登录讯签到'}); await expect(auth.locator('..')).toHaveCSS('opacity','1'); await fits(page); await capture('auth'); await page.keyboard.press('Escape'); await expect(auth).toHaveCount(0);
  }
 }
 expect(api.unexpected).toEqual([]); expect(api.writes).toEqual([]);
});
test('新版脚本不可用时仍有旧版入口',async({page})=>{
 await page.route('**/src/main.tsx',route=>route.abort()); await page.goto(''); const legacy=page.getByRole('link',{name:'进入旧版',exact:true}); await expect(legacy).toBeVisible(); await expect(legacy).toHaveAttribute('href','/legacy/');
});
