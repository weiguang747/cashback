import { router, useLocalSearchParams } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, Notice, ResultHero, Screen } from '@/components';
import { colors, spacing } from '@/theme';

// `insufficient` is a definite decline. `error` covers unknown outcomes
// (timeouts, lost connections, server errors): the payment may have gone
// through, so the screen must not promise that no money moved.
const copy = {
  insufficient: {
    title: 'Payment failed',
    message: "You don't have enough balance for this payment. Please enter a smaller amount.",
    notice: 'No money has left your account.',
  },
  error: {
    title: 'Payment not confirmed',
    message: "We couldn't confirm your payment. Please check your connection and try again.",
    notice: 'If this payment went through, trying again won’t charge you twice.',
  },
} as const;

export default function PaymentFailedScreen() {
  const {
    amount,
    currency,
    merchantId,
    reason,
    key,
  } = useLocalSearchParams<{
    amount: string;
    currency: string;
    merchantId?: string;
    reason?: keyof typeof copy;
    /** Idempotency-Key to reuse when the outcome is unknown. */
    key?: string;
  }>();
  const declined = reason === 'insufficient';
  const text = declined ? copy.insufficient : copy.error;

  return (
    <Screen>
      <ResultHero
        status="error"
        title={text.title}
        figure={<Amount money={{ value: amount, currency }} size="md" color={colors.textMuted} strikethrough={declined} />}
        message={text.message}
      />

      <View style={styles.bottom}>
        <Notice icon="shield" variant="surface" iconColor={colors.positive}>
          {text.notice}
        </Notice>
        <Button
          label="Try again"
          style={styles.cta}
          onPress={() => router.replace({ pathname: '/pay', params: { merchantId, amount, currency, ...(key ? { key } : {}) } })}
        />
        <Button label="Back to home" variant="secondary" onPress={() => router.dismissTo('/home')} />
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  bottom: { gap: spacing.sm + 2 },
  cta: { marginTop: spacing.lg },
});
