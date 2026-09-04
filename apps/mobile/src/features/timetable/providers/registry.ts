import { schoolAdapter } from '../../../config/school.generated';
import * as Crypto from 'expo-crypto';

import type { CourseBrowser } from './course-provider';
import { MockCourseProvider } from './mock-course-provider';
import { JWAPPCourseProvider } from './jwapp/jwapp-course-provider';

export function createTimetableProviders(browser: CourseBrowser) {
  return { primary: new JWAPPCourseProvider(browser, () => Crypto.randomUUID()), mock: new MockCourseProvider() };
}

export function getProviderLabel(id: string): string {
  return ({ [schoolAdapter.timetable.provider_id]: schoolAdapter.timetable.label, mock: '演示课表（非教务数据）' } as Record<string, string>)[id] ?? '学校课表';
}
