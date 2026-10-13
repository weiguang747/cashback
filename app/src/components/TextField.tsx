import { useState } from 'react';
import { StyleSheet, TextInput, View, type TextInputProps } from 'react-native';

import { colors, radii, sizes, spacing, typography } from '@/theme';

import { Icon } from './Icon';
import { Text } from './Text';

type TextFieldProps = Omit<TextInputProps, 'style'> & {
  label: string;
  /** Show the label above the field. Search fields keep it for screen readers only. */
  showLabel?: boolean;
  /** Highlights the border when the value is valid. */
  valid?: boolean;
  variant?: 'default' | 'search';
};

export function TextField({ label, showLabel = true, valid, variant = 'default', ...input }: TextFieldProps) {
  const [focused, setFocused] = useState(false);
  const isSearch = variant === 'search';
  const borderColor = valid ? colors.positive : focused ? colors.textMuted : colors.border;

  return (
    <View style={styles.wrapper}>
      {showLabel && !isSearch ? (
        <Text variant="label" color="textSecondary">
          {label}
        </Text>
      ) : null}
      <View style={[styles.field, isSearch ? styles.search : styles.default, { borderColor }]}>
        {isSearch ? <Icon name="search" size={18} color={colors.textMuted} /> : null}
        <TextInput
          {...input}
          accessibilityLabel={label}
          placeholderTextColor={colors.textPlaceholder}
          onFocus={(e) => {
            setFocused(true);
            input.onFocus?.(e);
          }}
          onBlur={(e) => {
            setFocused(false);
            input.onBlur?.(e);
          }}
          style={[styles.input, isSearch ? typography.body : { ...typography.body, fontSize: 16 }]}
        />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  wrapper: { gap: spacing.sm },
  field: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm + 2,
    paddingHorizontal: spacing.lg,
    borderWidth: 1,
    backgroundColor: colors.surface,
  },
  default: { height: sizes.input, borderRadius: radii.lg },
  search: { height: sizes.search, borderRadius: radii.lg },
  input: { flex: 1, minWidth: 0, color: colors.text, paddingVertical: 0 },
});
