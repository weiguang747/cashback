import type { ReactNode } from 'react';
import { StyleSheet, View, type StyleProp, type ViewStyle } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { colors, spacing } from '@/theme';

type ScreenProps = {
  children: ReactNode;
  /** Apply the standard horizontal gutter. Default true. */
  padded?: boolean;
  style?: StyleProp<ViewStyle>;
};

/** Root container for every screen: safe areas, background and gutter. */
export function Screen({ children, padded = true, style }: ScreenProps) {
  return (
    <SafeAreaView style={styles.safe} edges={['top', 'bottom']}>
      <View style={[styles.body, padded && styles.padded, style]}>{children}</View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.background },
  body: { flex: 1, paddingTop: spacing.md, paddingBottom: spacing.lg },
  padded: { paddingHorizontal: spacing.gutter },
});
