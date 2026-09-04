import { StyleSheet, Text, View } from 'react-native';

import { colors } from '../../../theme';

export const WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日'];
export const SECTION_GUTTER = 28;

export function WeekHeader({ dates, today }: { dates: string[]; today: string }) {
  return (
    <View style={styles.row}>
      <View style={styles.gutter}><Text style={styles.month}>{Number(dates[0].slice(5, 7))}月</Text></View>
      {dates.map((date, i) => (
        <View key={date} style={styles.day}>
          <Text style={styles.weekday}>{WEEKDAYS[i]}</Text>
          <Text style={[styles.date, date === today && styles.active]}>{Number(date.slice(8))}</Text>
          <View style={[styles.dot, date === today && styles.activeDot]} />
        </View>
      ))}
    </View>
  );
}
const styles = StyleSheet.create({
  row: { flexDirection: 'row', borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border, paddingTop: 12, paddingBottom: 5, marginHorizontal: 8 },
  gutter: { width: SECTION_GUTTER, alignItems: 'center', justifyContent: 'center' },
  month: { fontSize: 10, color: colors.textMuted },
  day: { flex: 1, alignItems: 'center', gap: 7 },
  weekday: { fontSize: 11, color: colors.textSecondary },
  date: { fontSize: 15, fontWeight: '500', color: colors.textPrimary },
  active: { color: colors.accent, fontWeight: '700' },
  dot: { width: 4, height: 4, borderRadius: 2, backgroundColor: colors.transparent },
  activeDot: { backgroundColor: colors.accent },
});
