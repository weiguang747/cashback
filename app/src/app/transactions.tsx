import { useMemo, useState } from 'react';
import { RefreshControl, ScrollView, SectionList, StyleSheet, View } from 'react-native';

import { Button, Chip, Screen, ScreenHeader, Spinner, Text, TextField, TransactionRow } from '@/components';
import type { TransactionKind } from '@/lib/api';
import { groupByDay, timeLabel, useTransactions } from '@/lib/transactions';
import { colors, spacing } from '@/theme';

type Filter = 'all' | TransactionKind;

const FILTERS: { id: Filter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'in', label: 'Money in' },
  { id: 'out', label: 'Money out' },
  { id: 'cashback', label: 'Cash back' },
];

/** Full history, grouped by day, with search and type filters. */
export default function TransactionsScreen() {
  const [filter, setFilter] = useState<Filter>('all');
  const [query, setQuery] = useState('');
  const { data, error, loading, reload } = useTransactions();

  const sections = useMemo(() => {
    const q = query.trim().toLowerCase();
    const matches = (data ?? []).filter(
      (t) => (filter === 'all' || t.kind === filter) && (!q || t.name.toLowerCase().includes(q)),
    );
    return groupByDay(matches);
  }, [data, filter, query]);

  const filtered = filter !== 'all' || query.trim() !== '';

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
