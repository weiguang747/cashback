import type { ReactNode } from 'react';
import { StyleSheet, View } from 'react-native';

import { colors, spacing } from '@/theme';

import { Icon, type IconName } from './Icon';
import { Text } from './Text';

type ResultHeroProps = {
  status: 'success' | 'error';
  title: string;
  /** Usually an <Amount />. */
  figure?: ReactNode;
  message?: string;
  /** Outer ring color; defaults by status. */
  ringColor?: string;
};

const statusStyle: Record<ResultHeroProps['status'], { icon: IconName; ring: string; fill: string }> = {
  success: { icon: 'check', ring: colors.positiveSoft, fill: colors.positive },
  error: { icon: 'close', ring: colors.dangerSoft, fill: colors.danger },
};

/** Centered outcome block: status badge, title, figure and message. */
export function ResultHero({ status, title, figure, message, ringColor }: ResultHeroProps) {
  const s = statusStyle[status];
  return (
    <View style={styles.wrap}>
      <View style={[styles.ring, { backgroundColor: ringColor ?? s.ring }]}>
        <View style={[styles.badge, { backgroundColor: s.fill }]}>
          <Icon name={s.icon} size={32} color="#FFFFFF" strokeWidth={2.4} />
        </View>
      </View>
      <Text variant="title" accessibilityRole="header" align="center" style={styles.title}>
        {title}
      </Text>
      {figure}
      {message ? (
        <Text variant="body" color="textMuted" align="center" style={styles.message}>
          {message}
        </Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: spacing.lg },
  ring: { width: 96, height: 96, borderRadius: 48, alignItems: 'center', justifyContent: 'center' },
  badge: { width: 64, height: 64, borderRadius: 32, alignItems: 'center', justifyContent: 'center' },
  title: { marginTop: spacing.sm },
  message: { maxWidth: 300 },
});
