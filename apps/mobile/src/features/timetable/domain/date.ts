const DAY_MS = 86_400_000;
export const DEFAULT_TIMEZONE = 'Asia/Shanghai';

export function isCalendarDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const date = new Date(`${value}T00:00:00Z`);
  return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value;
}

function dayIndex(value: string): number {
  if (!isCalendarDate(value)) throw new Error('Invalid calendar date');
  return Date.parse(`${value}T00:00:00Z`) / DAY_MS;
}

export function getSchoolDate(now: Date = new Date(), timezone = DEFAULT_TIMEZONE): string {
  const parts = new Intl.DateTimeFormat('en-US', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(now);
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((item) => item.type === type)!.value;
  return `${part('year')}-${part('month')}-${part('day')}`;
}

export function getWeekday(date: string): number {
  return ((dayIndex(date) + 3) % 7 + 7) % 7 + 1;
}

export function addDays(date: string, days: number): string {
  return new Date((dayIndex(date) + days) * DAY_MS).toISOString().slice(0, 10);
}

export function getWeekMonday(termStartDate: string, week: number): string {
  return addDays(termStartDate, 1 - getWeekday(termStartDate) + (week - 1) * 7);
}

/** Deliberately not clamped: callers can distinguish before-term and after-term. */
export function getCurrentAcademicWeek(termStartDate: string, timezone = DEFAULT_TIMEZONE, now: Date = new Date()): number {
  return Math.floor((dayIndex(getSchoolDate(now, timezone)) - dayIndex(getWeekMonday(termStartDate, 1))) / 7) + 1;
}

export function getWeekDates(termStartDate: string, week: number): string[] {
  const monday = getWeekMonday(termStartDate, week);
  return Array.from({ length: 7 }, (_, i) => addDays(monday, i));
}

export function formatDateRange(dates: string[]): string {
  const short = (value: string) => `${Number(value.slice(5, 7))}月${Number(value.slice(8))}日`;
  return `${short(dates[0])} – ${short(dates[6])}`;
}

export function formatLastImported(value: string, timezone: string, now: Date = new Date()): string {
  const imported = new Date(value);
  const date = getSchoolDate(imported, timezone);
  const prefix = date === getSchoolDate(now, timezone) ? '今天' : date;
  const time = new Intl.DateTimeFormat('en-GB', { timeZone: timezone, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(imported);
  return `${prefix} ${time}`;
}
