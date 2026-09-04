import { Ionicons } from '@expo/vector-icons';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { colors } from '../../../theme';
import { formatDateRange } from '../domain/date';

export function WeekNavigator({ week, currentWeek, maxWeek, dates, onChange, onToday }: {
  week: number; currentWeek: number; maxWeek: number; dates: string[]; onChange(week: number): void; onToday(): void;
}) {
  return (
    <View style={styles.row}>
      <Pressable accessibilityRole="button" accessibilityLabel="上一周" disabled={week <= 1} onPress={() => onChange(week - 1)} style={styles.arrow}>
        <Ionicons name="chevron-back" size={22} color={week <= 1 ? colors.border : colors.textSecondary} />
      </Pressable>
      <Pressable accessibilityRole="button" accessibilityLabel={`第 ${week} 周，点击返回当前周`} onPress={onToday} style={styles.center}>
        <Text style={[styles.week, week === currentWeek && styles.active]}>第 {week} 周</Text>
        <Text style={styles.dates}>{formatDateRange(dates)}</Text>
        <Text style={styles.hint}>{currentWeek < 1 ? '尚未开学' : currentWeek > maxWeek ? '本学期已结束' : week === currentWeek ? '本周' : '点击回到本周'}</Text>
      </Pressable>
      <Pressable accessibilityRole="button" accessibilityLabel="下一周" disabled={week >= maxWeek} onPress={() => onChange(week + 1)} style={styles.arrow}>
        <Ionicons name="chevron-forward" size={22} color={week >= maxWeek ? colors.border : colors.textSecondary} />
      </Pressable>
    </View>
  );
}
const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: 20, paddingVertical: 8 },
  arrow: { width: 48, height: 48, alignItems: 'center', justifyContent: 'center' },
  center: { alignItems: 'center', padding: 8 },
  week: { fontSize: 24, fontWeight: '600', color: colors.textPrimary },
  active: { color: colors.accent },
  dates: { color: colors.textSecondary, fontSize: 12, marginTop: 8 },
  hint: { color: colors.textMuted, fontSize: 10, marginTop: 6 },
});
