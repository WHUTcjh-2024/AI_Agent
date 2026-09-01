import { Ionicons } from '@expo/vector-icons';
import { ActivityIndicator, Animated, StyleSheet, Text, View } from 'react-native';
import { useEffect, useRef } from 'react';

import { colors, radius, spacing, typography } from '../../theme';

export type AgentStage = 'retrieving' | 'sources' | 'composing' | 'completed';

const labels: Record<AgentStage, string> = {
  retrieving: '正在查找学校官方资料',
  sources: '已找到相关来源',
  composing: '正在整理答案',
  completed: '已完成回答整理',
};

export function AgentStatus({ stage, sourceCount }: { stage: AgentStage; sourceCount: number }) {
  const opacity = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    opacity.setValue(0);
    Animated.timing(opacity, { toValue: 1, duration: 180, useNativeDriver: true }).start();
  }, [opacity, stage]);

  const complete = stage === 'completed';
  const noReliableSource = stage === 'sources' && sourceCount === 0;
  const label = noReliableSource
    ? '暂未找到可靠的官方来源'
    : stage === 'sources'
      ? `已找到 ${sourceCount} 个相关来源`
      : labels[stage];
  return (
    <Animated.View style={[styles.root, complete && styles.rootCompleted, noReliableSource && styles.rootWarning, { opacity }]}>
      {noReliableSource ? (
        <Ionicons name="alert-circle" size={18} color={colors.warning} />
      ) : complete || stage === 'sources' ? (
        <Ionicons name="checkmark-circle" size={18} color={colors.success} />
      ) : (
        <ActivityIndicator color={colors.accent} size="small" />
      )}
      <Text style={[styles.text, complete && styles.textCompleted, noReliableSource && styles.textWarning]}>{label}</Text>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  root: { flexDirection: 'row', alignItems: 'center', alignSelf: 'flex-start', gap: spacing[2], minHeight: 42, paddingHorizontal: spacing[3], borderRadius: radius.md, backgroundColor: colors.surfaceMuted },
  rootCompleted: { minHeight: 34, backgroundColor: colors.successSoft },
  rootWarning: { backgroundColor: colors.warningSoft },
  text: { ...typography.caption, color: colors.textSecondary },
  textCompleted: { ...typography.metadata, color: colors.success },
  textWarning: { color: colors.warning },
});
