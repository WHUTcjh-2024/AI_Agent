import './build-school-config.mjs';
import { build } from 'esbuild';
import { readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const directory = new URL('../src/features/timetable/providers/jwapp/', import.meta.url);
const result = await build({
  absWorkingDir: fileURLToPath(new URL('../', import.meta.url)),
  entryPoints: [fileURLToPath(new URL('jwapp-browser-entry.ts', directory))],
  bundle: true, write: false, format: 'iife', globalName: 'AskUTimetableImport',
  platform: 'browser', target: 'es2019', minify: false, legalComments: 'none',
});
const output = '// Generated from AskU-owned browser entry/parser. Run npm run timetable:bundle.\n' +
  `export const jwappBrowserBundle = ${JSON.stringify(result.outputFiles[0].text)};\n`;
const destination = new URL('jwapp-browser.generated.ts', directory);
if (process.argv.includes('--check')) {
  if (await readFile(destination, 'utf8') !== output) throw new Error('Timetable browser bundle is stale. Run npm run timetable:bundle.');
} else {
  await writeFile(destination, output);
}
