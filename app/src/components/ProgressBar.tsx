import { StyleSheet, View } from 'react-native';

type ProgressBarProps = {
  /** 0–1 */
  progress: number;
  trackColor: string;
  fillColor: string;
  accessibilityLabel: string;
};

export function ProgressBar({ progress, trackColor, fillColor, accessibilityLabel }: ProgressBarProps) {
  const pct = Math.round(Math.max(0, Math.min(1, progress)) * 100);
  return (
    <View
      accessibilityRole="progressbar"
      accessibilityLabel={accessibilityLabel}
      accessibilityValue={{ min: 0, max: 100, now: pct }}
      style={[styles.track, { backgroundColor: trackColor }]}
    >
      <View style={[styles.fill, { width: `${pct}%`, backgroundColor: fillColor }]} />
    </View>
  );
}

const styles = StyleSheet.create({
  track: { height: 6, borderRadius: 3, overflow: 'hidden' },
  fill: { height: '100%', borderRadius: 3 },
});
