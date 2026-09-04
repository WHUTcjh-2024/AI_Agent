import { schoolAdapter } from '../../../../config/school.generated';
import { z } from 'zod';

import { timetableSchema } from '../../domain/timetable';
import { CourseImportError, type CourseBrowser, type CourseProvider } from '../course-provider';
import { allowsJwappNavigation, isJwappImportPage, JWAPP_LOGIN_URL } from './jwapp-auth';
import { buildJwappImportScript } from './jwapp-script';

const messageSchema = z.discriminatedUnion('type', [
  z.object({ channel: z.literal('asku.timetable'), version: z.literal(1), requestId: z.string(), type: z.literal('success'), payload: timetableSchema }),
  z.object({ channel: z.literal('asku.timetable'), version: z.literal(1), requestId: z.string(), type: z.literal('error'),
    code: z.enum(['NETWORK', 'SYSTEM', 'FORMAT', 'AUTH', 'TIMEOUT']) }),
]);

export function validateJwappMessage(data: string, url: string, requestId: string) {
  if (!isJwappImportPage(url)) return null;
  if (data.length > 2_000_000) throw new CourseImportError('FORMAT');
  let raw: unknown;
  try { raw = JSON.parse(data); } catch { return null; }
  // Ignore unrelated school-page messages before validating our own envelope.
  if (!raw || typeof raw !== 'object' || !('channel' in raw) || raw.channel !== 'asku.timetable' ||
      !('requestId' in raw) || raw.requestId !== requestId) return null;
  const parsed = messageSchema.safeParse(raw);
  if (!parsed.success) throw new CourseImportError('FORMAT');
  if (parsed.data.type === 'error') throw new CourseImportError(parsed.data.code);
  const payload = parsed.data.payload;
  if (payload.schoolId !== schoolAdapter.schoolId || payload.provider !== schoolAdapter.timetable.provider_id || payload.timezone !== schoolAdapter.timetable.timezone) throw new CourseImportError('FORMAT');
  // Import timestamp is trusted Native time, not an arbitrary page-provided value.
  return { ...payload, lastImportedAt: new Date().toISOString() };
}

export class JWAPPCourseProvider implements CourseProvider {
  readonly schoolId = schoolAdapter.schoolId;
  readonly id = schoolAdapter.timetable.provider_id;
  readonly label = schoolAdapter.timetable.label;

  constructor(private readonly browser: CourseBrowser, private readonly newRequestId: () => string) {}

  importCourses(signal?: AbortSignal) {
    if (!schoolAdapter.timetable.enabled) return Promise.reject(new CourseImportError('SYSTEM'));
    const requestId = this.newRequestId();
    return this.browser.open({
      requestId, loginUrl: JWAPP_LOGIN_URL, script: buildJwappImportScript(requestId),
      allowsNavigation: allowsJwappNavigation, isImportPage: isJwappImportPage,
      validateMessage: (data, url) => validateJwappMessage(data, url, requestId),
    }, signal);
  }
}
