import { router } from 'expo-router';
import { ScrollView, StyleSheet, View } from 'react-native';

import { Button, EmptyState, Screen, SectionHeader, Text, Tile, TransactionRow } from '@/components';
import { demo } from '@/config/demo';
import { account, recentTransactions } from '@/data/mock';
import { CashbackBanner } from '@/features/cashback/CashbackBanner';
import { BalanceCard } from '@/features/wallet/BalanceCard';
import { colors, radii, spacing } from '@/theme';

export default function HomeScreen() {
  const transactions = demo.hasTransactions ? recentTransactions : [];
  const goPay = () => router.push('/merchants');
  const goRedeem = () => router.push('/redeem');

  return (
    <Screen padded={false}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <View style={styles.header}>
          <View style={styles.user}>
            <Tile tone="positive" size={44} radius={22} label={account.initials}>
              <Text variant="headline" color="positive">
                {account.initials}
              </Text>
            </Tile>
            <View style={styles.greeting}>
              <Text variant="caption" color="textMuted">
                Good morning
              </Text>
              <Text variant="headline">{account.name}</Text>
            </View>
          </View>
          <Button
            label="Log out"
            icon="log-out"
            variant="secondary"
            size="sm"
            block={false}
            onPress={() => router.replace('/')}
            style={styles.logout}
          />
        </View>

        <BalanceCard
          balance={account.balance}
          currency={account.currency}
          cashback={account.cashback}
          onRedeem={goRedeem}
          onPay={goPay}
        />

        {demo.campaign === 'active' ? (
          <CashbackBanner
            state="active"
            ratePercent={Math.round(demo.cashbackRate * 100)}
            earnedToday={demo.earnedToday}
            dailyCap={demo.dailyCap}
            onPress={goPay}
          />
        ) : (
          <CashbackBanner state="ended" onPress={goRedeem} />
        )}

        <View style={styles.section}>
          <SectionHeader
            title="Transactions"
            actionLabel={transactions.length ? 'See all' : undefined}
            onAction={() => router.push('/transactions')}
          />
          {transactions.length ? (
            transactions.map((t) => <TransactionRow key={t.id} transaction={t} />)
          ) : (
            <EmptyState
              icon="receipt"
              title="No transactions yet"
              message="Your payments and the money you receive will show up here."
              actionLabel="Make your first payment"
              onAction={goPay}
            />
          )}
        </View>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  content: { paddingHorizontal: spacing.gutter, paddingBottom: spacing.xxxl, gap: spacing.lg },
  header: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: spacing.sm },
  user: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
  greeting: { gap: spacing.xxs },
  logout: { backgroundColor: colors.surface, borderColor: colors.border, borderRadius: radii.pill },
  section: { marginTop: spacing.sm, minHeight: 280 },
});
