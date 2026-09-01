import { mockUser } from '../../mocks/user';
import type { User } from '../../types/domain';
import type { AuthService } from './AuthService';

export class MockAuthService implements AuthService {
  async getCurrentUser(): Promise<User> {
    return mockUser;
  }

  async signInWithWeChat(): Promise<User> {
    return mockUser;
  }
}
