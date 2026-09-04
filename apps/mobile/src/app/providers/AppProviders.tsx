import type { PropsWithChildren } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ServiceProvider } from '../../services/ServiceProvider';
import { AppStoreProvider } from '../../store/AppStore';
import { TimetableStoreProvider } from '../../features/timetable/store/timetable-store';

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <SafeAreaProvider>
      <ServiceProvider>
        <AppStoreProvider>
          <TimetableStoreProvider>{children}</TimetableStoreProvider>
        </AppStoreProvider>
      </ServiceProvider>
    </SafeAreaProvider>
  );
}
