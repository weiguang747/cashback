import { router, useFocusEffect } from 'expo-router';
import { useCallback, useRef } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';

import { Button, EmptyState, Screen, SectionHeader, Spinner, Text, Tile, TransactionRow } from '@/components';
import { CashbackBanner } from '@/features/cashback/CashbackBanner';
import { BalanceCard } from '@/features/wallet/BalanceCard';
import { useAccounts } from '@/lib/accounts';
import { useCampaign } from '@/lib/cashback';
import { clearUser, useUser } from '@/lib/session';
import { dayLabel, timeLabel, useTransactions } from '@/lib/transactions';
import { colors, radii, spacing } from '@/theme';

/** How many transactions Home shows before "See all". */
const RECENT_COUNT = 6;

export default function HomeScreen() {
  const user = useUser();
  const accounts = useAccounts();
  const { data: campaign, reload: reloadCampaign } = useCampaign();
  const { data: transactions, error, loading, reload } = useTransactions(RECENT_COUNT);

  // Refresh when coming back to Home (e.g. after paying). The first focus is
  // skipped because the hooks above already load on mount.
  const reloadAccounts = accounts.reload;
  const firstFocus = useRef(true);
  useFocusEffect(
    useCallback(() => {
      if (firstFocus.current) {
        firstFocus.current = false;
        return;
      }
      reloadAccounts();
      reloadCampaign();
      reload();
    }, [reloadAccounts, reloadCampaign, reload]),
  );
  const name = user?.name ?? '';
  const initials = name
    .split(/\s+/)
    .map((w) => w[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();
  const goPay = () => router.push('/merchants');
  const goRedeem = () => router.push('/redeem');
  const goCashback = () => router.push('/cashback');

  return (
    <Screen padded={false}>
      <ScrollView contentContainerStyle={styles.content} showsVerticalScrollIndicator={false}>
        <View style={styles.header}>
          <View style={styles.user}>
            <Tile tone="positive" size={44} radius={22} label={name}>
              <Text variant="headline" color="positive">
                {initials}
              </Text>
            </Tile>
            <View style={styles.greeting}>
              <Text variant="caption" color="textMuted">
                Good morning
              </Text>
              <Text variant="headline">{name}</Text>
            </View>
          </View>
          <Button
            label="Log out"
            icon="log-out"
            variant="secondary"
            size="sm"
            block={false}
            onPress={clearUser}
            style={styles.logout}
          />
        </View>

        <BalanceCard
          balance={accounts.main?.balance ?? null}
          cashback={accounts.cashback?.balance ?? null}
          error={accounts.error}
          onRetry={accounts.reload}
          onRedeem={goRedeem}
          onViewCashback={goCashback}
          onPay={goPay}
        />

        {campaign?.status === 'active' ? (
          <CashbackBanner state="active" campaign={campaign} onPress={goPay} onViewHistory={goCashback} />
        ) : campaign?.status === 'ended' ? (
          <CashbackBanner state="ended" onPress={goCashback} />
        ) : null}

        <View style={styles.section}>
          <SectionHeader
            title="Transactions"
            actionLabel={transactions?.length ? 'See all' : undefined}
            onAction={() => router.push('/transactions')}
          />
          {transactions === null ? (
            error ? (
              <View style={styles.status}>
                <Text variant="bodySmall" color="textMuted" align="center">
                  {error}
                </Text>
                <Button label="Try again" variant="secondary" size="sm" block={false} onPress={reload} loading={loading} style={styles.retry} />
              </View>
            ) : (
              <View style={styles.status} accessibilityLabel="Loading transactions">
                <Spinner color={colors.textMuted} />
              </View>
            )
          ) : transactions.length ? (
            transactions.map((t) => {
              const date = new Date(t.createdAt);
              return <TransactionRow key={t.id} transaction={t} subtitle={`${dayLabel(date)} · ${timeLabel(date)}`} />;
            })
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
  status: { alignItems: 'center', gap: spacing.md, paddingVertical: spacing.xxxl },
  retry: { alignSelf: 'center' },
});
