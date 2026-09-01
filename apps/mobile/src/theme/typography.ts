import type { TextStyle } from 'react-native';

type Typography = Record<'display' | 'pageTitle' | 'heading' | 'body' | 'bodyStrong' | 'caption' | 'metadata', TextStyle>;

export const typography: Typography = {
  display: { fontSize: 30, lineHeight: 39, fontWeight: '700', letterSpacing: -0.5 },
  pageTitle: { fontSize: 24, lineHeight: 32, fontWeight: '700', letterSpacing: -0.3 },
  heading: { fontSize: 18, lineHeight: 26, fontWeight: '700' },
  body: { fontSize: 16, lineHeight: 25, fontWeight: '400' },
  bodyStrong: { fontSize: 16, lineHeight: 25, fontWeight: '600' },
  caption: { fontSize: 14, lineHeight: 20, fontWeight: '400' },
  metadata: { fontSize: 12, lineHeight: 18, fontWeight: '400' },
};
