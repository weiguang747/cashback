import { Pressable, StyleSheet, View } from 'react-native';

import { colors, radii, sizes, spacing } from '@/theme';

import { Icon } from './Icon';
import { Text } from './Text';

export type KeypadKey = '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8' | '9' | '000' | '0' | 'back';

const KEYS: KeypadKey[] = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '000', '0', 'back'];

const labelFor = (k: KeypadKey) => (k === 'back' ? 'Delete' : k === '000' ? 'Three zeros' : k);

type KeypadProps = { onPress: (key: KeypadKey) => void; disabled?: boolean };

/** 3×4 numeric keypad for whole-rupiah amounts (no decimals; has a 000 key). */
export function Keypad({ onPress, disabled }: KeypadProps) {
  return (
    <View style={styles.grid}>
      {KEYS.map((k) => (
        <Pressable
          key={k}
          accessibilityRole="button"
          accessibilityLabel={labelFor(k)}
          disabled={disabled}
          onPress={() => onPress(k)}
          style={({ pressed }) => [styles.key, pressed && styles.pressed]}
        >
          {k === 'back' ? (
            <Icon name="backspace" size={26} strokeWidth={1.8} />
          ) : (
            <Text variant="keypad">{k}</Text>
          )}
        </Pressable>
      ))}
    </View>
  );
}

/** Applies a key press to the amount string. Max 11 digits, no leading zeros. */
export function applyKey(amount: string, key: KeypadKey): string {
  if (key === 'back') return amount.slice(0, -1);
  if (amount === '' && (key === '0' || key === '000')) return amount;
  const next = amount + key;
  return next.length > 11 ? amount : next;
}

const styles = StyleSheet.create({
  grid: { flexDirection: 'row', flexWrap: 'wrap', rowGap: spacing.sm, columnGap: spacing.sm },
  key: {
    width: '31.5%',
    flexGrow: 1,
    height: sizes.keypadKey,
    borderRadius: radii.xl,
    alignItems: 'center',
    justifyContent: 'center',
  },
  pressed: { backgroundColor: colors.surfaceMuted },
});
