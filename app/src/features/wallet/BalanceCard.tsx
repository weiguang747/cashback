import { Pressable, StyleSheet, View } from 'react-native';

import { Amount, Button, Icon, Spinner, Text } from '@/components';
import type { Money } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { colors, radii, spacing } from '@/theme';

type BalanceCardProps = {
  /** null while loading. */
  balance: Money | null;
  cashback: Money | null;
  /** Shown in place of the balance when it couldn't be loaded. */
  error?: string | null;
  onRetry?: () => void;
  onRedeem: () => void;
  /** Opens the cash back history. */
  onViewCashback: () => void;
  onPay: () => void;
};

/** Dark hero card on Home: available balance, cash back row and Pay. */
export function BalanceCard({ balance, cashback, error, onRetry, onRedeem, onViewCashback, onPay }: BalanceCardProps) {
  return (
    <View style={styles.card}>
      <View style={styles.top}>
        <Text variant="bodySmall" color="onBrandMuted">
          Available balance
        </Text>
        {balance ? (
          <View style={styles.badge}>
            <Text variant="microStrong" color="onBrand" style={styles.badgeText}>
              {balance.currency}
            </Text>
          </View>
        ) : null}
      </View>

      {balance ? (
        <Amount money={balance} size="sm" color={colors.onBrand} currencyColor={colors.onBrandMuted} />
      ) : error ? (
        <Pressable accessibilityRole="button" onPress={onRetry} style={styles.status}>
          <Text variant="bodySmall" color="onBrand">
            {`Couldn’t load your balance. ${error}`}
          </Text>
          <Text variant="label" color="reward">
            Try again
          </Text>
        </Pressable>
      ) : (
        <View style={styles.status} accessibilityLabel="Loading balance">
          <Spinner color={colors.onBrandMuted} />
        </View>
      )}

      <Pressable
        accessibilityRole="button"
        accessibilityLabel={cashback ? `Cash back ${formatMoney(cashback)}. View history` : 'Cash back. View history'}
        onPress={onViewCashback}
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
            {cashback ? formatMoney(cashback) : '–'}
          </Text>
        </View>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Redeem cash back"
          onPress={onRedeem}
          hitSlop={spacing.sm}
          style={({ pressed }) => [styles.redeem, pressed && styles.redeemPressed]}
        >
          <Text variant="label" color="reward">
            Redeem
          </Text>
          <Icon name="chevron-right" size={16} color={colors.reward} strokeWidth={2.2} />
        </Pressable>
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
  status: { minHeight: 44, justifyContent: 'center', gap: spacing.xs },
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
  redeemPressed: { opacity: 0.6 },
});
