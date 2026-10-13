import { StyleSheet, View } from 'react-native';

import type { Money } from '@/lib/api';
import { formatMoney, splitMoney } from '@/lib/format';
import { amountSizes, colors, fonts, type AmountSize } from '@/theme';

import { Text } from './Text';

type AmountProps = {
  money: Money;
  size?: AmountSize;
  color?: string;
  currencyColor?: string;
  strikethrough?: boolean;
};

/**
 * Large money figure: a small currency symbol next to the number, on
 * whichever side the device locale puts it ("Rp 35.000", "35.000,00 €").
 */
export function Amount({ money, size = 'md', color = colors.text, currencyColor, strikethrough }: AmountProps) {
  const s = amountSizes[size];
  const { symbol, figure, symbolFirst } = splitMoney(money);

  const symbolText = (
    <Text
      style={{
        fontFamily: fonts.numericSemibold,
        fontSize: s.currency,
        // Must be set explicitly: the inherited body line height (22) is smaller
        // than the currency font size and clips the top of the symbol.
        lineHeight: s.currency * 1.2,
        color: currencyColor ?? color,
      }}
    >
      {symbol}
    </Text>
  );

  return (
    <View style={[styles.row, { gap: s.gap }]} accessible accessibilityLabel={formatMoney(money)}>
      {symbolFirst ? symbolText : null}
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
      {symbolFirst ? null : symbolText}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'baseline' },
});
