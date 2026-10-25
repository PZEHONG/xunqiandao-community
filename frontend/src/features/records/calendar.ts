// Exact algorithm migrated from web/static/calendar.js; original lunar/China-date semantics retained.
import type { SignRecord } from '../../api/contracts';
const zone = 'Asia/Shanghai';
const dateFormat = new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' });
let lunarFormat: Intl.DateTimeFormat | undefined;
try { const formatter = new Intl.DateTimeFormat('zh-CN-u-ca-chinese', { timeZone: zone, month: 'long', day: 'numeric' }); if (formatter.resolvedOptions().calendar === 'chinese') lunarFormat = formatter; } catch { /* Solar calendar remains available. */ }
const lunarDays = ['初一','初二','初三','初四','初五','初六','初七','初八','初九','初十','十一','十二','十三','十四','十五','十六','十七','十八','十九','二十','廿一','廿二','廿三','廿四','廿五','廿六','廿七','廿八','廿九','三十'];
const festivals: Record<string, string> = { '正月-1':'春节','正月-15':'元宵节','五月-5':'端午节','七月-7':'七夕','八月-15':'中秋节','九月-9':'重阳节','腊月-8':'腊八节' };
export interface CalendarDay { date: string; day: number; lunar: string; today: boolean; normalCount: number; abnormalCount: number; unknownCount: number }
function chinaDate(date = new Date()) { const parts = Object.fromEntries(dateFormat.formatToParts(date).map(part => [part.type, part.value])); return `${parts.year}-${parts.month}-${parts.day}`; }
function parseMonth(month: string) { if (typeof month !== 'string' || !/^[1-9]\d{3}-(0[1-9]|1[0-2])$/.test(month)) throw new Error('查询月份格式应为 YYYY-MM。'); return month.split('-').map(Number); }
export function currentMonth(now = new Date()) { return chinaDate(now).slice(0, 7); }
export function shiftMonth(month: string, offset: number) { const [year, number] = parseMonth(month); if (!Number.isInteger(offset)) throw new Error('月份偏移无效。'); return chinaDate(new Date(Date.UTC(year, number - 1 + offset, 1, 4))).slice(0, 7); }
function lunarLabel(date: Date) { if (!lunarFormat) return ''; const parts = Object.fromEntries(lunarFormat.formatToParts(date).map(part => [part.type, part.value])); const day = Number(parts.day); return festivals[`${parts.month}-${day}`] || (day === 1 ? parts.month : lunarDays[day - 1]) || ''; }
export function recordDate(record: SignRecord) {
  const raw = record.sign_time;
  if ((typeof raw === 'number' || typeof raw === 'string') && /^\d{10}(\d{3})?$/.test(String(raw))) { const date = new Date(Number(raw) * (String(raw).length === 10 ? 1000 : 1)); if (!Number.isNaN(date.getTime())) return chinaDate(date); }
  const text = typeof record.sign_time_text === 'string' ? record.sign_time_text.trim() : ''; const match = text.match(/^(\d{4}-\d{2}-\d{2})(?:$|[ T])/); if (!match) return '';
  const date = new Date(`${match[1]}T12:00:00+08:00`); return !Number.isNaN(date.getTime()) && chinaDate(date) === match[1] ? match[1] : '';
}
export function buildMonth(month: string, records: unknown[] = [], now = new Date()) {
  const [year, number] = parseMonth(month); const first = new Date(`${month}-01T12:00:00+08:00`); const count = new Date(Date.UTC(year, number, 0)).getUTCDate(); const today = chinaDate(now);
  const byDate = new Map<string, Pick<CalendarDay, 'normalCount' | 'abnormalCount' | 'unknownCount'>>();
  for (const item of Array.isArray(records) ? records : []) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) continue; const record = item as SignRecord; const date = recordDate(record); if (!date.startsWith(`${month}-`)) continue;
    const status = typeof record.status_code_text === 'string' ? record.status_code_text.trim() : ''; const code = String(record.status_code ?? ''); const normal = status ? status === '正常' : code === '0'; const abnormal = status ? status === '异常' : code === '1';
    const entry = byDate.get(date) || { normalCount: 0, abnormalCount: 0, unknownCount: 0 }; entry[normal ? 'normalCount' : abnormal ? 'abnormalCount' : 'unknownCount']++; byDate.set(date, entry);
  }
  const days: (CalendarDay | null)[] = Array(first.getUTCDay()).fill(null);
  for (let day = 1; day <= count; day++) { const date = `${month}-${String(day).padStart(2, '0')}`; days.push({ date, day, lunar: lunarLabel(new Date(`${date}T12:00:00+08:00`)), today: date === today, ...(byDate.get(date) || { normalCount: 0, abnormalCount: 0, unknownCount: 0 }) }); }
  while (days.length % 7) days.push(null);
  return { month, title: `${year}年${number}月`, days, normalDays: [...byDate.values()].filter(day => day.normalCount > 0).length, recordDays: byDate.size };
}
