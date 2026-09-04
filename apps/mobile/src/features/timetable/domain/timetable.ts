import { z } from 'zod';

import { courseSchema, MAX_COURSES } from './course';
import { isCalendarDate } from './date';

export const timetableSchema = z.object({
  version: z.literal(1),
  schoolId: z.string().min(1).max(80),
  provider: z.string().min(1).max(80),
  termCode: z.string().min(1).max(80),
  termStartDate: z.string().refine(isCalendarDate),
  timezone: z.string().max(80).refine((value) => {
    try { new Intl.DateTimeFormat('en', { timeZone: value }); return true; } catch { return false; }
  }),
  lastImportedAt: z.iso.datetime(),
  courses: z.array(courseSchema).max(MAX_COURSES),
  skippedRows: z.number().int().min(0).max(MAX_COURSES),
}).superRefine((value, context) => {
  const ids = new Set<string>();
  for (const course of value.courses) {
    if (course.source.schoolId !== value.schoolId || course.source.provider !== value.provider || ids.has(course.id)) {
      context.addIssue({ code: 'custom', message: 'Inconsistent source or duplicate course ID' });
    }
    ids.add(course.id);
  }
});

export type CourseImportResult = z.infer<typeof timetableSchema>;
export type Timetable = CourseImportResult;
