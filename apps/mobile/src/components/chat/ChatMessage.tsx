import { memo } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { colors, radius, spacing, typography } from '../../theme';
import type { Feedback, Message, Source } from '../../types/domain';
import { AnswerActions } from '../feedback/AnswerActions';
import { SourceCard } from '../source/SourceCard';
import { CitationPills } from '../source/CitationPills';
import { MarkdownContent } from './MarkdownContent';
import { StreamingCursor } from './StreamingCursor';

type ChatMessageProps = {
  message: Message;
  sources: Source[];
  feedback?: Feedback['value'];
  onSourcePress: (sourceId: string) => void;
  onFeedback: (messageId: string, value: Feedback['value']) => void;
  onCopy: (message: Message) => void;
  onShare: (message: Message) => void;
};

function ChatMessageComponent({ message, sources, feedback, onSourcePress, onFeedback, onCopy, onShare }: ChatMessageProps) {
  if (message.role === 'user') {
    return (
      <View style={styles.userRow}>
        <View style={styles.userBubble}><Text style={styles.userText}>{message.content}</Text></View>
      </View>
    );
  }

  return (
    <View style={styles.assistant}>
      <Text style={styles.answerLabel}>AskU</Text>
      <MarkdownContent value={message.content} />
      {message.status === 'streaming' ? <StreamingCursor /> : null}
      {message.status === 'completed' ? <CitationPills citations={message.citations ?? []} onPress={onSourcePress} /> : null}
      {message.status === 'completed' && sources.length ? (
        <View style={styles.sources}>
          <Text style={styles.sourcesTitle}>参考来源</Text>
          {sources.map((source) => <SourceCard key={source.id} source={source} onPress={() => onSourcePress(source.id)} />)}
        </View>
      ) : null}
      {message.status === 'completed' ? (
        <>
          <Text style={styles.disclaimer}>具体安排请以学校最终发布的正式通知为准。</Text>
          <AnswerActions
            feedback={feedback}
            onCopy={() => onCopy(message)}
            onFeedback={(value) => onFeedback(message.id, value)}
            onShare={() => onShare(message)}
          />
        </>
      ) : null}
    </View>
  );
}

export const ChatMessage = memo(ChatMessageComponent);

const styles = StyleSheet.create({
  userRow: { alignItems: 'flex-end' },
  userBubble: { maxWidth: '86%', backgroundColor: colors.accentSoft, borderRadius: radius.lg, borderBottomRightRadius: radius.sm, paddingHorizontal: spacing[4], paddingVertical: spacing[3] },
  userText: { ...typography.body, color: colors.textPrimary },
  assistant: { gap: spacing[2] },
  answerLabel: { ...typography.caption, color: colors.accent, fontWeight: '700', marginBottom: spacing[1] },
  sources: { marginTop: spacing[5] },
  sourcesTitle: { ...typography.heading, color: colors.textPrimary, marginBottom: spacing[1] },
  disclaimer: { ...typography.metadata, color: colors.textMuted, marginTop: spacing[4] },
});
