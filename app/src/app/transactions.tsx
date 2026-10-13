import { useMemo, useState } from 'react';
import { ScrollView, SectionList, StyleSheet, View } from 'react-native';

import { Chip, Screen, ScreenHeader, Text, TextField, TransactionRow } from '@/components';
import { transactionGroups, type TransactionKind } from '@/data/mock';
import { spacing } from '@/theme';

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

  const sections = useMemo(() => {
    const q = query.trim().toLowerCase();
    return transactionGroups
      .map((g) => ({
        title: g.label,
        data: g.items.filter(
          (t) => (filter === 'all' || t.kind === filter) && (!q || t.name.toLowerCase().includes(q)),
        ),
      }))
      .filter((s) => s.data.length > 0);
  }, [filter, query]);

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
        contentContainerStyle={styles.list}
        renderSectionHeader={({ section }) => (
          <Text variant="overline" color="textMuted" accessibilityRole="header" style={styles.sectionHeader}>
            {section.title}
          </Text>
        )}
        renderItem={({ item }) => <TransactionRow transaction={item} />}
        ListEmptyComponent={
          <Text variant="bodySmall" color="textMuted" align="center" style={styles.empty}>
            No transactions found.
          </Text>
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
  empty: { paddingVertical: spacing.xxxl },
});
