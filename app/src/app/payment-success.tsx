import { router, useLocalSearchParams } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, DetailList, ResultHero, Screen } from '@/components';
import { findMerchant } from '@/data/mock';
import { dateTimeLabel } from '@/lib/transactions';
import { colors, spacing } from '@/theme';

export default function PaymentSuccessScreen() {
  const {
    amount = '0',
    currency = 'IDR',
    merchantId,
    transactionId = '',
    createdAt,
  } = useLocalSearchParams<{
    amount?: string;
    currency?: string;
    merchantId?: string;
    transactionId?: string;
    createdAt?: string;
  }>();
  const merchant = findMerchant(merchantId);

  return (
    <Screen>
      <ResultHero
        status="success"
        title="Payment successful"
        figure={<Amount money={{ value: amount, currency }} size="md" currencyColor={colors.textMuted} />}
        message="Your payment has been sent."
      />

      <View style={styles.bottom}>
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

const styles = StyleSheet.create({
  bottom: { gap: spacing.md },
  cta: { marginTop: spacing.md },
});
