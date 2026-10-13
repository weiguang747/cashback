import { useMemo, useState } from 'react';
import { RefreshControl, ScrollView, SectionList, StyleSheet, View } from 'react-native';

import { Button, Chip, LoadMoreFooter, Screen, ScreenHeader, Spinner, Text, TextField, TransactionRow } from '@/components';
import type { TransactionKind } from '@/lib/api';
import { groupByDay, timeLabel, useTransactionPages } from '@/lib/transactions';
import { useDebounced } from '@/lib/useDebounced';
import { colors, spacing } from '@/theme';

type Filter = 'all' | TransactionKind;

const FILTERS: { id: Filter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'in', label: 'Money in' },
  { id: 'out', label: 'Money out' },
  { id: 'cashback', label: 'Cash back' },
];

/** Wait this long after the last keystroke before searching. */
const SEARCH_DELAY_MS = 300;

/**
 * Full history, grouped by day, with search and type filters. Rows load a
 * page at a time as the user scrolls; search and filters run on the server,
 * so they cover the whole history, not just the pages loaded so far.
 */
export default function TransactionsScreen() {
  const [filter, setFilter] = useState<Filter>('all');
  const [query, setQuery] = useState('');
  const search = useDebounced(query.trim(), SEARCH_DELAY_MS);
  const { items: data, error, loading, loadingMore, moreError, hasMore, reload, loadMore } = useTransactionPages({
    kind: filter === 'all' ? undefined : filter,
    q: search || undefined,
  });

  const sections = useMemo(() => groupByDay(data ?? []), [data]);

  const filtered = filter !== 'all' || search !== '';

  return (
    <Screen>
      <View style={styles.top}>
        <ScreenHeader title="Transactions" backLabel="Back to home" />
        <TextField
          variant="search"
          label="Search transactions"
          placeholder="Search transactions"
          value={query}
          onChangeText={setQuery}
          returnKeyType="search"
          autoCorrect={false}
        />
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.chips}>
          {FILTERS.map((f) => (
            <Chip key={f.id} label={f.label} selected={filter === f.id} onPress={() => setFilter(f.id)} />
          ))}
        </ScrollView>
      </View>

      <SectionList
        sections={sections}
        keyExtractor={(t) => t.id}
        stickySectionHeadersEnabled={false}
        keyboardShouldPersistTaps="handled"
        showsVerticalScrollIndicator={false}
        contentContainerStyle={styles.list}
        renderSectionHeader={({ section }) => (
          <Text variant="overline" color="textMuted" accessibilityRole="header" style={styles.sectionHeader}>
            {section.title}
          </Text>
        )}
        renderItem={({ item }) => (
          <TransactionRow transaction={item} subtitle={`${timeLabel(new Date(item.createdAt))} · ${item.description}`} />
        )}
        refreshControl={<RefreshControl refreshing={loading && data !== null} onRefresh={reload} />}
        // A failed page waits for "Try again" rather than retrying on every scroll.
        onEndReached={hasMore && !moreError ? loadMore : undefined}
        onEndReachedThreshold={0.5}
        ListFooterComponent={<LoadMoreFooter loading={loadingMore} error={moreError} onRetry={loadMore} />}
        ListHeaderComponent={
          error && data ? (
            <Text variant="caption" color="danger" accessibilityRole="alert" style={styles.refreshError}>
              Couldn’t refresh. {error}
            </Text>
          ) : null
        }
        ListEmptyComponent={
          data === null ? (
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
          ) : (
            <Text variant="bodySmall" color="textMuted" align="center" style={styles.status}>
              {filtered ? 'No transactions match your search or filter.' : 'No transactions yet.'}
            </Text>
          )
        }
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  top: { gap: spacing.lg },
  chips: { gap: spacing.sm },
  list: { paddingBottom: spacing.xxxl },
  sectionHeader: { marginTop: spacing.xl, marginBottom: spacing.xs },
  status: { alignItems: 'center', gap: spacing.md, paddingVertical: spacing.xxxl },
  retry: { alignSelf: 'center' },
  refreshError: { marginTop: spacing.md },
});
