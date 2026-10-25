import { buildMonth } from './calendar';
import type { SignRecord } from '../../api/contracts';
import './records.css';
export function SignCalendar({ month, records }: { month: string; records: SignRecord[] }) {
  const calendar = buildMonth(month, records);
  return <div className="sign-calendar" role="grid" aria-label={`${calendar.title}签到日历`}><div className="calendar-row" role="row">{['日', '一', '二', '三', '四', '五', '六'].map(label => <div role="columnheader" key={label}>{label}</div>)}</div>{Array.from({ length: calendar.days.length / 7 }, (_, row) => <div className="calendar-row" role="row" key={row}>{calendar.days.slice(row * 7, row * 7 + 7).map((day, index) => <div role="gridcell" key={day?.date ?? `empty-${index}`} className={`calendar-cell ${day?.normalCount ? 'signed' : ''} ${day?.today ? 'today' : ''}`} aria-label={day ? `${day.date} ${day.lunar}${day.today ? ' 今天' : ''}${day.normalCount ? ` 正常${day.normalCount}条` : ''}${day.abnormalCount ? ` 异常${day.abnormalCount}条` : ''}${day.unknownCount ? ` 未知${day.unknownCount}条` : ''}` : '空白'}>{day && <><span className="day-number">{day.day}</span><span className="lunar">{day.lunar}</span>{(day.abnormalCount > 0 || day.unknownCount > 0) && <span className="day-warning">{day.abnormalCount ? '异常' : '未知'}</span>}</>}</div>)}</div>)}</div>;
}
