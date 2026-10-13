import { Pressable, StyleSheet } from 'react-native';

import { colors, radii, sizes, spacing } from '@/theme';

import { Text } from './Text';

type ChipProps = { label: string; selected?: boolean; onPress?: () => void };

/** Toggle pill used for filters. */
export function Chip({ label, selected = false, onPress }: ChipProps) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected }}
      onPress={onPress}
      style={[
        styles.chip,
        selected
          ? { backgroundColor: colors.brand, borderColor: colors.brand }
          : { backgroundColor: colors.surface, borderColor: colors.border },
      ]}
    >
      <Text variant="label" style={{ color: selected ? colors.onBrand : colors.text }} numberOfLines={1}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  chip: {
    height: sizes.chip,
    paddingHorizontal: spacing.md + 2,
    borderRadius: radii.pill,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
});
