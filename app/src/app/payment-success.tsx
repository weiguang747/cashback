import { router, useLocalSearchParams } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { Amount, Button, DetailList, ResultHero, Screen } from '@/components';
import { demo } from '@/config/demo';
import { findMerchant, payment } from '@/data/mock';
import { CashbackEarnedCard } from '@/features/cashback/CashbackEarnedCard';
import { cashbackFor, idr } from '@/lib/format';
import { colors, spacing } from '@/theme';

export default function PaymentSuccessScreen() {
  const { amount = '0', merchantId } = useLocalSearchParams<{ amount?: string; merchantId?: string }>();
  const merchant = findMerchant(merchantId);
  const earned = demo.campaign === 'active' ? cashbackFor(Number(amount), demo.cashbackRate) : 0;

  return (
    <Screen>
      <ResultHero
        status="success"
        title="Payment successful"
        figure={<Amount money={idr(amount)} size="md" currencyColor={colors.textMuted} />}
        message="Your payment has been sent."
      />

      <View style={styles.bottom}>
        {earned > 0 ? <CashbackEarnedCard amount={earned} onRedeem={() => router.push('/redeem')} /> : null}
        <DetailList
          items={[
            { label: 'Paid to', value: merchant.name },
            { label: 'Date', value: payment.date },
            { label: 'Paid from', value: payment.source },
            { label: 'Reference', value: payment.reference, numeric: true },
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
