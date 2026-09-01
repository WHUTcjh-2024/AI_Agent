import { Ionicons } from '@expo/vector-icons';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';

import { colors, radius, spacing, typography } from '../../theme';

type EmptyStateProps = {
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
  icon?: keyof typeof Ionicons.glyphMap;
};

export function EmptyState({ title, description, actionLabel, onAction, icon = 'chatbubble-ellipses-outline' }: EmptyStateProps) {
  return (
    <View style={styles.state}>
      <View style={styles.iconWrap}><Ionicons name={icon} size={25} color={colors.accent} /></View>
      <Text style={styles.title}>{title}</Text>
      <Text style={styles.description}>{description}</Text>
      {actionLabel && onAction ? (
        <Pressable accessibilityRole="button" onPress={onAction} style={({ pressed }) => [styles.action, pressed && styles.actionPressed]}>
          <Text style={styles.actionText}>{actionLabel}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <View style={[styles.state, styles.errorState]}>
      <Ionicons name="cloud-offline-outline" size={26} color={colors.danger} />
      <Text style={styles.title}>连接遇到问题</Text>
      <Text style={styles.description}>{message}</Text>
      <Pressable accessibilityRole="button" onPress={onRetry} style={({ pressed }) => [styles.action, pressed && styles.actionPressed]}>
        <Text style={styles.actionText}>重新尝试</Text>
      </Pressable>
    </View>
  );
}

export function LoadingState({ label = '正在加载' }: { label?: string }) {
  return (
    <View style={styles.loading}>
      <ActivityIndicator color={colors.accent} />
      <Text style={styles.loadingText}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  state: { alignItems: 'center', justifyContent: 'center', paddingHorizontal: spacing[8], paddingVertical: spacing[10] },
  errorState: { backgroundColor: colors.dangerSoft, borderRadius: radius.lg, margin: spacing[5] },
  iconWrap: { width: 52, height: 52, borderRadius: radius.pill, alignItems: 'center', justifyContent: 'center', backgroundColor: colors.accentSoft, marginBottom: spacing[4] },
  title: { ...typography.heading, color: colors.textPrimary, textAlign: 'center', marginBottom: spacing[2] },
  description: { ...typography.caption, color: colors.textSecondary, textAlign: 'center', maxWidth: 320 },
  action: { minHeight: 44, marginTop: spacing[5], paddingHorizontal: spacing[5], borderRadius: radius.pill, backgroundColor: colors.accent, alignItems: 'center', justifyContent: 'center' },
  actionPressed: { backgroundColor: colors.accentPressed },
  actionText: { ...typography.caption, color: colors.white, fontWeight: '600' },
  loading: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: spacing[3] },
  loadingText: { ...typography.caption, color: colors.textSecondary },
});
