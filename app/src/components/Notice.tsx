import { StyleSheet, View } from 'react-native';

import { colors, radii, spacing } from '@/theme';

import { Icon, type IconName } from './Icon';
import { Text } from './Text';

type NoticeProps = {
  icon: IconName;
  children: string;
  /** `muted` sits on the page background; `surface` is a white card. */
  variant?: 'muted' | 'surface';
  iconColor?: string;
};

/** One-line informational callout with a leading icon. */
export function Notice({ icon, children, variant = 'muted', iconColor }: NoticeProps) {
  const surface = variant === 'surface';
  return (
    <View style={[styles.box, surface ? styles.surface : styles.muted]}>
      <Icon name={icon} size={surface ? 20 : 18} color={iconColor ?? colors.textSecondary} />
      <Text variant={surface ? 'bodySmall' : 'caption'} color="textSecondary" style={styles.text}>
        {children}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  box: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm + 2 },
  muted: { padding: spacing.md, paddingHorizontal: spacing.md + 2, borderRadius: radii.lg, backgroundColor: colors.surfaceMuted },
  surface: {
    gap: spacing.md,
    paddingVertical: spacing.md + 2,
    paddingHorizontal: spacing.lg,
    borderRadius: radii.lg,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.divider,
  },
  text: { flex: 1 },
});
