import { Pressable, StyleSheet, View, type StyleProp, type ViewStyle } from 'react-native';

import { colors, radii, sizes, spacing } from '@/theme';

import { Icon, type IconName } from './Icon';
import { Spinner } from './Spinner';
import { Text } from './Text';

type Variant = 'primary' | 'secondary' | 'inverse';
type Size = 'lg' | 'md' | 'sm';

export type ButtonProps = {
  label: string;
  onPress?: () => void;
  variant?: Variant;
  size?: Size;
  icon?: IconName;
  loading?: boolean;
  disabled?: boolean;
  /** Stretch to fill the row (default) or hug its label. */
  block?: boolean;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel?: string;
};

const variantStyles: Record<Variant, { bg: string; fg: string; border?: string }> = {
  primary: { bg: colors.brand, fg: colors.onBrand },
  secondary: { bg: 'transparent', fg: colors.text, border: colors.borderStrong },
  inverse: { bg: colors.onBrand, fg: colors.text },
};

const sizeStyles: Record<Size, { height: number; radius: number; paddingX: number }> = {
  lg: { height: sizes.buttonLarge, radius: radii.xl, paddingX: spacing.xxl },
  md: { height: sizes.buttonMedium, radius: radii.lg, paddingX: spacing.xl },
  sm: { height: sizes.touch, radius: radii.md, paddingX: spacing.lg + 2 },
};

export function Button({
  label,
  onPress,
  variant = 'primary',
  size = 'lg',
  icon,
  loading = false,
  disabled = false,
  block = true,
  style,
  accessibilityLabel,
}: ButtonProps) {
  const inactive = disabled || loading;
  const v = disabled ? { bg: colors.disabled, fg: colors.onDisabled } : variantStyles[variant];
  const s = sizeStyles[size];

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? label}
      accessibilityState={{ disabled: inactive, busy: loading }}
      disabled={inactive}
      onPress={onPress}
      style={({ pressed }) => [
        styles.base,
        {
          height: s.height,
          borderRadius: s.radius,
          paddingHorizontal: s.paddingX,
          backgroundColor: v.bg,
          borderColor: 'border' in v && v.border ? v.border : 'transparent',
          borderWidth: 'border' in v && v.border ? 1 : 0,
          alignSelf: block ? 'stretch' : 'flex-start',
          opacity: pressed ? 0.85 : 1,
        },
        style,
      ]}
    >
      <View style={styles.content}>
        {loading ? <Spinner color={v.fg} /> : icon ? <Icon name={icon} size={18} color={v.fg} /> : null}
        <Text variant={size === 'lg' ? 'button' : size === 'md' ? 'bodyStrong' : 'buttonSmall'} style={{ color: v.fg }}>
          {label}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: { alignItems: 'center', justifyContent: 'center' },
  content: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm + 2 },
});
