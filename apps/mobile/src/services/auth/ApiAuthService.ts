import type { User } from '../../types/domain';
import { ApiClient } from '../api/ApiClient';
import type { AuthService } from './AuthService';

export class ApiAuthService implements AuthService {
  constructor(private readonly client: ApiClient) {}

  getCurrentUser(): Promise<User> { return this.client.currentUser(); }

  async signInWithWeChat(): Promise<User> {
    throw new Error('微信开放平台尚未配置；当前联调自动使用开发测试登录。');
  }
}
