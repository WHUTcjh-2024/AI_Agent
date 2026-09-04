import { Ionicons } from '@expo/vector-icons';
import { useNavigation } from '@react-navigation/native';
import type { NativeStackNavigationProp } from '@react-navigation/native-stack';
import { useCallback, useState } from 'react';
import { KeyboardAvoidingView, Pressable, ScrollView, StyleSheet, Text, useWindowDimensions, View } from 'react-native';

import { QuestionComposer } from '../../components/common/QuestionComposer';
import { Screen } from '../../components/common/Screen';
import { frequentlyAskedQuestions } from '../../mocks/scenarios';
import { keyboardAvoidingBehavior } from '../../platform/keyboard';
import { colors, layout, radius, spacing, typography } from '../../theme';
import type { RootStackParamList } from '../../types/navigation';

type Navigation = NativeStackNavigationProp<RootStackParamList>;

export function HomeScreen() {
  const navigation = useNavigation<Navigation>();
  const { width } = useWindowDimensions();
  const [question, setQuestion] = useState('');

  const openChat = useCallback((value: string) => {
    const trimmed = value.trim();
    if (!trimmed) return;
    setQuestion('');
    navigation.navigate('Chat', { initialQuestion: trimmed });
  }, [navigation]);

  return (
    <Screen includeBottomInset>
      <KeyboardAvoidingView behavior={keyboardAvoidingBehavior} style={styles.flex}>
        <ScrollView
          contentContainerStyle={styles.content}
          keyboardDismissMode="interactive"
          keyboardShouldPersistTaps="handled"
          showsVerticalScrollIndicator={false}
        >
          <View style={styles.brandRow}>
            <View>
              <Text accessibilityRole="header" style={styles.brand}>AskU</Text>
              <Text style={styles.brandSubtitle}>校园 AI 信息助手</Text>
            </View>
            <View style={styles.officialBadge}>
              <Ionicons name="shield-checkmark-outline" size={15} color={colors.accent} />
              <Text style={styles.officialText}>官方信息优先</Text>
            </View>
          </View>

          <View style={styles.hero}>
            <Text accessibilityRole="header" style={styles.title}>
              {width < 380 ? `今天想了解\n学校里的什么？` : '今天想了解学校里的什么？'}
            </Text>
            <Text style={styles.subtitle}>直接问，AskU 会帮你查找校园资料并标注来源。</Text>
            <QuestionComposer
              elevated
              onChangeText={setQuestion}
              onSubmit={() => openChat(question)}
              placeholder="问校园里的任何问题"
              value={question}
            />
          </View>

          <Pressable
            accessibilityRole="button"
            accessibilityLabel="打开课表"
            onPress={() => navigation.navigate('Timetable')}
            style={({ pressed }) => [styles.timetableEntry, pressed && styles.questionPressed]}
          >
            <Ionicons name="calendar-outline" size={21} color={colors.accent} />
            <View style={styles.flex}>
              <Text style={styles.questionText}>课表</Text>
              <Text style={styles.timetableHint}>一周安排，随时查看</Text>
            </View>
            <Ionicons name="arrow-forward" size={18} color={colors.textMuted} />
          </Pressable>

          <View style={styles.faqSection}>
            <Text style={styles.sectionTitle}>常问问题</Text>
            <View style={styles.questions}>
              {frequentlyAskedQuestions.map((item) => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={`提问：${item}`}
                  key={item}
                  onPress={() => openChat(item)}
                  style={({ pressed }) => [styles.question, pressed && styles.questionPressed]}
                >
                  <Text style={styles.questionText}>{item}</Text>
                  <Ionicons name="arrow-forward" size={18} color={colors.textMuted} />
                </Pressable>
              ))}
            </View>
          </View>

          <View style={styles.trustNote}>
            <Ionicons name="sparkles-outline" size={17} color={colors.accent} />
            <Text style={styles.trustText}>优先查找学校官网资料；找不到可靠信息时会明确告诉你。</Text>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  content: { flexGrow: 1, paddingHorizontal: layout.screenPadding, paddingTop: spacing[5], paddingBottom: spacing[8] },
  brandRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'flex-start', gap: spacing[3] },
  brand: { fontSize: 34, lineHeight: 40, fontWeight: '800', letterSpacing: -1, color: colors.accent },
  brandSubtitle: { ...typography.metadata, color: colors.textSecondary, marginTop: 2 },
  officialBadge: { flexDirection: 'row', alignItems: 'center', gap: 5, backgroundColor: colors.accentSoft, borderRadius: radius.pill, paddingHorizontal: spacing[3], paddingVertical: spacing[2], marginTop: spacing[1] },
  officialText: { ...typography.metadata, color: colors.accent, fontWeight: '600' },
  hero: { marginTop: spacing[12], gap: spacing[4] },
  title: { ...typography.display, color: colors.textPrimary, maxWidth: 420 },
  subtitle: { ...typography.caption, color: colors.textSecondary, maxWidth: 430, marginTop: -spacing[2] },
  faqSection: { marginTop: spacing[10] },
  timetableEntry: { marginTop: spacing[8], paddingVertical: spacing[4], flexDirection: 'row', alignItems: 'center', gap: spacing[3], borderTopWidth: StyleSheet.hairlineWidth, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  timetableHint: { ...typography.metadata, color: colors.textSecondary, marginTop: 4 },
  sectionTitle: { ...typography.caption, color: colors.textSecondary, fontWeight: '600', marginBottom: spacing[2] },
  questions: { borderTopWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  question: { minHeight: 52, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: spacing[3], borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border, paddingVertical: spacing[3] },
  questionPressed: { opacity: 0.56 },
  questionText: { ...typography.body, color: colors.textPrimary, flex: 1 },
  trustNote: { marginTop: 'auto', paddingTop: spacing[10], flexDirection: 'row', alignItems: 'flex-start', gap: spacing[2] },
  trustText: { ...typography.metadata, color: colors.textMuted, flex: 1 },
});
