import { StyleSheet, View } from 'react-native';

import { formatNumber } from '@/lib/format';
import { amountSizes, colors, fonts, type AmountSize } from '@/theme';

import { Text } from './Text';

type AmountProps = {
  value: number | string;
  size?: AmountSize;
  color?: string;
  currencyColor?: string;
  strikethrough?: boolean;
};

/** Large money figure: a small "Rp" next to the number. */
export function Amount({ value, size = 'md', color = colors.text, currencyColor, strikethrough }: AmountProps) {
  const s = amountSizes[size];
  const figure = formatNumber(value);
  return (
    <View style={[styles.row, { gap: s.gap }]} accessible accessibilityLabel={`Rp ${figure}`}>
      <Text
        style={{
          fontFamily: fonts.numericSemibold,
          fontSize: s.currency,
          // Must be set explicitly: the inherited body line height (22) is smaller
          // than the currency font size and clips the top of "Rp".
          lineHeight: s.currency * 1.2,
          color: currencyColor ?? color,
        }}
      >
        Rp
      </Text>
      <Text
        style={{
          fontFamily: fonts.numericBold,
          fontSize: s.figure,
          lineHeight: s.figure * 1.1,
          letterSpacing: -s.figure * 0.03,
          color,
          textDecorationLine: strikethrough ? 'line-through' : 'none',
        }}
      >
        {figure}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'baseline' },
});
