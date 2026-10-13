import type { Transaction } from '@/data/mock';
import { formatIDR } from '@/lib/format';
import { colors, type Tone } from '@/theme';

import { ListRow } from './ListRow';
import { Text } from './Text';
import { Tile } from './Tile';

const kindStyle: Record<Transaction['kind'], { tone: Tone; amountColor: string }> = {
  out: { tone: 'neutral', amountColor: colors.text },
  in: { tone: 'positive', amountColor: colors.positive },
  cashback: { tone: 'reward', amountColor: colors.onRewardMuted },
};

export function TransactionRow({ transaction, divider }: { transaction: Transaction; divider?: boolean }) {
  const k = kindStyle[transaction.kind];
  return (
    <ListRow
      leading={<Tile tone={k.tone} label={transaction.name} />}
      title={transaction.name}
      subtitle={transaction.meta}
      divider={divider}
      trailing={
        <Text variant="numeric" style={{ color: k.amountColor }}>
          {formatIDR(transaction.amount, { sign: true })}
        </Text>
      }
    />
  );
}
