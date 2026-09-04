import AsyncStorage from '@react-native-async-storage/async-storage';
import { createContext, type PropsWithChildren, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';

import type { Timetable } from '../domain/timetable';
import { CourseImportError } from '../providers/course-provider';
import { TimetableRepository } from './timetable-repository';

type TimetableState = {
  timetable: Timetable | null;
  loading: boolean;
  error: string | null;
  save(value: Timetable): Promise<void>;
  clear(): Promise<void>;
};
const Context = createContext<TimetableState | null>(null);
const repository = new TimetableRepository(AsyncStorage);

export function TimetableStoreProvider({ children }: PropsWithChildren) {
  const [timetable, setTimetable] = useState<Timetable | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const alive = useRef(false);
  // Serializes writes so clear/save cannot race and leave a stale disk snapshot.
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  useEffect(() => {
    alive.current = true;
    const hydration = repository.load().then((value) => { if (alive.current) setTimetable(value); })
      .catch(() => { if (alive.current) setError('本地课表无法读取，请重新导入'); })
      .finally(() => { if (alive.current) setLoading(false); });
    queue.current = hydration;
    return () => { alive.current = false; };
  }, []);

  const save = useCallback((value: Timetable): Promise<void> => {
    const operation = queue.current.then(async () => {
      try {
        const safe = await repository.save(value);
        if (alive.current) { setTimetable(safe); setError(null); }
      } catch { throw new CourseImportError('STORAGE'); }
    });
    queue.current = operation.catch(() => undefined);
    return operation;
  }, []);
  const clear = useCallback((): Promise<void> => {
    const operation = queue.current.then(async () => {
      try {
        await repository.clear();
        if (alive.current) { setTimetable(null); setError(null); }
      } catch { throw new CourseImportError('STORAGE'); }
    });
    queue.current = operation.catch(() => undefined);
    return operation;
  }, []);
  const value = useMemo(() => ({ timetable, loading, error, save, clear }), [timetable, loading, error, save, clear]);
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useTimetableStore() {
  const value = useContext(Context);
  if (!value) throw new Error('TimetableStoreProvider is missing');
  return value;
}
