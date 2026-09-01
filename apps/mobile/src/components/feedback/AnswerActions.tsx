import { Ionicons } from '@expo/vector-icons';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { colors, radius, spacing, typography } from '../../theme';
import type { Feedback } from '../../types/domain';

type ActionProps = {
  icon: keyof typeof Ionicons.glyphMap;
  activeIcon?: keyof typeof Ionicons.glyphMap;
  label: string;
  active?: boolean;
  onPress: () => void;
};

function Action({ icon, activeIcon, label, active, onPress }: ActionProps) {
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={label} onPress={onPress} style={({ pressed }) => [styles.action, active && styles.active, pressed && styles.pressed]}>
      <Ionicons name={active && activeIcon ? activeIcon : icon} size={18} color={active ? colors.accent : colors.textSecondary} />
      <Text style={[styles.label, active && styles.activeLabel]}>{label}</Text>
    </Pressable>
  );
}

type AnswerActionsProps = {
  feedback?: Feedback['value'];
  onFeedback: (value: Feedback['value']) => void;
  onCopy: () => void;
  onShare: () => void;
};

export function AnswerActions({ feedback, onFeedback, onCopy, onShare }: AnswerActionsProps) {
  return (
    <View style={styles.root}>
      <Action icon="thumbs-up-outline" activeIcon="thumbs-up" label="有帮助" active={feedback === 'helpful'} onPress={() => onFeedback('helpful')} />
      <Action icon="thumbs-down-outline" activeIcon="thumbs-down" label="没帮助" active={feedback === 'unhelpful'} onPress={() => onFeedback('unhelpful')} />
      <Action icon="copy-outline" label="复制" onPress={onCopy} />
      <Action icon="share-outline" label="分享" onPress={onShare} />
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing[2], marginTop: spacing[4] },
  action: { minHeight: 40, flexDirection: 'row', alignItems: 'center', gap: 5, paddingHorizontal: spacing[3], borderRadius: radius.pill, borderWidth: 1, borderColor: colors.border, backgroundColor: colors.surface },
  active: { borderColor: '#C9DAFF', backgroundColor: colors.accentSoft },
  pressed: { opacity: 0.65 },
  label: { ...typography.metadata, color: colors.textSecondary },
  activeLabel: { color: colors.accent, fontWeight: '600' },
});
