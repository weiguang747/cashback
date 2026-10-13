import { StyleSheet, View } from 'react-native';

import { colors, radii, spacing } from '@/theme';

import { Text } from './Text';

export type Detail = { label: string; value: string; numeric?: boolean };

/** White card of label/value pairs, e.g. a receipt. */
export function DetailList({ items }: { items: Detail[] }) {
  return (
    <View style={styles.card}>
      {items.map((item, i) => (
        <View key={item.label} style={[styles.row, i < items.length - 1 && styles.divider]}>
          <Text variant="bodySmall" color="textMuted">
            {item.label}
          </Text>
          <Text variant={item.numeric ? 'numericSmall' : 'label'} style={styles.value}>
            {item.value}
          </Text>
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.xl,
    borderRadius: radii.xxl,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.divider,
  },
  row: { flexDirection: 'row', justifyContent: 'space-between', gap: spacing.lg, paddingVertical: spacing.md },
  divider: { borderBottomWidth: 1, borderBottomColor: colors.dividerSoft },
  value: { flexShrink: 1, textAlign: 'right' },
});
