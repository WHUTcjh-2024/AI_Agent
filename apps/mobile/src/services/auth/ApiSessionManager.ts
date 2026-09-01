import type { AuthMode } from '../../config/runtime';
import { ApiError, ApiTransport } from '../api/ApiTransport';
import type { TokenPair, TokenStore } from './TokenStore';

type SessionTransport = Pick<ApiTransport, 'request'>;

export class ApiSessionManager {
  private tokens: TokenPair | null = null;
  private sessionMutation: Promise<TokenPair> | null = null;

  constructor(
    private readonly transport: SessionTransport,
    private readonly store: TokenStore,
    private readonly authMode: AuthMode,
  ) {}

  async getSession(): Promise<TokenPair> {
    if (this.tokens && Date.parse(this.tokens.accessExpiresAt) > Date.now() + 30_000) return this.tokens;
    return this.mutateSession(() => this.restoreOrBootstrap());
  }

  async recoverAfterUnauthorized(rejectedAccessToken: string): Promise<TokenPair> {
    if (this.tokens?.accessToken && this.tokens.accessToken !== rejectedAccessToken) return this.tokens;
    return this.mutateSession(() => this.renewOrBootstrap(rejectedAccessToken));
  }

  private async restoreOrBootstrap(): Promise<TokenPair> {
    const restored = this.tokens ?? await this.store.load();
    if (restored && hasValidRefreshToken(restored)) {
      this.tokens = restored;
      if (Date.parse(restored.accessExpiresAt) > Date.now() + 30_000) return restored;
      return this.refreshOrRecover(restored.refreshToken);
    }
    if (restored) await this.clear();
    return this.bootstrap();
  }

  private async renewOrBootstrap(rejectedAccessToken: string): Promise<TokenPair> {
    const current = this.tokens ?? await this.store.load();
    if (current?.accessToken && current.accessToken !== rejectedAccessToken) return current;
    if (current && hasValidRefreshToken(current)) {
      this.tokens = current;
      return this.refreshOrRecover(current.refreshToken);
    }
    if (current) await this.clear();
    return this.bootstrap();
  }

  private async refreshOrRecover(refreshToken: string): Promise<TokenPair> {
    try {
      return await this.refresh(refreshToken);
    } catch (error) {
      if (!isInvalidRefreshToken(error)) throw error;
      await this.clear();
      return this.bootstrap();
    }
  }

  private async refresh(refreshToken: string): Promise<TokenPair> {
    const pair = await this.transport.request<TokenPair>('/v1/auth/refresh', {
      method: 'POST', body: JSON.stringify({ refreshToken }),
    });
    return this.persist(pair);
  }

  private async bootstrap(): Promise<TokenPair> {
    if (this.authMode !== 'dev') {
      throw new ApiError('请先完成微信登录。', 401, 'auth_required');
    }
    const pair = await this.transport.request<TokenPair>('/v1/auth/dev-login', {
      method: 'POST', body: JSON.stringify({ externalId: 'mobile-tester', nickname: 'AskU 测试同学' }),
    });
    return this.persist(pair);
  }

  private async persist(pair: TokenPair): Promise<TokenPair> {
    this.tokens = pair;
    await this.store.save(pair);
    return pair;
  }

  private async clear(): Promise<void> {
    this.tokens = null;
    await this.store.clear();
  }

  private mutateSession(operation: () => Promise<TokenPair>): Promise<TokenPair> {
    if (this.sessionMutation) return this.sessionMutation;
    const pending = operation().finally(() => {
      if (this.sessionMutation === pending) this.sessionMutation = null;
    });
    this.sessionMutation = pending;
    return pending;
  }
}

function hasValidRefreshToken(pair: TokenPair): boolean {
  const expiresAt = Date.parse(pair.refreshExpiresAt);
  return Number.isFinite(expiresAt) && expiresAt > Date.now();
}

function isInvalidRefreshToken(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401 && error.code === 'invalid_refresh_token';
}
