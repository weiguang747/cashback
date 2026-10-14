import { Pressable, StyleSheet, View } from 'react-native';

import { Icon, Text } from '@/components';
import type { Money } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { colors, radii, spacing } from '@/theme';

type CashbackEarnedCardProps = {
  amount: Money;
  /** Earned but not yet credited: the server adds it to the balance shortly. */
  pending?: boolean;
  onPress: () => void;
};

/** Gold card on the success screen showing cash back earned. */
export function CashbackEarnedCard({ amount, pending = false, onPress }: CashbackEarnedCardProps) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`You earned ${formatMoney(amount)} cash back. View cash back`}
      onPress={onPress}
      style={({ pressed }) => [styles.card, pressed && { opacity: 0.9 }]}
    >
      <View style={styles.icon}>
        <Icon name="percent" size={18} color={colors.reward} strokeWidth={2.2} />
      </View>
      <View style={styles.copy}>
        <Text variant="bodyStrong" color="onReward">
          You earned {formatMoney(amount)} cash back
        </Text>
        <Text variant="caption" color="onRewardMuted">
          {pending ? 'On its way to your cash back balance' : 'Added to your cash back balance'}
        </Text>
      </View>
      <Text variant="label" color="onReward">
        View
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  card: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    paddingVertical: spacing.md + 2,
    paddingHorizontal: spacing.lg,
    borderRadius: radii.xxl,
    backgroundColor: colors.reward,
  },
  icon: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: colors.onReward,
    alignItems: 'center',
    justifyContent: 'center',
  },
  copy: { flex: 1, gap: 2 },
});
