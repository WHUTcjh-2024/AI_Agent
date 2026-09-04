import type { CourseImportResult } from '../domain/timetable';

export const importErrorMessages = {
  NETWORK: '网络连接失败，请检查网络后重试',
  SYSTEM: '暂时无法连接教务系统，请稍后重试',
  FORMAT: '课表数据格式发生变化，请稍后再试',
  AUTH: '学校登录状态已失效，请重新登录',
  TIMEOUT: '教务系统响应超时，请重试',
  CANCELLED: '已取消导入',
  STORAGE: '课表保存失败，请检查设备存储后重试',
  NAVIGATION: '已阻止非课表登录所需的页面跳转',
} as const;
export type ImportErrorCode = keyof typeof importErrorMessages;

export class CourseImportError extends Error {
  constructor(public readonly code: ImportErrorCode) {
    super(importErrorMessages[code]);
    this.name = 'CourseImportError';
  }
}

export interface CourseProvider {
  readonly schoolId: string;
  readonly id: string;
  readonly label: string;
  importCourses(signal?: AbortSignal): Promise<CourseImportResult>;
}

export interface BrowserImportRequest {
  requestId: string;
  loginUrl: string;
  script: string;
  allowsNavigation(url: string): boolean;
  isImportPage(url: string): boolean;
  validateMessage(data: string, url: string): CourseImportResult | null;
}

export interface CourseBrowser {
  open(request: BrowserImportRequest, signal?: AbortSignal): Promise<CourseImportResult>;
}
