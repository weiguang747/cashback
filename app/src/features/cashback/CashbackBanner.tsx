import { Pressable, StyleSheet, View } from 'react-native';

import { Icon, ProgressBar, Text } from '@/components';
import { formatMoney, idr } from '@/lib/format';
import { colors, fonts, radii, spacing } from '@/theme';

type CashbackBannerProps =
  | {
      state: 'active';
      ratePercent: number;
      earnedToday: number;
      dailyCap: number;
      onPress: () => void;
    }
  | { state: 'ended'; onPress: () => void };

/** Home promo for the cash back campaign: active (with daily cap) or ended. */
export function CashbackBanner(props: CashbackBannerProps) {
  if (props.state === 'ended') {
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Cash back campaign has ended. Redeem your cash back"
        onPress={props.onPress}
        style={({ pressed }) => [styles.banner, styles.ended, pressed && styles.pressed]}
      >
        <View style={styles.row}>
          <View style={[styles.badge, { backgroundColor: colors.textMuted }]}>
            <Icon name="clock" size={22} color={colors.onBrand} />
          </View>
          <View style={styles.copy}>
            <Text variant="headline">Cash back campaign has ended</Text>
            <Text variant="caption" color="textMuted">
              The budget is fully claimed. You can still redeem your cash back.
            </Text>
          </View>
          <Icon name="chevron-right" size={20} strokeWidth={2.2} />
        </View>
      </Pressable>
    );
  }

  const { ratePercent, earnedToday, dailyCap, onPress } = props;
  const earned = Math.min(earnedToday, dailyCap);
  const capReached = earned >= dailyCap;

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`Get ${ratePercent}% cash back on every successful payment`}
      onPress={onPress}
      style={({ pressed }) => [styles.banner, styles.active, pressed && styles.pressed]}
    >
      <View style={styles.row}>
        <View style={[styles.badge, { backgroundColor: colors.onReward }]}>
          <Text style={styles.rate}>{ratePercent}%</Text>
        </View>
        <View style={styles.copy}>
          <Text variant="headline" color="onReward">
            Get {ratePercent}% cash back
          </Text>
          <Text variant="caption" color="onRewardMuted">
            On every successful payment. Redeem it anytime.
          </Text>
        </View>
        <Icon name="chevron-right" size={20} color={colors.onReward} strokeWidth={2.2} />
      </View>

      <View style={styles.cap}>
        <View style={styles.capLabels}>
          <Text variant="micro" color="onRewardMuted">
            Today’s cash back
          </Text>
          <Text variant="micro" color="onRewardMuted">
            <Text variant="numericSmall" color="onReward">
              {formatMoney(idr(earned))}
            </Text>{' '}
            / {formatMoney(idr(dailyCap))} daily cap
          </Text>
        </View>
        <ProgressBar
          progress={earned / dailyCap}
          trackColor={colors.rewardTrack}
          fillColor={colors.onReward}
          accessibilityLabel="Daily cash back cap used"
        />
        {capReached ? (
          <Text variant="microStrong" color="onReward">
            Daily cap reached · earn again tomorrow
          </Text>
        ) : (
          <Text variant="micro" color="onRewardMuted">
            {formatMoney(idr(dailyCap - earned))} left to earn today
          </Text>
        )}
      </View>
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
