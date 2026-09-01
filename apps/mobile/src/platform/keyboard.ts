import { Platform, type KeyboardAvoidingViewProps } from 'react-native';

export const keyboardAvoidingBehavior: KeyboardAvoidingViewProps['behavior'] = Platform.select({
  ios: 'padding',
  android: 'height',
});
