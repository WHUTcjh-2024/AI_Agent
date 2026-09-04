import { z } from 'zod';

export const MAX_WEEKS = 64;
export const MAX_SECTIONS = 16;
export const MAX_COURSES = 2000;

const text = z.string().trim().min(1).max(200);
export const courseSourceSchema = z.object({ schoolId: text, provider: text });
export const courseSchema = z.object({
  id: text,
  name: text,
  teacher: z.string().trim().max(200).optional(),
  room: z.string().trim().max(200).optional(),
  weekday: z.number().int().min(1).max(7),
  startSection: z.number().int().min(1).max(MAX_SECTIONS),
  endSection: z.number().int().min(1).max(MAX_SECTIONS),
  weeks: z.array(z.number().int().min(1).max(MAX_WEEKS)).min(1).max(MAX_WEEKS)
    .refine((weeks) => weeks.every((week, i) => i === 0 || week > weeks[i - 1]), 'Weeks must be sorted and unique'),
  source: courseSourceSchema,
}).refine((course) => course.startSection <= course.endSection, 'Invalid section range');

export type Course = z.infer<typeof courseSchema>;

export function belongsToWeek(course: Course, week: number): boolean {
  return course.weeks.includes(week);
}

export function formatWeeks(weeks: readonly number[]): string {
  const sorted = [...new Set(weeks)].sort((a, b) => a - b);
  if (sorted.length === 0) return '无上课周次';
  if (sorted.length >= 3 && sorted.every((week, i) => i === 0 || week === sorted[i - 1] + 2)) {
    return `${sorted[0]}–${sorted[sorted.length - 1]}周（${sorted[0] % 2 ? '单' : '双'}）`;
  }
  const ranges: string[] = [];
  for (let i = 0; i < sorted.length; i++) {
    const first = sorted[i];
    let last = first;
    while (sorted[i + 1] === last + 1) last = sorted[++i];
    ranges.push(first === last ? `${first}` : `${first}–${last}`);
  }
  return `${ranges.join('、')}周`;
}

const palette = [
  { background: '#EAF1FC', foreground: '#264775' },
  { background: '#EAF4F3', foreground: '#295856' },
  { background: '#F0EDF7', foreground: '#51456F' },
  { background: '#FAF0E6', foreground: '#775333' },
  { background: '#EDF3E9', foreground: '#415B38' },
] as const;

export function getCourseColor(name: string) {
  let hash = 0;
  for (const character of name.normalize('NFC')) hash = (Math.imul(hash, 31) + character.codePointAt(0)!) >>> 0;
  return palette[hash % palette.length];
}

/** Interval partitioning: conflicting courses remain separately visible and tappable. */
export function layoutDay(courses: readonly Course[]) {
  const sorted = [...courses].sort((a, b) => a.startSection - b.startSection || a.endSection - b.endSection || a.id.localeCompare(b.id));
  const result: { course: Course; lane: number; laneCount: number }[] = [];
  let group: typeof result = [];
  let ends: number[] = [];
  let groupEnd = 0;
  const flush = () => {
    for (const item of group) result.push({ ...item, laneCount: ends.length });
    group = [];
    ends = [];
  };
  for (const course of sorted) {
    if (course.startSection > groupEnd) flush();
    let lane = ends.findIndex((end) => end < course.startSection);
    if (lane < 0) lane = ends.length;
    ends[lane] = course.endSection;
    group.push({ course, lane, laneCount: 1 });
    groupEnd = Math.max(groupEnd, course.endSection);
  }
  flush();
  return result;
}
