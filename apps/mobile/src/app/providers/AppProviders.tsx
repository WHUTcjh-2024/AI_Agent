import type { PropsWithChildren } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ServiceProvider } from '../../services/ServiceProvider';
import { AppStoreProvider } from '../../store/AppStore';

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <SafeAreaProvider>
      <ServiceProvider>
        <AppStoreProvider>{children}</AppStoreProvider>
      </ServiceProvider>
    </SafeAreaProvider>
  );
}
