import AsyncStorage from '@react-native-async-storage/async-storage';

import type { User } from '../../types/domain';

const TOKEN_STORAGE_KEY = 'asku.api.tokens.v1';

export type TokenPair = {
  accessToken: string;
  accessExpiresAt: string;
  refreshToken: string;
  refreshExpiresAt: string;
  user: User;
};

export interface TokenStore {
  load(): Promise<TokenPair | null>;
  save(pair: TokenPair): Promise<void>;
  clear(): Promise<void>;
}

export class AsyncStorageTokenStore implements TokenStore {
  async load(): Promise<TokenPair | null> {
    const raw = await AsyncStorage.getItem(TOKEN_STORAGE_KEY);
    if (!raw) return null;
    try {
      const value = JSON.parse(raw) as Partial<TokenPair>;
      if (!value.accessToken || !value.refreshToken || !value.accessExpiresAt || !value.refreshExpiresAt || !value.user?.id) {
        await this.clear();
        return null;
      }
      return value as TokenPair;
    } catch {
      await this.clear();
      return null;
    }
  }

  save(pair: TokenPair): Promise<void> {
    return AsyncStorage.setItem(TOKEN_STORAGE_KEY, JSON.stringify(pair));
  }

  clear(): Promise<void> {
    return AsyncStorage.removeItem(TOKEN_STORAGE_KEY);
  }
}
