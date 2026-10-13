import { router, useLocalSearchParams } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, DetailList, ResultHero, Screen, Text } from '@/components';
import { findMerchant } from '@/data/mock';
import { CashbackEarnedCard } from '@/features/cashback/CashbackEarnedCard';
import type { CashbackReason } from '@/lib/api';
import { isZeroMoney } from '@/lib/format';
import { dateTimeLabel } from '@/lib/transactions';
import { colors, spacing } from '@/theme';

export default function PaymentSuccessScreen() {
  const {
    amount = '0',
    currency = 'IDR',
    merchantId,
    transactionId = '',
    createdAt,
    cashbackAmount,
    cashbackReason,
  } = useLocalSearchParams<{
    amount?: string;
    currency?: string;
    merchantId?: string;
    transactionId?: string;
    createdAt?: string;
    /** Set when a cash back campaign was live for the payment. */
    cashbackAmount?: string;
    cashbackReason?: CashbackReason;
  }>();
  const merchant = findMerchant(merchantId);
  const earned = cashbackAmount ? { value: cashbackAmount, currency } : null;
  const noCashbackNote = earned && isZeroMoney(earned) ? NO_CASHBACK[cashbackReason ?? 'below_minimum'] : null;

  return (
    <Screen>
      <ResultHero
        status="success"
        title="Payment successful"
        figure={<Amount money={{ value: amount, currency }} size="md" currencyColor={colors.textMuted} />}
        message="Your payment has been sent."
      />

      <View style={styles.bottom}>
        {earned && !isZeroMoney(earned) ? (
          <CashbackEarnedCard amount={earned} onPress={() => router.push('/cashback')} />
        ) : null}
        {noCashbackNote ? (
          <Text variant="caption" color="textMuted" align="center">
            {noCashbackNote}
          </Text>
        ) : null}
        <DetailList
          items={[
            { label: 'Paid to', value: merchant.name },
            { label: 'Date', value: createdAt ? dateTimeLabel(new Date(createdAt)) : '' },
            { label: 'Paid from', value: 'Main balance' },
            { label: 'Reference', value: transactionId.slice(0, 8).toUpperCase(), numeric: true },
          ]}
        />
        <Button label="Done" onPress={() => router.dismissTo('/home')} style={styles.cta} />
      </View>
    </Screen>
  );
}

// Why a payment during a campaign earned nothing.
const NO_CASHBACK: Record<CashbackReason, string> = {
  awarded: '',
  below_minimum: 'This payment was below the minimum for cash back.',
  daily_cap_reached: 'You’ve reached today’s cash back limit. Earn again tomorrow.',
  budget_exhausted: 'The cash back campaign has ended: its budget is fully claimed.',
};

const styles = StyleSheet.create({
  bottom: { gap: spacing.md },
  cta: { marginTop: spacing.md },
});
