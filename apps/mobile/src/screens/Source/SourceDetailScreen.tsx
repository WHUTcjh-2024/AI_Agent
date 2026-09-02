import { Ionicons } from '@expo/vector-icons';
import type { RouteProp } from '@react-navigation/native';
import { useRoute } from '@react-navigation/native';
import { useCallback, useEffect, useState } from 'react';
import { Alert, Linking, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { ErrorState, LoadingState } from '../../components/common/StateView';
import { Screen } from '../../components/common/Screen';
import { useServices } from '../../services/ServiceProvider';
import { colors, layout, radius, spacing, typography } from '../../theme';
import type { Source } from '../../types/domain';
import type { RootStackParamList } from '../../types/navigation';
import { formatPublishedDate } from '../../utils/date';

type SourceRoute = RouteProp<RootStackParamList, 'SourceDetail'>;

function MetadataRow({ label, value, last = false }: { label: string; value: string; last?: boolean }) {
  return (
    <View style={[styles.metadataRow, !last && styles.metadataBorder]}>
      <Text style={styles.metadataLabel}>{label}</Text>
      <Text style={styles.metadataValue}>{value}</Text>
    </View>
  );
}

export function SourceDetailScreen() {
  const route = useRoute<SourceRoute>();
  const { chat } = useServices();
  const [source, setSource] = useState<Source | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadSource = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await chat.getSource(route.params.sourceId);
      if (!result) throw new Error('来源不存在');
      setSource(result);
    } catch {
      setError('暂时无法读取这条来源。');
    } finally {
      setLoading(false);
    }
  }, [chat, route.params.sourceId]);

  useEffect(() => { void loadSource(); }, [loadSource]);

  if (loading) return <Screen includeTopInset={false}><LoadingState label="正在读取来源" /></Screen>;
  if (error || !source) return <Screen includeTopInset={false}><ErrorState message={error ?? '来源不存在'} onRetry={() => void loadSource()} /></Screen>;

  const openURL = async (url: string) => {
    if (await Linking.canOpenURL(url)) {
      await Linking.openURL(url);
      return;
    }
    Alert.alert('无法打开', '这个学校链接当前不可用。');
  };
  const officialURL = source.officialUrl || source.url;

  return (
    <Screen includeTopInset={false}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <View style={styles.badgeRow}>
          <View style={styles.badge}><Ionicons name={source.official ? 'shield-checkmark' : 'information-circle'} size={15} color={colors.accent} /><Text style={styles.badgeText}>{source.official ? '官方来源' : '非官方来源'}</Text></View>
        </View>
        <Text accessibilityRole="header" style={styles.title}>{source.title}</Text>

        <View style={styles.metadata}>
          <MetadataRow label="发布部门" value={source.department || source.publisher} />
          <MetadataRow label="发布时间" value={formatPublishedDate(source.publishedAt)} />
          <MetadataRow label="来源类型" value={source.documentType || source.sourceType || '学校网页'} />
          <MetadataRow label="适用对象" value={source.audience} last />
        </View>

        <View style={styles.summarySection}>
          <Text style={styles.sectionTitle}>内容摘要</Text>
          <Text style={styles.summary}>{source.summary}</Text>
          {(source.evidence ?? []).length ? (
            <View style={styles.evidenceBox}>
              <Text style={styles.evidenceLabel}>回答所依据的原文片段</Text>
              {(source.evidence ?? []).map((excerpt, index) => <Text key={`${index}-${excerpt.slice(0, 16)}`} style={styles.evidence}>“{excerpt}”</Text>)}
            </View>
          ) : null}
          <Text style={styles.mockNote}>摘要用于快速确认来源，具体内容请以学校原文为准。</Text>
        </View>

        {(source.attachments ?? []).length ? (
          <View style={styles.attachmentsSection}>
            <Text style={styles.sectionTitle}>官方附件</Text>
            {(source.attachments ?? []).map((attachment) => (
              <Pressable accessibilityRole="link" key={attachment.id || attachment.url} onPress={() => void openURL(attachment.url)} style={({ pressed }) => [styles.attachmentButton, pressed && styles.openPressed]}>
                <Ionicons name="document-attach-outline" size={19} color={colors.accent} />
                <Text numberOfLines={2} style={styles.attachmentText}>{attachment.name}</Text>
                <Ionicons name="open-outline" size={18} color={colors.accent} />
              </Pressable>
            ))}
          </View>
        ) : null}

        {officialURL ? (
          <Pressable accessibilityRole="link" onPress={() => void openURL(officialURL)} style={({ pressed }) => [styles.openButton, pressed && styles.openPressed]}>
            <Ionicons name="open-outline" size={19} color={colors.white} />
            <Text style={styles.openText}>查看学校原文</Text>
          </Pressable>
        ) : (
          <View accessibilityRole="text" style={styles.unavailableSource}>
            <Ionicons name="information-circle-outline" size={18} color={colors.textSecondary} />
            <Text style={styles.unavailableSourceText}>学校原文链接尚未录入</Text>
          </View>
        )}
        {source.parentPageUrl && source.parentPageUrl !== officialURL ? (
          <Pressable accessibilityRole="link" onPress={() => void openURL(source.parentPageUrl!)} style={({ pressed }) => [styles.parentButton, pressed && styles.openPressed]}>
            <Text style={styles.parentText}>查看原通知</Text>
          </Pressable>
        ) : null}
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  content: { paddingHorizontal: layout.screenPadding, paddingTop: spacing[6], paddingBottom: spacing[10] },
  badgeRow: { flexDirection: 'row' },
  badge: { flexDirection: 'row', alignItems: 'center', gap: 5, backgroundColor: colors.accentSoft, borderRadius: radius.pill, paddingHorizontal: spacing[3], paddingVertical: spacing[2] },
  badgeText: { ...typography.metadata, color: colors.accent, fontWeight: '600' },
  title: { ...typography.pageTitle, color: colors.textPrimary, marginTop: spacing[5] },
  metadata: { marginTop: spacing[8], borderTopWidth: StyleSheet.hairlineWidth, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  metadataRow: { minHeight: 58, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: spacing[5], paddingVertical: spacing[3] },
  metadataBorder: { borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  metadataLabel: { ...typography.caption, color: colors.textSecondary },
  metadataValue: { ...typography.caption, color: colors.textPrimary, fontWeight: '500', textAlign: 'right', flex: 1 },
  summarySection: { marginTop: spacing[8] },
  sectionTitle: { ...typography.heading, color: colors.textPrimary, marginBottom: spacing[3] },
  summary: { ...typography.body, color: colors.textPrimary },
  evidenceBox: { marginTop: spacing[5], padding: spacing[4], backgroundColor: colors.surfaceMuted, borderRadius: radius.md, gap: spacing[2] },
  evidenceLabel: { ...typography.metadata, color: colors.textSecondary, fontWeight: '700' },
  evidence: { ...typography.caption, color: colors.textPrimary },
  mockNote: { ...typography.metadata, color: colors.textMuted, marginTop: spacing[4] },
  attachmentsSection: { marginTop: spacing[8], gap: spacing[2] },
  attachmentButton: { minHeight: 52, flexDirection: 'row', alignItems: 'center', gap: spacing[3], paddingHorizontal: spacing[4], borderWidth: StyleSheet.hairlineWidth, borderColor: colors.borderStrong, borderRadius: radius.md },
  attachmentText: { ...typography.caption, color: colors.textPrimary, fontWeight: '600', flex: 1 },
  openButton: { minHeight: 52, marginTop: spacing[10], flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: spacing[2], backgroundColor: colors.accent, borderRadius: radius.md },
  openPressed: { backgroundColor: colors.accentPressed },
  openText: { ...typography.bodyStrong, color: colors.white },
  parentButton: { minHeight: 48, marginTop: spacing[3], alignItems: 'center', justifyContent: 'center', borderWidth: StyleSheet.hairlineWidth, borderColor: colors.borderStrong, borderRadius: radius.md },
  parentText: { ...typography.bodyStrong, color: colors.accent },
  unavailableSource: { minHeight: 52, marginTop: spacing[10], flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: spacing[2], backgroundColor: colors.surfaceMuted, borderRadius: radius.md },
  unavailableSourceText: { ...typography.bodyStrong, color: colors.textSecondary },
});
