import { Platform } from 'react-native';

const configuredBaseUrl = process.env.EXPO_PUBLIC_ASKU_API_BASE_URL?.trim();

function normalizeBaseUrl(value: string): string {
  const normalized = value.replace(/\/+$/, '');
  if (!/^https?:\/\//i.test(normalized)) throw new Error('EXPO_PUBLIC_ASKU_API_BASE_URL must use HTTP or HTTPS');
  return normalized;
}

/** Emulator defaults only. Physical devices must use the development machine's LAN address. */
export const apiBaseUrl = configuredBaseUrl
  ? normalizeBaseUrl(configuredBaseUrl)
  : Platform.select({ android: 'http://10.0.2.2:18080', default: 'http://127.0.0.1:18080' });
