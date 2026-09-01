import { Ionicons } from '@expo/vector-icons';
import { useEffect, useState } from 'react';
import { Alert, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { Avatar } from '../../components/common/Avatar';
import { Screen } from '../../components/common/Screen';
import { runtimeConfig } from '../../config/runtime';
import { useServices } from '../../services/ServiceProvider';
import { colors, layout, radius, spacing, typography } from '../../theme';
import type { User } from '../../types/domain';

type MenuItem = { icon: keyof typeof Ionicons.glyphMap; label: string; description?: string; action: () => void };

export function ProfileScreen() {
  const { auth } = useServices();
  const [user, setUser] = useState<User | null>(null);
  const [userError, setUserError] = useState(false);
  useEffect(() => {
    let active = true;
    void auth.getCurrentUser()
      .then((value) => { if (active) setUser(value); })
      .catch(() => { if (active) setUserError(true); });
    return () => { active = false; };
  }, [auth]);

  const mockAlert = (title: string, message: string) => Alert.alert(title, message, [{ text: '知道了' }]);
  const items: MenuItem[] = [
    { icon: 'time-outline', label: '历史记录', description: '在底部“历史”中查看', action: () => mockAlert('历史记录', '请使用底部导航进入历史对话。') },
    { icon: 'chatbox-ellipses-outline', label: '意见反馈', action: () => mockAlert('意见反馈', '感谢关注。正式反馈入口将在后续版本接入。') },
    { icon: 'information-circle-outline', label: '关于 AskU', action: () => mockAlert('AskU', `武汉理工大学校园 AI 信息助手\n架构优化版 V${runtimeConfig.version}`) },
    { icon: 'settings-outline', label: '设置', action: () => mockAlert('设置', '当前版本已连接 AskU 开发后端，使用浅色主题。') },
  ];

  return (
    <Screen includeBottomInset>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <Text accessibilityRole="header" style={styles.title}>我的</Text>
        <View style={styles.profile}>
          <Avatar name={user?.nickname ?? '陈'} />
          <View style={styles.profileText}>
            <Text style={styles.name}>{user?.nickname ?? (userError ? '登录状态不可用' : '正在加载')}</Text>
            <Text style={styles.school}>{user?.schoolName ?? '武汉理工大学'}</Text>
          </View>
          <View style={styles.mockBadge}><Text style={styles.mockText}>{runtimeConfig.authMode === 'dev' ? '开发登录' : '微信登录'}</Text></View>
        </View>

        <View style={styles.menu}>
          {items.map((item, index) => (
            <Pressable
              accessibilityRole="button"
              key={item.label}
              onPress={item.action}
              style={({ pressed }) => [styles.menuItem, index < items.length - 1 && styles.menuBorder, pressed && styles.menuPressed]}
            >
              <View style={styles.menuIcon}><Ionicons name={item.icon} size={20} color={colors.accent} /></View>
              <View style={styles.menuText}>
                <Text style={styles.menuLabel}>{item.label}</Text>
                {item.description ? <Text style={styles.menuDescription}>{item.description}</Text> : null}
              </View>
              <Ionicons name="chevron-forward" size={18} color={colors.textMuted} />
            </Pressable>
          ))}
        </View>

        <View style={styles.note}>
          <Ionicons name="shield-checkmark-outline" size={18} color={colors.textSecondary} />
          <Text style={styles.noteText}>会话和反馈已持久化；真实微信登录仍需开放平台凭证后启用。</Text>
        </View>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  content: { flexGrow: 1, paddingHorizontal: layout.screenPadding, paddingTop: spacing[5], paddingBottom: spacing[8] },
  title: { ...typography.pageTitle, color: colors.textPrimary },
  profile: { flexDirection: 'row', alignItems: 'center', gap: spacing[4], marginTop: spacing[8], marginBottom: spacing[8] },
  profileText: { flex: 1, gap: 3 },
  name: { ...typography.heading, color: colors.textPrimary },
  school: { ...typography.caption, color: colors.textSecondary },
  mockBadge: { backgroundColor: colors.surfaceMuted, borderRadius: radius.pill, paddingHorizontal: spacing[3], paddingVertical: spacing[2] },
  mockText: { ...typography.metadata, color: colors.textSecondary },
  menu: { borderTopWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  menuItem: { minHeight: 68, flexDirection: 'row', alignItems: 'center', gap: spacing[3], paddingVertical: spacing[3] },
  menuBorder: { borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  menuPressed: { opacity: 0.6 },
  menuIcon: { width: 38, height: 38, borderRadius: radius.sm, backgroundColor: colors.accentSoft, alignItems: 'center', justifyContent: 'center' },
  menuText: { flex: 1 },
  menuLabel: { ...typography.bodyStrong, color: colors.textPrimary },
  menuDescription: { ...typography.metadata, color: colors.textMuted, marginTop: 2 },
  note: { marginTop: 'auto', paddingTop: spacing[10], flexDirection: 'row', alignItems: 'flex-start', gap: spacing[2] },
  noteText: { ...typography.metadata, color: colors.textMuted, flex: 1 },
});
