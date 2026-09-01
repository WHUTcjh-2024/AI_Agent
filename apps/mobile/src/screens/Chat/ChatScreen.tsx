import { Ionicons } from '@expo/vector-icons';
import * as Clipboard from 'expo-clipboard';
import { useNavigation, useRoute, type RouteProp } from '@react-navigation/native';
import type { NativeStackNavigationProp } from '@react-navigation/native-stack';
import { useCallback, useMemo, useRef } from 'react';
import { Alert, FlatList, KeyboardAvoidingView, Share, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { AgentStatus } from '../../components/chat/AgentStatus';
import { ChatMessage } from '../../components/chat/ChatMessage';
import { QuestionComposer } from '../../components/common/QuestionComposer';
import { ErrorState } from '../../components/common/StateView';
import { Screen } from '../../components/common/Screen';
import { useChatController } from '../../hooks/useChatController';
import { keyboardAvoidingBehavior } from '../../platform/keyboard';
import { useServices } from '../../services/ServiceProvider';
import { colors, layout, spacing, typography } from '../../theme';
import type { Message } from '../../types/domain';
import type { RootStackParamList } from '../../types/navigation';

type ChatRoute = RouteProp<RootStackParamList, 'Chat'>;
type Navigation = NativeStackNavigationProp<RootStackParamList>;
type ChatListItem =
  | { type: 'message'; id: string; message: Message }
  | { type: 'status'; id: 'agent-status' }
  | { type: 'error'; id: 'agent-error' };

export function ChatScreen() {
  const route = useRoute<ChatRoute>();
  const navigation = useNavigation<Navigation>();
  const insets = useSafeAreaInsets();
  const { chat } = useServices();
  const controller = useChatController({ initialSessionId: route.params?.sessionId, initialQuestion: route.params?.initialQuestion });
  const {
    activeAssistantId, error, feedback, input, isGenerating, lastQuestion, messages,
    sendQuestion, setInput, sourceCount, sources, stage, stopGeneration, submitFeedback,
  } = controller;
  const listRef = useRef<FlatList<ChatListItem>>(null);

  const listData = useMemo<ChatListItem[]>(() => {
    const items: ChatListItem[] = [];
    let statusInserted = false;
    messages.forEach((message) => {
      if (message.role === 'assistant' && message.id === activeAssistantId && stage) {
        items.push({ type: 'status', id: 'agent-status' });
        statusInserted = true;
      }
      items.push({ type: 'message', id: message.id, message });
    });
    if (stage && !statusInserted) items.push({ type: 'status', id: 'agent-status' });
    if (error) items.push({ type: 'error', id: 'agent-error' });
    return items;
  }, [activeAssistantId, error, messages, stage]);

  const handleFeedback = useCallback(async (messageId: string, value: 'helpful' | 'unhelpful') => {
    try {
      await submitFeedback(messageId, value);
      Alert.alert('感谢反馈', value === 'helpful' ? '这条回答已标记为有帮助。' : '我们会用这条反馈改进回答质量。');
    } catch {
      Alert.alert('提交失败', '反馈暂时没有保存，请稍后重试。');
    }
  }, [submitFeedback]);

  const copyAnswer = useCallback(async (message: Message) => {
    await Clipboard.setStringAsync(chat.copyableAnswer(message));
    Alert.alert('已复制', '回答内容已复制到剪贴板。');
  }, [chat]);

  const shareAnswer = useCallback(async (message: Message) => {
    await Share.share({ message: chat.copyableAnswer(message), title: 'AskU 校园问答' });
  }, [chat]);

  const renderItem = useCallback(({ item }: { item: ChatListItem }) => {
    if (item.type === 'status' && stage) return <AgentStatus sourceCount={sourceCount} stage={stage} />;
    if (item.type === 'error') return <ErrorState message={error ?? '生成失败'} onRetry={() => { void sendQuestion(lastQuestion); }} />;
    if (item.type === 'message') {
      const messageSources = (item.message.sourceIds ?? []).map((id) => sources[id]).filter(Boolean);
      return (
        <ChatMessage
          feedback={feedback[item.message.id]}
          message={item.message}
          onCopy={(message) => { void copyAnswer(message); }}
          onFeedback={(messageId, value) => { void handleFeedback(messageId, value); }}
          onShare={(message) => { void shareAnswer(message); }}
          onSourcePress={(sourceId) => navigation.navigate('SourceDetail', { sourceId })}
          sources={messageSources}
        />
      );
    }
    return null;
  }, [copyAnswer, error, feedback, handleFeedback, lastQuestion, navigation, sendQuestion, shareAnswer, sourceCount, sources, stage]);

  return (
    <Screen includeTopInset={false}>
      <KeyboardAvoidingView behavior={keyboardAvoidingBehavior} keyboardVerticalOffset={0} style={styles.flex}>
        <FlatList
          ListEmptyComponent={
            <View style={styles.emptyChat}>
              <View style={styles.emptyIcon}><Ionicons name="sparkles" size={22} color={colors.accent} /></View>
              <Text style={styles.emptyTitle}>直接问校园里的问题</Text>
              <Text style={styles.emptyText}>AskU 会查找学校资料，并给出可以追溯的来源。</Text>
            </View>
          }
          contentContainerStyle={[styles.listContent, !listData.length && styles.emptyContent]}
          data={listData}
          initialNumToRender={10}
          keyboardDismissMode="interactive"
          keyboardShouldPersistTaps="handled"
          keyExtractor={(item) => item.id}
          maxToRenderPerBatch={10}
          onContentSizeChange={() => listRef.current?.scrollToEnd({ animated: true })}
          ref={listRef}
          removeClippedSubviews={false}
          renderItem={renderItem}
          showsVerticalScrollIndicator={false}
          windowSize={7}
        />
        <View style={[styles.composerWrap, { paddingBottom: Math.max(insets.bottom, spacing[2]) }]}>
          <QuestionComposer
            isGenerating={isGenerating}
            onChangeText={setInput}
            onStop={stopGeneration}
            onSubmit={() => { void sendQuestion(input); }}
            placeholder="继续提问…"
            value={input}
          />
        </View>
      </KeyboardAvoidingView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  listContent: { paddingHorizontal: layout.screenPadding, paddingTop: spacing[5], paddingBottom: spacing[6], gap: spacing[6] },
  emptyContent: { flexGrow: 1 },
  emptyChat: { flex: 1, alignItems: 'center', justifyContent: 'center', paddingHorizontal: spacing[8], paddingBottom: spacing[16] },
  emptyIcon: { width: 52, height: 52, borderRadius: 26, backgroundColor: colors.accentSoft, alignItems: 'center', justifyContent: 'center', marginBottom: spacing[4] },
  emptyTitle: { ...typography.heading, color: colors.textPrimary, textAlign: 'center' },
  emptyText: { ...typography.caption, color: colors.textSecondary, textAlign: 'center', marginTop: spacing[2], maxWidth: 320 },
  composerWrap: { borderTopWidth: StyleSheet.hairlineWidth, borderColor: colors.border, backgroundColor: colors.background, paddingHorizontal: layout.screenPadding, paddingTop: spacing[3] },
});
