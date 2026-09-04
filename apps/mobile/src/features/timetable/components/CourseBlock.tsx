import { Pressable, StyleSheet, Text } from 'react-native';

import { getCourseColor, type Course } from '../domain/course';
import { WEEKDAYS } from './WeekHeader';

export const SECTION_HEIGHT = 58;

export function CourseBlock({ course, lane, laneCount, onPress }: { course: Course; lane: number; laneCount: number; onPress(course: Course): void }) {
  const color = getCourseColor(course.name);
  const sections = course.endSection - course.startSection + 1;
  const conflict = laneCount > 1;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${course.name}，周${WEEKDAYS[course.weekday - 1]}，第${course.startSection}至${course.endSection}节，${course.room || '地点待定'}${conflict ? '，存在时间冲突' : ''}`}
      onPress={() => onPress(course)}
      style={({ pressed }) => [styles.block, { top: (course.startSection - 1) * SECTION_HEIGHT + 2, height: sections * SECTION_HEIGHT - 4,
        left: `${lane * 100 / laneCount}%`, width: `${100 / laneCount}%`, backgroundColor: color.background, opacity: pressed ? 0.65 : 1 }]}
    >
      <Text numberOfLines={Math.max(2, Math.min(sections * 2, 5))} style={[styles.name, { color: color.foreground, fontSize: conflict ? 10 : 11 }]}>{course.name}</Text>
      {!conflict && <Text numberOfLines={sections > 1 ? 3 : 1} style={[styles.room, { color: color.foreground }]}>{course.room || '地点待定'}</Text>}
      {!conflict && sections >= 3 && course.teacher ? <Text numberOfLines={1} style={[styles.room, { color: color.foreground }]}>{course.teacher}</Text> : null}
      {conflict && <Text style={[styles.conflict, { color: color.foreground }]}>冲突</Text>}
    </Pressable>
  );
}
const styles = StyleSheet.create({
  block: { position: 'absolute', borderRadius: 5, paddingHorizontal: 4, paddingVertical: 8, borderWidth: 1, borderColor: '#FFFFFF', overflow: 'hidden' },
  name: { fontWeight: '600', lineHeight: 16 },
  room: { marginTop: 6, fontSize: 9, lineHeight: 13, opacity: 0.85 },
  conflict: { fontSize: 8, marginTop: 4 },
});
