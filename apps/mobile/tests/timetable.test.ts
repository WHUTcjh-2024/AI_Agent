import assert from 'node:assert/strict';
import { test } from 'node:test';

import { belongsToWeek, courseSchema, formatWeeks, getCourseColor, layoutDay, type Course } from '../src/features/timetable/domain/course';
import { addDays, formatDateRange, formatLastImported, getCurrentAcademicWeek, getSchoolDate, getWeekDates, getWeekday, isCalendarDate } from '../src/features/timetable/domain/date';
import { timetableSchema } from '../src/features/timetable/domain/timetable';
import { MockCourseProvider } from '../src/features/timetable/providers/mock-course-provider';
import { normalizeJwappCourses, parseWeekBitmap, parseJwappCurrentUser } from '../src/features/timetable/providers/jwapp/jwapp-parser';
import { TimetableRepository, TIMETABLE_STORAGE_KEY } from '../src/features/timetable/store/timetable-repository';

const row = { KCM: '大学物理', JASMC: '博学北楼305', SKJS: '张老师', SKXQ: '3', KSJC: '3', JSJC: '5', SKZC: '11110000' };
const calendar = { datas: { cxxljc: { rows: [{ XQKSRQ: '2026-08-31 00:00:00' }] } } };
function normalize(rows: unknown[]) {
  return normalizeJwappCourses({ datas: { cxxskcb: { rows } } }, calendar, '2026-2027-1');
}
const course = (): Course => normalize([row]).courses[0];

for (const [label, bitmap, expected] of [
  ['连续周', '11110000', [1, 2, 3, 4]], ['单周', '10101010', [1, 3, 5, 7]],
  ['双周', '01010101', [2, 4, 6, 8]], ['不连续周', '11001001', [1, 2, 5, 8]],
  ['空字符串', '', []], ['全零', '000000', []], ['最大周数', '0'.repeat(63) + '1', [64]],
] as const) test(`SKZC ${label}`, () => assert.deepEqual(parseWeekBitmap(bitmap), expected));

