import { Pressable, StyleSheet } from 'react-native';

import { colors, radii, sizes } from '@/theme';

import { Icon, type IconName } from './Icon';

type IconButtonProps = {
  icon: IconName;
  /** Required: icon-only buttons need a spoken label. */
  accessibilityLabel: string;
  onPress?: () => void;
};

/** Round 44pt button with an outline, used for back and header actions. */
export function IconButton({ icon, accessibilityLabel, onPress }: IconButtonProps) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      onPress={onPress}
      hitSlop={4}
      style={({ pressed }) => [styles.button, pressed && styles.pressed]}
    >
      <Icon name={icon} size={20} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    width: sizes.touch,
    height: sizes.touch,
    borderRadius: radii.pill,
    borderWidth: 1,
    borderColor: colors.border,
    backgroundColor: colors.surface,
    alignItems: 'center',
    justifyContent: 'center',
  },
  pressed: { backgroundColor: colors.surfaceSubtle },
});
