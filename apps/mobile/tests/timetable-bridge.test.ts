import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createContext, runInContext } from 'node:vm';

import { timetableSchema } from '../src/features/timetable/domain/timetable';
import { CourseImportError, type BrowserImportRequest } from '../src/features/timetable/providers/course-provider';
import { allowsJwappNavigation, isJwappImportPage } from '../src/features/timetable/providers/jwapp/jwapp-auth';
import { validateJwappMessage, JWAPPCourseProvider } from '../src/features/timetable/providers/jwapp/jwapp-course-provider';
import { normalizeJwappCourses } from '../src/features/timetable/providers/jwapp/jwapp-parser';
import { buildJwappImportScript } from '../src/features/timetable/providers/jwapp/jwapp-script';

const home = 'https://jwxt.whut.edu.cn/jwapp/sys/homeapp/index.do?forceCas=1';
const requestId = 'synthetic-test-nonce';
const calendar = { datas: { cxxljc: { rows: [{ XQKSRQ: '2026-08-31 00:00:00' }] } } };
const rows = { datas: { cxxskcb: { rows: [{ KCM: '测试课程', SKXQ: 7, KSJC: 1, JSJC: 2, SKZC: '1010' }] } } };
const payload = normalizeJwappCourses(rows, calendar, '2026-2027-1');
const envelope = (extra = {}) => JSON.stringify({ channel: 'asku.timetable', version: 1, requestId, type: 'success', payload, ...extra });

test('navigation allowlist rejects suffix tricks, HTTP, userinfo, ports and unsafe protocols', () => {
  assert.equal(allowsJwappNavigation(home), true);
  assert.equal(allowsJwappNavigation('https://zhlgd.whut.edu.cn/tpass/login'), true);
  for (const value of ['https://jwxt.whut.edu.cn.evil.test/', 'https://jwxt.whut.edu.cn@evil.test/', 'https://evil.test@jwxt.whut.edu.cn/',
    'http://jwxt.whut.edu.cn/', 'https://jwxt.whut.edu.cn:444/', 'file:///test', 'javascript:alert(1)', 'intent://test', 'about:blank']) {
    assert.equal(allowsJwappNavigation(value), false, value);
  }
  assert.equal(isJwappImportPage('https://jwxt.whut.edu.cn/jwapp/sys/homeapp-evil/'), false);
  assert.equal(isJwappImportPage('https://zhlgd.whut.edu.cn/jwapp/sys/homeapp/'), false);
});
test('bridge checks origin, nonce, version, type and complete schema', () => {
  assert.ok(validateJwappMessage(envelope(), home, requestId));
  assert.equal(validateJwappMessage(envelope(), 'https://example.com/', requestId), null);
  assert.equal(validateJwappMessage(envelope(), home, 'stale-request'), null);
  assert.equal(validateJwappMessage('not json', home, requestId), null);
  assert.equal(validateJwappMessage('{}', home, requestId), null);
  for (const change of [{ version: 2 }, { type: 'debug' }, { payload: {} }, { payload: { ...payload, schoolId: 'other' } }]) {
    assert.throws(() => validateJwappMessage(envelope(change), home, requestId), CourseImportError);
  }
  assert.throws(() => validateJwappMessage('x'.repeat(2_000_001), home, requestId), CourseImportError);
});
test('bridge exposes only enumerated errors, never school error strings', () => {
  assert.throws(() => validateJwappMessage(envelope({ type: 'error', code: 'AUTH', message: 'sensitive' }), home, requestId),
    (error: unknown) => error instanceof CourseImportError && error.code === 'AUTH' && !error.message.includes('sensitive'));
  assert.throws(() => validateJwappMessage(envelope({ type: 'error', code: 'raw sensitive failure' }), home, requestId),
    (error: unknown) => error instanceof CourseImportError && error.code === 'FORMAT');
});

