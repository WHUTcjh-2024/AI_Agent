import type { User } from '../../types/domain';

export interface AuthService {
  getCurrentUser(): Promise<User>;
  signInWithWeChat(): Promise<User>;
}
