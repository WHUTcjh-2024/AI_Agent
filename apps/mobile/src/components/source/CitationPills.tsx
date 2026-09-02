import { Pressable, StyleSheet, Text, View } from 'react-native';

import { colors, layout, radius, spacing, typography } from '../../theme';
import type { Citation } from '../../types/domain';

type Props = {
  citations: Citation[];
  onPress: (sourceId: string) => void;
};

export function CitationPills({ citations, onPress }: Props) {
  if (!citations.length) return null;
  return (
    <View accessibilityLabel="回答引用" style={styles.wrap}>
      {citations.map((citation) => {
        const label = citation.department || citation.sourceName || citation.title;
        return (
          <Pressable
            accessibilityHint="打开来源详情"
            accessibilityLabel={`引用 ${citation.index}：${citation.title}`}
            accessibilityRole="button"
            key={citation.citationId}
            onPress={() => onPress(citation.sourceId)}
            style={({ pressed }) => [styles.pill, pressed && styles.pressed]}
          >
            <Text style={styles.index}>[{citation.index}]</Text>
            <Text numberOfLines={1} style={styles.label}>{label}</Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing[2], marginTop: spacing[3] },
  pill: {
    minHeight: layout.minimumTouchTarget, maxWidth: '100%', flexDirection: 'row', alignItems: 'center',
    gap: spacing[1], paddingHorizontal: spacing[3], backgroundColor: colors.accentSoft,
    borderRadius: radius.pill, borderWidth: StyleSheet.hairlineWidth, borderColor: colors.borderStrong,
  },
  pressed: { opacity: 0.65 },
  index: { ...typography.metadata, color: colors.accent, fontWeight: '700' },
  label: { ...typography.metadata, color: colors.textPrimary, fontWeight: '600', flexShrink: 1 },
});
