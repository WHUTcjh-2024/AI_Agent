import { schoolAdapter } from '../../../config/school.generated';
import { Ionicons } from '@expo/vector-icons';
import { useFocusEffect } from '@react-navigation/native';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ActivityIndicator, AppState, Modal, Pressable, StyleSheet, Text, View } from 'react-native';

import { Screen } from '../../../components/common/Screen';
import { colors } from '../../../theme';
import { CourseDetailSheet } from '../components/CourseDetailSheet';
import { useCourseBrowser } from '../components/CourseImportBrowser';
import { TimetableGrid } from '../components/TimetableGrid';
import { WeekHeader } from '../components/WeekHeader';
import { WeekNavigator } from '../components/WeekNavigator';
import { belongsToWeek, type Course } from '../domain/course';
import { formatLastImported, getCurrentAcademicWeek, getSchoolDate, getWeekDates } from '../domain/date';
import { CourseImportError, type CourseProvider } from '../providers/course-provider';
import { createTimetableProviders, getProviderLabel } from '../providers/registry';
import { useTimetableStore } from '../store/timetable-store';

export function TimetableScreen() {
  const { timetable, loading, error: storageError, save, clear } = useTimetableStore();
  const { browser, browserNode } = useCourseBrowser();
  const providers = createTimetableProviders(browser);
  const [selectedWeek, setSelectedWeek] = useState<number | null>(null);
  const [selectedCourse, setSelectedCourse] = useState<Course | null>(null);
  const [now, setNow] = useState(() => new Date());
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirmClear, setConfirmClear] = useState(false);
  const operation = useRef<AbortController | null>(null);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    const timer = setInterval(() => setNow(new Date()), 60000);
    const subscription = AppState.addEventListener('change', (state) => { if (state === 'active') setNow(new Date()); });
    return () => { alive.current = false; operation.current?.abort(); clearInterval(timer); subscription.remove(); };
  }, []);
  useFocusEffect(useCallback(() => { setNow(new Date()); }, []));

  const currentWeek = timetable ? getCurrentAcademicWeek(timetable.termStartDate, timetable.timezone, now) : 1;
  const maxWeek = timetable?.courses.reduce((last, course) => Math.max(last, course.weeks[course.weeks.length - 1]), 20) ?? 20;
  const clamp = (week: number) => Math.min(maxWeek, Math.max(1, week));
  const week = clamp(selectedWeek ?? currentWeek);
  const swipe = useCallback((delta: number) => setSelectedWeek(Math.min(maxWeek, Math.max(1, week + delta))), [maxWeek, week]);

  async function importCourses(provider: CourseProvider) {
    if (operation.current || loading) return;
    const controller = new AbortController();
    operation.current = controller;
    setBusy(true); setError(null); setNotice(null);
    try {
      const result = await provider.importCourses(controller.signal);
      if (controller.signal.aborted) return;
      await save(result);
      if (!alive.current) return;
      setSelectedWeek(null); setNow(new Date());
      const count = new Set(result.courses.map((course) => course.name)).size;
      setNotice(`✓ 已导入 ${count} 门课程${result.skippedRows ? `，${result.skippedRows} 条无效安排已跳过` : ''}`);
    } catch (cause) {
      if (!alive.current) return;
      const failure = cause instanceof CourseImportError ? cause : new CourseImportError('SYSTEM');
      if (failure.code !== 'CANCELLED') {
        setError(failure.message);
        // Only an enumerated code; never log exceptions, URLs, payloads or identifiers.
        if (__DEV__) console.warn('[timetable]', failure.code);
      }
    } finally {
      operation.current = null;
      if (alive.current) setBusy(false);
    }
  }

  async function clearCourses() {
    if (busy) return;
    setConfirmClear(false); setBusy(true);
    try { await clear(); if (alive.current) { setNotice(null); setError(null); setSelectedWeek(null); } }
    catch { if (alive.current) setError('清除课表失败，请重试'); }
    finally { if (alive.current) setBusy(false); }
  }

  const shownError = error || storageError;
  return (
    <Screen includeTopInset={false} includeBottomInset>
      {loading ? <View style={styles.center}><ActivityIndicator color={colors.accent} /></View> : (
        <>
          {shownError && <View style={styles.error}><Text accessibilityRole="alert" style={styles.errorText}>{shownError}</Text><Text style={styles.errorHint}>已有课表不会因导入失败而被覆盖。</Text></View>}
          {notice && <Text accessibilityLiveRegion="polite" style={styles.notice}>{notice}</Text>}
          {!timetable ? (
            <View style={styles.center}>
              <Ionicons name="calendar-outline" size={32} color={colors.accent} />
              <Text accessibilityRole="header" style={styles.emptyTitle}>还没有课表</Text>
              <Text style={styles.emptyText}>{schoolAdapter.timetable.enabled ? `从${schoolAdapter.timetable.label}\n导入本学期本科课表` : '本校暂未配置课表导入，可先体验演示课表'}</Text>
              <Pressable accessibilityRole="button" disabled={busy || !schoolAdapter.timetable.enabled} onPress={() => void importCourses(providers.primary)} style={styles.primary}>
                <Text style={styles.primaryText}>{busy ? '正在导入…' : '导入课表'}</Text>
              </Pressable>
              <Pressable accessibilityRole="button" disabled={busy} onPress={() => void importCourses(providers.mock)} style={styles.demoButton}>
                <Text style={styles.demoText}>先体验演示课表</Text>
              </Pressable>
              <Text style={styles.privacy}>学校页面登录 · 课表仅保存在本机</Text>
            </View>
          ) : (
            <>
              <View style={styles.toolbar}>
                <View style={styles.meta}>
                  <Text numberOfLines={1} style={styles.source}>{getProviderLabel(timetable.provider)}</Text>
                  <Text style={styles.term}>学期 {timetable.termCode}</Text>
                  <Text style={styles.updated}>最后更新：{formatLastImported(timetable.lastImportedAt, timetable.timezone, now)}</Text>
                </View>
                <Pressable accessibilityRole="button" accessibilityLabel="重新导入课表" disabled={busy || !schoolAdapter.timetable.enabled} onPress={() => void importCourses(providers.primary)} style={styles.refresh}>
                  <Ionicons name="refresh-outline" color={colors.accent} size={16} /><Text style={styles.refreshText}>{busy ? '导入中' : '重新导入'}</Text>
                </Pressable>
                <Pressable accessibilityRole="button" accessibilityLabel="清除本地课表" disabled={busy} onPress={() => setConfirmClear(true)} style={styles.delete}>
                  <Ionicons name="trash-outline" size={16} color={colors.textMuted} />
                </Pressable>
              </View>
              {timetable.provider === 'mock' && <Text style={styles.demoNotice}>演示数据 · 不代表真实课程安排</Text>}
              <WeekNavigator week={week} currentWeek={currentWeek} maxWeek={maxWeek} dates={getWeekDates(timetable.termStartDate, week)} onChange={setSelectedWeek} onToday={() => setSelectedWeek(null)} />
              <WeekHeader dates={getWeekDates(timetable.termStartDate, week)} today={getSchoolDate(now, timetable.timezone)} />
              <TimetableGrid courses={timetable.courses.filter((course) => belongsToWeek(course, week))} onCoursePress={setSelectedCourse} onSwipe={swipe} />
              <Text style={styles.footer}>按学校时区显示 · 左右滑动切换周次</Text>
            </>
          )}
        </>
      )}
      {browserNode}
      <CourseDetailSheet course={selectedCourse} sourceLabel={getProviderLabel(timetable?.provider ?? '')} onClose={() => setSelectedCourse(null)} />
      <Modal transparent visible={confirmClear} animationType="fade" onRequestClose={() => setConfirmClear(false)}>
        <View style={styles.backdrop}>
          <View style={styles.confirm}>
            <Text style={styles.confirmTitle}>清除本地课表？</Text>
            <Text style={styles.confirmText}>仅删除本机缓存，不影响学校数据。之后可重新登录导入。</Text>
            <Pressable accessibilityRole="button" onPress={() => void clearCourses()} style={styles.primary}><Text style={styles.primaryText}>确认清除</Text></Pressable>
            <Pressable accessibilityRole="button" onPress={() => setConfirmClear(false)} style={styles.demoButton}><Text style={styles.demoText}>取消</Text></Pressable>
          </View>
        </View>
      </Modal>
    </Screen>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 28, paddingBottom: 64 },
  emptyTitle: { color: colors.textPrimary, fontWeight: '600', fontSize: 25, marginTop: 24 },
  emptyText: { color: colors.textSecondary, fontSize: 15, lineHeight: 25, textAlign: 'center', marginTop: 12, marginBottom: 28 },
  primary: { minWidth: 180, minHeight: 48, borderRadius: 8, backgroundColor: colors.accent, justifyContent: 'center', alignItems: 'center', paddingHorizontal: 24 },
  primaryText: { color: colors.white, fontSize: 15, fontWeight: '600' },
  demoButton: { minHeight: 48, alignItems: 'center', justifyContent: 'center', marginTop: 8 },
  demoText: { color: colors.textSecondary, fontSize: 13 },
  privacy: { color: colors.textMuted, fontSize: 11, marginTop: 36 },
  toolbar: { flexDirection: 'row', alignItems: 'center', paddingLeft: 20, paddingRight: 8, paddingVertical: 8, gap: 4 },
  meta: { flex: 1 },
  source: { fontSize: 12, color: colors.textSecondary },
  term: { fontSize: 12, color: colors.textPrimary, marginTop: 6 },
  updated: { fontSize: 10, color: colors.textMuted, marginTop: 6 },
  refresh: { minHeight: 44, flexDirection: 'row', alignItems: 'center', gap: 5, paddingHorizontal: 6 },
  refreshText: { fontSize: 12, color: colors.accent },
  delete: { width: 40, height: 44, alignItems: 'center', justifyContent: 'center' },
  demoNotice: { backgroundColor: colors.warningSoft, color: '#8A560F', paddingVertical: 6, textAlign: 'center', fontSize: 11 },
  notice: { paddingHorizontal: 20, paddingVertical: 8, color: colors.success, fontSize: 12 },
  error: { margin: 12, padding: 12, backgroundColor: colors.dangerSoft, borderRadius: 8 },
  errorText: { fontSize: 13, color: colors.danger },
  errorHint: { fontSize: 11, color: colors.textSecondary, marginTop: 5 },
  footer: { fontSize: 10, color: colors.textMuted, textAlign: 'center', paddingVertical: 8 },
  backdrop: { flex: 1, justifyContent: 'center', alignItems: 'center', padding: 28, backgroundColor: colors.overlay },
  confirm: { backgroundColor: colors.surface, borderRadius: 16, padding: 24, maxWidth: 360, width: '100%' },
  confirmTitle: { fontSize: 19, fontWeight: '600', color: colors.textPrimary },
  confirmText: { fontSize: 14, lineHeight: 23, color: colors.textSecondary, marginVertical: 20 },
});
