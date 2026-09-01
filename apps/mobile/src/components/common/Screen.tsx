import type { PropsWithChildren } from 'react';
import { StyleSheet, View, type ViewStyle } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { colors, layout } from '../../theme';

type ScreenProps = PropsWithChildren<{
  style?: ViewStyle;
  includeTopInset?: boolean;
  includeBottomInset?: boolean;
}>;

export function Screen({ children, style, includeTopInset = true, includeBottomInset = false }: ScreenProps) {
  const insets = useSafeAreaInsets();
  return (
    <View
      style={[
        styles.root,
        {
          paddingTop: includeTopInset ? insets.top : 0,
          paddingBottom: includeBottomInset ? insets.bottom : 0,
        },
        style,
      ]}
    >
      <View style={styles.content}>{children}</View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.background, alignItems: 'center' },
  content: { flex: 1, width: '100%', maxWidth: layout.maxContentWidth },
});
