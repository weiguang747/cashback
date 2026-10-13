import type { ReactNode } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { colors, spacing } from '@/theme';

import { Text } from './Text';

type ListRowProps = {
  leading?: ReactNode;
  title: string;
  subtitle?: string;
  trailing?: ReactNode;
  onPress?: () => void;
  /** Draw the bottom divider. Default true. */
  divider?: boolean;
  accessibilityLabel?: string;
};

/** A row with a leading tile, two lines of text and optional trailing content. */
export function ListRow({ leading, title, subtitle, trailing, onPress, divider = true, accessibilityLabel }: ListRowProps) {
  const content = (
    <>
      {leading}
      <View style={styles.text}>
        <Text variant="bodyStrong" numberOfLines={1}>
          {title}
        </Text>
        {subtitle ? (
          <Text variant="caption" color="textMuted" numberOfLines={1}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      {trailing}
    </>
  );

  const rowStyle = [styles.row, divider && styles.divider];

  if (!onPress) {
    return <View style={rowStyle}>{content}</View>;
  }
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? [title, subtitle].filter(Boolean).join(', ')}
      onPress={onPress}
      style={({ pressed }) => [rowStyle, pressed && styles.pressed]}
    >
      {content}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md + 2, paddingVertical: spacing.md },
  divider: { borderBottomWidth: 1, borderBottomColor: colors.divider },
  text: { flex: 1, minWidth: 0, gap: spacing.xxs },
  pressed: { opacity: 0.6 },
});
