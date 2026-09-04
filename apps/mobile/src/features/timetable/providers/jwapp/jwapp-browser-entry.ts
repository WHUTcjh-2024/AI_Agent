import { schoolAdapter } from '../../../../config/school.generated';
import { createJwappNormalizer } from './jwapp-parser';

/** Runs only in the school's main frame. Do not add logging or app network calls. */
export async function runSchoolImport(requestId: string) {
  if (window.top !== window.self || !schoolAdapter.timetable.enabled || location.origin !== schoolAdapter.timetable.origin ||
      !location.pathname.startsWith(schoolAdapter.timetable.import_path)) return;
  const scope = window as unknown as {
    __askuTimetableRequest?: string;
    ReactNativeWebView: { postMessage(data: string): void };
  };
  if (scope.__askuTimetableRequest === requestId) return;
  scope.__askuTimetableRequest = requestId;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 25000);
  const post = (message: object) => scope.ReactNativeWebView.postMessage(JSON.stringify({
    channel: 'asku.timetable', version: 1, requestId, ...message,
  }));
  const normalize = createJwappNormalizer();
  const request = async (path: string, init: RequestInit, json = true) => {
    const response = await fetch(path, { ...init, credentials: 'include', signal: controller.signal });
    if (response.status === 401 || response.status === 403) throw new Error('AUTH');
    if (!response.ok) throw new Error('SYSTEM');
    if (response.url) {
      const target = new URL(response.url);
      const login = new URL(schoolAdapter.timetable.login_url);
      if (target.origin !== schoolAdapter.timetable.origin || target.pathname.startsWith('/tpass/') ||
          (target.origin === login.origin && target.pathname === login.pathname)) throw new Error('AUTH');
    }
    const text = await response.text();
    // changeAppRole may return an empty body; data endpoints must return JSON.
    if (!json) {
      const trimmed = text.trim();
      if (!trimmed) return null;
      // This endpoint's success body is not consumed by the school client.
      // Preserve plain acknowledgements; inspect JSON errors and reject HTML.
      if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
        if (trimmed.startsWith('<')) throw new Error('FORMAT');
        return null;
      }
    }
    let body: unknown;
    try { body = JSON.parse(text); } catch { throw new Error('FORMAT'); }
    normalize.assertSuccess(body);
    return body;
  };
  try {
    await request(schoolAdapter.timetable.role_path, {
      method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    }, false);
    const user = normalize.currentUser(await request(schoolAdapter.timetable.user_path, {
      method: 'GET', headers: { 'Fetch-Api': 'true' },
    }));
    const formHeaders = { 'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8', 'X-Requested-With': 'XMLHttpRequest' };
    const courseResponse = await request(schoolAdapter.timetable.courses_path, {
      method: 'POST', headers: { ...formHeaders, Accept: 'application/json, text/javascript, */*; q=0.01' },
      body: `XH=${encodeURIComponent(user.studentId)}&XNXQDM=${encodeURIComponent(user.termCode)}`,
    });
    const [firstYear, secondYear, semester] = user.termCode.split('-');
    const calendar = await request(schoolAdapter.timetable.calendar_path, {
      method: 'POST', headers: formHeaders,
      body: `XN=${encodeURIComponent(`${firstYear}-${secondYear}`)}&XQ=${encodeURIComponent(semester)}`,
    });
    post({ type: 'success', payload: normalize.normalize(courseResponse, calendar, user.termCode) });
  } catch (error) {
    const known = ['AUTH', 'SYSTEM', 'FORMAT'];
    const code = controller.signal.aborted ? 'TIMEOUT' : error instanceof Error && known.includes(error.message) ? error.message : 'NETWORK';
    post({ type: 'error', code });
  } finally { clearTimeout(timer); }
}
