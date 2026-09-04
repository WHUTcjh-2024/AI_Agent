import { useMemo } from 'react';
import { PanResponder, ScrollView, StyleSheet, Text, View } from 'react-native';

import { colors } from '../../../theme';
import { layoutDay, type Course } from '../domain/course';
import { CourseBlock, SECTION_HEIGHT } from './CourseBlock';
import { SECTION_GUTTER } from './WeekHeader';

export function TimetableGrid({ courses, onCoursePress, onSwipe }: { courses: Course[]; onCoursePress(course: Course): void; onSwipe(delta: number): void }) {
  const count = Math.max(12, ...courses.map((course) => course.endSection));
  const days = useMemo(() => Array.from({ length: 7 }, (_, i) => layoutDay(courses.filter((course) => course.weekday === i + 1))), [courses]);
  const gesture = useMemo(() => PanResponder.create({
    onMoveShouldSetPanResponderCapture: (_, state) => Math.abs(state.dx) > 18 && Math.abs(state.dx) > Math.abs(state.dy) * 1.8,
    onMoveShouldSetPanResponder: (_, state) => Math.abs(state.dx) > 18 && Math.abs(state.dx) > Math.abs(state.dy) * 1.8,
    onPanResponderRelease: (_, state) => { if (Math.abs(state.dx) > 55) onSwipe(state.dx < 0 ? 1 : -1); },
  }), [onSwipe]);
  return (
    <View style={styles.flex} {...gesture.panHandlers}>
      {courses.length === 0 && <Text accessibilityRole="text" style={styles.empty}>本周暂无课程</Text>}
      <ScrollView showsVerticalScrollIndicator={false} contentContainerStyle={styles.scroll}>
        <View style={{ height: count * SECTION_HEIGHT, flexDirection: 'row' }}>
          <View style={styles.gutter}>
            {Array.from({ length: count }, (_, i) => <View key={i} style={styles.section}><Text style={styles.number}>{i + 1}</Text></View>)}
          </View>
          <View pointerEvents="none" style={styles.lines}>
            {Array.from({ length: count }, (_, i) => <View key={i} style={styles.line} />)}
          </View>
          {days.map((items, i) => (
            <View key={i} style={styles.column}>
              {items.map((item) => <CourseBlock key={item.course.id} {...item} onPress={onCoursePress} />)}
            </View>
          ))}
        </View>
      </ScrollView>
    </View>
  );
}
const styles = StyleSheet.create({
  flex: { flex: 1 },
  scroll: { paddingHorizontal: 8, paddingBottom: 24 },
  gutter: { width: SECTION_GUTTER },
  section: { height: SECTION_HEIGHT, alignItems: 'center', paddingTop: 12 },
  number: { fontSize: 10, color: colors.textMuted },
  lines: { position: 'absolute', top: 0, right: 0, left: SECTION_GUTTER },
  line: { height: SECTION_HEIGHT, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#F0F2F5' },
  column: { flex: 1 },
  empty: { textAlign: 'center', padding: 16, fontSize: 13, color: colors.textSecondary },
});
