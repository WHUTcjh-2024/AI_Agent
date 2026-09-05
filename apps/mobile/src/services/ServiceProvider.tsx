import AsyncStorage from '@react-native-async-storage/async-storage';
import { createContext, type PropsWithChildren, useContext, useEffect, useMemo } from 'react';

import { runtimeConfig } from '../config/runtime';
import type { AuthService } from './auth/AuthService';
import { ApiSessionManager } from './auth/ApiSessionManager';
import { ApiAuthService } from './auth/ApiAuthService';
import { AsyncStorageTokenStore } from './auth/TokenStore';
import { ApiChatService } from './api/ApiChatService';
import { ApiClient } from './api/ApiClient';
import { ApiTransport } from './api/ApiTransport';
import type { ChatService } from './chat/ChatService';

const LEGACY_MOCK_STORAGE_KEYS = ['asku.mock.sessions.v1', 'asku.mock.feedback.v1'] as const;

type Services = {
  chat: ChatService;
  auth: AuthService;
};

const ServicesContext = createContext<Services | null>(null);

export function ServiceProvider({ children }: PropsWithChildren) {
  useEffect(() => {
    void AsyncStorage.multiRemove([...LEGACY_MOCK_STORAGE_KEYS]);
  }, []);
  const services = useMemo<Services>(() => {
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
