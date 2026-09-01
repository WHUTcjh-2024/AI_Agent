import { Ionicons } from '@expo/vector-icons';
import { useFocusEffect, useNavigation } from '@react-navigation/native';
import type { NativeStackNavigationProp } from '@react-navigation/native-stack';
import { useCallback, useMemo, useState } from 'react';
import { Alert, Pressable, SectionList, StyleSheet, Text, View } from 'react-native';

import { EmptyState, ErrorState, LoadingState } from '../../components/common/StateView';
import { Screen } from '../../components/common/Screen';
import { useServices } from '../../services/ServiceProvider';
import { useAppStore } from '../../store/AppStore';
import { colors, layout, radius, spacing, typography } from '../../theme';
import type { Session } from '../../types/domain';
import type { RootStackParamList } from '../../types/navigation';
import { formatDateLabel } from '../../utils/date';

type Navigation = NativeStackNavigationProp<RootStackParamList>;

export function HistoryScreen() {
  const navigation = useNavigation<Navigation>();
  const { chat } = useServices();
  const { historyRevision, notifyHistoryChanged } = useAppStore();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadHistory = useCallback(async () => {
    setError(null);
    try {
      setSessions(await chat.getHistory());
    } catch {
      setError('历史记录暂时无法读取。');
    } finally {
      setLoading(false);
    }
  }, [chat]);

  useFocusEffect(useCallback(() => { void loadHistory(); }, [historyRevision, loadHistory]));

  const sections = useMemo(() => {
    const groups = new Map<string, Session[]>();
    sessions.forEach((session) => {
      const label = formatDateLabel(session.updatedAt);
      groups.set(label, [...(groups.get(label) ?? []), session]);
    });
    return Array.from(groups, ([title, data]) => ({ title, data }));
  }, [sessions]);

  const applyMutation = useCallback(async (mutation: () => Promise<void>) => {
    setLoading(true);
    setError(null);
    try {
      await mutation();
      notifyHistoryChanged();
      setSessions(await chat.getHistory());
    } catch {
      setError('历史记录操作失败，请稍后重试。');
    } finally {
      setLoading(false);
    }
  }, [chat, notifyHistoryChanged]);

  const removeSession = useCallback((session: Session) => {
    Alert.alert('删除这条对话？', session.title, [
      { text: '取消', style: 'cancel' },
      {
        text: '删除',
        style: 'destructive',
        onPress: () => { void applyMutation(() => chat.deleteSession(session.id)); },
      },
    ]);
  }, [applyMutation]);

  const clearHistory = useCallback(() => {
    Alert.alert('清空历史记录？', '这会删除当前测试账号在开发后端中的全部对话。', [
      { text: '取消', style: 'cancel' },
      {
        text: '清空',
        style: 'destructive',
        onPress: () => { void applyMutation(() => chat.clearHistory()); },
      },
    ]);
  }, [applyMutation]);

  const restoreDemo = useCallback(() => applyMutation(() => chat.resetDemoHistory()), [applyMutation]);

  return (
    <Screen includeBottomInset>
      <View style={styles.header}>
        <Text accessibilityRole="header" style={styles.title}>历史对话</Text>
        {sessions.length ? (
          <Pressable accessibilityRole="button" onPress={clearHistory} style={styles.clearButton}>
            <Text style={styles.clearText}>清空</Text>
          </Pressable>
        ) : null}
      </View>
      {loading ? <LoadingState label="正在读取历史对话" /> : error ? <ErrorState message={error} onRetry={() => void loadHistory()} /> : !sessions.length ? (
        <EmptyState
          actionLabel="添加联调示例"
          description="开始提问后，对话会保存在这里，方便你随时继续。"
          icon="time-outline"
          onAction={() => void restoreDemo()}
          title="还没有历史对话"
        />
      ) : (
        <SectionList
          contentContainerStyle={styles.listContent}
          keyExtractor={(item) => item.id}
          renderSectionHeader={({ section }) => <Text style={styles.sectionHeader}>{section.title}</Text>}
          renderItem={({ item }) => (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={`打开对话：${item.title}`}
              onPress={() => navigation.navigate('Chat', { sessionId: item.id })}
              style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
            >
              <View style={styles.chatIcon}><Ionicons name="chatbubble-outline" size={18} color={colors.accent} /></View>
              <View style={styles.rowContent}>
                <Text numberOfLines={2} style={styles.rowTitle}>{item.title}</Text>
                <Text style={styles.rowMeta}>{item.messages.length} 条消息</Text>
              </View>
              <Pressable accessibilityRole="button" accessibilityLabel={`删除对话：${item.title}`} hitSlop={8} onPress={() => removeSession(item)} style={styles.deleteButton}>
                <Ionicons name="trash-outline" size={18} color={colors.textMuted} />
              </Pressable>
            </Pressable>
          )}
          sections={sections}
          showsVerticalScrollIndicator={false}
          stickySectionHeadersEnabled={false}
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  header: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingHorizontal: layout.screenPadding, paddingTop: spacing[5], paddingBottom: spacing[4] },
  title: { ...typography.pageTitle, color: colors.textPrimary },
  clearButton: { minHeight: 44, minWidth: 44, justifyContent: 'center', alignItems: 'flex-end' },
  clearText: { ...typography.caption, color: colors.textSecondary },
  listContent: { paddingHorizontal: layout.screenPadding, paddingBottom: spacing[8] },
  sectionHeader: { ...typography.caption, color: colors.textSecondary, fontWeight: '600', paddingTop: spacing[4], paddingBottom: spacing[2], backgroundColor: colors.background },
  row: { minHeight: 78, flexDirection: 'row', alignItems: 'center', gap: spacing[3], borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border, paddingVertical: spacing[3] },
  rowPressed: { opacity: 0.62 },
  chatIcon: { width: 38, height: 38, borderRadius: radius.pill, backgroundColor: colors.accentSoft, alignItems: 'center', justifyContent: 'center' },
  rowContent: { flex: 1, gap: 3 },
  rowTitle: { ...typography.bodyStrong, color: colors.textPrimary },
  rowMeta: { ...typography.metadata, color: colors.textMuted },
  deleteButton: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center', borderRadius: radius.pill },
});
