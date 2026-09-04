import { Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { colors } from '../../../theme';
import { formatWeeks, getCourseColor, type Course } from '../domain/course';
import { WEEKDAYS } from './WeekHeader';

export function CourseDetailSheet({ course, sourceLabel, onClose }: { course: Course | null; sourceLabel: string; onClose(): void }) {
  const insets = useSafeAreaInsets();
  if (!course) return null;
  const rows = [
    ['教师', course.teacher || '暂未提供'], ['地点', course.room || '暂未提供'],
    ['时间', `周${WEEKDAYS[course.weekday - 1]} 第${course.startSection}–${course.endSection}节`],
    ['周次', formatWeeks(course.weeks)], ['来源', sourceLabel],
  ];
  return (
    <Modal transparent visible animationType="slide" onRequestClose={onClose}>
      <View style={styles.backdrop}>
        <Pressable accessibilityRole="button" accessibilityLabel="关闭课程详情" style={StyleSheet.absoluteFill} onPress={onClose} />
        <View accessibilityViewIsModal style={[styles.sheet, { paddingBottom: Math.max(insets.bottom, 24) }]}>
          <View style={styles.handle} />
          <ScrollView>
            <Text accessibilityRole="header" style={[styles.title, { color: getCourseColor(course.name).foreground }]}>{course.name}</Text>
            {rows.map(([label, value]) => <View key={label} style={styles.row}><Text style={styles.label}>{label}</Text><Text style={styles.value}>{value}</Text></View>)}
          </ScrollView>
          <Pressable accessibilityRole="button" onPress={onClose} style={styles.close}><Text style={styles.closeText}>完成</Text></Pressable>
        </View>
      </View>
    </Modal>
  );
}
const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: colors.overlay, justifyContent: 'flex-end' },
  sheet: { backgroundColor: colors.surface, borderTopLeftRadius: 20, borderTopRightRadius: 20, paddingHorizontal: 28, maxHeight: '85%', width: '100%', maxWidth: 640, alignSelf: 'center' },
  handle: { width: 32, height: 4, borderRadius: 2, backgroundColor: colors.borderStrong, alignSelf: 'center', marginTop: 12, marginBottom: 28 },
  title: { fontSize: 24, lineHeight: 32, fontWeight: '600', marginBottom: 24 },
  row: { flexDirection: 'row', alignItems: 'flex-start', marginBottom: 20, gap: 24 },
  label: { width: 36, color: colors.textSecondary, fontSize: 13, lineHeight: 22 },
  value: { flex: 1, color: colors.textPrimary, fontSize: 15, lineHeight: 22 },
  close: { minHeight: 48, justifyContent: 'center', alignItems: 'center', marginTop: 4, backgroundColor: colors.surfaceMuted, borderRadius: 8 },
  closeText: { fontSize: 15, color: colors.textSecondary },
});
