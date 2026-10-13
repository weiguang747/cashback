import { router } from 'expo-router';
import { useMemo } from 'react';
import { RefreshControl, SectionList, StyleSheet, View } from 'react-native';

import { Amount, Button, EmptyState, LoadMoreFooter, Screen, ScreenHeader, Spinner, Text, TransactionRow } from '@/components';
import { ratePercent, useCampaign, useCashbackHistory } from '@/lib/cashback';
import { formatMoney, isZeroMoney } from '@/lib/format';
import { groupByDay, timeLabel } from '@/lib/transactions';
import { colors, radii, spacing } from '@/theme';

/** Cash back balance and every award and redemption behind it. */
export default function CashbackScreen() {
  const { items, meta, error, loading, loadingMore, moreError, hasMore, reload, loadMore } = useCashbackHistory();
  const { data: campaign } = useCampaign();
  const sections = useMemo(() => groupByDay(items ?? []), [items]);

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
        refreshControl={<RefreshControl refreshing={loading && items !== null} onRefresh={reload} />}
        // A failed page waits for "Try again" rather than retrying on every scroll.
        onEndReached={hasMore && !moreError ? loadMore : undefined}
        onEndReachedThreshold={0.5}
        ListFooterComponent={<LoadMoreFooter loading={loadingMore} error={moreError} onRetry={loadMore} />}
        ListHeaderComponent={
          meta ? (
            <View style={styles.hero}>
              <Text variant="captionStrong" color="onRewardMuted">
                Cash back balance
              </Text>
              <Amount money={meta.balance} size="md" color={colors.onReward} />
              <Button
                label="Redeem"
                variant="secondary"
                size="sm"
                block={false}
                disabled={isZeroMoney(meta.balance)}
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
          items === null ? (
            error ? (
              <View style={styles.status}>
                <Text variant="bodySmall" color="textMuted" align="center">
                  {error}
                </Text>
                <Button label="Try again" variant="secondary" size="sm" block={false} onPress={reload} loading={loading} style={styles.retry} />
              </View>
            ) : (
              <View style={styles.status} accessibilityLabel="Loading cash back">
                <Spinner color={colors.textMuted} />
              </View>
            )
          ) : (
            <View style={styles.empty}>
              <EmptyState icon="percent" title="No cash back yet" message={howToEarn} />
            </View>
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
  retry: { alignSelf: 'center' },
  empty: { paddingTop: spacing.xxxl },
});
