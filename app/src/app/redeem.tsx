import * as Crypto from 'expo-crypto';
import { router } from 'expo-router';
import { useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, Icon, ListRow, ResultHero, Screen, ScreenHeader, Spinner, Text } from '@/components';
import { useAccounts } from '@/lib/accounts';
import { ApiError, redeemCashback, type Redemption } from '@/lib/api';
import { formatMoney, isZeroMoney } from '@/lib/format';
import { recordRedemption } from '@/lib/wallet';
import { colors, radii, spacing } from '@/theme';

/** Move the full cash back balance into the available balance. */
export default function RedeemScreen() {
  const { main, cashback, error, loading, reload } = useAccounts();
  const [processing, setProcessing] = useState(false);
  const [done, setDone] = useState<Redemption | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  // One Idempotency-Key per amount: retrying after an error or timeout
  // reuses it, so cash back can never be moved twice.
  const attempt = useRef<{ key: string; value: string } | null>(null);

  const redeem = async () => {
    if (!cashback || processing) return;
    const amount = cashback.balance;
    if (attempt.current?.value !== amount.value) attempt.current = { key: Crypto.randomUUID(), value: amount.value };

    setProcessing(true);
    setFailure(null);
    try {
      const redemption = await redeemCashback(amount, attempt.current.key);
      recordRedemption(redemption);
      setDone(redemption);
    } catch (e) {
      if (e instanceof ApiError && e.code === 'insufficient_funds') {
        // The balance changed since it loaded: show the new one and start over.
        attempt.current = null;
        setFailure('Your cash back balance has changed. Check the amount and try again.');
        reload();
      } else {
        setFailure(e instanceof Error ? e.message : 'Something went wrong. Please try again.');
      }
    } finally {
      setProcessing(false);
    }
  };

  if (done) {
    const redeemed = done.transaction.amount;
    return (
      <Screen>
        <ResultHero
          status="success"
          ringColor={colors.reward}
          title="Cash back redeemed"
          figure={<Amount money={redeemed} size="md" currencyColor={colors.textMuted} />}
          message={`Added to your available balance. Your new balance is ${formatMoney(done.balance)}.`}
        />
        <Button label="Done" onPress={() => router.dismissTo('/home')} />
      </Screen>
    );
  }

  if (!main || !cashback) {
    return (
      <Screen>
        <ScreenHeader title="Redeem cash back" backLabel="Back to home" />
        <View style={styles.status}>
          {error ? (
            <>
              <Text variant="bodySmall" color="textMuted" align="center">
                {error}
              </Text>
              <Button label="Try again" variant="secondary" size="sm" block={false} onPress={reload} loading={loading} />
            </>
          ) : (
            <Spinner color={colors.textMuted} />
          )}
        </View>
      </Screen>
    );
  }

  const nothingToRedeem = isZeroMoney(cashback.balance);


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
        <Amount money={cashback.balance} size="md" color={colors.onReward} />
        <Text variant="caption" color="onRewardMuted">
          Earned from cash back campaigns
        </Text>
        <Text variant="label" color="onReward" accessibilityRole="link" onPress={() => router.push('/cashback')}>
          View cash back history
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
            subtitle={formatMoney(main.balance)}
          />
        </View>
        <Text variant="caption" color="textMuted">
          {nothingToRedeem
            ? 'You have no cash back to redeem yet. Earn some on your next payment.'
            : 'Your full cash back balance will be added to your available balance instantly.'}
        </Text>
      </View>

      <View style={styles.flex} />

      {failure ? (
        <Text variant="caption" color="danger" align="center" accessibilityRole="alert" style={styles.failure}>
          {failure}
        </Text>
      ) : null}
      <Button
        label={
          processing
            ? 'Redeeming…'
            : nothingToRedeem
              ? 'Nothing to redeem yet'
              : `Redeem ${formatMoney(cashback.balance)}`
        }
        loading={processing}
        disabled={nothingToRedeem}
        onPress={redeem}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  failure: { marginBottom: spacing.md },
  status: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: spacing.md },
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
