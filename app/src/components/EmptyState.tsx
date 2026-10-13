import { StyleSheet, View } from 'react-native';

import { colors, radii, spacing } from '@/theme';

import { Button } from './Button';
import { Icon, type IconName } from './Icon';
import { Text } from './Text';

type EmptyStateProps = {
  icon: IconName;
  title: string;
  message: string;
  actionLabel?: string;
  onAction?: () => void;
};

export function EmptyState({ icon, title, message, actionLabel, onAction }: EmptyStateProps) {
  return (
    <View style={styles.wrap}>
      <View style={styles.icon}>
        <Icon name={icon} size={28} color={colors.textMuted} strokeWidth={1.8} />
      </View>
      <Text variant="headline" align="center" style={styles.title}>
        {title}
      </Text>
      <Text variant="bodySmall" color="textMuted" align="center" style={styles.message}>
        {message}
      </Text>
      {actionLabel ? (
        <Button label={actionLabel} onPress={onAction} size="sm" block={false} style={styles.action} />
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: spacing.sm + 2, paddingHorizontal: spacing.lg, paddingBottom: spacing.xxl },
  icon: {
    width: 64,
    height: 64,
    borderRadius: radii.xxl,
    backgroundColor: colors.surfaceMuted,
    alignItems: 'center',
    justifyContent: 'center',
  },
  title: { marginTop: spacing.xs },
  message: { maxWidth: 260 },
  action: { alignSelf: 'center', marginTop: spacing.xs + 2 },
});
