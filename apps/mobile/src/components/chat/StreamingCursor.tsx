import { useEffect, useRef } from 'react';
import { Animated, StyleSheet } from 'react-native';

import { colors, motion } from '../../theme';

export function StreamingCursor() {
  const opacity = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(opacity, { toValue: 0.16, duration: motion.streamCursorInterval, useNativeDriver: true }),
        Animated.timing(opacity, { toValue: 1, duration: motion.streamCursorInterval, useNativeDriver: true }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [opacity]);

  return <Animated.View accessibilityLabel="正在生成" style={[styles.cursor, { opacity }]} />;
}

const styles = StyleSheet.create({
  cursor: { width: 3, height: 18, borderRadius: 2, backgroundColor: colors.accent, marginTop: 4 },
});
