import { Pressable, StyleSheet, View } from 'react-native';

import { spacing } from '@/theme';

import { Text } from './Text';

type SectionHeaderProps = { title: string; actionLabel?: string; onAction?: () => void };

export function SectionHeader({ title, actionLabel, onAction }: SectionHeaderProps) {
  return (
    <View style={styles.row}>
      <Text variant="heading" accessibilityRole="header">
        {title}
      </Text>
      {actionLabel ? (
        <Pressable accessibilityRole="link" onPress={onAction} hitSlop={12}>
          <Text variant="linkSmall" color="positive">
            {actionLabel}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: spacing.xs },
});
