import { createContext, type PropsWithChildren, useContext, useMemo } from 'react';

import { runtimeConfig } from '../config/runtime';
import type { AuthService } from './auth/AuthService';
import { ApiSessionManager } from './auth/ApiSessionManager';
import { ApiAuthService } from './auth/ApiAuthService';
import { MockAuthService } from './auth/MockAuthService';
import { AsyncStorageTokenStore } from './auth/TokenStore';
import { ApiChatService } from './api/ApiChatService';
import { ApiClient } from './api/ApiClient';
import { ApiTransport } from './api/ApiTransport';
import type { ChatService } from './chat/ChatService';
import { MockChatService } from './chat/MockChatService';

type Services = {
  chat: ChatService;
  auth: AuthService;
};

const ServicesContext = createContext<Services | null>(null);

export function ServiceProvider({ children }: PropsWithChildren) {
  const services = useMemo<Services>(() => {
    if (runtimeConfig.serviceMode === 'mock') {
      return { chat: new MockChatService(), auth: new MockAuthService() };
    }
    const transport = new ApiTransport(runtimeConfig.apiBaseUrl);
    const sessions = new ApiSessionManager(transport, new AsyncStorageTokenStore(), runtimeConfig.authMode);
    const client = new ApiClient(transport, sessions);
    return { chat: new ApiChatService(client), auth: new ApiAuthService(client) };
  }, []);
  return <ServicesContext.Provider value={services}>{children}</ServicesContext.Provider>;
}

export function useServices(): Services {
  const services = useContext(ServicesContext);
  if (!services) throw new Error('useServices must be used inside ServiceProvider');
  return services;
}
