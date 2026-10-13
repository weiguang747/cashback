import { router, useLocalSearchParams } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, Keypad, Pill, Screen, ScreenHeader, Text, Tile, applyKey } from '@/components';
import { demo } from '@/config/demo';
import { account, findMerchant } from '@/data/mock';
import { formatIDR, formatNumber } from '@/lib/format';
import { colors, radii, spacing, type AmountSize } from '@/theme';

/** Enter an amount and simulate paying the chosen merchant. */
export default function PayScreen() {
  const params = useLocalSearchParams<{ merchantId?: string; amount?: string }>();
  const merchant = findMerchant(params.merchantId);

  const [amount, setAmount] = useState(params.amount ?? '');
  const [processing, setProcessing] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  const value = Number(amount || '0');
  const empty = value === 0;
  const overBalance = value > account.balance;
  const blocked = empty || overBalance;

  const size: AmountSize = amount.length > 9 ? 'sm' : amount.length > 6 ? 'lg' : 'xl';
  const figureColor = empty ? colors.textPlaceholder : overBalance ? colors.danger : colors.text;

  const pay = () => {
    if (blocked || processing) return;
    setProcessing(true);
    timer.current = setTimeout(() => {
      const shared = { amount, merchantId: merchant.id };
      if (demo.paymentOutcome === 'error') {
        router.replace({ pathname: '/payment-failed', params: { ...shared, reason: 'error' } });
      } else {
        router.replace({ pathname: '/payment-success', params: shared });
      }
    }, demo.processingMs);
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
          <Amount value={amount} size={size} color={figureColor} />
        </View>
        {overBalance ? (
          <Pill tone="danger" icon="alert">
            {`Amount exceeds your balance of ${formatIDR(account.balance)}`}
          </Pill>
        ) : (
          <Pill>{`Balance ${formatIDR(account.balance)}`}</Pill>
        )}
      </View>

      <Keypad disabled={processing} onPress={(k) => setAmount((a) => applyKey(a, k))} />

      <Button
        label={processing ? 'Processing…' : `Pay Rp ${formatNumber(amount)}`}
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
