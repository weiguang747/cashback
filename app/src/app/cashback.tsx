import { router } from 'expo-router';
import { useMemo } from 'react';
import { RefreshControl, SectionList, StyleSheet, View } from 'react-native';

import { Amount, Button, EmptyState, Screen, ScreenHeader, Spinner, Text, TransactionRow } from '@/components';
import { ratePercent, useCampaign, useCashbackHistory } from '@/lib/cashback';
import { formatMoney, isZeroMoney } from '@/lib/format';
import { groupByDay, timeLabel } from '@/lib/transactions';
import { colors, radii, spacing } from '@/theme';

/** Cash back balance and every credit (and, later, redemption) behind it. */
export default function CashbackScreen() {
  const { data, error, loading, reload } = useCashbackHistory();
  const { data: campaign } = useCampaign();
  const sections = useMemo(() => groupByDay(data?.transactions ?? []), [data]);

  const howToEarn =
    campaign?.status === 'active'
      ? `Pay ${formatMoney(campaign.minSpend)} or more to earn ${ratePercent(campaign.rateBps)} cash back.`
      : 'Cash back from campaigns will show up here.';

  return (
    <Screen>
      <ScreenHeader title="Cash back" backLabel="Back" />

      <SectionList
        sections={sections}
        keyExtractor={(t) => t.id}
        stickySectionHeadersEnabled={false}
        showsVerticalScrollIndicator={false}
        contentContainerStyle={styles.list}
        refreshControl={<RefreshControl refreshing={loading && data !== null} onRefresh={reload} />}
        ListHeaderComponent={
          data ? (
            <View style={styles.hero}>
              <Text variant="captionStrong" color="onRewardMuted">
                Cash back balance
              </Text>
              <Amount money={data.balance} size="md" color={colors.onReward} />
              <Button
                label="Redeem"
                variant="secondary"
                size="sm"
                block={false}
                disabled={isZeroMoney(data.balance)}
                onPress={() => router.push('/redeem')}
                style={styles.redeem}
              />
              {error ? (
                <Text variant="caption" color="danger" accessibilityRole="alert">
                  Couldn’t refresh. {error}
                </Text>
              ) : null}
            </View>
          ) : null
        }
        renderSectionHeader={({ section }) => (
          <Text variant="overline" color="textMuted" accessibilityRole="header" style={styles.sectionHeader}>
            {section.title}
          </Text>
        )}
        renderItem={({ item }) => (
          <TransactionRow transaction={item} subtitle={`${timeLabel(new Date(item.createdAt))} · ${item.description}`} />
        )}
        ListEmptyComponent={
          data === null ? (
            error ? (
              <View style={styles.status}>
                <Text variant="bodySmall" color="textMuted" align="center">
                  {error}
                </Text>
                <Button label="Try again" variant="secondary" size="sm" block={false} onPress={reload} loading={loading} />
              </View>
            ) : (
              <View style={styles.status} accessibilityLabel="Loading cash back">
                <Spinner color={colors.textMuted} />
              </View>
            )
          ) : (
            <EmptyState icon="percent" title="No cash back yet" message={howToEarn} />
          )
        }
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  list: { paddingBottom: spacing.xxxl },
  hero: {
    marginTop: spacing.xl,
    paddingVertical: spacing.xl,
    paddingHorizontal: spacing.xl,
    borderRadius: radii.hero,
    backgroundColor: colors.reward,
    alignItems: 'center',
    gap: spacing.sm,
  },
  redeem: { alignSelf: 'center' },
  sectionHeader: { marginTop: spacing.xl, marginBottom: spacing.xs },
  status: { alignItems: 'center', gap: spacing.md, paddingVertical: spacing.xxxl },
});
