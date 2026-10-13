import { StyleSheet, View } from 'react-native';

import { colors, spacing } from '@/theme';

import { Button } from './Button';
import { Spinner } from './Spinner';
import { Text } from './Text';

type LoadMoreFooterProps = {
  loading: boolean;
  /** Why the next page failed; shows a retry button instead of the spinner. */
  error: string | null;
  onRetry: () => void;
};

/** The end of a paged list: a spinner while the next page loads, or why it didn't. */
export function LoadMoreFooter({ loading, error, onRetry }: LoadMoreFooterProps) {
  if (error) {
    return (
      <View style={styles.wrap}>
        <Text variant="caption" color="danger" align="center" accessibilityRole="alert">
          Couldn’t load more. {error}
        </Text>
        <Button label="Try again" variant="secondary" size="sm" block={false} onPress={onRetry} style={styles.retry} />
      </View>
    );
  }
  if (!loading) return null;
  return (
    <View style={styles.wrap} accessibilityLabel="Loading more">
      <Spinner color={colors.textMuted} />
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { alignItems: 'center', gap: spacing.md, paddingVertical: spacing.xl },
  retry: { alignSelf: 'center' },
});
