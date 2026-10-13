import type { Transaction } from '@/lib/api';
import { formatMoney } from '@/lib/format';
import { colors, type Tone } from '@/theme';

import { ListRow } from './ListRow';
import { Text } from './Text';
import { Tile } from './Tile';

const kindStyle: Record<Transaction['kind'], { tone: Tone; amountColor: string }> = {
  out: { tone: 'neutral', amountColor: colors.text },
  in: { tone: 'positive', amountColor: colors.positive },
  cashback: { tone: 'reward', amountColor: colors.onRewardMuted },
};

type TransactionRowProps = {
  transaction: Transaction;
  /** e.g. "8:42 AM · Payment" */
  subtitle: string;
  divider?: boolean;
};

export function TransactionRow({ transaction, subtitle, divider }: TransactionRowProps) {
  const k = kindStyle[transaction.kind];
  return (
    <ListRow
      leading={<Tile tone={k.tone} label={transaction.name} />}
      title={transaction.name}
      subtitle={subtitle}
      divider={divider}
      trailing={
        <Text variant="numeric" style={{ color: k.amountColor }}>
          {formatMoney(transaction.amount, { sign: true })}
        </Text>
      }
    />
  );
}
