import {test} from 'vitest';
import assert from 'node:assert/strict';
import * as calendar from './calendar';

test('October 2026 starts on Thursday in a Sunday-first, five-week calendar', () => {
  const result = calendar.buildMonth('2026-10', [], new Date('2026-10-15T08:00:00Z'));
  assert.equal(result.title, '2026年10月');
  assert.equal(result.days.length, 35);
  assert.deepEqual(result.days.slice(0, 4), [null, null, null, null]);
  assert.equal(result.days[4]!.date, '2026-10-01');
  assert.equal(result.days.at(-1)!.day, 31);
  assert.equal(result.days.filter(day => day?.today).length, 1);
});

test('lunar labels match the reference calendar, including Chongyang', () => {
  const days = calendar.buildMonth('2026-10').days.filter(day => day !== null);
  assert.equal(days[0].lunar, '廿一');
  assert.equal(days[9].lunar, '九月');
  assert.equal(days[14].lunar, '初六');
  assert.equal(days[17].lunar, '重阳节');
});

test('uses China dates, deduplicates signed days, and keeps abnormal or unknown records distinct', () => {
  const result = calendar.buildMonth('2026-10', [
    {sign_time: '1791984776', status_code_text: '正常'},
    {sign_time_text: '2026-10-14', status_code: '0'},
    {sign_time_text: '2026-10-15', status_code_text: '异常'},
    {sign_time_text: '2026-10-16', status_code: '99'},
    {sign_time_text: '2026-10-17', status_code: '0', status_code_text: '异常'},
    {sign_time_text: '2026-09-14', status_code: '0'},
    {sign_time_text: '2026-10-32', status_code: '0'},
    {sign_time_text: '<img src=x>', status_code: '0'},
    null
  ], new Date('2026-10-14T17:00:00Z'));
  const days = result.days.filter(day => day !== null);
  assert.equal(days[13].normalCount, 2);
  assert.equal(days[14].abnormalCount, 1);
  assert.equal(days[15].unknownCount, 1);
  assert.equal(days[16].normalCount, 0);
  assert.equal(days[16].abnormalCount, 1);
  assert.equal(days[14].today, true);
  assert.equal(result.normalDays, 1);
  assert.equal(result.recordDays, 4);
});

test('month navigation handles year boundaries, leap years and six-week months', () => {
  assert.equal(calendar.shiftMonth('2026-01', -1), '2025-12');
  assert.equal(calendar.shiftMonth('2026-12', 1), '2027-01');
  assert.equal(calendar.buildMonth('2024-02').days.filter(day => day !== null).length, 29);
  assert.equal(calendar.buildMonth('2026-05').days.length, 42);
  assert.throws(() => calendar.buildMonth('2026-13'));
  assert.throws(() => calendar.buildMonth('2026-1'));
  assert.throws(() => calendar.buildMonth('<script>'));
});