async function runScript(options: { url?: string; failAt?: number; status?: number; malformed?: boolean; network?: boolean; iframe?: boolean; abort?: boolean; duplicate?: boolean;
  responseUrl?: string; bodies?: Record<number, unknown>; rawBodies?: Record<number, string> } = {}) {
  const calls: { path: string; init: RequestInit }[] = [];
  const messages: unknown[] = [];
  let resolveMessage: () => void = () => {};
  const received = new Promise<void>((resolve) => { resolveMessage = resolve; });
  const windowObject: Record<string, unknown> = { ReactNativeWebView: { postMessage: (data: string) => { messages.push(JSON.parse(data)); resolveMessage(); } } };
  windowObject.self = windowObject;
  windowObject.top = options.iframe ? {} : windowObject;
  const timers = new Set<ReturnType<typeof setTimeout>>();
  const context = createContext({
    window: windowObject, location: new URL(options.url ?? home), AbortController, URL,
    setTimeout: (callback: () => void, delay: number) => { const timer = setTimeout(callback, options.abort ? 1 : delay); timers.add(timer); return timer; },
    clearTimeout: (timer: ReturnType<typeof setTimeout>) => { clearTimeout(timer); timers.delete(timer); },
    fetch: async (path: string, init: RequestInit) => {
      calls.push({ path, init });
      if (options.abort) return new Promise((_, reject) => init.signal?.addEventListener('abort', () => reject(new Error('aborted'))));
      if (options.network) throw new Error('sensitive network error');
      const body = options.bodies && calls.length in options.bodies ? options.bodies[calls.length] :
        calls.length === 1 ? null : calls.length === 2 ? { datas: { userId: 'SYNTHETIC-STUDENT', welcomeInfo: { xnxqdm: '2026-2027-1' } } } : calls.length === 3 ? rows : calendar;
      const status = calls.length === options.failAt ? options.status ?? 503 : 200;
      return { ok: status < 400, status, redirected: false, url: calls.length === options.failAt ? options.responseUrl ?? home : home,
        text: async () => options.rawBodies?.[calls.length] ?? (options.malformed ? '<html>sensitive invalid JSON</html>' : JSON.stringify(body)) };
    },
  });
  try {
    const script = buildJwappImportScript(requestId);
    runInContext(script, context);
    if (options.duplicate) runInContext(script, context);
    if (!options.iframe && isJwappImportPage(options.url ?? home)) {
      let timeout: ReturnType<typeof setTimeout> | undefined;
      try { await Promise.race([received, new Promise((_, reject) => { timeout = setTimeout(() => reject(new Error('No bridge message')), 2000); })]); }
      finally { clearTimeout(timeout); }
    }
    return { calls, messages, remainingTimers: timers.size };
  } finally { for (const timer of timers) clearTimeout(timer); }
}

test('actual generated browser bundle completes four protocol requests and Native validation', async () => {
  const { calls, messages, remainingTimers } = await runScript();
  assert.equal(calls.length, 4); assert.equal(messages.length, 1); assert.equal(remainingTimers, 0);
  assert.ok(calls.every((call) => call.init.credentials === 'include' && call.path.startsWith('/jwapp/')));
  assert.equal(calls[0].init.method, 'POST'); assert.ok(calls[0].path.includes('appRole=ef212c48c8f84be79acbd9d81b090f51'));
  assert.equal(calls[1].init.method, 'GET'); assert.deepEqual(JSON.parse(JSON.stringify(calls[1].init.headers)), { 'Fetch-Api': 'true' });
  assert.equal(calls[2].init.body, 'XH=SYNTHETIC-STUDENT&XNXQDM=2026-2027-1');
  assert.equal(calls[3].init.body, 'XN=2026-2027&XQ=1');
  const value = validateJwappMessage(JSON.stringify(messages[0]), home, requestId);
  assert.ok(timetableSchema.safeParse(value).success);
  assert.deepEqual(value?.courses[0].weeks, [1, 3]);
  assert.equal(JSON.stringify(messages).includes('SYNTHETIC-STUDENT'), false);
  assert.equal(JSON.stringify(messages).includes('SKZC'), false);
});
test('double injection runs once; iframe and unexpected origin execute nothing', async () => {
  assert.equal((await runScript({ duplicate: true })).calls.length, 4);
  assert.equal((await runScript({ iframe: true })).calls.length, 0);
  assert.equal((await runScript({ url: 'https://example.com/' })).calls.length, 0);
});
for (const [label, options, code] of [
  ['HTTP error', { failAt: 3, status: 503 }, 'SYSTEM'], ['expired auth', { failAt: 2, status: 401 }, 'AUTH'],
  ['malformed JSON', { malformed: true }, 'FORMAT'], ['offline', { network: true }, 'NETWORK'], ['timeout', { abort: true }, 'TIMEOUT'],
] as const) test(`browser ${label} maps to safe error`, async () => {
  const { messages, remainingTimers } = await runScript(options);
  assert.equal((messages[0] as { code: string }).code, code); assert.equal(remainingTimers, 0);
  assert.equal(JSON.stringify(messages).includes('sensitive'), false);
});
test('provider delegates browser lifecycle with signal and new nonce', async () => {
  let captured: BrowserImportRequest | null = null;
  const signal = new AbortController().signal;
  const provider = new JWAPPCourseProvider({ open: async (request, receivedSignal) => {
    assert.equal(receivedSignal, signal); captured = request; return payload;
  } }, () => requestId);
  assert.deepEqual(await provider.importCourses(signal), payload);
  const request = captured as BrowserImportRequest | null;
  assert.equal(request?.requestId, requestId); assert.equal(request?.isImportPage(home), true);
});

