import { router, useLocalSearchParams } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, Notice, ResultHero, Screen } from '@/components';
import { colors, spacing } from '@/theme';

const messages = {
  insufficient: "You don't have enough balance for this payment. Please enter a smaller amount.",
  error: "We couldn't process your payment right now. Please check your connection and try again.",
} as const;

export default function PaymentFailedScreen() {
  const {
    amount = '0',
    currency = 'IDR',
    merchantId,
    reason,
    key,
  } = useLocalSearchParams<{
    amount?: string;
    currency?: string;
    merchantId?: string;
    reason?: keyof typeof messages;
    /** Idempotency-Key to reuse when the outcome is unknown. */
    key?: string;
  }>();

  return (
    <Screen>
      <ResultHero
        status="error"
        title="Payment failed"
        figure={<Amount money={{ value: amount, currency }} size="md" color={colors.textMuted} strikethrough />}
        message={messages[reason ?? 'error'] ?? messages.error}
      />

      <View style={styles.bottom}>
        <Notice icon="shield" variant="surface" iconColor={colors.positive}>
          No money has left your account.
        </Notice>
        <Button
          label="Try again"
          style={styles.cta}
          onPress={() => router.replace({ pathname: '/pay', params: { merchantId, amount, ...(key ? { key } : {}) } })}
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
