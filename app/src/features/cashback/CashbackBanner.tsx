import { Pressable, StyleSheet, View } from 'react-native';

import { Icon, ProgressBar, Text } from '@/components';
import type { Campaign } from '@/lib/api';
import { ratePercent } from '@/lib/cashback';
import { compareMoney, formatMoney, isZeroMoney, subtractMoney } from '@/lib/format';
import { timeLabel } from '@/lib/transactions';
import { colors, fonts, radii, spacing } from '@/theme';

type CashbackBannerProps =
  | {
      state: 'active';
      campaign: Campaign;
      onPress: () => void;
      /** Opens the cash back history from the daily progress. */
      onViewHistory: () => void;
    }
  | { state: 'ended'; campaign: Campaign; onPress: () => void };

/** Home promo for the cash back campaign: active (with daily cap) or ended. */
export function CashbackBanner(props: CashbackBannerProps) {
  if (props.state === 'ended') {
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={`${props.campaign.name} has ended. View your cash back`}
        onPress={props.onPress}
        style={({ pressed }) => [styles.banner, styles.ended, pressed && styles.pressed]}
      >
        <View style={styles.row}>
          <View style={[styles.badge, { backgroundColor: colors.textMuted }]}>
            <Icon name="clock" size={22} color={colors.onBrand} />
          </View>
          <View style={styles.copy}>
            <Text variant="headline">{props.campaign.name} has ended</Text>
            <Text variant="caption" color="textMuted">
              Cash back you’ve earned stays yours to redeem.
            </Text>
          </View>
          <Icon name="chevron-right" size={20} strokeWidth={2.2} />
        </View>
      </Pressable>
    );
  }

  const { campaign, onPress, onViewHistory } = props;
  const rate = ratePercent(campaign.rateBps);
  const { earnedToday, dailyCap, minSpend } = campaign;
  const capReached = compareMoney(earnedToday, dailyCap) >= 0;
  const leftToday = formatMoney(capReached ? dailyCap : subtractMoney(dailyCap, earnedToday));
  // Display only, so a float ratio is fine here.
  const progress = Math.min(Number(earnedToday.value) / Number(dailyCap.value), 1);
  const condition = isZeroMoney(minSpend) ? 'On every payment.' : `On payments of ${formatMoney(minSpend)} or more.`;

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`Get ${rate} cash back. ${condition}`}
      onPress={onPress}
      style={({ pressed }) => [styles.banner, styles.active, pressed && styles.pressed]}
    >
      <View style={styles.row}>
        <View style={[styles.badge, { backgroundColor: colors.onReward }]}>
          <Text style={styles.rate}>{rate}</Text>
        </View>
        <View style={styles.copy}>
          <Text variant="headline" color="onReward">
            Get {rate} cash back
          </Text>
          <Text variant="caption" color="onRewardMuted">
            {condition}
          </Text>
        </View>
        <Icon name="chevron-right" size={20} color={colors.onReward} strokeWidth={2.2} />
      </View>

      <Pressable accessibilityRole="button" accessibilityLabel="View cash back history" onPress={onViewHistory} style={styles.cap}>
        <View style={styles.capLabels}>
          <Text variant="micro" color="onRewardMuted">
            Today’s cash back
          </Text>
          <Text variant="micro" color="onRewardMuted">
            <Text variant="numericSmall" color="onReward">
              {formatMoney(earnedToday)}
            </Text>{' '}
            / {formatMoney(dailyCap)} daily cap
          </Text>
        </View>
        <ProgressBar
          progress={progress}
          trackColor={colors.rewardTrack}
          fillColor={colors.onReward}
          accessibilityLabel="Daily cash back cap used"
        />
        {capReached ? (
          <Text variant="microStrong" color="onReward">
            Daily cap reached · earn again from {timeLabel(new Date(campaign.resetAt))}
          </Text>
        ) : (
          <Text variant="micro" color="onRewardMuted">
            {leftToday} left to earn today
          </Text>
        )}
      </Pressable>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  banner: { paddingVertical: spacing.lg, paddingHorizontal: spacing.lg + 2, borderRadius: radii.card, gap: spacing.md },
  active: { backgroundColor: colors.reward },
  ended: { backgroundColor: colors.surfaceMuted },
  pressed: { opacity: 0.9 },
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md + 2 },
  badge: { width: 52, height: 52, borderRadius: radii.lg, alignItems: 'center', justifyContent: 'center' },
  rate: { fontFamily: fonts.numericBold, fontSize: 18, color: colors.reward },
  copy: { flex: 1, gap: 3 },
  cap: { gap: 6 },
  capLabels: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline' },
});
