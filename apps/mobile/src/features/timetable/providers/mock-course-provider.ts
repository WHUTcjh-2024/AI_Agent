import { getSchoolDate, getWeekMonday } from '../domain/date';
import { timetableSchema } from '../domain/timetable';
import { CourseImportError, type CourseProvider } from './course-provider';

export class MockCourseProvider implements CourseProvider {
  readonly schoolId = 'demo';
  readonly id = 'mock';
  readonly label = '演示课表（非教务数据）';

  async importCourses(signal?: AbortSignal) {
    if (signal?.aborted) throw new CourseImportError('CANCELLED');
    const all = Array.from({ length: 16 }, (_, i) => i + 1);
    const source = { schoolId: this.schoolId, provider: this.id };
    const courses = [
      { name: '高等数学', teacher: '陈老师', room: '博学北楼 203', weekday: 1, startSection: 1, endSection: 2, weeks: all },
      { name: '大学英语', teacher: '李老师', room: '博学东楼 302', weekday: 2, startSection: 3, endSection: 4, weeks: all },
      { name: '大学物理', teacher: '张老师', room: '鉴主楼 401', weekday: 3, startSection: 1, endSection: 3, weeks: all },
      { name: '信号与系统', room: '博学北楼 305', weekday: 4, startSection: 5, endSection: 6, weeks: all.filter((w) => w % 2 === 1) },
      { name: '体育', teacher: '王老师', weekday: 5, startSection: 7, endSection: 8, weeks: all.filter((w) => w % 2 === 0) },
      { name: '大学物理实验', room: '实验中心 201', weekday: 6, startSection: 3, endSection: 5, weeks: [1, 3, 6, 9, 12] },
      { name: '学术写作', weekday: 7, startSection: 9, endSection: 10, weeks: [1, 2, 4, 8, 12] },
      { name: '创新实践', teacher: '赵老师', room: '创新楼 105', weekday: 3, startSection: 2, endSection: 4, weeks: [1, 5, 9] },
    ].map((course, i) => ({ ...course, id: `demo-${i}`, source }));
    return timetableSchema.parse({ version: 1, schoolId: this.schoolId, provider: this.id, termCode: 'DEMO',
      termStartDate: getWeekMonday(getSchoolDate(), 1), timezone: 'Asia/Shanghai',
      lastImportedAt: new Date().toISOString(), courses, skippedRows: 0 });
  }
}
