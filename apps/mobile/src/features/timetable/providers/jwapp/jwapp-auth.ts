import { schoolAdapter } from '../../../../config/school.generated';

export const JWAPP_LOGIN_URL = schoolAdapter.timetable.login_url;
export const JWAPP_ORIGIN = schoolAdapter.timetable.origin;

export function allowsJwappNavigation(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === 'https:' && !url.username && !url.password && !url.port &&
      schoolAdapter.timetable.enabled && schoolAdapter.timetable.allowed_hosts.includes(url.hostname);
  } catch { return false; }
}

export function isJwappImportPage(value: string): boolean {
  if (!allowsJwappNavigation(value)) return false;
  const url = new URL(value);
  return url.origin === JWAPP_ORIGIN && url.pathname.startsWith(schoolAdapter.timetable.import_path);
}
