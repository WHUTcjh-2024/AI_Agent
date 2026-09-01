import { StatusBar } from 'expo-status-bar';

import { RootNavigator } from './src/app/navigation/RootNavigator';
import { AppProviders } from './src/app/providers/AppProviders';

export default function App() {
  return (
    <AppProviders>
      <StatusBar style="dark" />
      <RootNavigator />
    </AppProviders>
  );
}
