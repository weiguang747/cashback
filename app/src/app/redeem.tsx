import { router } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, Icon, ListRow, ResultHero, Screen, ScreenHeader, Text } from '@/components';
import { demo } from '@/config/demo';
import { account } from '@/data/mock';
import { formatMoney, idr } from '@/lib/format';
import { colors, radii, spacing } from '@/theme';

/** Move the full cash back balance into the available balance. */
export default function RedeemScreen() {
  const [step, setStep] = useState<'form' | 'processing' | 'done'>('form');
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  const redeem = () => {
    setStep('processing');
    timer.current = setTimeout(() => setStep('done'), demo.processingMs - 200);
  };

  if (step === 'done') {
    return (
      <Screen>
        <ResultHero
          status="success"
          ringColor={colors.reward}
          title="Cash back redeemed"
          figure={<Amount money={idr(account.cashback)} size="md" currencyColor={colors.textMuted} />}
          message={`Added to your available balance. Your new balance is ${formatMoney(idr(account.balance + account.cashback))}.`}
        />
        <Button label="Done" onPress={() => router.dismissTo('/home')} />
      </Screen>
    );
  }

  return (
    <Screen>
      <ScreenHeader title="Redeem cash back" backLabel="Back to home" />

      <View style={styles.hero}>
        <View style={styles.heroIcon}>
          <Icon name="percent" size={26} color={colors.reward} />
        </View>
        <Text variant="captionStrong" color="onRewardMuted" style={styles.heroLabel}>
          Cash back balance
        </Text>
        <Amount money={idr(account.cashback)} size="md" color={colors.onReward} />
        <Text variant="caption" color="onRewardMuted">
          Earned from {Math.round(demo.cashbackRate * 100)}% cash back on your payments
        </Text>
      </View>

      <View style={styles.target}>
        <Text variant="label" color="textSecondary">
          Redeem to
        </Text>
        <View style={styles.targetCard}>
          <ListRow
            divider={false}
            leading={
              <View style={styles.walletTile}>
                <Icon name="wallet" size={20} color={colors.onBrand} />
              </View>
            }
            title="Available balance"
            subtitle={formatMoney(idr(account.balance))}
          />
        </View>
        <Text variant="caption" color="textMuted">
          Your full cash back balance will be added to your available balance instantly.
        </Text>
      </View>

      <View style={styles.flex} />

      <Button
        label={step === 'processing' ? 'Redeeming…' : `Redeem ${formatMoney(idr(account.cashback))}`}
        loading={step === 'processing'}
        onPress={redeem}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  hero: {
    marginTop: spacing.xxxl,
    paddingVertical: spacing.xxl + 4,
    paddingHorizontal: spacing.xxl,
    borderRadius: radii.hero,
    backgroundColor: colors.reward,
    alignItems: 'center',
    gap: spacing.sm + 2,
  },
  heroIcon: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.onReward,
    alignItems: 'center',
    justifyContent: 'center',
  },
  heroLabel: { fontSize: 14 },
  target: { marginTop: spacing.xxl, gap: spacing.sm },
  targetCard: {
    paddingHorizontal: spacing.lg,
    borderRadius: radii.xxl,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.divider,
  },
  walletTile: {
    width: 44,
    height: 44,
    borderRadius: radii.md,
    backgroundColor: colors.brand,
    alignItems: 'center',
    justifyContent: 'center',
  },
});
