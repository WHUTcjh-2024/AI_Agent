import { schoolAdapter } from '../../../../config/school.generated';

/** Pure normalizer, bundled into the school context and exercised by Native tests. */
export function createJwappNormalizer() {
  const record = (value: unknown): Record<string, unknown> =>
    value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
  const fail = (): never => { throw new Error('FORMAT'); };
  const integer = (value: unknown): number => {
    if (typeof value === 'string') value = value.trim();
    if (typeof value !== 'number' && !(typeof value === 'string' && /^\d+$/.test(value))) return NaN;
    return Number(value);
  };
  const cleanText = (value: unknown) => typeof value === 'string' ? value.trim() : '';
  // Explicit failure envelopes must never be mistaken for a valid empty result.
  // Do not infer success from undocumented code values or forward school messages.
  function assertSuccess(value: unknown) {
    const response = record(value);
    if (response.code === 401 || response.code === '401' || response.code === 403 || response.code === '403') throw new Error('AUTH');
    if (response.success === false) throw new Error('SYSTEM');
  }
  function parseWeekBitmap(value: string): number[] {
    if (typeof value !== 'string' || value.length > 64) return fail();
    const weeks: number[] = [];
    for (let index = 0; index < value.length; index++) {
      if (value[index] === '1') weeks.push(index + 1);
      else if (value[index] !== '0') return fail();
    }
    return weeks;
  }
  function currentUser(value: unknown) {
    assertSuccess(value);
    const data = record(record(value).datas);
    const studentId = data.userId;
    const termCode = record(data.welcomeInfo).xnxqdm;
    if (typeof studentId !== 'string' || !studentId.trim() || studentId.length > 80 ||
        typeof termCode !== 'string' || !/^\d{4}-\d{4}-[1-3]$/.test(termCode)) return fail();
    return { studentId, termCode };
  }
  function normalize(courseResponse: unknown, calendarResponse: unknown, termCode: string) {
    assertSuccess(courseResponse);
    assertSuccess(calendarResponse);
    if (!/^\d{4}-\d{4}-[1-3]$/.test(termCode)) return fail();
    const rows = record(record(record(courseResponse).datas).cxxskcb).rows;
    const calendar = record(record(record(calendarResponse).datas).cxxljc).rows;
    if (!Array.isArray(rows) || rows.length > 2000 || !Array.isArray(calendar) || !calendar.length) return fail();
    const start = record(calendar[0]).XQKSRQ;
    const termStartDate = typeof start === 'string' ? start.split(/[ T]/)[0] : '';
    const parsed = new Date(`${termStartDate}T00:00:00Z`);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(termStartDate) || !Number.isFinite(parsed.getTime()) || parsed.toISOString().slice(0, 10) !== termStartDate) return fail();
    const courses = [];
    const seen = new Set<string>();
    let skippedRows = 0;
    for (const value of rows) {
      const row = record(value);
      let weeks: number[];
      try { weeks = parseWeekBitmap(row.SKZC as string); } catch { skippedRows++; continue; }
      const name = cleanText(row.KCM);
      const teacher = cleanText(row.SKJS);
      const room = cleanText(row.JASMC);
      const weekday = integer(row.SKXQ);
      const startSection = integer(row.KSJC);
      const endSection = integer(row.JSJC);
      if (!name || [name, teacher, room].some((text) => text.length > 200) || !weeks.length || !Number.isInteger(weekday) || weekday < 1 || weekday > 7 ||
          !Number.isInteger(startSection) || !Number.isInteger(endSection) || startSection < 1 || endSection > 16 || endSection < startSection) {
        skippedRows++; continue;
      }
      const signature = JSON.stringify([name, teacher, room, weekday, startSection, endSection, weeks]);
      if (seen.has(signature)) continue;
      seen.add(signature);
      courses.push({ id: `${schoolAdapter.schoolId}-${courses.length + 1}`, name, teacher, room, weekday, startSection, endSection, weeks,
        source: { schoolId: schoolAdapter.schoolId, provider: schoolAdapter.timetable.provider_id } });
    }
    if (rows.length && !courses.length) return fail();
    return { version: 1 as const, schoolId: schoolAdapter.schoolId, provider: schoolAdapter.timetable.provider_id, termCode, termStartDate,
      timezone: schoolAdapter.timetable.timezone, lastImportedAt: new Date().toISOString(), courses, skippedRows };
  }
  return { parseWeekBitmap, currentUser, normalize, assertSuccess };
}

export const { parseWeekBitmap, currentUser: parseJwappCurrentUser, normalize: normalizeJwappCourses } = createJwappNormalizer();
