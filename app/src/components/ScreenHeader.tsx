import { router } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { sizes } from '@/theme';

import { IconButton } from './IconButton';
import { Text } from './Text';

type ScreenHeaderProps = {
  title: string;
  /** Defaults to going back in the stack. */
  onBack?: () => void;
  backLabel?: string;
};

/** Back button, centered title and a spacer to keep the title centered. */
export function ScreenHeader({ title, onBack, backLabel = 'Back' }: ScreenHeaderProps) {
  return (
    <View style={styles.row}>
      <IconButton icon="chevron-left" accessibilityLabel={backLabel} onPress={onBack ?? (() => router.back())} />
      <Text variant="headline" accessibilityRole="header" numberOfLines={1} style={styles.title}>
        {title}
      </Text>
      <View style={styles.spacer} />
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  title: { flex: 1, textAlign: 'center' },
  spacer: { width: sizes.touch, height: sizes.touch },
});
