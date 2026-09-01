import { Ionicons } from '@expo/vector-icons';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { colors, radius, spacing, typography } from '../../theme';
import type { Source } from '../../types/domain';
import { formatPublishedDate } from '../../utils/date';

export function SourceCard({ source, onPress }: { source: Source; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`查看来源：${source.title}`}
      onPress={onPress}
      style={({ pressed }) => [styles.card, pressed && styles.pressed]}
    >
      <View style={styles.icon}><Ionicons name="document-text-outline" size={19} color={colors.accent} /></View>
      <View style={styles.content}>
        <View style={styles.publisherRow}>
          <Text numberOfLines={1} style={styles.publisher}>{source.publisher}</Text>
          {source.official ? <Text style={styles.badge}>官方</Text> : null}
        </View>
        <Text numberOfLines={2} style={styles.title}>{source.title}</Text>
        <Text style={styles.meta}>{formatPublishedDate(source.publishedAt)} · 查看原文</Text>
      </View>
      <Ionicons name="chevron-forward" size={18} color={colors.textMuted} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: 'row', alignItems: 'center', gap: spacing[3], paddingVertical: spacing[3], borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  pressed: { opacity: 0.68 },
  icon: { width: 36, height: 36, borderRadius: radius.sm, alignItems: 'center', justifyContent: 'center', backgroundColor: colors.accentSoft },
  content: { flex: 1, gap: 3 },
  publisherRow: { flexDirection: 'row', alignItems: 'center', gap: spacing[2] },
  publisher: { ...typography.metadata, color: colors.textSecondary, flexShrink: 1 },
  badge: { ...typography.metadata, color: colors.accent, backgroundColor: colors.accentSoft, paddingHorizontal: 6, paddingVertical: 1, borderRadius: radius.pill, overflow: 'hidden' },
  title: { ...typography.caption, color: colors.textPrimary, fontWeight: '600' },
  meta: { ...typography.metadata, color: colors.textMuted },
});
