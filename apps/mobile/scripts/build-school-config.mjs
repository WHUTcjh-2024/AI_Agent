import { readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { parse } from 'yaml';

// Deployment input, shared with Backend and the knowledge pipeline. Only this
// public subset reaches the client; knowledge IDs and server secrets do not.
const path = process.env.ASKU_SCHOOL_CONFIG || fileURLToPath(new URL('../../../config/schools/whut.yaml', import.meta.url));
const school = parse(await readFile(path, 'utf8'));
if (!school.school_id || !school.school_name) throw new Error('School identity is required');
const input = school.mobile_timetable ?? {};
const timetable = {
  enabled: input.enabled === true,
  provider_id: input.provider_id ?? '', label: input.label ?? '学校教务系统',
  timezone: input.timezone ?? 'Asia/Shanghai', login_url: input.login_url ?? '',
  allowed_hosts: input.allowed_hosts ?? [], origin: input.origin ?? '',
  import_path: input.import_path ?? '', role_path: input.role_path ?? '',
  user_path: input.user_path ?? '', courses_path: input.courses_path ?? '', calendar_path: input.calendar_path ?? '',
};
if (timetable.enabled) {
  for (const key of ['provider_id', 'label', 'origin', 'import_path', 'role_path', 'user_path', 'courses_path', 'calendar_path']) {
    if (typeof timetable[key] !== 'string' || !timetable[key]) throw new Error(`Missing timetable ${key}`);
  }
  for (const endpoint of [timetable.login_url, timetable.origin]) {
    const url = new URL(endpoint);
    if (url.protocol !== 'https:' || url.username || url.password || url.port || !timetable.allowed_hosts.includes(url.hostname)) throw new Error('Invalid timetable endpoint');
  }
  for (const key of ['import_path', 'role_path', 'user_path', 'courses_path', 'calendar_path']) {
    if (!timetable[key].startsWith('/') || timetable[key].startsWith('//')) throw new Error('Expected relative timetable path');
  }
}
const output = '// Generated from config/schools; run npm run timetable:bundle. Public adapter configuration only.\n' +
  "import type { SchoolAdapter } from './school-adapter';\n" +
  `export const schoolAdapter: SchoolAdapter = ${JSON.stringify({ schoolId: school.school_id, schoolName: school.school_name, timetable }, null, 2)};\n`;
const destination = new URL('../src/config/school.generated.ts', import.meta.url);
if (process.argv.includes('--check')) {
  if (await readFile(destination, 'utf8') !== output) throw new Error('School adapter is stale. Run npm run timetable:bundle.');
} else {
  await writeFile(destination, output);
}