test('all four requests preserve exact protocol paths, headers and methods', async () => {
  const { calls } = await runScript();
  assert.deepEqual(calls.map(({ path }) => path), [
    '/jwapp/sys/homeapp/api/home/changeAppRole.do?appRole=ef212c48c8f84be79acbd9d81b090f51',
    '/jwapp/sys/homeapp/api/home/currentUser.do',
    '/jwapp/sys/kcbcxby/modules/xskcb/cxxskcb.do',
    '/jwapp/sys/kcbcxby/modules/xskcb/cxxljc.do',
  ]);
  assert.deepEqual(calls.map(({ init }) => init.method), ['POST', 'GET', 'POST', 'POST']);
  const form = { 'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8', 'X-Requested-With': 'XMLHttpRequest' };
  assert.deepEqual(JSON.parse(JSON.stringify(calls[2].init.headers)), { ...form, Accept: 'application/json, text/javascript, */*; q=0.01' });
  assert.deepEqual(JSON.parse(JSON.stringify(calls[3].init.headers)), form);
  assert.equal(calls[0].init.body, undefined);
});

for (const step of [1, 2, 3, 4]) {
  test(`request ${step}: final CAS URL identifies AUTH even without redirected flag`, async () => {
    const result = await runScript({ failAt: step, status: 200, responseUrl: 'https://zhlgd.whut.edu.cn/tpass/login?service=sensitive' });
    assert.equal(result.calls.length, step);
    assert.deepEqual(result.messages, [{ channel: 'asku.timetable', version: 1, requestId, type: 'error', code: 'AUTH' }]);
  });
  test(`request ${step}: explicit failed envelope cannot publish data`, async () => {
    for (const [body, code] of [[{ success: false, ...rows, message: 'sensitive' }, 'SYSTEM'], [{ code: '401', ...rows }, 'AUTH']] as const) {
      const result = await runScript({ bodies: { [step]: body } });
      assert.equal(result.calls.length, step);
      assert.equal((result.messages[0] as { code: string }).code, code);
      assert.equal(JSON.stringify(result.messages).includes('sensitive'), false);
    }
  });
}

test('role acknowledgements may be empty/plain; malformed data JSON fails safely', async () => {
  for (const body of ['', 'ok', 'null', '{"success":true}']) {
    assert.equal((await runScript({ rawBodies: { 1: body } })).calls.length, 4);
  }
  for (const step of [2, 3, 4]) {
    const result = await runScript({ rawBodies: { [step]: '<html>sensitive</html>' } });
    assert.equal((result.messages[0] as { code: string }).code, 'FORMAT');
    assert.equal(result.calls.length, step);
  }
});

test('legal empty timetable traverses generated bundle, but all-bad rows and invalid calendar do not', async () => {
  const empty = await runScript({ bodies: { 3: { datas: { cxxskcb: { rows: [] } } } } });
  assert.deepEqual(validateJwappMessage(JSON.stringify(empty.messages[0]), home, requestId)?.courses, []);
  for (const bodies of [{ 3: { datas: { cxxskcb: { rows: [{}] } } } }, { 4: { datas: { cxxljc: { rows: [] } } } }]) {
    const result = await runScript({ bodies });
    assert.equal((result.messages[0] as { code: string }).code, 'FORMAT');
  }
});
