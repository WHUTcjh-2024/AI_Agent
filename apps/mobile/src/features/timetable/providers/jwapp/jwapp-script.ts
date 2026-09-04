import { jwappBrowserBundle } from './jwapp-browser.generated';

export function buildJwappImportScript(requestId: string): string {
  // Bundle at build time; Function.toString() breaks under minifiers / Hermes.
  return `(() => { ${jwappBrowserBundle}\nvoid AskUTimetableImport.runSchoolImport(${JSON.stringify(requestId)}); })(); true;`;
}
