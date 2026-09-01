import { createContext, type PropsWithChildren, useCallback, useContext, useMemo, useState } from 'react';

type AppStore = {
  historyRevision: number;
  notifyHistoryChanged: () => void;
};

const AppStoreContext = createContext<AppStore | null>(null);

export function AppStoreProvider({ children }: PropsWithChildren) {
  const [historyRevision, setHistoryRevision] = useState(0);
  const notifyHistoryChanged = useCallback(() => setHistoryRevision((value) => value + 1), []);
  const value = useMemo(() => ({ historyRevision, notifyHistoryChanged }), [historyRevision, notifyHistoryChanged]);
  return <AppStoreContext.Provider value={value}>{children}</AppStoreContext.Provider>;
}

export function useAppStore(): AppStore {
  const value = useContext(AppStoreContext);
  if (!value) throw new Error('useAppStore must be used inside AppStoreProvider');
  return value;
}
