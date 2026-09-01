import type { User } from '../../types/domain';
import { ApiSessionManager } from '../auth/ApiSessionManager';
import { ApiTransport } from './ApiTransport';

export class ApiClient {
  constructor(
    private readonly transport: ApiTransport,
    private readonly sessions: ApiSessionManager,
  ) {}

  currentUser(): Promise<User> {
    return this.request<User>('/v1/me');
  }

  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    return this.transport.parse<T>(await this.authenticatedFetch(path, init));
  }

  async authenticatedFetch(path: string, init: RequestInit = {}): Promise<Response> {
    const session = await this.sessions.getSession();
    let response = await this.transport.fetch(path, init, session.accessToken);
    if (response.status !== 401) return response;

    const renewed = await this.sessions.recoverAfterUnauthorized(session.accessToken);
    response = await this.transport.fetch(path, init, renewed.accessToken);
    return response;
  }

  parse<T>(response: Response): Promise<T> {
    return this.transport.parse<T>(response);
  }
}
