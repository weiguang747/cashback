import { router } from 'expo-router';
import { useMemo, useState } from 'react';
import { FlatList, StyleSheet, View } from 'react-native';

import { Icon, ListRow, Notice, Screen, ScreenHeader, Text, TextField, Tile } from '@/components';
import { merchants } from '@/data/merchants';
import { colors, spacing } from '@/theme';

/** Simulated merchants to pay. Sits between Home and Pay. */
export default function MerchantsScreen() {
  const [query, setQuery] = useState('');
  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? merchants.filter((m) => `${m.name} ${m.category}`.toLowerCase().includes(q)) : merchants;
  }, [query]);

  return (
    <Screen>
      <View style={styles.top}>
        <ScreenHeader title="Choose merchant" backLabel="Back to home" />
        <Notice icon="info">These are simulated merchants for demo purposes. No real money is sent.</Notice>
        <TextField
          variant="search"
          label="Search merchants"
          placeholder="Search merchants"
          value={query}
          onChangeText={setQuery}
          returnKeyType="search"
          autoCorrect={false}
        />
      </View>

      <FlatList
        data={results}
        keyExtractor={(m) => m.id}
        keyboardShouldPersistTaps="handled"
        showsVerticalScrollIndicator={false}
        contentContainerStyle={styles.list}
        ListHeaderComponent={
          <Text variant="overline" color="textMuted" style={styles.overline}>
            Merchants
          </Text>
        }
        ListEmptyComponent={
          <Text variant="bodySmall" color="textMuted" style={styles.empty}>
            No merchants match “{query}”.
          </Text>
        }
        renderItem={({ item }) => (
          <ListRow
            leading={<Tile tone={item.tone} label={item.name} />}
            title={item.name}
            subtitle={item.category}
            trailing={<Icon name="chevron-right" size={18} color={colors.textPlaceholder} />}
            onPress={() => router.push({ pathname: '/pay', params: { merchantId: item.id } })}
          />
        )}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  top: { gap: spacing.lg },
  list: { paddingTop: spacing.lg, paddingBottom: spacing.xxxl },
  overline: { marginBottom: spacing.xs },
  empty: { paddingVertical: spacing.xxl },
});
