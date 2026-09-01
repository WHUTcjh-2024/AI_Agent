import { Ionicons } from '@expo/vector-icons';
import type { ComponentProps } from 'react';
import { Pressable, StyleSheet, type ViewStyle } from 'react-native';

import { colors, layout, radius } from '../../theme';

type IconName = ComponentProps<typeof Ionicons>['name'];

type IconButtonProps = {
  icon: IconName;
  label: string;
  onPress: () => void;
  disabled?: boolean;
  size?: number;
  color?: string;
  style?: ViewStyle;
};

export function IconButton({ icon, label, onPress, disabled, size = 22, color = colors.textPrimary, style }: IconButtonProps) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      disabled={disabled}
      hitSlop={8}
      onPress={onPress}
      style={({ pressed }) => [styles.button, style, pressed && !disabled && styles.pressed, disabled && styles.disabled]}
    >
      <Ionicons name={icon} size={size} color={color} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    width: layout.minimumTouchTarget,
    height: layout.minimumTouchTarget,
    borderRadius: radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
  },
  pressed: { backgroundColor: colors.surfaceMuted, transform: [{ scale: 0.97 }] },
  disabled: { opacity: 0.4 },
});
