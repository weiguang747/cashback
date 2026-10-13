import { Pressable, StyleSheet, View } from 'react-native';

import { Amount, Button, Icon, Text } from '@/components';
import { formatIDR } from '@/lib/format';
import { colors, radii, spacing } from '@/theme';

type BalanceCardProps = {
  balance: number;
  currency: string;
  cashback: number;
  onRedeem: () => void;
  onPay: () => void;
};

/** Dark hero card on Home: available balance, cash back row and Pay. */
export function BalanceCard({ balance, currency, cashback, onRedeem, onPay }: BalanceCardProps) {
  return (
    <View style={styles.card}>
      <View style={styles.top}>
        <Text variant="bodySmall" color="onBrandMuted">
          Available balance
        </Text>
        <View style={styles.badge}>
          <Text variant="microStrong" color="onBrand" style={styles.badgeText}>
            {currency}
          </Text>
        </View>
      </View>

      <Amount value={balance} size="sm" color={colors.onBrand} currencyColor={colors.onBrandMuted} />

      <Pressable
        accessibilityRole="button"
        accessibilityLabel={`Cash back ${formatIDR(cashback)}. Redeem`}
        onPress={onRedeem}
        style={({ pressed }) => [styles.cashback, pressed && styles.cashbackPressed]}
      >
        <View style={styles.cashbackIcon}>
          <Icon name="percent" size={15} color={colors.onReward} strokeWidth={2.4} />
        </View>
        <View style={styles.cashbackText}>
          <Text variant="micro" color="onBrandMuted">
            Cash back
          </Text>
          <Text variant="numeric" color="onBrand">
            {formatIDR(cashback)}
          </Text>
        </View>
        <View style={styles.redeem}>
          <Text variant="label" color="reward">
            Redeem
          </Text>
          <Icon name="chevron-right" size={16} color={colors.reward} strokeWidth={2.2} />
        </View>
      </Pressable>

      <Button label="Pay" icon="arrow-up-right" variant="inverse" size="md" onPress={onPay} />
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    padding: spacing.xxl,
    borderRadius: radii.hero,
    backgroundColor: colors.brand,
    gap: spacing.xl,
  },
  top: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  badge: {
    paddingVertical: 6,
    paddingHorizontal: spacing.sm + 2,
    borderRadius: radii.pill,
    backgroundColor: colors.onBrandOverlayStrong,
  },
  badgeText: { letterSpacing: 0.5 },
  cashback: {
    marginTop: -spacing.xs,
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm + 2,
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.md + 2,
    borderRadius: radii.lg,
    backgroundColor: colors.onBrandOverlay,
    borderWidth: 1,
    borderColor: colors.onBrandBorder,
  },
  cashbackPressed: { backgroundColor: colors.onBrandOverlayStrong },
  cashbackIcon: {
    width: 28,
    height: 28,
    borderRadius: 14,
    backgroundColor: colors.reward,
    alignItems: 'center',
    justifyContent: 'center',
  },
  cashbackText: { flex: 1, gap: 1 },
  redeem: { flexDirection: 'row', alignItems: 'center', gap: spacing.xs },
});
