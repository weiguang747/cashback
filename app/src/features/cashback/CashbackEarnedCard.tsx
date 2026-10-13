import { Pressable, StyleSheet, View } from 'react-native';

import { Icon, Text } from '@/components';
import { formatIDR } from '@/lib/format';
import { colors, radii, spacing } from '@/theme';

/** Gold card on the success screen showing cash back earned. */
export function CashbackEarnedCard({ amount, onRedeem }: { amount: number; onRedeem: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`You earned ${formatIDR(amount)} cash back. Redeem`}
      onPress={onRedeem}
      style={({ pressed }) => [styles.card, pressed && { opacity: 0.9 }]}
    >
      <View style={styles.icon}>
        <Icon name="percent" size={18} color={colors.reward} strokeWidth={2.2} />
      </View>
      <View style={styles.copy}>
        <Text variant="bodyStrong" color="onReward">
          You earned {formatIDR(amount)} cash back
        </Text>
        <Text variant="caption" color="onRewardMuted">
          Added to your cash back balance
        </Text>
      </View>
      <Text variant="label" color="onReward">
        Redeem
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
