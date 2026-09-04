import { schoolAdapter } from '../../../config/school.generated';
import { timetableSchema, type Timetable } from '../domain/timetable';

export interface TimetableStorage {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  removeItem(key: string): Promise<void>;
}

export const TIMETABLE_STORAGE_KEY = `@asku/timetable/v1/${schoolAdapter.schoolId}`;

export class TimetableRepository {
  constructor(private readonly storage: TimetableStorage) {}

  async load(): Promise<Timetable | null> {
    const raw = await this.storage.getItem(TIMETABLE_STORAGE_KEY);
    if (raw === null) return null;
    if (raw.length > 2_000_000) throw new Error('Invalid timetable cache');
    return timetableSchema.parse(JSON.parse(raw));
  }

  async save(value: Timetable): Promise<Timetable> {
    const safe = timetableSchema.parse(value);
    await this.storage.setItem(TIMETABLE_STORAGE_KEY, JSON.stringify(safe));
    return safe;
  }

  clear() { return this.storage.removeItem(TIMETABLE_STORAGE_KEY); }
}