for (const value of ['10201', '1 0', 'abc', '１１０', '1\n', '1'.repeat(65), '0'.repeat(100000)]) {
  test(`SKZC rejects invalid input ${value.slice(0, 12)} / ${value.length}`, () => assert.throws(() => parseWeekBitmap(value)));
}
test('SKZC never coerces non-strings', () => assert.throws(() => parseWeekBitmap(111 as unknown as string)));
test('normalize maps fields and coerces numeric strings', () => {
  assert.deepEqual(course(), { id: 'whut-1', name: '大学物理', room: '博学北楼305', teacher: '张老师', weekday: 3,
    startSection: 3, endSection: 5, weeks: [1, 2, 3, 4], source: { schoolId: 'whut', provider: 'whut-bachelor' } });
  assert.ok(timetableSchema.safeParse(normalize([row])).success);
});
test('normalize tolerates optional teacher/room and weekends', () => {
  const result = normalize([{ ...row, SKJS: null, JASMC: undefined, SKXQ: 6 }, { ...row, SKXQ: 7 }]);
  assert.equal(result.courses[0].teacher, ''); assert.equal(result.courses[0].room, '');
  assert.deepEqual(result.courses.map((item) => item.weekday), [6, 7]);
});
test('invalid rows filtered; duplicates removed; conflicts retained', () => {
  const result = normalize([row, row, null, { ...row, KCM: '' }, { ...row, SKXQ: 8 }, { ...row, KSJC: 6, JSJC: 5 },
    { ...row, JSJC: 17 }, { ...row, SKZC: '10x' }, { ...row, KCM: '冲突课程' }]);
  assert.equal(result.courses.length, 2); assert.equal(result.skippedRows, 6);
});
test('valid empty timetable succeeds but all-invalid rows fail', () => {
  assert.deepEqual(normalize([]).courses, []);
  assert.throws(() => normalize([{}]));
});
test('unexpected school response and missing calendar rejected', () => {
  for (const invalid of [null, {}, { datas: { cxxskcb: { rows: {} } } }]) {
    assert.throws(() => normalizeJwappCourses(invalid, calendar, '2026-2027-1'));
  }
  assert.throws(() => normalizeJwappCourses({ datas: { cxxskcb: { rows: [] } } }, {}, '2026-2027-1'));
});
test('current user and term must come from authenticated response', () => {
  assert.deepEqual(parseJwappCurrentUser({ datas: { userId: 'SYNTHETIC-STUDENT', welcomeInfo: { xnxqdm: '2026-2027-1' } } }),
    { studentId: 'SYNTHETIC-STUDENT', termCode: '2026-2027-1' });
  for (const invalid of [{}, { datas: { userId: 'x', welcomeInfo: { xnxqdm: 'unknown' } } }]) assert.throws(() => parseJwappCurrentUser(invalid));
});
test('calendar rejects impossible dates instead of silently rolling over', () => {
  assert.equal(isCalendarDate('2026-02-29'), false); assert.equal(isCalendarDate('2028-02-29'), true);
  assert.equal(isCalendarDate('2026-9-1'), false); assert.equal(isCalendarDate('2026-04-31'), false);
  assert.throws(() => normalizeJwappCourses({ datas: { cxxskcb: { rows: [] } } }, { datas: { cxxljc: { rows: [{ XQKSRQ: '2026-02-30' }] } } }, '2026-2027-1'));
});
test('academic week changes at Shanghai Monday midnight', () => {
  assert.equal(getCurrentAcademicWeek('2026-08-31', undefined, new Date('2026-09-06T15:59:59Z')), 1);
  assert.equal(getCurrentAcademicWeek('2026-08-31', undefined, new Date('2026-09-06T16:00:00Z')), 2);
  assert.equal(getCurrentAcademicWeek('2026-08-31', undefined, new Date('2026-08-30T15:59:59Z')), 0);
});
test('non-Monday start aligns to calendar Monday and keeps weekends', () => {
  assert.equal(getWeekDates('2026-09-02', 1)[0], '2026-08-31');
  assert.equal(getWeekday('2026-09-05'), 6); assert.equal(getWeekday('2026-09-06'), 7);
});
test('month/year rollover is calendar-safe', () => {
  assert.deepEqual(getWeekDates('2026-12-28', 1), ['2026-12-28', '2026-12-29', '2026-12-30', '2026-12-31', '2027-01-01', '2027-01-02', '2027-01-03']);
  assert.equal(getCurrentAcademicWeek('2026-12-28', undefined, new Date('2027-01-03T16:00:00Z')), 2);
  assert.equal(addDays('2028-02-28', 1), '2028-02-29');
  assert.equal(formatDateRange(getWeekDates('2026-09-28', 1)), '9月28日 – 10月4日');
});
test('Shanghai date is independent of device time zone, including US DST', () => {
  const prior = process.env.TZ;
  try {
    for (const timezone of ['Europe/London', 'America/New_York', 'America/Los_Angeles', 'Asia/Shanghai']) {
      process.env.TZ = timezone;
      assert.equal(getSchoolDate(new Date('2026-11-01T16:01:00Z')), '2026-11-02');
      assert.equal(getCurrentAcademicWeek('2026-08-31', undefined, new Date('2026-11-01T16:01:00Z')), 10);
    }
  } finally { if (prior === undefined) delete process.env.TZ; else process.env.TZ = prior; }
});
test('timezone argument supports future school providers', () => {
  const instant = new Date('2026-09-06T16:30:00Z');
  assert.equal(getCurrentAcademicWeek('2026-08-31', 'Europe/London', instant), 1);
  assert.equal(getCurrentAcademicWeek('2026-08-31', 'Asia/Shanghai', instant), 2);
});
test('last imported text uses the school date and time', () => {
  assert.equal(formatLastImported('2026-09-03T10:32:00Z', 'Asia/Shanghai', new Date('2026-09-03T12:00:00Z')), '今天 18:32');
});
test('exact-week membership and formatting', () => {
  const value = { ...course(), weeks: [1, 3, 5, 7] };
  assert.equal(belongsToWeek(value, 2), false); assert.equal(belongsToWeek(value, 3), true);
  assert.equal(formatWeeks(value.weeks), '1–7周（单）'); assert.equal(formatWeeks([2, 4, 6]), '2–6周（双）');
  assert.equal(formatWeeks([1, 2, 3, 7, 10, 11]), '1–3、7、10–11周');
});
test('course colors deterministic and Unicode normalization stable', () => {
  assert.deepEqual(getCourseColor('大学物理'), getCourseColor('大学物理'));
  assert.deepEqual(getCourseColor('café'), getCourseColor('cafe\u0301'));
  assert.deepEqual(getCourseColor('高等数学'), { background: '#F0EDF7', foreground: '#51456F' });
});
test('schema rejects invalid ranges, weekdays, empty/unsorted/duplicate weeks and numeric strings', () => {
  for (const patch of [{ name: '' }, { id: '' }, { weekday: 0 }, { weekday: 8 }, { weekday: '1' }, { startSection: 0 },
    { endSection: 17 }, { startSection: 6, endSection: 5 }, { weeks: [] }, { weeks: [2, 1] }, { weeks: [1, 1] }, { weeks: [65] }]) {
    assert.equal(courseSchema.safeParse({ ...course(), ...patch }).success, false, JSON.stringify(patch));
  }
});
test('schema strips extra sensitive fields at both boundaries', () => {
  const safe = timetableSchema.parse({ ...normalize([row]), studentId: 'private', password: 'private',
    courses: [{ ...course(), cookie: 'private', source: { ...course().source, session: 'private' } }] });
  assert.equal(JSON.stringify(safe).includes('private'), false);
});
test('schema rejects inconsistent source, duplicate IDs, unknown version/timezone', () => {
  const original = normalize([row]);
  for (const invalid of [{ ...original, schoolId: 'other' }, { ...original, courses: [course(), course()] },
    { ...original, version: 2 }, { ...original, timezone: 'invalid-zone' }, { ...original, lastImportedAt: 'oops' }]) {
    assert.equal(timetableSchema.safeParse(invalid).success, false);
  }
});
test('conflict layout preserves both courses and never overlaps within a lane', () => {
  const courses = [
    { ...course(), id: 'a', startSection: 1, endSection: 3 },
    { ...course(), id: 'b', startSection: 2, endSection: 4 },
    { ...course(), id: 'c', startSection: 4, endSection: 5 },
    { ...course(), id: 'd', startSection: 8, endSection: 9 },
  ];
  const result = layoutDay(courses);
  assert.deepEqual(result.map((item) => [item.lane, item.laneCount]), [[0, 2], [1, 2], [0, 2], [0, 1]]);
  assert.equal(result.length, courses.length);
});
test('mock includes five requested subjects, weekends, conflicts and missing optional data', async () => {
  const result = await new MockCourseProvider().importCourses();
  assert.ok(timetableSchema.safeParse(result).success);
  for (const name of ['大学物理', '高等数学', '大学英语', '信号与系统', '体育']) assert.ok(result.courses.some((item) => item.name === name));
  assert.ok(result.courses.some((item) => item.weekday === 6)); assert.ok(result.courses.some((item) => item.weekday === 7));
  assert.ok(result.courses.some((item) => !item.teacher)); assert.ok(result.courses.some((item) => !item.room));
  const controller = new AbortController(); controller.abort();
  await assert.rejects(new MockCourseProvider().importCourses(controller.signal));
});
test('repository survives reopen, clears only its own key, and strips unknown values', async () => {
  const map = new Map<string, string>([['unrelated', 'keep']]);
  const storage = { getItem: async (key: string) => map.get(key) ?? null, setItem: async (key: string, value: string) => { map.set(key, value); }, removeItem: async (key: string) => { map.delete(key); } };
  const first = new TimetableRepository(storage);
  assert.equal(await first.load(), null);
  const data = await new MockCourseProvider().importCourses();
  await first.save(data);
  assert.deepEqual(await new TimetableRepository(storage).load(), data);
  assert.ok(map.has(TIMETABLE_STORAGE_KEY));
  await first.clear(); assert.equal(await first.load(), null); assert.equal(map.get('unrelated'), 'keep');
});
test('corrupt storage fails safely, failed refresh retains old disk data', async () => {
  let value = JSON.stringify(normalize([row]));
  const repository = new TimetableRepository({ getItem: async () => value, setItem: async () => { throw new Error('disk full'); }, removeItem: async () => {} });
  await assert.rejects(repository.save(await new MockCourseProvider().importCourses()));
  assert.equal((await repository.load())?.courses[0].name, '大学物理');
  value = '{broken'; await assert.rejects(repository.load());
});
