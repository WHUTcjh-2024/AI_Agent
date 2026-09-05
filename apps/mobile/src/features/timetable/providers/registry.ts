import { schoolAdapter } from '../../../config/school.generated';
import * as Crypto from 'expo-crypto';

import type { CourseBrowser } from './course-provider';
import { JWAPPCourseProvider } from './jwapp/jwapp-course-provider';

export function createTimetableProvider(browser: CourseBrowser) {
  return new JWAPPCourseProvider(browser, () => Crypto.randomUUID());
}

export function getProviderLabel(id: string): string {
  return id === schoolAdapter.timetable.provider_id ? schoolAdapter.timetable.label : '未知课表来源';
}
