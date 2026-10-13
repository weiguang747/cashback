import * as Crypto from 'expo-crypto';
import { router, useLocalSearchParams } from 'expo-router';
import { useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, Keypad, Pill, Screen, ScreenHeader, Text, Tile, applyKey } from '@/components';
import { findMerchant } from '@/data/mock';
import { useAccounts } from '@/lib/accounts';
import { ApiError, createPayment } from '@/lib/api';
import { compareMoney, formatMoney, idr } from '@/lib/format';
import { colors, radii, spacing, type AmountSize } from '@/theme';

/** Enter an amount and pay the chosen merchant from the main balance. */
export default function PayScreen() {
  // `key` comes back from the failure screen so "Try again" retries the
  // same payment instead of starting a new one.
  const params = useLocalSearchParams<{ merchantId?: string; amount?: string; key?: string }>();
  const merchant = findMerchant(params.merchantId);
  const { main, error } = useAccounts();

  const [amount, setAmount] = useState(params.amount ?? '');
  const [processing, setProcessing] = useState(false);
  // One Idempotency-Key per payment. Retrying the same amount reuses it, so
  // a retry after a timeout can never charge twice; a new amount gets a new key.
  const attempt = useRef(params.key && params.amount ? { key: params.key, amount: params.amount } : null);

  const value = Number(amount || '0');
  const empty = value === 0;
  // Only a hint for the user; the server enforces the real check.
  const overBalance = main ? compareMoney(idr(amount), main.balance) > 0 : false;
  const blocked = empty || overBalance || !main;

  const size: AmountSize = amount.length > 9 ? 'sm' : amount.length > 6 ? 'lg' : 'xl';
  const figureColor = empty ? colors.textPlaceholder : overBalance ? colors.danger : colors.text;

  const pay = async () => {
    if (blocked || processing) return;
    if (attempt.current?.amount !== amount) attempt.current = { key: Crypto.randomUUID(), amount };
    const { key } = attempt.current;
    const money = idr(amount);

    setProcessing(true);
    try {
      const { transaction, cashback } = await createPayment(
        { merchantId: merchant.id, merchantName: merchant.name, amount: money },
        key,
      );
      router.replace({
        pathname: '/payment-success',
        params: {
          merchantId: merchant.id,
          amount: money.value,
          currency: money.currency,
          transactionId: transaction.id,
          createdAt: transaction.createdAt,
          ...(cashback ? { cashbackAmount: cashback.amount.value, cashbackReason: cashback.reason } : {}),
        },
      });
    } catch (e) {
      const shared = { merchantId: merchant.id, amount, currency: money.currency };
      if (e instanceof ApiError && e.code === 'insufficient_funds') {
        // A definite decline: the next attempt is a new payment.
        router.replace({ pathname: '/payment-failed', params: { ...shared, reason: 'insufficient' } });
      } else {
        // The outcome may be unknown (e.g. a timeout), so retry with the same key.
        router.replace({ pathname: '/payment-failed', params: { ...shared, reason: 'error', key } });
      }
    }
  };

  return (
    <Screen>
      <ScreenHeader title="Simulate Pay" backLabel="Back to merchants" />

      <View style={styles.payee}>
        <Tile tone={merchant.tone} size={40} radius={radii.sm} label={merchant.name} />
        <View style={styles.payeeText}>
          <Text variant="micro" color="textMuted">
            Paying to
          </Text>
          <Text variant="bodyStrong">{merchant.name}</Text>
        </View>
        <Text variant="micro" color="textMuted">
          {merchant.category}
        </Text>
      </View>

      <View style={styles.entry}>
        <Text variant="bodySmall" color="textMuted">
          Enter amount
        </Text>
        <View accessibilityLiveRegion="polite">
          <Amount money={idr(amount)} size={size} color={figureColor} />
        </View>
        {!main ? (
          <Pill tone={error ? 'danger' : undefined}>{error ? `Couldn’t load your balance. ${error}` : 'Loading balance…'}</Pill>
        ) : overBalance ? (
          <Pill tone="danger" icon="alert">
            {`Amount exceeds your balance of ${formatMoney(main.balance)}`}
          </Pill>
        ) : (
          <Pill>{`Balance ${formatMoney(main.balance)}`}</Pill>
        )}
      </View>

      <Keypad disabled={processing} onPress={(k) => setAmount((a) => applyKey(a, k))} />

      <Button
        label={processing ? 'Processing…' : `Pay ${formatMoney(idr(amount))}`}
        loading={processing}
        disabled={blocked}
        onPress={pay}
        style={styles.cta}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  payee: {
    marginTop: spacing.xl,
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.md + 2,
    borderRadius: radii.xl,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.divider,
  },
  payeeText: { flex: 1, gap: 1 },
  entry: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: spacing.md + 2 },
  cta: { marginTop: spacing.lg },
});
