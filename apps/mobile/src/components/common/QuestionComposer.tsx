import { Ionicons } from '@expo/vector-icons';
import { Pressable, StyleSheet, TextInput, View } from 'react-native';

import { colors, layout, radius, shadows, spacing, typography } from '../../theme';

type QuestionComposerProps = {
  value: string;
  onChangeText: (value: string) => void;
  onSubmit: () => void;
  isGenerating?: boolean;
  onStop?: () => void;
  placeholder?: string;
  autoFocus?: boolean;
  elevated?: boolean;
};

export function QuestionComposer({
  value,
  onChangeText,
  onSubmit,
  isGenerating = false,
  onStop,
  placeholder = '问校园里的任何问题',
  autoFocus = false,
  elevated = false,
}: QuestionComposerProps) {
  const canSend = value.trim().length > 0 && !isGenerating;

  return (
    <View style={[styles.container, elevated && shadows.floating]}>
      <TextInput
        accessibilityLabel="输入校园问题"
        autoCapitalize="sentences"
        autoCorrect
        autoFocus={autoFocus}
        maxFontSizeMultiplier={1.35}
        multiline
        onChangeText={onChangeText}
        onSubmitEditing={() => canSend && onSubmit()}
        placeholder={placeholder}
        placeholderTextColor={colors.textMuted}
        returnKeyType="send"
        style={styles.input}
        value={value}
      />
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={isGenerating ? '停止生成' : '发送问题'}
        disabled={!isGenerating && !canSend}
        onPress={isGenerating ? onStop : onSubmit}
        style={({ pressed }) => [
          styles.sendButton,
          !isGenerating && !canSend && styles.sendDisabled,
          pressed && (isGenerating || canSend) && styles.sendPressed,
        ]}
      >
        <Ionicons name={isGenerating ? 'stop' : 'arrow-up'} size={20} color={colors.white} />
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    minHeight: 58,
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: spacing[2],
    paddingLeft: spacing[4],
    paddingRight: spacing[2],
    paddingVertical: spacing[2],
    borderWidth: 1,
    borderColor: colors.borderStrong,
    borderRadius: radius.lg,
    backgroundColor: colors.surface,
  },
  input: {
    ...typography.body,
    flex: 1,
    minHeight: layout.minimumTouchTarget,
    maxHeight: 120,
    paddingTop: 10,
    paddingBottom: 9,
    color: colors.textPrimary,
    textAlignVertical: 'top',
  },
  sendButton: {
    width: layout.minimumTouchTarget,
    height: layout.minimumTouchTarget,
    borderRadius: radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: colors.accent,
  },
  sendDisabled: { backgroundColor: colors.borderStrong },
  sendPressed: { backgroundColor: colors.accentPressed, transform: [{ scale: 0.97 }] },
});
