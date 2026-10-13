import { StyleSheet, View } from 'react-native';

import { colors, radii, spacing } from '@/theme';

import { Icon, type IconName } from './Icon';
import { Text } from './Text';

type PillProps = { children: string; tone?: 'neutral' | 'danger'; icon?: IconName };

/** Small rounded status label, e.g. "Balance Rp 12.450.000". */
export function Pill({ children, tone = 'neutral', icon }: PillProps) {
  const danger = tone === 'danger';
  const fg = danger ? colors.danger : colors.textMuted;
  return (
    <View
      style={[styles.pill, { backgroundColor: danger ? colors.dangerSoft : colors.surfaceMuted }]}
      accessibilityRole={danger ? 'alert' : undefined}
      accessibilityLiveRegion={danger ? 'polite' : undefined}
    >
      {icon ? <Icon name={icon} size={15} color={fg} strokeWidth={2.2} /> : null}
      <Text variant={danger ? 'captionStrong' : 'caption'} style={{ color: fg }}>
        {children}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  pill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.xs + 2,
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.md + 2,
    borderRadius: radii.pill,
  },
});
