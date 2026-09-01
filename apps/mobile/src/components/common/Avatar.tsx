import { StyleSheet, Text, View } from 'react-native';

import { colors, radius, typography } from '../../theme';

type AvatarProps = { name: string; size?: number };

export function Avatar({ name, size = 62 }: AvatarProps) {
  return (
    <View style={[styles.avatar, { width: size, height: size, borderRadius: size / 2 }]} accessibilityLabel={`${name}的头像`}>
      <Text allowFontScaling maxFontSizeMultiplier={1.3} style={[styles.text, { fontSize: Math.max(18, size * 0.34) }]}>
        {name.slice(0, 1)}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  avatar: { backgroundColor: colors.accentSoft, alignItems: 'center', justifyContent: 'center', borderWidth: 1, borderColor: '#D6E4FF' },
  text: { ...typography.heading, color: colors.accent },
});
