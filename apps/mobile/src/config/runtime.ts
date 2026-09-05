import { apiBaseUrl } from '../platform/api';

export type AuthMode = 'dev' | 'wechat';

function enumValue<T extends string>(value: string | undefined, allowed: readonly T[], fallback: T): T {
  const normalized = value?.trim() as T | undefined;
  if (!normalized) return fallback;
  if (!allowed.includes(normalized)) throw new Error(`Invalid runtime setting: ${normalized}`);
  return normalized;
}

export const runtimeConfig = Object.freeze({
  version: '0.9.0',
  authMode: enumValue(process.env.EXPO_PUBLIC_ASKU_AUTH_MODE, ['dev', 'wechat'] as const, 'dev'),
  apiBaseUrl,
});
